package shipping

import (
	"cmp"
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

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// Returned by Track when the carrier exists but implements no Tracker, as with the built-in
var ErrTrackingUnsupported = errors.New("shipping: carrier does not support tracking")

var ErrLabelsUnsupported = errors.New("shipping: carrier does not support labels")

var (
	errBoxCode   = apperr.New("SHIPPING_BOX_CODE_REQUIRED", apperr.ClassValidation, "a box needs a code")
	errBoxDims   = apperr.New("SHIPPING_BOX_DIMENSIONS_INVALID", apperr.ClassValidation, "a box's inner dimensions must be positive")
	errBoxWeight = apperr.New("SHIPPING_BOX_WEIGHT_INVALID", apperr.ClassValidation, "a box's weights can't be negative")
)

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
		if ok, err := s.carrierServes(ctx, cfg, req.Destination.CountryCode); err != nil {
			return QuoteResult{}, err
		} else if !ok {
			continue
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
				if m.PriceCents < 0 {
					log.Printf("[shipping] carrier %q quoted %q below zero (skipped)", a.reg.Slug, m.Code)
					continue
				}
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
		kinds := reg.DeliveryKinds()
		if reg.Slug == BuiltinCarrierSlug {
			// It declares every kind a zone method can name, but quotes only those some active method prices
			if kinds, err = s.pricedKinds(ctx, channelID); err != nil {
				return nil, err
			}
		}
		for _, k := range kinds {
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

// pricedKinds are the kinds the channel's active zone methods price
func (s *Service) pricedKinds(ctx context.Context, channelID uuid.UUID) ([]string, error) {
	zones, err := s.repo.ListZonesByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, z := range zones {
		if !z.IsActive {
			continue
		}
		methods, err := s.repo.ListActiveMethodsByZone(ctx, z.ID)
		if err != nil {
			return nil, err
		}
		for _, m := range methods {
			if k := cmp.Or(m.DeliveryKind, DefaultKind); !slices.Contains(out, k) {
				out = append(out, k)
			}
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
		if ok, err := s.carrierServes(ctx, cfg, req.Destination.CountryCode); err != nil {
			return 0, nil, err
		} else if !ok {
			return 0, nil, ErrMethodNotFound
		}
		req, err := s.withDimensions(ctx, req)
		if err != nil {
			return 0, nil, err
		}
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
			if rErr != nil || cents < 0 {
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
	if ok, err := s.carrierServes(ctx, cfg, req.Destination.CountryCode); err != nil {
		return 0, nil, err
	} else if !ok {
		return 0, nil, ErrMethodNotFound
	}
	req = s.withParcels(ctx, req)
	methods, err := guardedQuote(ctx, reg, req)
	if err != nil {
		return 0, nil, err
	}
	for _, m := range methods {
		if m.Code == code && m.PriceCents >= 0 {
			return m.PriceCents, nil, nil
		}
	}
	return 0, nil, ErrMethodNotFound
}

// guardedQuote re-quotes one carrier at placement under the same budget and panic guard QuoteCarriers gives it
func guardedQuote(ctx context.Context, reg CarrierRegistration, req RateRequest) (methods []RatedMethod, err error) {
	qctx, cancel := context.WithTimeout(ctx, quoteAllBudget)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			methods, err = nil, fmt.Errorf("carrier %q quote panicked: %v", reg.Slug, r)
		}
	}()
	return reg.Carrier.Quote(qctx, req)
}

// carrierServes reports whether a carrier's channel row lets it ship to the country - no row, or mode "all", serves everywhere
func (s *Service) carrierServes(ctx context.Context, cfg CarrierConfig, country string) (bool, error) {
	if cfg.ID == uuid.Nil || cfg.CountryMode != "specific" {
		return true, nil
	}
	allowed, err := s.repo.ListCountriesForCarrier(ctx, cfg.ID)
	if err != nil {
		return false, err
	}
	return containsFold(allowed, country), nil
}

// ensureCarrier makes sure the channel has a row for the carrier before a setting is written to it - a channel created after boot has none until the next SyncCarriers
func (s *Service) ensureCarrier(ctx context.Context, channelID uuid.UUID, slug string) error {
	reg, err := GetCarrier(slug)
	if err != nil {
		return err
	}
	if _, err := s.repo.GetCarrierConfig(ctx, channelID, slug); !errors.Is(err, ErrCarrierNotFound) {
		return err
	}
	_, err = s.repo.UpsertCarrier(ctx, channelID, slug, slug == BuiltinCarrierSlug, reg.DisplayName)
	return err
}

// Toggles a carrier for a channel, validating the slug against the registry first
func (s *Service) SetCarrierEnabled(ctx context.Context, channelID uuid.UUID, slug string, enabled bool) (CarrierConfig, error) {
	if err := s.ensureCarrier(ctx, channelID, slug); err != nil {
		return CarrierConfig{}, err
	}
	return s.repo.SetCarrierEnabled(ctx, channelID, slug, enabled)
}

// Sets whether Saturday and Sunday count toward a carrier's delivery estimates on a channel. The built-in carrier is refused - its methods carry their own
func (s *Service) SetCarrierWeekendDelivery(ctx context.Context, channelID uuid.UUID, slug string, weekends bool) (CarrierConfig, error) {
	if slug == BuiltinCarrierSlug {
		return CarrierConfig{}, errBuiltinWeekends
	}
	if err := s.ensureCarrier(ctx, channelID, slug); err != nil {
		return CarrierConfig{}, err
	}
	return s.repo.SetCarrierWeekendDelivery(ctx, channelID, slug, weekends)
}

func (s *Service) SetCarrierSortOrder(ctx context.Context, channelID uuid.UUID, slug string, sortOrder int32) (CarrierConfig, error) {
	if err := s.ensureCarrier(ctx, channelID, slug); err != nil {
		return CarrierConfig{}, err
	}
	return s.repo.SetCarrierSortOrder(ctx, channelID, slug, sortOrder)
}

// Sets a carrier's ship-to country mode and allow-list for a channel
func (s *Service) SetCarrierCountries(ctx context.Context, channelID uuid.UUID, slug, mode string, countries []string) error {
	countries, err := normCountries(countries)
	if err != nil {
		return err
	}
	if err := s.ensureCarrier(ctx, channelID, slug); err != nil {
		return err
	}
	return s.inTx(ctx, func(r Repository) error {
		cfg, err := r.SetCarrierCountryMode(ctx, channelID, slug, mode)
		if err != nil {
			return err
		}
		if err := r.ClearCarrierCountries(ctx, cfg.ID); err != nil {
			return err
		}
		for _, c := range countries {
			if err := r.AddCountryToCarrier(ctx, cfg.ID, c); err != nil {
				return err
			}
		}
		return nil
	})
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
	// Weights only shape the parcels an external carrier sees - it prices them itself, so a lookup failure packs what the cart says
	if withDims, err := s.withDimensions(ctx, req); err != nil {
		log.Printf("[shipping] product dimensions for cartonization: %v", err)
	} else {
		req = withDims
	}
	boxes, err := s.repo.ListActiveBoxesByChannel(ctx, req.ChannelID)
	if err != nil {
		log.Printf("[shipping] list boxes for cartonization: %v", err)
		return req
	}
	req.Parcels = Pack(req.Items, boxes)
	return req
}

// Callers only know SKUs and quantities, so weights and product types come from the catalog here
func (s *Service) withDimensions(ctx context.Context, req RateRequest) (RateRequest, error) {
	if s.dims == nil {
		return req, nil
	}
	var skus []string
	for _, it := range req.Items {
		if (it.WeightGrams == 0 && it.LengthMm == 0 && it.WidthMm == 0 && it.HeightMm == 0) || it.ProductTypeID == uuid.Nil {
			skus = append(skus, it.SKU)
		}
	}
	if len(skus) == 0 {
		return req, nil
	}
	// Fail closed - without the product types a fridge's own rate would silently fall back to the method's cheap default
	dims, err := s.dims.DimensionsBySKUs(ctx, skus)
	if err != nil {
		return req, fmt.Errorf("product dimensions for rating: %w", err)
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
	return req, nil
}

func (s *Service) ListBoxes(ctx context.Context, channelID uuid.UUID) ([]Box, error) {
	return s.repo.ListBoxesByChannel(ctx, channelID)
}

// Creates or updates a carton, keyed on channel plus code
func (s *Service) UpsertBox(ctx context.Context, in Box) (Box, error) {
	in.Code = strings.TrimSpace(in.Code)
	if in.Code == "" {
		return Box{}, errBoxCode
	}
	if in.InnerLengthMm <= 0 || in.InnerWidthMm <= 0 || in.InnerHeightMm <= 0 {
		return Box{}, errBoxDims
	}
	if in.MaxWeightGrams < 0 || in.BoxWeightGrams < 0 {
		return Box{}, errBoxWeight
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
	return slices.ContainsFunc(haystack, func(h string) bool { return strings.EqualFold(h, needle) })
}
