package sdk

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Auto-prefixes keys with the plugin's own namespace, so a Stripe plugin asks for "api_key" and gets "payment/stripe/api_key"
// That's ergonomics, NOT a boundary: PluginConfig reads any path, decrypted secrets included, and a Tier 1 plugin is compiled into the process with full privileges regardless - see Manifest.Capabilities for what isolation does and doesn't mean at this tier
// A channel read falls through to default scope when unset, mirroring internal/config - pass uuid.Nil for default-only
type ConfigReader interface {
	Get(ctx context.Context, key string, channelID uuid.UUID) (string, error)
}

// Handed to the provider at order placement - it reserves funds and returns a reference the core stores against the order
type AuthorizeRequest struct {
	OrderID      uuid.UUID
	ChannelID    uuid.UUID
	CurrencyCode string // ISO 4217, uppercase
	AmountCents  int64
	CustomerID   *uuid.UUID        // for providers attaching methods to a customer object, e.g. Stripe Customers
	Email        string            // always populated, guests included
	Metadata     map[string]string // provider-opaque passthrough (UTM tags, fraud signals) - propagate where supported
	// The client-side tokenization handle: a Stripe payment-method id, a Braintree nonce, a Klarna authorization token, an Affirm checkout
	// token - every gateway needs one, so it gets a typed slot rather than a per-provider magic string in Metadata
	PaymentToken string
	// The stored method's provider token (VaultedMethod.ProviderToken) for saved-card checkout and off-session renewals - set instead of PaymentToken, never both
	VaultedToken string
	// Injected per request - never persist or share it, since a later request may carry a reader scoped to a different channel
	Config ConfigReader
}

// ProviderReference is stored against the order so later Capture/Refund calls locate the upstream record
type AuthorizeResult struct {
	ProviderReference string
	RawResponse       []byte // persisted verbatim for forensics, never parsed by core
	// Non-zero when the gateway parked on a CLIENT-side step, so the payment is NOT authorized yet - the storefront performs it, then finalizeOrderPayment confirms with the gateway rather than trusting the client that it worked
	Action PendingAction
}

// How the client completes a parked authorization - the Kind is what lets the storefront handle
// every gateway with one branch instead of switching on the provider slug
type ActionKind string

const (
	ActionNone ActionKind = "" // zero value - the authorization completed server-side
	// Send the browser to Value and wait for the customer to return, e.g. Mollie's hosted checkout
	ActionRedirect ActionKind = "redirect"
	// Run an in-page challenge with Value using the gateway's JS SDK, e.g. Stripe 3DS
	ActionChallenge ActionKind = "challenge"
)

// Value is safe to hand to the paying client either way - a client secret is scoped to its own
// intent, and a redirect URL is public by construction
type PendingAction struct {
	Kind  ActionKind
	Value string
}

func (a PendingAction) Required() bool { return a.Kind != ActionNone }

type VerifyAuthorizationRequest struct {
	OrderID   uuid.UUID
	ChannelID uuid.UUID
	// What Authorize returned and core stored against the order - the only handle a redirect-based
	// gateway has, since Mollie and friends offer no metadata search to find the payment by order
	ProviderReference string
	AmountCents       int64
	CurrencyCode      string
	Config            ConfigReader
}

type VerifyAuthorizationResult struct {
	Authorized        bool
	ProviderReference string
	RawResponse       []byte
}

// Optional - implement it when authorize can park on a client-side action, and the core will confirm with the gateway instead of trusting the client that the challenge passed
type AuthorizationVerifier interface {
	VerifyAuthorization(ctx context.Context, req VerifyAuthorizationRequest) (VerifyAuthorizationResult, error)
}

// Token is the client tokenization handle - a Braintree nonce, a Stripe payment-method id
type VaultRequest struct {
	CustomerID uuid.UUID
	Email      string
	Token      string
	ChannelID  uuid.UUID
	Config     ConfigReader
}

// ProviderToken must be self-contained - whatever the provider needs back at authorize/delete time, composites included
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
	Config        ConfigReader
}

// Optional - saved cards and subscription renewals; a provider reading the vaulted-token authorize metadata key can charge these off-session
type Vaulter interface {
	VaultPaymentMethod(ctx context.Context, req VaultRequest) (VaultedMethod, error)
	DeleteVaultedPaymentMethod(ctx context.Context, req VaultDeleteRequest) error
}

type CaptureRequest struct {
	OrderID           uuid.UUID
	ChannelID         uuid.UUID
	ProviderReference string
	AmountCents       int64  // 0 = capture the full authorized amount
	CurrencyCode      string // ISO 4217 - gateways that want a currency on every amount would otherwise re-read the payment to learn its own
	IdempotencyKey    string // gateway dedupe key for a retried capture; empty = none
	Config            ConfigReader
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
	AmountCents       int64  // 0 = refund the full captured amount
	CurrencyCode      string // ISO 4217, as on CaptureRequest
	Reason            string
	IdempotencyKey    string // gateway dedupe key for a retried refund; empty = none
	Config            ConfigReader
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
	Config            ConfigReader
}

type VoidResult struct {
	VoidedAt    time.Time
	RawResponse []byte
}

// The provider describes what happened and the core publishes it, so plugins never touch the event bus
type WebhookResult struct {
	OrderID uuid.UUID
	// Published as a generic payment.provider_webhook carrying this string - the order bridge maps the payment.authorized/captured/refunded/voided constants onto an order payment status, and any other value publishes but settles nothing, which is the seam for provider-specific events your own subscribers branch on
	// Empty means nothing to publish, and the core returns 204 without touching the order
	EventName         string
	ProviderReference string // upstream record id, so subscribers can correlate
	// Optional movement amount in minor units - set it on captured/refunded events so a PARTIAL capture or refund
	// settles by amount instead of blindly flipping the whole order's payment status; 0 means the event carries no
	// amount and the order bridge falls back to name mapping
	AmountCents  int64
	CurrencyCode string // ISO 4217, set when AmountCents is
}

// A struct so the SDK can add fields later without breaking the provider interface
type WebhookRequest struct {
	HTTPRequest *http.Request
	Config      ConfigReader
	// The channel the push belongs to, when the registered webhook URL carries ?channel=<uuid> -
	// uuid.Nil otherwise, and config reads then resolve at default scope. Merchants with per-channel
	// credentials register one URL per channel with the gateway, channel param included
	ChannelID uuid.UUID
}

// What a gateway currently believes about a payment, independent of any webhook
type PaymentState string

const (
	PaymentStatePending    PaymentState = "pending"    // parked on a client step, or the gateway is still deciding
	PaymentStateAuthorized PaymentState = "authorized" // funds held, nothing captured
	PaymentStateCaptured   PaymentState = "captured"   // fully or partly captured - CapturedCents says which
	PaymentStateRefunded   PaymentState = "refunded"   // fully or partly refunded - RefundedCents says which
	PaymentStateVoided     PaymentState = "voided"     // released or cancelled, no money moved
	PaymentStateFailed     PaymentState = "failed"     // declined, expired or abandoned
)

type SyncPaymentRequest struct {
	OrderID           uuid.UUID
	ChannelID         uuid.UUID
	ProviderReference string
	Config            ConfigReader
}

// Amounts are minor units in the payment's own currency, and 0 means "the gateway didn't say"
// rather than "zero" - treat State as authoritative and the amounts as best-effort detail
type SyncPaymentResult struct {
	State           PaymentState
	AuthorizedCents int64
	CapturedCents   int64
	RefundedCents   int64
	RawResponse     []byte
}

// Lets core ASK a gateway where a payment stands rather than only learning from webhooks it may
// never receive - a redirect or BNPL customer who closes the tab leaves an order that no
// notification will ever settle
// Implementing it also collapses the re-fetch each async provider already does by hand inside
// HandleWebhook and VerifyAuthorization onto one method
type PaymentSynchronizer interface {
	SyncPayment(ctx context.Context, req SyncPaymentRequest) (SyncPaymentResult, error)
}

// Honour the ctx deadline on outbound calls - an error aborts the originating operation, so a failed Authorize fails order placement
type PaymentProvider interface {
	Authorize(ctx context.Context, req AuthorizeRequest) (AuthorizeResult, error)
	Capture(ctx context.Context, req CaptureRequest) (CaptureResult, error)
	Refund(ctx context.Context, req RefundRequest) (RefundResult, error)
	Void(ctx context.Context, req VoidRequest) (VoidResult, error)
	// Called on POST to /api/v1/payments/webhook/<slug> - verify the signature yourself against your own webhook secret from req.Config, and return an error for anything unverified or malformed
	HandleWebhook(ctx context.Context, req WebhookRequest) (WebhookResult, error)
}

type ProviderOption func(*ProviderRegistration)

type ProviderRegistration struct {
	// The owning plugin's manifest slug - what the host keys the kill switch, timeout, rate limit and metrics on, as distinct from Slug which names the provider
	PluginSlug  string
	Slug        string
	Provider    PaymentProvider
	DisplayName string
	// Never charged through a gateway - the host skips authorize-at-placement, the order places unpaid, and an operator reconciles it later
	IsOffline bool
}

// Falls back to the raw provider slug when unset - NOT title-cased, so an unset name renders as "bank-transfer" in the admin
func WithDisplayName(name string) ProviderOption {
	return func(r *ProviderRegistration) {
		r.DisplayName = name
	}
}

// Manual reconciliation - no client tokenization and no authorization at placement, so the order stays unpaid until an operator records it
func WithOffline() ProviderOption {
	return func(r *ProviderRegistration) {
		r.IsOffline = true
	}
}

// Mirrors internal/config, redeclared so plugins don't import internal/* - the host adapter keeps the two in sync by hand
type ConfigFieldType string

const (
	TypeString ConfigFieldType = "string"
	TypeInt    ConfigFieldType = "int"
	TypeBool   ConfigFieldType = "bool"
	TypeSecret ConfigFieldType = "secret" // stored encrypted, never returned via API
	TypeJSON   ConfigFieldType = "json"
)

// Path is internal/config's "section/group/field" form
type ConfigField struct {
	// Required - an empty or unrecognised value fails boot rather than defaulting to string, since a mistyped TypeSecret would store an API key in plaintext and hand it back through the admin API
	Type         ConfigFieldType
	Description  string
	DefaultValue string // rendered as a string, normalised per Type on read
}

// How the OUTBOUND webhook dispatcher treats the event - async fires post-commit onto a durable queue with retries and ignores the receiver's response, sync fires pre-commit inline and lets the receiver reject the whole operation
// The two pools are disjoint: a sync webhook may only subscribe to sync event types
type EventKind string

const (
	EventKindAsync EventKind = "async"
	EventKindSync  EventKind = "sync"
)
