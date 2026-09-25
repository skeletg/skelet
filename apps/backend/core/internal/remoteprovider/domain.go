// Package remoteprovider turns pluggable roles - shipping rates, tax calculation, address validation, promotion conditions and actions, flow actions - into merchant-registered HTTP endpoints, provider-shaped webhooks Where an event webhook OBSERVES an operation to veto or enrich it, a remote provider IMPLEMENTS a decision: core calls it mid-request and its response IS the answer Each consumer subsystem adapts its own request and response schema on top of the shared Invoke transport, which handles HMAC signing, timeout, response cache, circuit breaker and failure policy
package remoteprovider

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
)

type Kind string

const (
	KindShippingCarrier    Kind = "shipping_carrier"
	KindTaxProvider        Kind = "tax_provider"
	KindAddressValidator   Kind = "address_validator"
	KindPromotionCondition Kind = "promotion_condition"
	KindPromotionAction    Kind = "promotion_action"
	KindFraudRisk          Kind = "fraud_risk"
	KindCurrencyRate       Kind = "currency_rate"
	KindPricing            Kind = "pricing"
	KindRecommendation     Kind = "recommendation"
	KindReviews            Kind = "reviews"
	KindSearch             Kind = "search"
	KindCustomerSegments   Kind = "customer_segments"
	KindReturnsPolicy      Kind = "returns_policy"
	KindInventoryAvail     Kind = "inventory_availability"
	KindShippingTracking   Kind = "shipping_tracking"
)

// No payment kind on purpose - the roles above are near-pure functions, but an authorization needs a client secret reaching the browser for 3DS, a vault token contract, and verification of the gateway's own inbound webhooks, so proxying it would put the merchant's endpoint in PCI scope and leave core unable to verify what the gateway sent
// Gateways are Tier 1 plugins; gateway-less offline methods are core (internal/payment/offline.go)
var AllKinds = []Kind{
	KindShippingCarrier, KindTaxProvider, KindAddressValidator,
	KindPromotionCondition, KindPromotionAction,
	KindFraudRisk, KindCurrencyRate, KindPricing,
	KindRecommendation, KindReviews, KindSearch,
	KindCustomerSegments, KindReturnsPolicy,
	KindInventoryAvail, KindShippingTracking,
}

func (k Kind) Valid() bool {
	return slices.Contains(AllKinds, k)
}

// Decides what an invocation failure means to the caller - fail_open, the default, has the consumer fall back to core behavior, while fail_closed has it abort
type FailurePolicy string

const (
	FailOpen   FailurePolicy = "fail_open"
	FailClosed FailurePolicy = "fail_closed"
)

type Provider struct {
	ID        uuid.UUID
	Kind      Kind
	Slug      string
	Name      string
	TargetURL string
	// A free-form JSON blob forwarded verbatim in every invocation - API keys the receiver needs, rate table hints and the like
	Config    []byte
	TimeoutMs int32
	// Above 0, caches successful responses per request body for that many seconds - essential for hot paths like promotion evaluation
	CacheTTLs     int32
	FailurePolicy FailurePolicy
	ChannelID     *uuid.UUID
	IsActive      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type CreateInput struct {
	Kind          Kind
	Slug          string
	Name          string
	TargetURL     string
	Config        []byte
	TimeoutMs     int32
	CacheTTLs     int32
	FailurePolicy FailurePolicy
	ChannelID     *uuid.UUID
}

type UpdateInput struct {
	ID            uuid.UUID
	Name          *string
	TargetURL     *string
	Config        []byte // nil = no change, empty JSON = clear
	TimeoutMs     *int32
	CacheTTLs     *int32
	FailurePolicy *FailurePolicy
}

type CreateResult struct {
	RawSecret string
	Provider  Provider
}

var (
	ErrNotFound        = errors.New("remote provider not found")
	ErrSlugTaken       = errors.New("remote provider slug already exists")
	ErrInvalidKind     = errors.New("invalid remote provider kind")
	ErrInvalidSlug     = errors.New("invalid remote provider slug")
	ErrInvalidTimeout  = errors.New("invalid timeout")
	ErrInvalidCacheTTL = errors.New("invalid cache ttl")
	ErrInvalidPolicy   = errors.New("invalid failure policy")
	ErrCircuitOpen     = errors.New("remote provider circuit open")
)

type Repository interface {
	Create(ctx context.Context, input CreateInput, signingSecret string) (Provider, error)
	GetByID(ctx context.Context, id uuid.UUID) (Provider, error)
	GetBySlug(ctx context.Context, kind Kind, slug string, channelID *uuid.UUID) (Provider, error)
	List(ctx context.Context) ([]Provider, error)
	ListActiveByKind(ctx context.Context, kind Kind, channelID *uuid.UUID) ([]Provider, error)
	Update(ctx context.Context, input UpdateInput) (Provider, error)
	SetActive(ctx context.Context, id uuid.UUID, active bool) (Provider, error)
	UpdateSecret(ctx context.Context, id uuid.UUID, secret string) (Provider, error)
	GetSecret(ctx context.Context, id uuid.UUID) (string, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
