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
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/paymentvault"
)

func addressInputToOrderAddress(in model.AddressInput) order.OrderAddress {
	out := order.OrderAddress{
		FirstName:   in.FirstName,
		LastName:    in.LastName,
		Street1:     in.Street1,
		City:        in.City,
		PostalCode:  in.PostalCode,
		CountryCode: strings.ToUpper(in.CountryCode), // force-uppercased so the storefront can stay lenient about input casing
	}
	if in.Company != nil {
		out.Company = in.Company
	}
	if in.Street2 != nil {
		out.Street2 = in.Street2
	}
	if in.State != nil {
		out.State = in.State
	}
	if in.Phone != nil {
		out.Phone = in.Phone
	}
	return out
}

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
		if strings.EqualFold(o.InputType, "file") {
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
func reorderSimpleInput(line order.OrderLine) cart.AddCartLineInput {
	return cart.AddCartLineInput{
		ProductName: line.ProductName,
		ProductSku:  line.ProductSku,
		UnitPrice:   line.UnitPrice,
		Quantity:    line.Quantity,
	}
}

func reorderBundleInput(line order.OrderLine) cart.AddBundleToCartInput {
	byOption := map[uuid.UUID][]uuid.UUID{}
	var optionOrder []uuid.UUID
	for _, c := range line.Components {
		if _, seen := byOption[c.BundleOptionID]; !seen {
			optionOrder = append(optionOrder, c.BundleOptionID)
		}
		byOption[c.BundleOptionID] = append(byOption[c.BundleOptionID], c.BundleSelectionID)
	}
	sels := make([]cart.AddBundleSelectionInput, 0, len(optionOrder))
	for _, optID := range optionOrder {
		sels = append(sels, cart.AddBundleSelectionInput{OptionID: optID, SelectionIDs: byOption[optID]})
	}
	return cart.AddBundleToCartInput{ProductSku: line.ProductSku, Quantity: line.Quantity, Selections: sels}
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

// An explicit id wins, and a signed-in customer without one falls back to their active cart - which is exactly the cart PlaceOrder will go on to use
func (r *Resolver) cartForMinimumCheck(ctx context.Context, input order.PlaceOrderInput, channelID uuid.UUID) (cart.Cart, error) {
	callerID := channel.OptionalCustomerID(ctx)
	if input.CartID != nil {
		return r.CartSvc.GetCart(ctx, *input.CartID, callerID)
	}
	if callerID == nil {
		return cart.Cart{}, fmt.Errorf("no cart to measure")
	}
	return r.CartSvc.GetOrCreateCart(ctx, *callerID, channelID)
}

func orderOnCurrentStore(ctx context.Context, o order.Order) bool {
	sv, ok := channel.FromContext(ctx)
	return ok && sv != nil && o.ChannelID == sv.ID
}
