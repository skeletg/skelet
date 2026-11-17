package plugin

import (
	"bytes"
	"cmp"
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/documents"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// DocumentRender hands the document to the first plugin on the channel that renders its kind
func DocumentRender(ctx context.Context, in documents.Input, core func() ([]byte, error)) (pdf []byte, ok bool, err error) {
	for _, r := range sdk.DocumentRenderers() {
		if string(r.Kind) != string(in.Kind) || !Enabled(ctx, r.PluginSlug, in.Order.ChannelID) {
			continue
		}
		ctx, finish, gate := Instrument(ctx, r.PluginSlug, "document "+string(r.Kind), in.Order.ChannelID)
		defer finish(&err)
		if gate != nil {
			return nil, true, gate
		}
		// ok is set before the call, so a panic finish turns into an error still reads as this plugin's answer, never core's
		ok = true
		pdf, err = r.Render(ctx, documentRequest(in, core))
		return pdf, ok, translateUserError(err)
	}
	return nil, false, nil
}

func documentRequest(in documents.Input, core func() ([]byte, error)) sdk.DocumentRequest {
	o := in.Order
	req := sdk.DocumentRequest{
		Kind:      sdk.DocumentKind(in.Kind),
		ChannelID: o.ChannelID,
		Seller: sdk.DocumentSeller{Name: in.Store.Name, Details: slices.Clone(in.Store.Lines), Logo: bytes.Clone(in.Store.Logo), Footer: in.Footer,
			Note: in.Note},
		Default: core,
		Order: sdk.DocumentOrder{
			ID: o.ID, Number: o.Number, PlacedAt: o.CreatedAt, Email: o.Email, CurrencyCode: o.CurrencyCode,
			Decimals: in.Decimals, TaxIncluded: in.TaxInclusive, PONumber: o.PONumber,
			Billing: documentAddress(o.BillingAddress), Shipping: documentAddress(o.ShippingAddress),
			SubtotalCents: o.SubtotalAmount, DiscountCents: o.DiscountAmount, ShippingCents: o.ShippingAmount,
			TaxCents: o.TaxAmount, TotalCents: o.TotalAmount, GiftCardCents: o.GiftCardAmount, StoreCreditCents: o.StoreCreditAmount,
		},
	}
	byID := make(map[uuid.UUID]order.OrderLine, len(o.Lines))
	for _, l := range o.Lines {
		byID[l.ID] = l
		req.Order.Lines = append(req.Order.Lines, documentLine(l, l.Quantity, l.UnitPrice, l.TotalPrice))
	}
	for _, id := range o.BusinessIDs {
		req.Order.BusinessIDs = append(req.Order.BusinessIDs, sdk.DocumentField{Label: id.Label, Value: id.Value})
	}
	for _, f := range o.Fees {
		req.Order.Fees = append(req.Order.Fees, sdk.DocumentFee{Label: f.Label, Cents: f.AmountCents})
	}
	if m := in.Memo; m != nil {
		req.Memo = &sdk.DocumentMemo{
			ID: m.ID, Number: m.Number, CreatedAt: m.CreatedAt, ShippingCents: m.ShippingRefundCents, FeesCents: m.FeesRefundCents,
			AdjustmentCents: m.AdjustmentRefundCents, FeeCents: m.AdjustmentFeeCents, TotalCents: m.TotalCents, Reason: m.Reason,
		}
		for _, ml := range m.Lines {
			req.Memo.Lines = append(req.Memo.Lines, documentLine(byID[ml.OrderLineID], ml.Quantity, ml.UnitCents, ml.TotalCents))
		}
	}
	return req
}

func documentLine(l order.OrderLine, qty float64, unit, total int64) sdk.DocumentLine {
	out := sdk.DocumentLine{SKU: l.ProductSku, Name: l.ProductName, Quantity: qty, UnitPriceCents: unit, TotalCents: total}
	for _, opt := range l.Options {
		if v := cmp.Or(opt.ValueLabel, opt.TextValue); v != "" {
			out.Options = append(out.Options, opt.OptionTitle+": "+v)
		}
	}
	return out
}

func documentAddress(a order.OrderAddress) sdk.DocumentAddress {
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	return sdk.DocumentAddress{
		FirstName: a.FirstName, LastName: a.LastName, Company: deref(a.Company), Street1: a.Street1, Street2: deref(a.Street2),
		City: a.City, State: deref(a.State), PostalCode: a.PostalCode, CountryCode: a.CountryCode, Phone: deref(a.Phone),
	}
}
