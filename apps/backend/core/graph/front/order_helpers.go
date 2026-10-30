package front

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	frontmodel "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/delivery"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/paymentvault"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func toOrderModel(o order.Order) *model.Order {
	var paymentClientSecret *string // PaymentActionSecret is transient - it only ever rides the placement response
	if o.PaymentActionSecret != "" {
		v := o.PaymentActionSecret
		paymentClientSecret = &v
	}

	lines := make([]*model.OrderLine, len(o.Lines))
	for i, l := range o.Lines {
		lines[i] = toOrderLineModel(l)
	}
	events := make([]*model.OrderEvent, len(o.Events))
	for i, e := range o.Events {
		events[i] = toOrderEventModel(e)
	}
	m := &model.Order{
		ID:                  o.ID.String(),
		Number:              o.Number,
		Status:              model.OrderStatus(strings.ToUpper(o.Status)),
		PaymentStatus:       model.PaymentStatus(strings.ToUpper(o.PaymentStatus)),
		PaymentProvider:     o.PaymentProvider,
		PaymentClientSecret: paymentClientSecret,
		BillingAddress:      toOrderAddressModel(o.BillingAddress),
		ShippingAddress:     toOrderAddressModel(o.ShippingAddress),
		Lines:               lines,
		Events:              events,
		Subtotal:            int32(o.SubtotalAmount),
		ShippingAmount:      int32(o.ShippingAmount),
		DiscountAmount:      int32(o.DiscountAmount),
		TaxAmount:           int32(o.TaxAmount),
		Total:               int32(o.TotalAmount),
		GiftCardAmount:      int32(o.GiftCardAmount),
		StoreCreditAmount:   int32(o.StoreCreditAmount),
		CurrencyCode:        o.CurrencyCode,
		Carrier:             o.Carrier,
		TrackingNumber:      o.TrackingNumber,
		CreatedAt:           o.CreatedAt,
	}
	if o.PONumber != "" {
		m.PoNumber = &o.PONumber
	}
	if o.CustomerNote != "" {
		m.CustomerNote = &o.CustomerNote
	}
	return m
}

func toOrderReturnModel(r order.OrderReturn) *model.OrderReturn {
	lines := make([]*model.OrderReturnLine, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = &model.OrderReturnLine{
			ID:              l.ID.String(),
			OrderLineID:     l.OrderLineID.String(),
			Quantity:        l.Quantity,
			Reason:          l.Reason,
			RefundUnitCents: int32(l.RefundUnitCents),
		}
	}
	return &model.OrderReturn{
		ID:           r.ID.String(),
		RmaNumber:    r.RMANumber,
		Status:       r.Status,
		Reason:       r.Reason,
		Restock:      r.Restock,
		RefundTarget: r.RefundTarget,
		Resolution:   r.Resolution,
		RefundAmount: int32(r.RefundAmountCents),
		Lines:        lines,
		CreatedAt:    r.CreatedAt,
	}
}

// FILE-type picks snapshot a public URL rather than a re-uploadable asset, so they're dropped - if the product still requires one the add fails and the line comes back as skipped
func reorderConfiguredInput(line order.OrderLine) cart.AddConfiguredToCartInput {
	opts := make([]cart.CartLineOptionInput, 0, len(line.Options))
	// multiselect picks snapshot one row per value - fold them back into one input per option or the add trips the supplied-twice guard
	byOption := make(map[uuid.UUID]int, len(line.Options))
	for _, o := range line.Options {
		if strings.EqualFold(o.InputType, "file") || o.InputType == cart.OptionRuleInputType { // a rule row is an effect of the picks, not one
			continue
		}
		if i, ok := byOption[o.ProductOptionID]; ok {
			if o.OptionValueID != nil {
				opts[i].ValueIDs = append(opts[i].ValueIDs, *o.OptionValueID)
			}
			continue
		}
		ci := cart.CartLineOptionInput{OptionID: o.ProductOptionID, TextValue: o.TextValue}
		if o.OptionValueID != nil {
			ci.ValueIDs = []uuid.UUID{*o.OptionValueID}
		}
		byOption[o.ProductOptionID] = len(opts)
		opts = append(opts, ci)
	}
	return cart.AddConfiguredToCartInput{ProductSku: line.ProductSku, Quantity: line.Quantity, Options: opts}
}

// Name/price ride along but AddToCart overwrites both from the current catalog row, so the stale order-snapshot price never sticks
func reorderBundleInput(line order.OrderLine) cart.AddBundleToCartInput {
	byOption := map[uuid.UUID][]uuid.UUID{}
	variants := map[uuid.UUID]uuid.UUID{} // a configurable pick's variant is the component's product
	quantities := map[uuid.UUID]float64{} // and a customer-quantity pick's amount is the component's, per bundle
	var optionOrder []uuid.UUID
	for _, c := range line.Components {
		variants[c.BundleSelectionID] = c.ProductID
		if line.Quantity > 0 {
			quantities[c.BundleSelectionID] = c.Quantity / line.Quantity
		}
		if _, seen := byOption[c.BundleOptionID]; !seen {
			optionOrder = append(optionOrder, c.BundleOptionID)
		}
		byOption[c.BundleOptionID] = append(byOption[c.BundleOptionID], c.BundleSelectionID)
	}
	sels := make([]cart.AddBundleSelectionInput, 0, len(optionOrder))
	for _, optID := range optionOrder {
		sels = append(sels, cart.AddBundleSelectionInput{OptionID: optID, SelectionIDs: byOption[optID]})
	}
	return cart.AddBundleToCartInput{ProductSku: line.ProductSku, Quantity: line.Quantity, Selections: sels, Variants: variants, Quantities: quantities}
}

func toSavedPaymentMethod(m paymentvault.Method) *frontmodel.SavedPaymentMethod {
	return &frontmodel.SavedPaymentMethod{
		ID:           m.ID.String(),
		ProviderSlug: m.ProviderSlug,
		Brand:        m.Brand,
		Last4:        m.Last4,
		ExpMonth:     int32(m.ExpMonth),
		ExpYear:      int32(m.ExpYear),
	}
}

// The cart placeOrder places - an explicit id wins (it must be on this store), and a signed-in customer without one gets their active cart
func (r *Resolver) checkoutCart(ctx context.Context, cartID *string) (cart.Cart, error) {
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil {
		return cart.Cart{}, storeRequired()
	}
	callerID := channel.OptionalCustomerID(ctx)
	if cartID != nil && *cartID != "" {
		cid, err := uuid.Parse(*cartID)
		if err != nil {
			return cart.Cart{}, invalidID("cart")
		}
		c, err := r.CartSvc.GetCart(ctx, cid, callerID)
		if err != nil {
			return cart.Cart{}, err
		}
		if c.ChannelID != sv.ID {
			return cart.Cart{}, cart.ErrCartStoreMismatch
		}
		return c, nil
	}
	if callerID == nil {
		return cart.Cart{}, apperr.New("CART_REQUIRED", apperr.ClassValidation, "a cart id is required for guest checkout")
	}
	return r.CartSvc.GetOrCreateCart(ctx, *callerID, sv.ID)
}

// checkoutComplete names the first thing the cart still owes, as a code the storefront can send the shopper back to - the delivery requirements first, then what a business purchase owes. A missing payment method isn't refused here: gift cards or store credit may cover the total, and placement refuses with PAYMENT_REQUIRED when they don't
func (r *Resolver) checkoutComplete(ctx context.Context, v *checkoutView) error {
	if v.profilesErr != nil {
		return fmt.Errorf("read the cart's delivery profiles: %w", v.profilesErr)
	}
	// A carrier that couldn't answer refuses placement rather than reading as "nothing chosen" or "nothing to choose" - falling back would charge for a delivery the shopper didn't pick
	for _, g := range v.groups {
		if g.quoteFailed() {
			return errShippingQuoteFailed
		}
		// A choice the carriers no longer offer is refused, never swapped for another - the shopper picks again
		if g.hasSel && g.chosen == nil {
			return errShippingMethodOffered
		}
		// The point was read when the shopper picked it - read it again, so one that closed since is refused and the order carries what the source says now
		if g.chosen != nil && g.sel.Point != nil && g.kind.Needs(delivery.BlockPoint) && r.DeliverySvc != nil {
			p, err := r.DeliverySvc.GetPoint(ctx, *g.option, g.sel.Point.ID)
			if err != nil {
				return err
			}
			g.sel.Point = &p
		}
	}
	for _, q := range delivery.Requirements(r.checkoutSnapshot(ctx, v)) {
		if !q.Blocking() || (q.Type == delivery.BlockPayment && q.Status == delivery.StatusMissing) {
			continue
		}
		return apperr.New(q.Code, apperr.ClassValidation, q.Message)
	}
	co := v.c.Checkout
	set := r.checkoutSettings(ctx, v.c.ChannelID)
	if set.Business && co.PurchaseType == cart.PurchaseBusiness {
		if co.Company == "" {
			return errCheckoutNeedsCompany
		}
		if r.missingBusinessID(ctx, co, set) {
			return apperr.New("CHECKOUT_BUSINESS_ID_REQUIRED", apperr.ClassValidation, "enter the company's identification numbers")
		}
	}
	return nil
}

var errTermsNotAccepted = apperr.New("TERMS_NOT_ACCEPTED", apperr.ClassValidation, "accept the terms and conditions to place the order")

// termsAccepted holds placement to the store's sales/checkout/terms_consent - under checkbox the shopper must have ticked it, which the storefront's form alone can't guarantee to a caller that skips it. Notice and off ask nothing
func (r *Resolver) termsAccepted(ctx context.Context, channelID uuid.UUID, accepted *bool) error {
	mode := "checkbox"
	if r.ConfigSvc != nil {
		if v, err := r.ConfigSvc.GetString(ctx, "sales/checkout/terms_consent", config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}); err == nil && v != "" {
			mode = v
		}
	}
	if mode == "checkbox" && (accepted == nil || !*accepted) {
		return errTermsNotAccepted
	}
	return nil
}

func orderOnCurrentStore(ctx context.Context, o order.Order) bool {
	sv, ok := channel.FromContext(ctx)
	return ok && sv != nil && o.ChannelID == sv.ID
}
