package order

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

// A charge a plugin added to the order, as the shopper saw it - the order keeps the amount and its tax rather than
// asking the plugin again, so the total always adds up from what's stored
type Fee struct {
	Key         string `json:"key"` // "<plugin>:<key>"
	Label       string `json:"label"`
	AmountCents int64  `json:"amountCents"`
	TaxCents    int64  `json:"taxCents"` // the part of the order's tax charged on it - 0 when a remote provider or plugin priced the tax
	TaxClass    string `json:"taxClass,omitempty"`
	Taxable     bool   `json:"taxable"`
}

// What a fee is priced against - the checkout as it stands after shipping and before tax
type FeeRequest struct {
	ChannelID         uuid.UUID
	CurrencyCode      string
	TaxInclusive      bool
	CustomerID        *uuid.UUID
	CustomerGroupSlug string
	CountryCode       string
	RegionCode        string
	PaymentMethod     string
	ShippingCarrier   string
	ShippingMethod    string
	SubtotalCents     int64
	DiscountCents     int64
	ShippingCents     int64
	Lines             []FeeLine
	Answers           map[string]string
}

type FeeLine struct {
	SKU        string
	Quantity   float64
	UnitCents  int64
	TotalCents int64
	Gift       bool
	Custom     bool
}

// The fees every plugin on the channel charges for a checkout - an error stops the quote or the placement
type FeeSource func(ctx context.Context, req FeeRequest) ([]Fee, error)

// Optional - nil charges no fees
func (s *Service) SetFees(src FeeSource) { s.fees = src }

// Asks the plugins what the checkout's fees come to, onto cc.fees - priced after shipping, so a fee can follow it
func (s *Service) priceFees(ctx context.Context, cc *checkoutContext, in PlaceOrderInput, discount int64, ship shippingSelection, shippingCents int64) error {
	cc.fees = nil
	if s.fees == nil {
		return nil
	}
	lines := make([]FeeLine, len(cc.cart.Lines))
	for i, l := range cc.cart.Lines {
		lines[i] = FeeLine{
			SKU: l.ProductSku, Quantity: l.Quantity, UnitCents: l.UnitPrice,
			TotalCents: money.LineTotal(l.UnitPrice, l.Quantity), Gift: l.IsGift, Custom: l.IsCustom,
		}
	}
	region := ""
	if cc.shipping.State != nil {
		region = *cc.shipping.State
	}
	carrier, method := ship.CarrierSlug, ship.MethodCode
	if carrier == "" && in.ShippingCarrierSlug != nil {
		carrier = *in.ShippingCarrierSlug
	}
	if method == "" && in.ShippingMethodCode != nil {
		method = *in.ShippingMethodCode
	}
	fees, err := s.fees(ctx, FeeRequest{
		ChannelID:         cc.store.ID,
		CurrencyCode:      cc.store.CurrencyCode,
		TaxInclusive:      cc.store.TaxInclusivePricing,
		CustomerID:        in.CustomerID,
		CustomerGroupSlug: customerGroupSlug(cc.customer),
		CountryCode:       cc.shipping.CountryCode,
		RegionCode:        region,
		PaymentMethod:     cc.paymentProvider,
		ShippingCarrier:   carrier,
		ShippingMethod:    method,
		SubtotalCents:     cc.subtotal,
		DiscountCents:     max(discount, 0),
		ShippingCents:     shippingCents,
		Lines:             lines,
		Answers:           answers(in.CheckoutFields),
	})
	if err != nil {
		return err
	}
	cc.fees = fees
	return nil
}

func feeTotal(fees []Fee) int64 {
	var n int64
	for _, f := range fees {
		n += f.AmountCents
	}
	return n
}

func feeTax(fees []Fee) int64 {
	var n int64
	for _, f := range fees {
		n += f.TaxCents
	}
	return n
}

// FeeAmount is what the order's fees come to, their tax aside
func (o Order) FeeAmount() int64 { return feeTotal(o.Fees) }

// FeeTaxAmount is the part of TaxAmount charged on the fees
func (o Order) FeeTaxAmount() int64 { return feeTax(o.Fees) }

// A fee's tax rides along only where tax was added on top, as shipping's does
func (o Order) FeesRefundable() int64 {
	if taxAddedOnTop(o) {
		return o.FeeAmount() + o.FeeTaxAmount()
	}
	return o.FeeAmount()
}

// Placement and edits re-tax under core's rules or not at all, so a fee's tax is set again each time it's computed
func withoutFeeTax(fees []Fee) []Fee {
	out := make([]Fee, len(fees))
	for i, f := range fees {
		f.TaxCents = 0
		out[i] = f
	}
	return out
}

func feesJSON(fees []Fee) json.RawMessage {
	if fees == nil {
		fees = []Fee{}
	}
	b, _ := json.Marshal(fees) // strings and integers - can't fail
	return b
}

func decodeFees(raw json.RawMessage) []Fee {
	var fees []Fee
	if json.Unmarshal(raw, &fees) != nil {
		return nil
	}
	return fees
}
