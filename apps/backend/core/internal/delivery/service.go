package delivery

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// CarrierKinds reports which delivery kinds the carriers enabled on a channel quote, with the carriers' names - readiness asks it whether an option can ever be priced
type CarrierKinds interface {
	CarrierKinds(ctx context.Context, channelID uuid.UUID) (map[string][]string, error)
}

type Service struct {
	repo     Repository
	carriers CarrierKinds
	remote   *remoteSources
}

// NewService also points the manual-points source at repo - one service per process, as with the other registries
func NewService(repo Repository) *Service {
	manualSource.repo = repo
	return &Service{repo: repo}
}

func (s *Service) SetCarrierKinds(c CarrierKinds) { s.carriers = c }

// ---- options ----

func (s *Service) ListOptions(ctx context.Context, channelID uuid.UUID) ([]Option, error) {
	return s.repo.ListOptions(ctx, channelID)
}

func (s *Service) GetOption(ctx context.Context, id uuid.UUID) (Option, error) {
	return s.repo.GetOption(ctx, id)
}

// OfferedOptions are the options checkout shows on a channel - enabled and of a registered kind. Readiness is checked when an option is enabled, not on every storefront read. A channel whose admin has built no options yet gets DefaultOptions, so checkout works out of the box
func (s *Service) OfferedOptions(ctx context.Context, channelID uuid.UUID) ([]Option, error) {
	all, err := s.repo.ListOptions(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return DefaultOptions(channelID), nil
	}
	out := make([]Option, 0, len(all))
	for _, o := range all {
		if _, ok := KindBySlug(o.Kind); ok && o.Enabled {
			out = append(out, o)
		}
	}
	return out, nil
}

// defaultOptionNS keys the implicit options' ids, stable per channel so a cart's selection survives across requests
var defaultOptionNS = uuid.MustParse("5f0c6b0e-6f0a-4d36-9a4e-2f3d0f1b7c11")

// DefaultOptions are what a channel offers before its admin builds any - delivery to the shopper's address and pickup from any pickup-enabled location. A pickup point option needs points and a priced method first, so the admin adds it. Saving a first option replaces them
func DefaultOptions(channelID uuid.UUID) []Option {
	mk := func(kind, name string, sort int32, source string) Option {
		return Option{
			ID: uuid.NewSHA1(defaultOptionNS, []byte(channelID.String()+"/"+kind)), ChannelID: channelID, Kind: kind, Name: name,
			Enabled: true, SortOrder: sort, BlockModes: map[string]Mode{}, PointSource: source,
		}
	}
	return []Option{
		mk(KindShipToAddress, "Delivery", 0, ""),
		mk(KindStorePickup, "Store pickup", 1, InventoryPointSource),
	}
}

// DefaultOptionsFor are the defaults as checkout offers them - a store that asks billing of a pickup asks it of the default pickup too
func DefaultOptionsFor(channelID uuid.UUID, pickupNeedsBilling bool) []Option {
	opts := DefaultOptions(channelID)
	for i, o := range opts {
		if o.Kind == KindStorePickup && pickupNeedsBilling {
			opts[i].BlockModes = map[string]Mode{BlockBillingAddress: ModeRequired}
		}
	}
	return opts
}

// OfferedKinds are the kinds the offered options cover, in option order
func OfferedKinds(opts []Option) []string {
	var out []string
	for _, o := range opts {
		if !slices.Contains(out, o.Kind) {
			out = append(out, o.Kind)
		}
	}
	return out
}

func (s *Service) CreateOption(ctx context.Context, channelID uuid.UUID, in SaveOptionInput) (Option, error) {
	k, err := s.checkInput(in)
	if err != nil {
		return Option{}, err
	}
	if in.BlockModes == nil {
		in.BlockModes = map[string]Mode{}
	}
	if in.PointSource == "" && k.Needs(BlockPoint) {
		in.PointSource = defaultPointSource(in.Kind)
	}
	// A new option starts off - it is enabled once its setup checklist passes
	enable := in.Enabled
	in.Enabled = false
	o, err := s.repo.CreateOption(ctx, channelID, in)
	if err != nil || !enable {
		return o, err
	}
	in.Enabled = true
	return s.UpdateOption(ctx, o.ID, in)
}

func (s *Service) UpdateOption(ctx context.Context, id uuid.UUID, in SaveOptionInput) (Option, error) {
	cur, err := s.repo.GetOption(ctx, id)
	if err != nil {
		return Option{}, err
	}
	in.Kind = cur.Kind
	if _, err := s.checkInput(in); err != nil {
		return Option{}, err
	}
	if in.Enabled {
		probe := cur
		probe.Name, probe.BlockModes, probe.PointSource, probe.CarrierSlug = in.Name, in.BlockModes, in.PointSource, in.CarrierSlug
		for _, c := range s.Readiness(ctx, cur.ChannelID, probe) {
			if !c.OK && !c.Advisory {
				return Option{}, apperr.New("DELIVERY_OPTION_NOT_READY", apperr.ClassValidation, fmt.Sprintf("%q can't be enabled yet: %s", in.Name, c.Label))
			}
		}
	}
	return s.repo.UpdateOption(ctx, id, in)
}

func (s *Service) DeleteOption(ctx context.Context, id uuid.UUID) error {
	if _, err := s.repo.GetOption(ctx, id); err != nil {
		return err
	}
	return s.repo.DeleteOption(ctx, id)
}

func (s *Service) checkInput(in SaveOptionInput) (Kind, error) {
	k, ok := KindBySlug(in.Kind)
	if !ok {
		return Kind{}, ErrUnknownKind
	}
	if strings.TrimSpace(in.Name) == "" {
		return Kind{}, apperr.New("DELIVERY_OPTION_NAME", apperr.ClassValidation, "a delivery option needs a name")
	}
	if err := k.CheckModes(in.BlockModes); err != nil {
		return Kind{}, err
	}
	return k, nil
}

func (s *Service) LineProfiles(ctx context.Context, skus []string) (map[string]LineProfile, error) {
	return s.repo.LineProfiles(ctx, skus)
}

func (s *Service) SetProductTypeKinds(ctx context.Context, productTypeID uuid.UUID, kinds []string) error {
	if kinds != nil {
		kinds = slices.Compact(slices.Sorted(slices.Values(kinds)))
		for _, k := range kinds {
			if _, ok := KindBySlug(k); !ok {
				return apperr.New("DELIVERY_KIND_UNKNOWN", apperr.ClassValidation, "unknown delivery kind "+k)
			}
		}
	}
	return s.repo.SetProductTypeKinds(ctx, productTypeID, kinds)
}

// ---- points ----

// SourceInfo is a point source as the dashboard lists them - registered in code or a channel's remote provider
type SourceInfo struct {
	Slug   string   `json:"slug"`
	Label  string   `json:"label"`
	Kinds  []string `json:"kinds"`
	Owner  string   `json:"owner"`
	Remote bool     `json:"remote"`
	Active bool     `json:"active"`
}

func (s *Service) PointSources(ctx context.Context, channelID uuid.UUID) []SourceInfo {
	var out []SourceInfo
	for _, r := range PointSources() {
		out = append(out, SourceInfo{Slug: r.Slug, Label: r.Label, Kinds: r.Kinds, Owner: r.Owner, Active: true})
	}
	return append(out, s.remote.pointSources(ctx, channelID)...)
}

// defaultPointSource is where a new option of a kind finds its points until the admin picks another
func defaultPointSource(kind string) string {
	switch kind {
	case KindStorePickup:
		return InventoryPointSource
	case KindParcelLocker:
		return ManualPointSource
	}
	return ""
}

// pointSourceFor resolves the option's source - registered, or a channel's remote provider
func (s *Service) pointSourceFor(ctx context.Context, o Option) (PointSource, error) {
	slug := o.PointSource
	if slug == "" {
		slug = defaultPointSource(o.Kind)
	}
	if slug == "" {
		return nil, ErrSourceUnavailable
	}
	if r, ok := pointSourceBySlug(slug); ok {
		// Stores can't be a pickup point's source - a source serves the kinds it registered for
		if !serves(r.Kinds, o.Kind) {
			return nil, ErrSourceUnavailable
		}
		return r.Source, nil
	}
	if src, ok := s.remote.pointSource(ctx, o.ChannelID, slug); ok {
		return src, nil
	}
	return nil, ErrSourceUnavailable
}

func (s *Service) SearchPoints(ctx context.Context, o Option, q PointQuery) ([]Point, error) {
	src, err := s.pointSourceFor(ctx, o)
	if err != nil {
		return nil, err
	}
	q.ChannelID, q.Kind = o.ChannelID, o.Kind
	if q.Limit <= 0 || q.Limit > 50 {
		q.Limit = 20
	}
	pts, err := src.Search(ctx, q)
	if err != nil {
		// The source's own error can carry a provider's URL or response body - it is logged, and the shopper hears only that points are unavailable
		log.Printf("[delivery] search points for option %s: %v", o.ID, err)
		return nil, ErrSourceUnavailable
	}
	if len(pts) > q.Limit {
		pts = pts[:q.Limit]
	}
	return pts, nil
}

// GetPoint re-reads a point from its source - what the shopper picks is never taken on their word
func (s *Service) GetPoint(ctx context.Context, o Option, id string) (Point, error) {
	src, err := s.pointSourceFor(ctx, o)
	if err != nil {
		return Point{}, err
	}
	p, err := src.Get(ctx, o.ChannelID, id)
	switch {
	case errors.Is(err, ErrPointNotFound):
		return Point{}, ErrPointNotFound
	case err != nil:
		// A source that couldn't answer says nothing about the point - the shopper tries again rather than picking another that fails the same way
		log.Printf("[delivery] get point %q for option %s: %v", id, o.ID, err)
		return Point{}, ErrSourceUnavailable
	}
	return p, nil
}

// AdoptDefaults saves the implicit default options as real ones so the admin can edit them - a no-op once the channel has any. They keep the ids carts already chose them by, the pickup billing rule checkout applied to them, and stay switched on: checkout offered them a moment ago, and a readiness check failing now would leave the channel with nothing to offer. pickupNeedsBilling is sales/checkout/pickup_requires_billing_address
func (s *Service) AdoptDefaults(ctx context.Context, channelID uuid.UUID, pickupNeedsBilling bool) error {
	return s.repo.AdoptOptions(ctx, channelID, DefaultOptionsFor(channelID, pickupNeedsBilling))
}
