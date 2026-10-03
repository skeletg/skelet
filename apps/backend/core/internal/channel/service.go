package channel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
)

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

// Wires a new channel to the configured default inventory location - declared locally so channel doesn't import inventory, and when nil new channels start with no assignments
type LocationAssigner interface {
	AssignDefaultLocation(ctx context.Context, channelID uuid.UUID) error
}

type Service struct {
	repo       Repository
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

const (
	// Generous because channel rows mutate rarely - any admin write drops the namespace immediately, so this only bounds staleness for writes that bypass the service
	channelCacheTTL = 30 * time.Minute
	// Every storefront request resolves its channel by id, so the row is cached - briefly, since a write racing a read could park the old row
	channelIDCacheTTL = time.Minute
	// Misses are remembered so unknown hosts and default-less installs don't re-query, briefly so a new row shows up soon
	channelMissCacheTTL = time.Minute
)

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) SetCache(c *cache.Cache) {
	s.cache = c
}

func (s *Service) invalidate(ctx context.Context) {
	s.cache.Flush(ctx)
}

func (s *Service) publish(ctx context.Context, e events.Event) {
	if s.bus != nil {
		s.bus.Publish(ctx, e)
	}
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

func (s *Service) normalizeCurrency(ctx context.Context, code string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 3 {
		return "", ErrInvalidCurrency
	}
	if s.currencies != nil {
		if err := s.currencies.EnsureExists(ctx, code); err != nil {
			return "", err
		}
	}
	return code, nil
}

// The default channel's default locale, else FallbackLocale
func (s *Service) InstallDefaultLocale(ctx context.Context) string {
	if sv, err := s.GetDefault(ctx); err == nil && sv.DefaultLocale != "" {
		return sv.DefaultLocale
	}
	return FallbackLocale
}

func (s *Service) Create(ctx context.Context, input CreateChannelInput) (Channel, error) {
	if err := validateSlug(input.Slug); err != nil {
		return Channel{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return Channel{}, ErrNameRequired
	}
	if err := validateMetadata(input.Metadata); err != nil {
		return Channel{}, err
	}
	var err error
	if input.PublicURL, err = normalizePublicURL(input.PublicURL); err != nil {
		return Channel{}, err
	}
	// Required at creation - defaulting to USD silently would mask misconfiguration until a non-USD operator's first order stamped the wrong totals
	if input.CurrencyCode, err = s.normalizeCurrency(ctx, input.CurrencyCode); err != nil {
		return Channel{}, err
	}
	input.DefaultLocale = normalizeLocale(input.DefaultLocale)
	if input.DefaultLocale == "" {
		input.DefaultLocale = s.InstallDefaultLocale(ctx)
	}
	input.Locales = localeSet(input.Locales, input.DefaultLocale)
	if err := s.validateLocaleCodes(ctx, input.Locales); err != nil {
		return Channel{}, err
	}
	var sv Channel
	err = s.repo.Atomic(ctx, func(repo Repository) error {
		created, err := repo.Create(ctx, input)
		if err != nil {
			return err
		}
		if err := repo.SetLocales(ctx, created.ID, input.Locales); err != nil {
			return err
		}
		if created.Locales, err = repo.ListLocales(ctx, created.ID); err != nil {
			return err
		}
		sv = created
		return nil
	})
	if err != nil {
		return Channel{}, err
	}
	// New channels inherit the shared-warehouse stock by default - best-effort, since the channel itself exists
	if s.locations != nil {
		if err := s.locations.AssignDefaultLocation(ctx, sv.ID); err != nil {
			slog.Error("channel: assign default inventory location", "channel_id", sv.ID, "err", err)
		}
	}
	s.invalidate(ctx)
	s.publish(ctx, &Created{ChannelID: sv.ID})
	return sv, nil
}

// One Valkey read-through: a hit returns the cached row, a miss loads and hydrates locales, and with cacheMiss a not-found is remembered as a zero-ID row
func (s *Service) cachedRead(ctx context.Context, key string, ttl time.Duration, cacheMiss bool, load func() (Channel, error)) (Channel, error) {
	if cached, ok := cache.GetJSON[Channel](ctx, s.cache, key); ok {
		if cached.ID == uuid.Nil {
			return Channel{}, ErrChannelNotFound
		}
		return cached, nil
	}
	sv, err := load()
	if err != nil {
		if cacheMiss && errors.Is(err, ErrChannelNotFound) {
			s.cache.SetJSON(ctx, key, Channel{}, channelMissCacheTTL)
		}
		return Channel{}, err
	}
	if sv.Locales, err = s.repo.ListLocales(ctx, sv.ID); err != nil {
		return Channel{}, err
	}
	s.cache.SetJSON(ctx, key, sv, ttl)
	return sv, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Channel, error) {
	return s.cachedRead(ctx, "id:"+id.String(), channelIDCacheTTL, false, func() (Channel, error) { return s.repo.GetByID(ctx, id) })
}

func (s *Service) GetBySlug(ctx context.Context, slug string) (Channel, error) {
	return s.cachedRead(ctx, "slug:"+slug, channelCacheTTL, false, func() (Channel, error) { return s.repo.GetBySlug(ctx, slug) })
}

// host must already be normalized - an empty or oversized one is a miss that never reaches the cache
func (s *Service) GetByStorefrontHost(ctx context.Context, host string) (Channel, error) {
	if host == "" || len(host) > maxHostPortLength {
		return Channel{}, ErrChannelNotFound
	}
	return s.cachedRead(ctx, "host:"+host, channelCacheTTL, true, func() (Channel, error) { return s.repo.GetByStorefrontHost(ctx, host) })
}

// ErrChannelNotFound when no row carries is_default - the middleware reads that as "no store context"; the miss is cached because the default-locale fallback alone asks several times per storefront request
func (s *Service) GetDefault(ctx context.Context) (Channel, error) {
	return s.cachedRead(ctx, "default", channelCacheTTL, true, func() (Channel, error) { return s.repo.GetDefault(ctx) })
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

// Replaces the supported set - the channel's default locale stays in it, appended when the codes leave it out
func (s *Service) SetLocales(ctx context.Context, channelID uuid.UUID, codes []string) (Channel, error) {
	if err := s.validateLocaleCodes(ctx, codes); err != nil {
		return Channel{}, err
	}
	var sv Channel
	err := s.repo.Atomic(ctx, func(repo Repository) error {
		current, err := repo.GetForUpdate(ctx, channelID)
		if err != nil {
			return err
		}
		if err := repo.SetLocales(ctx, channelID, localeSet(codes, current.DefaultLocale)); err != nil {
			return err
		}
		if current.Locales, err = repo.ListLocales(ctx, channelID); err != nil {
			return err
		}
		sv = current
		return nil
	})
	if err != nil {
		return Channel{}, err
	}
	s.invalidate(ctx)
	s.publish(ctx, &LocalesChanged{ChannelID: channelID})
	return sv, nil
}

func (s *Service) Update(ctx context.Context, input UpdateChannelInput) (Channel, error) {
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return Channel{}, ErrNameRequired
	}
	if input.CurrencyCode != nil {
		code, err := s.normalizeCurrency(ctx, *input.CurrencyCode)
		if err != nil {
			return Channel{}, err
		}
		input.CurrencyCode = &code
	}
	if input.PublicURL != nil {
		u, err := normalizePublicURL(*input.PublicURL)
		if err != nil {
			return Channel{}, err
		}
		input.PublicURL = &u
	}
	if input.Metadata != nil {
		if err := validateMetadata(*input.Metadata); err != nil {
			return Channel{}, err
		}
	}
	if input.DefaultLocale != nil {
		code := normalizeLocale(*input.DefaultLocale)
		if err := s.validateLocaleCodes(ctx, []string{code}); err != nil {
			return Channel{}, err
		}
		input.DefaultLocale = &code
	}
	if input.Locales != nil {
		if err := s.validateLocaleCodes(ctx, input.Locales); err != nil {
			return Channel{}, err
		}
	}
	var sv Channel
	err := s.repo.Atomic(ctx, func(repo Repository) error {
		current, err := repo.GetForUpdate(ctx, input.ID)
		if err != nil {
			return err
		}
		next := applyUpdate(current, input)
		if current.IsDefault && !next.IsActive {
			return ErrDeactivateDefaultChannel
		}
		// Listings inheriting price would silently reprice in the new currency, so operators convert them to explicit first - counted under the row lock, in this transaction
		if next.CurrencyCode != current.CurrencyCode {
			count, err := repo.CountInheritingListings(ctx, input.ID)
			if err != nil {
				return err
			}
			if count > 0 {
				return fmt.Errorf("%w: %d listing(s) inherit price on this store", ErrStoreCurrencyChangeBlocked, count)
			}
		}
		updated, err := repo.Update(ctx, next, input.Metadata != nil)
		if err != nil {
			return err
		}
		if updated.Locales, err = repo.ListLocales(ctx, updated.ID); err != nil {
			return err
		}
		if input.Locales != nil || input.DefaultLocale != nil {
			have := localeCodes(updated)
			base := have
			if input.Locales != nil {
				base = input.Locales
			}
			if want := localeSet(base, updated.DefaultLocale); !slices.Equal(want, have) {
				if err := repo.SetLocales(ctx, updated.ID, want); err != nil {
					return err
				}
				if updated.Locales, err = repo.ListLocales(ctx, updated.ID); err != nil {
					return err
				}
			}
		}
		sv = updated
		return nil
	})
	if err != nil {
		return Channel{}, err
	}
	s.invalidate(ctx)
	s.publish(ctx, &Updated{ChannelID: sv.ID})
	if input.Locales != nil || input.DefaultLocale != nil {
		s.publish(ctx, &LocalesChanged{ChannelID: sv.ID})
	}
	return sv, nil
}

func applyUpdate(ch Channel, in UpdateChannelInput) Channel {
	if in.Name != nil {
		ch.Name = *in.Name
	}
	if in.DefaultLocale != nil {
		ch.DefaultLocale = *in.DefaultLocale
	}
	if in.IsActive != nil {
		ch.IsActive = *in.IsActive
	}
	if in.CurrencyCode != nil {
		ch.CurrencyCode = *in.CurrencyCode
	}
	if in.PublicURL != nil {
		ch.PublicURL = *in.PublicURL
	}
	if in.Metadata != nil {
		ch.Metadata = *in.Metadata
	}
	return ch
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.invalidate(ctx)
	s.publish(ctx, &Deleted{ChannelID: id})
	return nil
}

// Unsets whichever channel previously held the flag - only an active channel can take it
func (s *Service) SetDefault(ctx context.Context, id uuid.UUID) (Channel, error) {
	var (
		out     Channel
		cleared []uuid.UUID
	)
	err := s.repo.Atomic(ctx, func(repo Repository) error {
		current, err := repo.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if !current.IsActive {
			return ErrDefaultChannelInactive
		}
		if out, cleared, err = repo.SetDefault(ctx, id); err != nil {
			return err
		}
		out.Locales, err = repo.ListLocales(ctx, id)
		return err
	})
	if err != nil {
		return Channel{}, err
	}
	s.invalidate(ctx)
	s.publish(ctx, &Updated{ChannelID: id})
	for _, prev := range cleared {
		s.publish(ctx, &Updated{ChannelID: prev})
	}
	return out, nil
}

func validateSlug(slug string) error {
	if slug == "" || len(slug) > maxSlugLength {
		return ErrInvalidSlug
	}
	for _, r := range slug {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return ErrInvalidSlug
		}
	}
	return nil
}

func validateMetadata(m Metadata) error {
	if len(m) > maxMetadataKeys {
		return ErrMetadataTooLarge
	}
	for _, v := range m {
		if len(v) > maxMetadataValueLength {
			return ErrMetadataTooLarge
		}
	}
	return nil
}

func normalizeLocale(code string) string { return strings.ToLower(strings.TrimSpace(code)) }

// Normalized, deduplicated codes in their given order, with def appended when missing
func localeSet(codes []string, def string) []string {
	out := make([]string, 0, len(codes)+1)
	for _, c := range codes {
		if c = normalizeLocale(c); c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	if def = normalizeLocale(def); def != "" && !slices.Contains(out, def) {
		out = append(out, def)
	}
	return out
}

func localeCodes(ch Channel) []string {
	out := make([]string, len(ch.Locales))
	for i, l := range ch.Locales {
		out[i] = l.Code
	}
	return out
}
