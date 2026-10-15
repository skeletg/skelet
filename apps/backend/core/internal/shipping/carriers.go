package shipping

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Returned by Track when the carrier exists but implements no Tracker, as with the built-in
var ErrTrackingUnsupported = errors.New("shipping: carrier does not support tracking")

var ErrLabelsUnsupported = errors.New("shipping: carrier does not support labels")

// Caps the combined wall-clock one checkout quote can spend across all carriers, on top of each plugin hook's own timeout
const quoteAllBudget = 8 * time.Second

// Ensures every (channel, carrier) pair has a shipping_carriers row - built-in seeds enabled and external seed disabled so an admin opts in, and UpsertCarrier's ON CONFLICT keeps the admin's choice, making re-runs safe
func (s *Service) SyncCarriers(ctx context.Context, channelIDs []uuid.UUID) error {
	regs := AllCarriers()
	if len(channelIDs) == 0 || len(regs) == 0 {
		return nil
	}
	for _, chID := range channelIDs {
		for _, reg := range regs {
			enabled := reg.Slug == BuiltinCarrierSlug
			if _, err := s.repo.UpsertCarrier(ctx, chID, reg.Slug, enabled, reg.DisplayName); err != nil {
				return fmt.Errorf("shipping.SyncCarriers: upsert %s on %s: %w", reg.Slug, chID, err)
			}
		}
	}
	return nil
}

// Every carrier with its per-channel config, zero-valued when unsynced, plus capability flags - for the admin carriers page
func (s *Service) ListCarriersForChannel(ctx context.Context, channelID uuid.UUID) ([]CarrierWithConfig, error) {
	configs, err := s.repo.ListCarrierConfigsByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	bySlug := make(map[string]CarrierConfig, len(configs))
	for _, c := range configs {
		if c.CountryMode == "specific" {
			countries, err := s.repo.ListCountriesForCarrier(ctx, c.ID)
			if err != nil {
				return nil, err
			}
			c.AllowedCountries = countries
		}
		bySlug[c.CarrierSlug] = c
	}
	regs := AllCarriers()
	out := make([]CarrierWithConfig, 0, len(regs))
	for _, reg := range regs {
		_, supportsTrack := reg.Carrier.(Tracker)
		_, supportsLabel := reg.Carrier.(Labeler)
		out = append(out, CarrierWithConfig{
			Slug:          reg.Slug,
			DisplayName:   reg.DisplayName,
			Config:        bySlug[reg.Slug],
			SupportsTrack: supportsTrack,
			SupportsLabel: supportsLabel,
		})
	}
	return out, nil
}

// Aggregates rate options across every enabled carrier, run concurrently under quoteAllBudget Results are PARTIAL - a carrier that errors, panics or times out is skipped, so a flaky external API never blanks checkout, with the built-in as the baseline - Sorted by (sort_order, price, code) so completion order can't scramble it With no config row the built-in fail-opens and external carriers fail-closed, so a channel newer than the last SyncCarriers still quotes
func (s *Service) QuoteAll(ctx context.Context, req RateRequest) ([]RatedQuote, error) {
	res, err := s.QuoteCarriers(ctx, req)
	return res.Quotes, err
}

// QuoteResult is every carrier's rated methods, with the carriers that couldn't answer - a method missing from Quotes
// is not offered only when its carrier isn't among Failed
type QuoteResult struct {
	Quotes []RatedQuote
	Failed []string // carrier slugs whose quote errored, panicked or ran out of time
}

// Answered reports whether the carrier gave an answer - an empty one counts, a failure doesn't
func (r QuoteResult) Answered(carrierSlug string) bool {
	return !slices.Contains(r.Failed, carrierSlug)
}

// QuoteCarriers is QuoteAll that also says which carriers failed, for a caller that must tell "not offered" from "couldn't ask"
func (s *Service) QuoteCarriers(ctx context.Context, req RateRequest) (QuoteResult, error) {
	configs, err := s.repo.ListCarrierConfigsByChannel(ctx, req.ChannelID)
	if err != nil {
		return QuoteResult{}, err
	}
	bySlug := make(map[string]CarrierConfig, len(configs))
	for _, c := range configs {
		bySlug[c.CarrierSlug] = c
	}

	type active struct {
		reg CarrierRegistration
		cfg CarrierConfig
	}
	var act []active
	for _, reg := range AllCarriers() {
		cfg, hasRow := bySlug[reg.Slug]
		enabled := reg.Slug == BuiltinCarrierSlug // fail-open builtin
		if hasRow {
			enabled = cfg.IsEnabled
		}
		if !enabled {
			continue
		}
		if hasRow && cfg.CountryMode == "specific" {
			allowed, err := s.repo.ListCountriesForCarrier(ctx, cfg.ID)
			if err != nil {
				return QuoteResult{}, err
			}
			if !containsFold(allowed, req.Destination.CountryCode) {
				continue
			}
		}
		act = append(act, active{reg: reg, cfg: cfg})
	}
	// No registry carriers still allows rates - a channel may quote entirely through remote providers
	if len(act) == 0 && s.remoteRates == nil {
		return QuoteResult{}, nil
	}

	req = s.withParcels(ctx, req)

	qctx, cancel := context.WithTimeout(ctx, quoteAllBudget)
	defer cancel()

	var (
		mu     sync.Mutex
		out    []RatedQuote
		failed []string
		wg     sync.WaitGroup
	)
	fail := func(slug string) {
		mu.Lock()
		failed = append(failed, slug)
		mu.Unlock()
	}
	for _, a := range act {
		wg.Add(1)
		go func(a active) {
			defer wg.Done()
			// A misbehaving carrier, especially a plugin, must not crash checkout
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[shipping] carrier %q quote panicked (skipped): %v", a.reg.Slug, r)
					fail(a.reg.Slug)
				}
			}()
			methods, err := a.reg.Carrier.Quote(qctx, req)
			if err != nil {
				log.Printf("[shipping] carrier %q quote failed (skipped): %v", a.reg.Slug, err)
				fail(a.reg.Slug)
				return
			}
			name := carrierName(a.reg, a.cfg)
			mu.Lock()
			for _, m := range methods {
				// The built-in carrier's methods say for themselves; any other carrier follows its channel row
				if a.reg.Slug != BuiltinCarrierSlug {
					m.DeliversOnWeekends = a.cfg.DeliversOnWeekends
				}
				out = append(out, RatedQuote{CarrierSlug: a.reg.Slug, CarrierName: name, Method: a.reg.tag(m)})
			}
			mu.Unlock()
		}(a)
	}
	// Remote providers quote alongside the registered carriers on the same budget, failing open inside Quotes
	if s.remoteRates != nil {
		wg.Go(func() {
			remote, remoteFailed := s.remoteRates.Quotes(qctx, req)
			mu.Lock()
			out = append(out, remote...)
			failed = append(failed, remoteFailed...)
			mu.Unlock()
		})
	}
	wg.Wait()

	sortKey := func(slug string) int32 {
		if c, ok := bySlug[slug]; ok {
			return c.SortOrder
		}
		return 0
	}
	sort.SliceStable(out, func(i, j int) bool {
		if si, sj := sortKey(out[i].CarrierSlug), sortKey(out[j].CarrierSlug); si != sj {
			return si < sj
		}
		if out[i].Method.PriceCents != out[j].Method.PriceCents {
			return out[i].Method.PriceCents < out[j].Method.PriceCents
		}
		return out[i].Method.Code < out[j].Method.Code
	})
	return QuoteResult{Quotes: out, Failed: failed}, nil
}

// EnabledCarrierKinds maps each delivery kind to the carriers enabled on the channel that quote it, as "slug:Display name" - the readiness check of a delivery option asks it whether anything can ever price the option. Registered carriers follow the same enable rule QuoteAll does; remote ones report what their provider config declares
func (s *Service) EnabledCarrierKinds(ctx context.Context, channelID uuid.UUID) (map[string][]string, error) {
	configs, err := s.repo.ListCarrierConfigsByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	bySlug := make(map[string]CarrierConfig, len(configs))
	for _, c := range configs {
		bySlug[c.CarrierSlug] = c
	}
	out := map[string][]string{}
	for _, reg := range AllCarriers() {
		cfg, hasRow := bySlug[reg.Slug]
		enabled := reg.Slug == BuiltinCarrierSlug
		if hasRow {
			enabled = cfg.IsEnabled
		}
		if !enabled {
			continue
		}
		for _, k := range reg.DeliveryKinds() {
			out[k] = append(out[k], reg.Slug+":"+carrierName(reg, cfg))
		}
	}
	if kr, ok := s.remoteRates.(remoteKindReporter); ok {
		for k, names := range kr.Kinds(ctx, channelID) {
			out[k] = append(out[k], names...)
		}
	}
	return out, nil
}

// remoteKindReporter is the optional half of a RemoteRateSource that knows which kinds its providers quote
type remoteKindReporter interface {
	Kinds(ctx context.Context, channelID uuid.UUID) map[string][]string
}

// Authoritatively re-prices the chosen carrier and method, never trusting the client's price - the built-in re-prices by code through ComputeForOrder and returns the method uuid, while external carriers re-quote and match by code
// ErrMethodNotFound, from a disabled or unknown carrier or a vanished method, is fail-closed so the customer retries
func (s *Service) PriceSelected(ctx context.Context, slug, code string, req RateRequest) (int64, *uuid.UUID, error) {
	if slug == "" || slug == BuiltinCarrierSlug {
		// Same rule QuoteAll uses - no row fails open, but a row that says disabled must not re-price stale zone rates
		cfg, cfgErr := s.repo.GetCarrierConfig(ctx, req.ChannelID, BuiltinCarrierSlug)
		if cfgErr != nil && !errors.Is(cfgErr, ErrCarrierNotFound) {
			return 0, nil, cfgErr
		}
		if cfgErr == nil && !cfg.IsEnabled {
			return 0, nil, ErrMethodNotFound
		}
		req = s.withDimensions(ctx, req)
		cents, method, err := s.ComputeForOrder(ctx, code, QuoteInput{
			ChannelID:     req.ChannelID,
			CountryCode:   req.Destination.CountryCode,
			SubtotalCents: req.SubtotalCents,
			Items:         pricedItems(req.Items),
		})
		if err != nil {
			return 0, nil, err
		}
		id := method.ID
		return cents, &id, nil
	}
	reg, err := GetCarrier(slug)
	if err != nil {
		// Not a registered carrier - it may be a remote shipping provider
		if s.remoteRates != nil {
			req = s.withParcels(ctx, req)
			cents, rErr := s.remoteRates.Price(ctx, slug, code, req)
			if rErr != nil {
				return 0, nil, ErrMethodNotFound
			}
			return cents, nil, nil
		}
		return 0, nil, ErrMethodNotFound
	}
	// An external carrier must be enabled on this channel - fail-closed, so no row means off
	cfg, cfgErr := s.repo.GetCarrierConfig(ctx, req.ChannelID, slug)
	if cfgErr != nil || !cfg.IsEnabled {
		return 0, nil, ErrMethodNotFound
	}
	req = s.withParcels(ctx, req)
	methods, err := reg.Carrier.Quote(ctx, req)
	if err != nil {
		return 0, nil, err
	}
	for _, m := range methods {
		if m.Code == code {
			return m.PriceCents, nil, nil
		}
	}
	return 0, nil, ErrMethodNotFound
}

// Toggles a carrier for a channel, validating the slug against the registry first
func (s *Service) SetCarrierEnabled(ctx context.Context, channelID uuid.UUID, slug string, enabled bool) (CarrierConfig, error) {
	if _, err := GetCarrier(slug); err != nil {
		return CarrierConfig{}, err
	}
	return s.repo.SetCarrierEnabled(ctx, channelID, slug, enabled)
}

// Sets whether Saturday and Sunday count toward a carrier's delivery estimates on a channel. The built-in carrier is refused - its methods carry their own
func (s *Service) SetCarrierWeekendDelivery(ctx context.Context, channelID uuid.UUID, slug string, weekends bool) (CarrierConfig, error) {
	if slug == BuiltinCarrierSlug {
		return CarrierConfig{}, errBuiltinWeekends
	}
	if _, err := GetCarrier(slug); err != nil {
		return CarrierConfig{}, err
	}
	return s.repo.SetCarrierWeekendDelivery(ctx, channelID, slug, weekends)
}

func (s *Service) SetCarrierSortOrder(ctx context.Context, channelID uuid.UUID, slug string, sortOrder int32) (CarrierConfig, error) {
	if _, err := GetCarrier(slug); err != nil {
		return CarrierConfig{}, err
	}
	return s.repo.SetCarrierSortOrder(ctx, channelID, slug, sortOrder)
}

// Sets a carrier's ship-to country mode and allow-list for a channel
func (s *Service) SetCarrierCountries(ctx context.Context, channelID uuid.UUID, slug, mode string, countries []string) error {
	if _, err := GetCarrier(slug); err != nil {
		return err
	}
	cfg, err := s.repo.SetCarrierCountryMode(ctx, channelID, slug, mode)
	if err != nil {
		return err
	}
	if err := s.repo.ClearCarrierCountries(ctx, cfg.ID); err != nil {
		return err
	}
	for _, c := range countries {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c == "" {
			continue
		}
		if err := s.repo.AddCountryToCarrier(ctx, cfg.ID, c); err != nil {
			return err
		}
	}
	return nil
}

// One carrier with its hydrated config, so admin mutations can return the updated row
func (s *Service) CarrierForChannel(ctx context.Context, channelID uuid.UUID, slug string) (CarrierWithConfig, error) {
	reg, err := GetCarrier(slug)
	if err != nil {
		return CarrierWithConfig{}, err
	}
	cfg, err := s.repo.GetCarrierConfig(ctx, channelID, slug)
	if err != nil && !errors.Is(err, ErrCarrierNotFound) {
		return CarrierWithConfig{}, err
	}
	if cfg.CountryMode == "specific" {
		countries, cerr := s.repo.ListCountriesForCarrier(ctx, cfg.ID)
		if cerr != nil {
			return CarrierWithConfig{}, cerr
		}
		cfg.AllowedCountries = countries
	}
	_, supportsTrack := reg.Carrier.(Tracker)
	_, supportsLabel := reg.Carrier.(Labeler)
	return CarrierWithConfig{
		Slug:          reg.Slug,
		DisplayName:   reg.DisplayName,
		Config:        cfg,
		SupportsTrack: supportsTrack,
		SupportsLabel: supportsLabel,
	}, nil
}

// Resolves a shipment's status and URL - ErrCarrierNotFound for an unknown slug, ErrTrackingUnsupported when the carrier has no Tracker
func (s *Service) Track(ctx context.Context, carrierSlug, trackingNumber string) (TrackResult, error) {
	// A universal remote tracker answers first, else the carrier's own Tracker
	if res, ok := s.trackRemoteFirst(ctx, carrierSlug, trackingNumber); ok {
		return res, nil
	}
	reg, err := GetCarrier(carrierSlug)
	if err != nil {
		return TrackResult{}, err
	}
	tracker, ok := reg.Carrier.(Tracker)
	if !ok {
		return TrackResult{}, ErrTrackingUnsupported
	}
	return tracker.Track(ctx, TrackRequest{CarrierSlug: carrierSlug, TrackingNumber: trackingNumber})
}

// Produces a shipping label - ErrCarrierNotFound for an unknown slug, ErrLabelsUnsupported when the carrier has no Labeler
func (s *Service) GenerateLabel(ctx context.Context, carrierSlug string, req LabelRequest) (LabelResult, error) {
	reg, err := GetCarrier(carrierSlug)
	if err != nil {
		return LabelResult{}, err
	}
	labeler, ok := reg.Carrier.(Labeler)
	if !ok {
		return LabelResult{}, ErrLabelsUnsupported
	}
	if req.Parcels == nil && req.ChannelID != uuid.Nil {
		if boxes, err := s.repo.ListActiveBoxesByChannel(ctx, req.ChannelID); err == nil {
			req.Parcels = Pack(req.Items, boxes)
		}
	}
	return labeler.Label(ctx, req)
}

// Fills the cartonized parcel view from the channel's boxes - fail-soft, so a box lookup error leaves Parcels nil and carriers fall back to Items rather than blocking a quote
func (s *Service) withParcels(ctx context.Context, req RateRequest) RateRequest {
	if req.Parcels != nil {
		return req
	}
	req = s.withDimensions(ctx, req)
	boxes, err := s.repo.ListActiveBoxesByChannel(ctx, req.ChannelID)
	if err != nil {
		log.Printf("[shipping] list boxes for cartonization: %v", err)
		return req
	}
	req.Parcels = Pack(req.Items, boxes)
	return req
}

// Callers only know SKUs and quantities, so weights and product types come from the catalog here
func (s *Service) withDimensions(ctx context.Context, req RateRequest) RateRequest {
	if s.dims == nil {
		return req
	}
	var skus []string
	for _, it := range req.Items {
		if (it.WeightGrams == 0 && it.LengthMm == 0 && it.WidthMm == 0 && it.HeightMm == 0) || it.ProductTypeID == uuid.Nil {
			skus = append(skus, it.SKU)
		}
	}
	if len(skus) == 0 {
		return req
	}
	dims, err := s.dims.DimensionsBySKUs(ctx, skus)
	if err != nil {
		log.Printf("[shipping] product dimensions for rating: %v", err)
		return req
	}
	items := slices.Clone(req.Items)
	for i, it := range items {
		d, ok := dims[it.SKU]
		if !ok {
			continue
		}
		if it.ProductTypeID == uuid.Nil {
			items[i].ProductTypeID = d.ProductTypeID
		}
		if it.WeightGrams != 0 || it.LengthMm != 0 || it.WidthMm != 0 || it.HeightMm != 0 {
			continue
		}
		items[i].WeightGrams, items[i].LengthMm, items[i].WidthMm, items[i].HeightMm = d.WeightGrams, d.LengthMm, d.WidthMm, d.HeightMm
	}
	req.Items = items
	return req
}

func (s *Service) ListBoxes(ctx context.Context, channelID uuid.UUID) ([]Box, error) {
	return s.repo.ListBoxesByChannel(ctx, channelID)
}

// Creates or updates a carton, keyed on channel plus code
func (s *Service) UpsertBox(ctx context.Context, in Box) (Box, error) {
	in.Code = strings.TrimSpace(in.Code)
	if in.Code == "" {
		return Box{}, fmt.Errorf("shipping: box code is required")
	}
	if in.InnerLengthMm <= 0 || in.InnerWidthMm <= 0 || in.InnerHeightMm <= 0 {
		return Box{}, fmt.Errorf("shipping: box inner dimensions must be positive")
	}
	if in.MaxWeightGrams < 0 || in.BoxWeightGrams < 0 {
		return Box{}, fmt.Errorf("shipping: box weights cannot be negative")
	}
	return s.repo.UpsertBox(ctx, in)
}

// GetBox is the single-box read the admin delete path authorizes against
func (s *Service) GetBox(ctx context.Context, id uuid.UUID) (Box, error) {
	return s.repo.GetBox(ctx, id)
}

func (s *Service) DeleteBox(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteBox(ctx, id)
}

// Prefers the admin-set display name from the config row, falling back to the registry's default
func carrierName(reg CarrierRegistration, cfg CarrierConfig) string {
	if cfg.DisplayName != "" {
		return cfg.DisplayName
	}
	return reg.DisplayName
}

func containsFold(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.EqualFold(h, needle) {
			return true
		}
	}
	return false
}
