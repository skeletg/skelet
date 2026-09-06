package channel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/cache"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/events"
)

// The slice of catalog.Service the currency-change guard needs - wired post-construction to dodge an import cycle, since catalog already depends on channel
type ListingCounter interface {
	CountInheritingListingsForChannel(ctx context.Context, channelID uuid.UUID) (int64, error)
}

// Operators must set explicit prices on inheriting listings before the store currency can change
var ErrStoreCurrencyChangeBlocked = errors.New("cannot change store currency while listings inherit price; set explicit prices first")

// Pre-validates a locale code before SetLocales / Create writes - declared locally so channel doesn't pull in locale's full Service surface
type LocaleValidator interface {
	EnsureExists(ctx context.Context, code string) error
}

// Same shape as LocaleValidator - surfaces ErrCurrencyNotFound instead of a raw FK violation when an admin types a code that doesn't exist
type CurrencyValidator interface {
	EnsureExists(ctx context.Context, code string) error
}

// Wires a new channel to the configured default inventory location - declared locally so channel doesn't import inventory, and when nil new views start with no assignments
type LocationAssigner interface {
	AssignDefaultLocation(ctx context.Context, channelID uuid.UUID, locationSlug string) error
}

// configSvc is consulted only at Create time, for the default inventory location - currency, locales, addresses and the email fields are columns on channels rather than config
type Service struct {
	repo       Repository
	configSvc  *config.Service
	listings   ListingCounter
	locales    LocaleValidator
	currencies CurrencyValidator
	locations  LocationAssigner
	// Valkey read-through for the hot resolution paths - slug, host, default
	// Writes wipe the whole namespace because one row is reachable under several keys, which is simpler than tracking inverse lookups and cheap since channel writes are rare admin actions - Nil disables caching
	cache *cache.Cache
	// Post-commit channel events (locale-set changes, deletion) so the search indexer can rebuild or drop per-channel docs - nil means silent writes
	bus *events.Bus
}

func (s *Service) SetEventBus(b *events.Bus) { s.bus = b }

// Generous because channel rows mutate rarely - any admin write drops the namespace immediately, so this only bounds staleness for writes that bypass the service
const channelCacheTTL = 30 * time.Minute

func NewService(repo Repository, configSvc *config.Service) *Service {
	return &Service{repo: repo, configSvc: configSvc}
}

func (s *Service) SetCache(c *cache.Cache) {
	s.cache = c
}

// DelPattern is O(keys-in-ns), but the channel namespace is tiny and writes are rare
func (s *Service) invalidate(ctx context.Context) {
	if s.cache == nil {
		return
	}
	s.cache.DelPattern(ctx, "*")
}

func (s *Service) SetListingCounter(c ListingCounter) {
	s.listings = c
}

// Optional - without it repo writes still succeed on FK validation, just with uglier error messages
func (s *Service) SetLocaleValidator(v LocaleValidator) {
	s.locales = v
}

func (s *Service) SetCurrencyValidator(v CurrencyValidator) {
	s.currencies = v
}

func (s *Service) SetLocationAssigner(a LocationAssigner) {
	s.locations = a
}

// Returns the first failing code so the caller surfaces a clean LOCALE_NOT_FOUND instead of a raw FK violation - no-op when no validator is installed
func (s *Service) validateLocaleCodes(ctx context.Context, codes []string) error {
	if s.locales == nil {
		return nil
	}
	for _, c := range codes {
		if err := s.locales.EnsureExists(ctx, c); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Create(ctx context.Context, input CreateChannelInput) (Channel, error) {
	if err := validateSlug(input.Slug); err != nil {
		return Channel{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return Channel{}, fmt.Errorf("name is required")
	}
	if input.DefaultLocale == "" {
		input.DefaultLocale = "en"
	}
	input.DefaultLocale = strings.ToLower(input.DefaultLocale)
	if len(input.Locales) == 0 {
		input.Locales = []string{input.DefaultLocale}
	}
	input.CurrencyCode = strings.ToUpper(strings.TrimSpace(input.CurrencyCode))
	// Required at creation - defaulting to USD silently would mask misconfiguration until a non-USD operator's first order stamped the wrong totals
	if len(input.CurrencyCode) != 3 {
		return Channel{}, ErrInvalidCurrency
	}
	// Pre-validate so a typo surfaces as ErrCurrencyNotFound rather than a raw FK violation after the insert
	if s.currencies != nil {
		if err := s.currencies.EnsureExists(ctx, input.CurrencyCode); err != nil {
			return Channel{}, err
		}
	}
	if err := s.validateLocaleCodes(ctx, append([]string{input.DefaultLocale}, input.Locales...)); err != nil {
		return Channel{}, err
	}
	sv, err := s.repo.Create(ctx, input)
	if err != nil {
		return Channel{}, err
	}
	if err := s.repo.SetLocales(ctx, sv.ID, input.Locales); err != nil {
		return Channel{}, err
	}
	// New channels inherit the shared-warehouse stock by default, and admins wanting regional separation override afterwards
	// Best-effort - a missing config or unknown slug logs and continues, since the channel itself was created
	if s.locations != nil && s.configSvc != nil {
		if slug, err := s.configSvc.GetString(ctx, "sales/inventory/default_location", config.DefaultScope); err == nil && strings.TrimSpace(slug) != "" {
			_ = s.locations.AssignDefaultLocation(ctx, sv.ID, strings.TrimSpace(slug))
		}
	}
	// Hydrate the returned struct so the caller doesn't need a follow-up read
	locales, err := s.repo.ListLocales(ctx, sv.ID)
	if err != nil {
		return Channel{}, err
	}
	sv.Locales = locales
	s.invalidate(ctx)
	return sv, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Channel, error) {
	sv, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Channel{}, err
	}
	sv.Locales, err = s.repo.ListLocales(ctx, sv.ID)
	if err != nil {
		return Channel{}, err
	}
	return sv, nil
}

func (s *Service) GetBySlug(ctx context.Context, slug string) (Channel, error) {
	if cached, ok := cache.GetJSON[Channel](ctx, s.cache, "slug:"+slug); ok {
		return cached, nil
	}
	sv, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		return Channel{}, err
	}
	sv.Locales, err = s.repo.ListLocales(ctx, sv.ID)
	if err != nil {
		return Channel{}, err
	}
	s.cache.SetJSON(ctx, "slug:"+slug, sv, channelCacheTTL)
	return sv, nil
}

func (s *Service) GetByStorefrontHost(ctx context.Context, host string) (Channel, error) {
	if cached, ok := cache.GetJSON[Channel](ctx, s.cache, "host:"+host); ok {
		return cached, nil
	}
	sv, err := s.repo.GetByStorefrontHost(ctx, host)
	if err != nil {
		return Channel{}, err
	}
	sv.Locales, err = s.repo.ListLocales(ctx, sv.ID)
	if err != nil {
		return Channel{}, err
	}
	s.cache.SetJSON(ctx, "host:"+host, sv, channelCacheTTL)
	return sv, nil
}

// ErrChannelNotFound when no row carries is_default - the middleware reads that as "no store context", matching the pre-default behaviour
func (s *Service) GetDefault(ctx context.Context) (Channel, error) {
	if cached, ok := cache.GetJSON[Channel](ctx, s.cache, "default"); ok {
		// Zero-ID sentinel meaning "no default elected" - without it those installs re-run the SQL on EVERY GetDefault, and the default-locale fallback alone hits it several times per storefront request
		if cached.ID == uuid.Nil {
			return Channel{}, ErrChannelNotFound
		}
		return cached, nil
	}
	sv, err := s.repo.GetDefault(ctx)
	if err != nil {
		if errors.Is(err, ErrChannelNotFound) {
			// Short TTL - electing a default also invalidates, this just caps staleness for out-of-band writes
			s.cache.SetJSON(ctx, "default", Channel{}, time.Minute)
		}
		return Channel{}, err
	}
	sv.Locales, err = s.repo.ListLocales(ctx, sv.ID)
	if err != nil {
		return Channel{}, err
	}
	s.cache.SetJSON(ctx, "default", sv, channelCacheTTL)
	return sv, nil
}

// No locale hydration - graph-layer caches pre-load the store rows listings reference, so each field resolver reads the map instead of firing a GetByID
func (s *Service) ListByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Channel, error) {
	return s.repo.ListByIDs(ctx, ids)
}

// Uses the batched ListLocalesByStoreIDs to avoid an N+1 in admin views
func (s *Service) List(ctx context.Context) ([]Channel, error) {
	stores, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(stores) == 0 {
		return stores, nil
	}
	ids := make([]uuid.UUID, len(stores))
	for i, sv := range stores {
		ids[i] = sv.ID
	}
	byStore, err := s.repo.ListLocalesByStoreIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range stores {
		stores[i].Locales = byStore[stores[i].ID]
	}
	return stores, nil
}

// Pre-validates each code so a typo returns ErrLocaleNotFound instead of an FK error
func (s *Service) SetLocales(ctx context.Context, channelID uuid.UUID, codes []string) (Channel, error) {
	if err := s.validateLocaleCodes(ctx, codes); err != nil {
		return Channel{}, err
	}
	if err := s.repo.SetLocales(ctx, channelID, codes); err != nil {
		return Channel{}, err
	}
	s.invalidate(ctx)
	s.publishLocalesChanged(ctx, channelID)
	return s.GetByID(ctx, channelID)
}

// Tells the search indexer that per-locale docs for removed locales are stale, and that added locales have none until the channel's products reindex
func (s *Service) publishLocalesChanged(ctx context.Context, channelID uuid.UUID) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, &LocalesChanged{ChannelID: channelID})
}

func (s *Service) Update(ctx context.Context, input UpdateChannelInput) (Channel, error) {
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return Channel{}, fmt.Errorf("name is required")
	}
	if input.CurrencyCode != nil {
		normalized := strings.ToUpper(strings.TrimSpace(*input.CurrencyCode))
		if len(normalized) != 3 {
			return Channel{}, ErrInvalidCurrency
		}
		input.CurrencyCode = &normalized
		if s.currencies != nil {
			if err := s.currencies.EnsureExists(ctx, normalized); err != nil {
				return Channel{}, err
			}
		}
		// Reject a currency change while any listing on this store still inherits price - operators convert those to explicit first - Skipped when the listings dependency isn't installed
		if s.listings != nil {
			current, err := s.repo.GetByID(ctx, input.ID)
			if err != nil {
				return Channel{}, err
			}
			if current.CurrencyCode != normalized {
				count, err := s.listings.CountInheritingListingsForChannel(ctx, input.ID)
				if err != nil {
					return Channel{}, err
				}
				if count > 0 {
					return Channel{}, fmt.Errorf("%w: %d listing(s) inherit price on this store", ErrStoreCurrencyChangeBlocked, count)
				}
			}
		}
	}
	if input.Locales != nil {
		if err := s.validateLocaleCodes(ctx, input.Locales); err != nil {
			return Channel{}, err
		}
	}
	if input.DefaultLocale != nil {
		if err := s.validateLocaleCodes(ctx, []string{*input.DefaultLocale}); err != nil {
			return Channel{}, err
		}
	}
	sv, err := s.repo.Update(ctx, input)
	if err != nil {
		return Channel{}, err
	}
	if input.Locales != nil {
		if err := s.repo.SetLocales(ctx, sv.ID, input.Locales); err != nil {
			return Channel{}, err
		}
	}
	sv.Locales, err = s.repo.ListLocales(ctx, sv.ID)
	if err != nil {
		return Channel{}, err
	}
	s.invalidate(ctx)
	if input.Locales != nil || input.DefaultLocale != nil {
		s.publishLocalesChanged(ctx, sv.ID)
	}
	return sv, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.invalidate(ctx)
	if s.bus != nil {
		s.bus.Publish(ctx, &Deleted{ChannelID: id})
	}
	return nil
}

// Unsets whichever channel previously held the flag
func (s *Service) SetDefault(ctx context.Context, id uuid.UUID) (Channel, error) {
	out, err := s.repo.SetDefault(ctx, id)
	if err != nil {
		return Channel{}, err
	}
	s.invalidate(ctx)
	return out, nil
}

func validateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug is required")
	}
	for _, r := range slug {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return fmt.Errorf("slug must contain only lowercase letters, digits, hyphens, and underscores")
		}
	}
	return nil
}
