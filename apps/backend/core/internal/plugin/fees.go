package plugin

import (
	"context"
	"fmt"
	"maps"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Fees asks every plugin enabled on the channel what its fees come to, in registration order. A fee of 0 leaves no line;
// any error stops the checkout, an sdk.UserError worded for the shopper
func Fees(ctx context.Context, req order.FeeRequest) ([]order.Fee, error) {
	var out []order.Fee
	on := map[string]bool{}
	for _, r := range sdk.Fees() {
		enabled, asked := on[r.PluginSlug]
		if !asked {
			enabled = Enabled(ctx, r.PluginSlug, req.ChannelID)
			on[r.PluginSlug] = enabled
		}
		if !enabled {
			continue
		}
		cents, err := charge(ctx, r, req)
		if err != nil {
			return nil, err
		}
		if cents == 0 {
			continue
		}
		out = append(out, order.Fee{
			Key: r.Key(), Label: r.Fee.Label, AmountCents: cents, TaxClass: r.Fee.TaxClass, Taxable: !r.Fee.Untaxed,
		})
	}
	return out, nil
}

// Unmetered because every checkout of the store prices it, several times over - a busy hour must not run a store out of
// fees it's owed
func charge(ctx context.Context, r sdk.RegisteredFee, req order.FeeRequest) (cents int64, err error) {
	ctx, finish, gate := Instrument(ctx, r.PluginSlug, "fee "+r.Fee.Key, req.ChannelID, Unmetered())
	defer finish(&err)
	if gate != nil {
		return 0, gate
	}
	cents, err = r.Fee.Charge(ctx, sdkFeeRequest(req))
	if err != nil {
		return 0, translateUserError(err)
	}
	if cents < 0 || cents > sdk.MaxFeeCents {
		return 0, fmt.Errorf("plugin %s: fee %s came to %d, outside 0..%d", r.PluginSlug, r.Fee.Key, cents, int64(sdk.MaxFeeCents))
	}
	return cents, nil
}

func sdkFeeRequest(req order.FeeRequest) sdk.FeeRequest {
	lines := make([]sdk.FeeLine, len(req.Lines))
	for i, l := range req.Lines {
		lines[i] = sdk.FeeLine(l)
	}
	return sdk.FeeRequest{
		ChannelID:         req.ChannelID,
		CurrencyCode:      req.CurrencyCode,
		TaxInclusive:      req.TaxInclusive,
		CustomerID:        req.CustomerID,
		CustomerGroupSlug: req.CustomerGroupSlug,
		CountryCode:       req.CountryCode,
		RegionCode:        req.RegionCode,
		PaymentMethod:     req.PaymentMethod,
		ShippingCarrier:   req.ShippingCarrier,
		ShippingMethod:    req.ShippingMethod,
		SubtotalCents:     req.SubtotalCents,
		DiscountCents:     req.DiscountCents,
		ShippingCents:     req.ShippingCents,
		Lines:             lines,
		Answers:           maps.Clone(req.Answers), // a plugin writing to it changes nothing the next fee reads
	}
}
