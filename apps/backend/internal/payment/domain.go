// Package payment owns the payment-provider surface - the Provider interface, the in-memory provider registry, and the per-(channel, provider) payment_methods table recording where each provider is installed and enabled
// Two kinds of provider land in that registry - the offline methods in offline.go ship with the engine, since COD and bank transfer are commerce primitives with no third party behind them, and gateway providers arrive as Tier 1 plugins through sdk.RegisterPaymentProvider
// The interface mirrors sdk.PaymentProvider rather than re-exporting it: plugins import the SDK type, core code uses this one, and the two are kept in lock-step by hand so the SDK stays free of internal/* imports and internal refactors can't break the SDK contract
package payment

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/permission"
)

func init() {
	permission.RegisterGroup(permission.Group{
		Slug:      "payments",
		Name:      "Payments",
		SortOrder: 80,
	})
	permission.Register(permission.Definition{
		Group: "payments",
		Slug:  "payments.view",
		Name:  "View payment methods",
	})
	permission.Register(permission.Definition{
		Group: "payments",
		Slug:  "payments.manage",
		Name:  "Manage payment methods",
	})
	// The section payment-provider plugins register their per-provider groups under - a plugin registers "payment/<slug>/api_key" as a ConfigField and the host adapter auto-registers the per-provider Group, but only if this Section exists first
	config.RegisterSection(config.Section{
		Slug:      "payment",
		Label:     "Payment",
		SortOrder: 50,
	})
}

// The per-(channel, provider) row - the provider itself lives in code, and this records that admin X enabled provider Y on channel Z at sort position N
type PaymentMethod struct {
	ID           uuid.UUID
	ChannelID    uuid.UUID
	ProviderSlug string
	IsEnabled    bool
	DisplayName  string
	SortOrder    int32
	// "all" accepts everywhere, "specific" only AllowedCountries, which the service hydrates
	CountryMode      string
	AllowedCountries []string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AuthorizeRequest, AuthorizeResult and friends mirror the sdk.* shapes - the Service translates, so providers see only SDK types and the core works in domain types
type AuthorizeRequest struct {
	OrderID      uuid.UUID
	ChannelID    uuid.UUID
	CurrencyCode string
	AmountCents  int64
	CustomerID   *uuid.UUID
	Email        string
	PaymentToken string // client-side tokenization handle - see sdk.AuthorizeRequest.PaymentToken
	// the stored method's provider token for saved-card and off-session charges - set instead of PaymentToken
	VaultedToken string
	Metadata     map[string]string
}

type AuthorizeResult struct {
	ProviderReference string
	RawResponse       []byte
	// Mirror sdk.AuthorizeResult - non-zero when the gateway parked on a client-side step
	Action PendingAction
}

// Mirrors sdk.ActionKind / sdk.PendingAction - the discriminator is what lets one storefront branch
// serve every gateway rather than switching on the provider slug
type ActionKind string

const (
	ActionNone      ActionKind = ""
	ActionRedirect  ActionKind = "redirect"
	ActionChallenge ActionKind = "challenge"
)

type PendingAction struct {
	Kind  ActionKind
	Value string
}

func (a PendingAction) Required() bool { return a.Kind != ActionNone }

type VerifyAuthorizationRequest struct {
	OrderID   uuid.UUID
	ChannelID uuid.UUID
	// What Authorize returned and core stored on the order - a redirect-based gateway has no other
	// handle on the parked payment
	ProviderReference string
	AmountCents       int64
	CurrencyCode      string
}

type VerifyAuthorizationResult struct {
	Authorized        bool
	ProviderReference string
	RawResponse       []byte
}

// The optional provider capability behind finalizeOrderPayment - type-assert a Provider to it
type AuthorizationVerifier interface {
	VerifyAuthorization(ctx context.Context, req VerifyAuthorizationRequest) (VerifyAuthorizationResult, error)
}

// VaultRequest, VaultedMethod and VaultDeleteRequest mirror the sdk.* vault shapes for saved payment methods
type VaultRequest struct {
	CustomerID uuid.UUID
	Email      string
	Token      string
	ChannelID  uuid.UUID
}

type VaultedMethod struct {
	ProviderToken string
	Brand         string
	Last4         string
	ExpMonth      int
	ExpYear       int
	RawResponse   []byte
}

type VaultDeleteRequest struct {
	ProviderToken string
	ChannelID     uuid.UUID
}

// Mirrors sdk.PaymentState / sdk.SyncPayment* - what the gateway believes now, asked rather than awaited
type PaymentState string

const (
	PaymentStatePending    PaymentState = "pending"
	PaymentStateAuthorized PaymentState = "authorized"
	PaymentStateCaptured   PaymentState = "captured"
	PaymentStateRefunded   PaymentState = "refunded"
	PaymentStateVoided     PaymentState = "voided"
	PaymentStateFailed     PaymentState = "failed"
)

type SyncPaymentRequest struct {
	OrderID           uuid.UUID
	ChannelID         uuid.UUID
	ProviderReference string
}

// Amounts are minor units, 0 meaning "not reported" rather than zero
type SyncPaymentResult struct {
	State           PaymentState
	AuthorizedCents int64
	CapturedCents   int64
	RefundedCents   int64
	RawResponse     []byte
}

// The optional provider capability behind reconciliation - type-assert a Provider to it
type PaymentSynchronizer interface {
	SyncPayment(ctx context.Context, req SyncPaymentRequest) (SyncPaymentResult, error)
}

// The optional provider capability behind saved payment methods - type-assert a Provider to it
type Vaulter interface {
	VaultPaymentMethod(ctx context.Context, req VaultRequest) (VaultedMethod, error)
	DeleteVaultedPaymentMethod(ctx context.Context, req VaultDeleteRequest) error
}

type CaptureRequest struct {
	OrderID           uuid.UUID
	ChannelID         uuid.UUID
	ProviderReference string
	AmountCents       int64
	CurrencyCode      string
	// Dedupes a retried capture at the gateway, e.g - Stripe's Idempotency-Key header - empty means no key
	IdempotencyKey string
}

type CaptureResult struct {
	ProviderReference string
	CapturedAt        time.Time
	RawResponse       []byte
}

type RefundRequest struct {
	OrderID           uuid.UUID
	ChannelID         uuid.UUID
	ProviderReference string
	AmountCents       int64
	CurrencyCode      string
	Reason            string
	// Dedupes a retried refund so a retry after a partial provider success can't double-refund - empty means no key
	IdempotencyKey string
}

type RefundResult struct {
	ProviderReference string
	RefundedAt        time.Time
	RawResponse       []byte
}

type VoidRequest struct {
	OrderID           uuid.UUID
	ChannelID         uuid.UUID
	ProviderReference string
}

type VoidResult struct {
	VoidedAt    time.Time
	RawResponse []byte
}

type WebhookResult struct {
	OrderID           uuid.UUID
	EventName         string
	ProviderReference string
	// optional movement amount in minor units - 0 falls back to name-mapped settlement
	AmountCents  int64
	CurrencyCode string
}

// The internal-side view of a payment plugin, aliased from sdk.PaymentProvider by the host adapter at install time
type Provider interface {
	Authorize(ctx context.Context, req AuthorizeRequest) (AuthorizeResult, error)
	Capture(ctx context.Context, req CaptureRequest) (CaptureResult, error)
	Refund(ctx context.Context, req RefundRequest) (RefundResult, error)
	Void(ctx context.Context, req VoidRequest) (VoidResult, error)
	HandleWebhook(ctx context.Context, r *http.Request) (WebhookResult, error)
}

// What RegisterProvider records - Slug is stable over the plugin's life, DisplayName is what the admin UI renders
type Registration struct {
	Slug        string
	Provider    Provider
	DisplayName string
	// Marks a manual-reconciliation method like COD or bank transfer - selectable at checkout but never authorized through a gateway, so the order-placement bridge skips authorize
	IsOffline bool
}

var (
	ErrProviderNotFound     = errors.New("payment: provider not found")
	ErrProviderNotInstalled = errors.New("payment: provider not installed on this channel")
	ErrProviderDisabled     = errors.New("payment: provider not enabled on this channel")
	ErrSyncUnsupported      = errors.New("payment: provider cannot report payment state")
)

// Provider rows themselves live in code, registered via the SDK, not in the DB
type Repository interface {
	// A Repository scoped to the given transaction - Service.Sync uses it to upsert every (store, provider) row atomically, since a partial Sync would leave orphan rows
	WithTx(tx *sql.Tx) Repository

	UpsertPaymentMethod(ctx context.Context, channelID uuid.UUID, providerSlug, displayName string) (PaymentMethod, error)
	GetPaymentMethod(ctx context.Context, channelID uuid.UUID, providerSlug string) (PaymentMethod, error)
	ListPaymentMethodsByChannel(ctx context.Context, channelID uuid.UUID) ([]PaymentMethod, error)
	ListEnabledPaymentMethodsByChannel(ctx context.Context, channelID uuid.UUID) ([]PaymentMethod, error)
	ListPaymentMethodsByProvider(ctx context.Context, providerSlug string) ([]PaymentMethod, error)
	SetPaymentMethodEnabled(ctx context.Context, channelID uuid.UUID, providerSlug string, enabled bool) (PaymentMethod, error)
	SetPaymentMethodSortOrder(ctx context.Context, channelID uuid.UUID, providerSlug string, sortOrder int32) (PaymentMethod, error)
	SetPaymentMethodCountryMode(ctx context.Context, channelID uuid.UUID, providerSlug, mode string) (PaymentMethod, error)
	AddCountryToPaymentMethod(ctx context.Context, methodID uuid.UUID, country string) error
	ListCountriesForPaymentMethod(ctx context.Context, methodID uuid.UUID) ([]string, error)
	ClearPaymentMethodCountries(ctx context.Context, methodID uuid.UUID) error
}
