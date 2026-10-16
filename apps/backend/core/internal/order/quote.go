package order

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// What a checkout costs right now, before tenders - gift cards and store credit are spent against TotalCents at placement
type CheckoutQuote struct {
	SubtotalCents int64
	DiscountCents int64
	ShippingCents int64
	TaxCents      int64
	TotalCents    int64
}

// QuoteCheckout prices a checkout the way PlaceOrder will - the same resolution, promotion evaluation, shipping selection, tax and total - without the transaction, the plugin chains or the fraud screen
// A plugin that claims the shipping, tax or total seam at placement can still move the final figure; core pricing can't
func (s *Service) QuoteCheckout(ctx context.Context, in PlaceOrderInput) (CheckoutQuote, error) {
	cc, err := s.resolveCheckout(ctx, in, true)
	if err != nil {
		return CheckoutQuote{}, err
	}

	var (
		eval      promotion.EvalResult
		ranEngine bool
		discount  int64
	)
	if s.promotion != nil {
		res, err := s.promotion.EvaluateCart(ctx, promotion.CartEvalInput{
			CartID:            cc.cart.ID,
			SubtotalCents:     cc.subtotal,
			Currency:          cc.store.CurrencyCode,
			CountryCode:       cc.shipping.CountryCode,
			ChannelID:         &cc.store.ID,
			ChannelSlug:       cc.store.Slug,
			CustomerID:        in.CustomerID,
			CustomerGroupSlug: customerGroupSlug(cc.customer),
			CustomerEmail:     in.Email,
			Lines:             cart.PromotionLineViews(cc.cart),
		})
		if err != nil {
			return CheckoutQuote{}, err
		}
		eval, ranEngine = res.Result, true
		cc.cart = s.appendFreeGiftLines(ctx, cc.cart, cc.store.ID, eval.Ledger.FreeGifts())
		discount = eval.Ledger.SubtotalDiscountCents()
	}

	var shippingCents int64
	if len(in.Shipments) > 0 || in.FulfillmentNeed == FulfillmentNone {
		sel, _, err := s.selectShipments(ctx, in, cc, discount, ranEngine, eval)
		if err != nil {
			return CheckoutQuote{}, err
		}
		shippingCents = sel.Cents
	} else if s.shipping != nil {
		sel, err := s.resolveShippingCents(ctx, in, cc, discount, ranEngine, eval)
		if err != nil {
			return CheckoutQuote{}, err
		}
		shippingCents = sel.Cents
	}

	var taxCents int64
	taxed := false
	if s.remoteTax != nil {
		if cents, ok, err := s.remoteTax.Tax(ctx, remoteTaxInputFromCheckout(cc, discount, shippingCents)); err == nil && ok {
			taxCents, taxed = cents, true
		}
	}
	if !taxed && s.tax != nil {
		t, err := s.computeLineTax(ctx, cc, discount, shippingCents)
		if err != nil {
			return CheckoutQuote{}, err
		}
		taxCents = t
	}

	return CheckoutQuote{
		SubtotalCents: cc.subtotal,
		DiscountCents: discount,
		ShippingCents: shippingCents,
		TaxCents:      taxCents,
		TotalCents:    computeOrderTotal(cc.subtotal, discount, taxCents, shippingCents, cc.store.TaxInclusivePricing, nil),
	}, nil
}
