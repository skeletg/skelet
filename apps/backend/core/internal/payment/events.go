package payment

import "github.com/google/uuid"

// "-ed" past tense is the post-commit, fire-and-forget async convention
const (
	EventAuthorized      = "payment.authorized"
	EventCaptured        = "payment.captured"
	EventRefunded        = "payment.refunded"
	EventVoided          = "payment.voided"
	EventProviderWebhook = "payment.provider_webhook"
)

type PaymentAuthorized struct {
	OrderID           uuid.UUID
	ProviderSlug      string
	ProviderReference string
	AmountCents       int64
	CurrencyCode      string
}

func (PaymentAuthorized) Type() string { return EventAuthorized }

type PaymentCaptured struct {
	OrderID           uuid.UUID
	ProviderSlug      string
	ProviderReference string
	AmountCents       int64
}

func (PaymentCaptured) Type() string { return EventCaptured }

type PaymentRefunded struct {
	OrderID           uuid.UUID
	ProviderSlug      string
	ProviderReference string
	AmountCents       int64
	Reason            string
}

func (PaymentRefunded) Type() string { return EventRefunded }

type PaymentVoided struct {
	OrderID           uuid.UUID
	ProviderSlug      string
	ProviderReference string
}

func (PaymentVoided) Type() string { return EventVoided }

// ProviderWebhook is published when a registered provider's HandleWebhook returns a non-empty result - Subscribers
// branch on EventName to decide what to do
type ProviderWebhook struct {
	ProviderSlug      string
	OrderID           uuid.UUID
	EventName         string // free-form, set by the provider
	ProviderReference string
	// optional movement amount in minor units - the order bridge settles by amount when set
	AmountCents  int64
	CurrencyCode string
}

func (ProviderWebhook) Type() string { return EventProviderWebhook }
