package order

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/remoteprovider"
)

// A merchant HTTP endpoint (remoteprovider kind=fraud_risk) screening an order before it commits - the escape hatch for Signifyd, Riskified or Kount without a Go plugin
// Runs at the pre-transaction gate, after checkout.validating and before the placement tx, so a slow screen never holds a DB transaction open
// The AUTHORITATIVE risk engine, one per channel - its decision can reject outright, and its score feeds the risk-hold gate, max'd with any sync-webhook riskScore signals
// fail_open approves on a transport error, so a down screen never blocks sales, while fail_closed aborts
// Wire contract: request {channelId, orderId, customerId, email, currency, subtotalCents, discountCents, paymentProvider, billingAddress, shippingAddress, lines}, response {decision, score, reason}

type RemoteFraudSource interface {
	// Returns a risk score, 0 with no provider or fail_open - reject carries a reason, and a non-nil error means a fail_closed provider failed, so abort
	Screen(ctx context.Context, in RemoteFraudInput) (score int, reject bool, reason string, err error)
}

type RemoteFraudInput struct {
	ChannelID       uuid.UUID
	OrderID         uuid.UUID
	CustomerID      *uuid.UUID
	Email           string
	Currency        string
	SubtotalCents   int64
	DiscountCents   int64
	PaymentProvider string
	Billing         OrderAddress
	Shipping        OrderAddress
	Lines           []RemoteTaxLine // reuse the {sku, quantity, unitPriceCents} shape
}

// Optional
func (s *Service) SetRemoteFraud(src RemoteFraudSource) { s.remoteFraud = src }

type remoteFraudRequest struct {
	ChannelID       string             `json:"channelId"`
	OrderID         string             `json:"orderId"`
	CustomerID      string             `json:"customerId,omitempty"`
	Email           string             `json:"email"`
	Currency        string             `json:"currency"`
	SubtotalCents   int64              `json:"subtotalCents"`
	DiscountCents   int64              `json:"discountCents"`
	PaymentProvider string             `json:"paymentProvider,omitempty"`
	BillingAddress  remoteTaxAddress   `json:"billingAddress"`
	ShippingAddress remoteTaxAddress   `json:"shippingAddress"`
	Lines           []remoteTaxReqLine `json:"lines"`
}

type remoteFraudResponse struct {
	Decision string `json:"decision"`
	Score    int    `json:"score"`
	Reason   string `json:"reason"`
}

type remoteFraudSource struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func NewRemoteFraudSource(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) RemoteFraudSource {
	return &remoteFraudSource{svc: svc, invoker: invoker}
}

func (r *remoteFraudSource) Screen(ctx context.Context, in RemoteFraudInput) (int, bool, string, error) {
	ch := in.ChannelID
	providers, err := r.svc.ListActiveByKind(ctx, remoteprovider.KindFraudRisk, &ch)
	if err != nil {
		// Not the same as "no provider": falling through skips screening entirely and the order places clean with no risk hold, which is exactly
		// what a carding run wants during any DB degradation. The Invoke path below already holds on FailClosed
		return 0, false, "", fmt.Errorf("fraud provider lookup: %w", err)
	}
	if len(providers) == 0 {
		return 0, false, "", nil // no provider - no screening
	}
	p := providers[0] // one authoritative fraud engine per channel

	var resp remoteFraudResponse
	if err := r.invoker.Invoke(ctx, p, remoteFraudRequestFrom(in), &resp); err != nil {
		if p.FailurePolicy == remoteprovider.FailClosed {
			return 0, true, fmt.Sprintf("fraud screen unavailable (%s)", p.Slug), nil
		}
		return 0, false, "", nil // fail_open - approve, no score
	}
	score := max(resp.Score, 0)
	if resp.Decision == "reject" {
		reason := resp.Reason
		if reason == "" {
			reason = "rejected by fraud screen"
		}
		return score, true, reason, nil
	}
	return score, false, "", nil
}

func remoteFraudRequestFrom(in RemoteFraudInput) remoteFraudRequest {
	lines := make([]remoteTaxReqLine, len(in.Lines))
	for i, l := range in.Lines {
		lines[i] = remoteTaxReqLine(l)
	}
	req := remoteFraudRequest{
		ChannelID:       in.ChannelID.String(),
		OrderID:         in.OrderID.String(),
		Email:           in.Email,
		Currency:        in.Currency,
		SubtotalCents:   in.SubtotalCents,
		DiscountCents:   in.DiscountCents,
		PaymentProvider: in.PaymentProvider,
		BillingAddress:  taxAddressFrom(in.Billing),
		ShippingAddress: taxAddressFrom(in.Shipping),
		Lines:           lines,
	}
	if in.CustomerID != nil {
		req.CustomerID = in.CustomerID.String()
	}
	return req
}

// Folds an OrderAddress into the shared wire address shape, also used by the tax provider
func taxAddressFrom(a OrderAddress) remoteTaxAddress {
	region := ""
	if a.State != nil {
		region = *a.State
	}
	street2 := ""
	if a.Street2 != nil {
		street2 = *a.Street2
	}
	return remoteTaxAddress{
		Street1: a.Street1, Street2: street2, City: a.City,
		Region: region, PostalCode: a.PostalCode, CountryCode: a.CountryCode,
	}
}
