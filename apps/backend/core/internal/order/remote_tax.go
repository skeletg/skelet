package order

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

// Order tax computed by a merchant HTTP endpoint (remoteprovider kind=tax_provider) instead of the core zone engine - the escape hatch for AvaTax or TaxJar without a Go plugin With one on a channel, placement writes its result through the OrderPlacing.TaxCents seam, and core's per-line calc only runs when no provider claims the order fail_open falls back to core and fail_closed aborts - Wire contract: request {channelId, currency, taxInclusive, subtotalCents, discountCents, shippingCents, customerGroup, taxExempt, address, lines}, response {taxCents}

type RemoteTaxSource interface {
	// The order's total tax - ok=false means no provider or fail_open, so use core, while a non-nil error means a fail_closed provider failed and placement aborts
	Tax(ctx context.Context, in RemoteTaxInput) (cents int64, ok bool, err error)
}

type RemoteTaxInput struct {
	ChannelID     uuid.UUID
	Currency      string
	TaxInclusive  bool
	SubtotalCents int64
	DiscountCents int64
	ShippingCents int64
	CustomerGroup string
	TaxExempt     bool
	Address       OrderAddress
	Lines         []RemoteTaxLine
}

type RemoteTaxLine struct {
	SKU            string
	Quantity       float64
	UnitPriceCents int64
}

// Optional - nil leaves tax to the core calculator
func (s *Service) SetRemoteTax(src RemoteTaxSource) { s.remoteTax = src }

func remoteTaxInputFromCheckout(cc *checkoutContext, discountCents, shippingCents int64) RemoteTaxInput {
	lines := make([]RemoteTaxLine, len(cc.cart.Lines))
	for i, l := range cc.cart.Lines {
		lines[i] = RemoteTaxLine{SKU: l.ProductSku, Quantity: l.Quantity, UnitPriceCents: l.UnitPrice}
	}
	if discountCents < 0 {
		discountCents = 0
	}
	return RemoteTaxInput{
		ChannelID:     cc.store.ID,
		Currency:      cc.store.CurrencyCode,
		TaxInclusive:  cc.store.TaxInclusivePricing,
		SubtotalCents: cc.subtotal,
		DiscountCents: discountCents,
		ShippingCents: shippingCents,
		CustomerGroup: customerGroupSlug(cc.customer),
		TaxExempt:     cc.customer.TaxExempt,
		Address:       cc.shipping,
		Lines:         lines,
	}
}

type remoteTaxRequest struct {
	ChannelID     string             `json:"channelId"`
	Currency      string             `json:"currency"`
	TaxInclusive  bool               `json:"taxInclusive"`
	SubtotalCents int64              `json:"subtotalCents"`
	DiscountCents int64              `json:"discountCents"`
	ShippingCents int64              `json:"shippingCents"`
	CustomerGroup string             `json:"customerGroup,omitempty"`
	TaxExempt     bool               `json:"taxExempt"`
	Address       remoteTaxAddress   `json:"address"`
	Lines         []remoteTaxReqLine `json:"lines"`
}

type remoteTaxAddress struct {
	Street1     string `json:"street1"`
	Street2     string `json:"street2,omitempty"`
	City        string `json:"city"`
	Region      string `json:"region,omitempty"`
	PostalCode  string `json:"postalCode"`
	CountryCode string `json:"countryCode"`
}

type remoteTaxReqLine struct {
	SKU            string  `json:"sku"`
	Quantity       float64 `json:"quantity"`
	UnitPriceCents int64   `json:"unitPriceCents"`
}

// Pointer so an absent field is distinguishable from a genuine zero. A provider that redeploys and starts answering {"tax_cents": 875} or {"status":"ok"} decoded cleanly into a zero before this, and every order committed with no tax and a short total - no error, no event, no log.
type remoteTaxResponse struct {
	TaxCents *int64 `json:"taxCents"`
}

type remoteTaxSource struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func NewRemoteTaxSource(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) RemoteTaxSource {
	return &remoteTaxSource{svc: svc, invoker: invoker}
}

func (r *remoteTaxSource) Tax(ctx context.Context, in RemoteTaxInput) (int64, bool, error) {
	ch := in.ChannelID
	providers, err := r.svc.ListActiveByKind(ctx, remoteprovider.KindTaxProvider, &ch)
	if err != nil {
		// A failed lookup is NOT "no provider configured" - we can't see whether one exists, let alone its failure policy, so the unknown resolves closed like the Invoke path below. Cheap in practice: this reads the same commerce DB the order write needs, so a lookup this broken means the placement was failing anyway - whereas falling through would commit the order at zero tax and leave the merchant holding it
		return 0, false, fmt.Errorf("tax provider lookup: %w", err)
	}
	if len(providers) == 0 {
		return 0, false, nil // no provider - core computes tax
	}
	// One tax provider per order - the first active for the channel wins, since multiple would double-tax
	p := providers[0]

	var resp remoteTaxResponse
	if err := r.invoker.Invoke(ctx, p, remoteTaxRequestFrom(in), &resp); err != nil {
		if p.FailurePolicy == remoteprovider.FailClosed {
			return 0, false, fmt.Errorf("tax provider %q failed: %w", p.Slug, err)
		}
		return 0, false, nil // fail_open - fall back to core
	}
	if resp.TaxCents == nil {
		// Decoded but shapeless - treat as a provider failure rather than free tax
		if p.FailurePolicy == remoteprovider.FailClosed {
			return 0, false, fmt.Errorf("tax provider %q response has no taxCents field", p.Slug)
		}
		log.Printf("[tax] provider %q returned no taxCents field - falling back to core tax", p.Slug)
		return 0, false, nil
	}
	if *resp.TaxCents < 0 {
		return 0, false, fmt.Errorf("tax provider %q returned negative tax", p.Slug)
	}
	return *resp.TaxCents, true, nil
}

func remoteTaxRequestFrom(in RemoteTaxInput) remoteTaxRequest {
	lines := make([]remoteTaxReqLine, len(in.Lines))
	for i, l := range in.Lines {
		lines[i] = remoteTaxReqLine(l) // same fields, differ only in JSON tags
	}
	return remoteTaxRequest{
		ChannelID:     in.ChannelID.String(),
		Currency:      in.Currency,
		TaxInclusive:  in.TaxInclusive,
		SubtotalCents: in.SubtotalCents,
		DiscountCents: in.DiscountCents,
		ShippingCents: in.ShippingCents,
		CustomerGroup: in.CustomerGroup,
		TaxExempt:     in.TaxExempt,
		Address:       taxAddressFrom(in.Address),
		Lines:         lines,
	}
}
