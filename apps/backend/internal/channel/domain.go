// Package channel manages channels - distinct storefront instances carrying the commerce policy for one subsidiary: currency, supported locales, branding, legal info and the storefront URL outbound emails use
// Translations are NOT keyed by channel - that's a separate orthogonal layer, keyed by locale only
package channel

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/locale"
	"github.com/skeletg/skelet/apps/backend/internal/permission"
)

func init() {
	permission.RegisterGroup(permission.Group{
		Slug:      "channels",
		Name:      "Channels",
		SortOrder: 60,
	})
	permission.Register(permission.Definition{
		Group: "channels",
		Slug:  "channels.view",
		Name:  "View channels",
	})
	permission.Register(permission.Definition{
		Group: "channels",
		Slug:  "channels.manage",
		Name:  "Manage channels",
	})
}

// One subsidiary storefront - it can serve multiple locales but holds exactly one currency
// Anything past the identity columns below - branding, contact, subsidiary, legal, timezone - lives in the config registry at per-channel scope and is edited on the Settings page
type Channel struct {
	ID            uuid.UUID
	Slug          string
	Name          string
	DefaultLocale string          // BCP 47 code; FK to locales.code
	Locales       []locale.Locale // resolved locale rows; populated by ListLocales lookups
	IsActive      bool
	// Marks the storefront the front endpoint falls back to when neither X-Channel-Id nor Host resolves - a unique partial index guarantees at most one row carries it
	// Empty when no operator has elected a default, in which case a request identifying no channel gets none, and resolvers needing one answer STORE_REQUIRED
	IsDefault    bool
	CurrencyCode string // ISO 4217, uppercase
	PublicURL    string // base URL used for password reset and verification email links
	Metadata     Metadata
	// Flips the storefront's display and checkout math - prices are stored gross, already VAT-inclusive, and the tax service splits the rate out of the gross amount at checkout rather than adding on top - Standard for EU consumer stores
	TaxInclusivePricing bool
	CreatedAt           time.Time
}

type Metadata map[string]string

type CreateChannelInput struct {
	Slug          string
	Name          string
	DefaultLocale string   // "" defaults to "en"
	Locales       []string // codes; empty = seed from DefaultLocale - validated via FK on insert
	CurrencyCode  string   // "" defaults to global config currency
	PublicURL     string
	Metadata      Metadata
}

type UpdateChannelInput struct {
	ID            uuid.UUID
	Name          *string
	DefaultLocale *string
	Locales       []string // nil = no change; pass codes to replace the supported-locales set
	IsActive      *bool
	CurrencyCode  *string
	PublicURL     *string
	Metadata      *Metadata
}

var (
	ErrChannelNotFound    = errors.New("channel not found")
	ErrChannelInUse       = errors.New("channel still has orders, listings or carts; reassign or remove them first")
	ErrSlugTaken          = errors.New("channel slug already in use")
	ErrLocaleNotSupported = errors.New("locale is not in the channel's supported locales list")
	ErrInvalidCurrency    = errors.New("currency_code must be a 3-letter ISO 4217 code")
)

type Repository interface {
	Create(ctx context.Context, input CreateChannelInput) (Channel, error)
	GetByID(ctx context.Context, id uuid.UUID) (Channel, error)
	GetBySlug(ctx context.Context, slug string) (Channel, error)
	GetByStorefrontHost(ctx context.Context, host string) (Channel, error)
	GetDefault(ctx context.Context) (Channel, error)
	List(ctx context.Context) ([]Channel, error)
	ListByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Channel, error)
	Update(ctx context.Context, input UpdateChannelInput) (Channel, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// Flips is_default=true on the given channel and clears it from every other row in the same transaction - the DB has a unique partial index on is_default=true, so doing it in two separate statements would race against itself
	SetDefault(ctx context.Context, id uuid.UUID) (Channel, error)

	ListLocales(ctx context.Context, channelID uuid.UUID) ([]locale.Locale, error)
	ListLocalesByStoreIDs(ctx context.Context, channelIDs []uuid.UUID) (map[uuid.UUID][]locale.Locale, error)
	// Replaces the entire locale set for a store atomically, delete plus insert in one tx - each code is FK-validated on insert, and callers should pre-validate through locale.Service.EnsureExists for a friendlier error
	SetLocales(ctx context.Context, channelID uuid.UUID, codes []string) error
}
