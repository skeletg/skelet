// Package channel manages channels - distinct storefront instances carrying the commerce policy for one subsidiary: currency, supported locales, branding, legal info and the storefront URL outbound emails use
// Translations are NOT keyed by channel - that's a separate orthogonal layer, keyed by locale only
package channel

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
)

const (
	PermView   = "channels.view"
	PermManage = "channels.manage"
)

// The request headers naming a channel - the front surface honours only the id, the admin surface both
const (
	HeaderChannelID = "X-Channel-Id"
	HeaderStoreSlug = "X-Store-Slug"
)

// The default locale of last resort, when no channel is flagged default to inherit one from
const FallbackLocale = "en"

const (
	maxSlugLength          = 64
	maxMetadataKeys        = 50
	maxMetadataValueLength = 1024
)

func init() {
	permission.RegisterGroup(permission.Group{
		Slug:      "channels",
		Name:      "Channels",
		SortOrder: 60,
	})
	permission.Register(permission.Definition{
		Group: "channels",
		Slug:  PermView,
		Name:  "View channels",
	})
	permission.Register(permission.Definition{
		Group: "channels",
		Slug:  PermManage,
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
	DefaultLocale string   // "" inherits the default channel's, else FallbackLocale
	Locales       []string // codes; the default locale is appended when missing
	CurrencyCode  string   // required
	PublicURL     string
	Metadata      Metadata
}

type UpdateChannelInput struct {
	ID            uuid.UUID
	Name          *string
	DefaultLocale *string
	Locales       []string // nil = no change; pass codes to replace the supported-locales set, which always keeps the default
	IsActive      *bool
	CurrencyCode  *string
	PublicURL     *string
	Metadata      *Metadata
}

var (
	ErrChannelNotFound          = errors.New("channel not found")
	ErrChannelInUse             = errors.New("channel still has orders, listings, carts or gift cards; reassign or remove them first")
	ErrSlugTaken                = errors.New("channel slug already in use")
	ErrInvalidSlug              = errors.New("slug must be 1-64 lowercase letters, digits, hyphens or underscores")
	ErrNameRequired             = errors.New("channel name is required")
	ErrInvalidCurrency          = errors.New("currency_code must be a 3-letter ISO 4217 code")
	ErrInvalidPublicURL         = errors.New("public URL must be empty or an absolute http(s) URL with a host and no credentials, query or fragment")
	ErrStorefrontHostTaken      = errors.New("another channel already serves this storefront host")
	ErrMetadataTooLarge         = errors.New("channel metadata allows at most 50 keys and 1024 characters per value")
	ErrDefaultChannelInactive   = errors.New("an inactive channel cannot be the default; activate it first")
	ErrDeactivateDefaultChannel = errors.New("the default channel cannot be deactivated; make another channel the default first")
)

type Repository interface {
	// Runs fn against a repository bound to one transaction - calls on an already-bound repository join it
	Atomic(ctx context.Context, fn func(Repository) error) error
	Create(ctx context.Context, input CreateChannelInput) (Channel, error)
	GetByID(ctx context.Context, id uuid.UUID) (Channel, error)
	// Holds the row lock until the surrounding Atomic commits
	GetForUpdate(ctx context.Context, id uuid.UUID) (Channel, error)
	GetBySlug(ctx context.Context, slug string) (Channel, error)
	GetByStorefrontHost(ctx context.Context, host string) (Channel, error)
	GetDefault(ctx context.Context) (Channel, error)
	List(ctx context.Context) ([]Channel, error)
	ListByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Channel, error)
	// Writes every column of ch; metadata is only written when writeMetadata is set
	Update(ctx context.Context, ch Channel, writeMetadata bool) (Channel, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// Flips is_default=true on the given channel and clears it from every other row, returning the channels that lost it - the unique partial index on is_default=true needs both statements in one transaction
	SetDefault(ctx context.Context, id uuid.UUID) (Channel, []uuid.UUID, error)
	CountInheritingListings(ctx context.Context, channelID uuid.UUID) (int64, error)

	ListLocales(ctx context.Context, channelID uuid.UUID) ([]locale.Locale, error)
	ListLocalesByStoreIDs(ctx context.Context, channelIDs []uuid.UUID) (map[uuid.UUID][]locale.Locale, error)
	// Replaces the entire locale set for a store atomically, delete plus insert in one tx - each code is FK-validated on insert, and callers should pre-validate through locale.Service.EnsureExists for a friendlier error
	SetLocales(ctx context.Context, channelID uuid.UUID, codes []string) error
}
