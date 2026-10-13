package front

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	model1 "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/address"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/money"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// Unauthenticated by design - it only asks the gateway whether the pending authorization completed and records gateway truth, so a caller who never finished the 3DS challenge changes nothing; order ids are unguessable uuidv7s, the property guest receipts already lean on
func (r *mutationResolver) FinalizeOrderPayment(ctx context.Context, orderID string) (bool, error) {
	oid, err := uuid.Parse(orderID)
	if err != nil {
		return false, invalidID("order")
	}
	o, err := r.OrderSvc.GetByID(ctx, oid)
	if err != nil {
		return false, err
	}
	if !orderOnCurrentStore(ctx, o) {
		return false, apperr.New("ORDER_NOT_FOUND", apperr.ClassNotFound, "order not found")
	}
	// An authenticated caller may only finalize their own order - guest finalize has no session and leans on the unguessable id, and either way the op captures only that order's own authorized amount so it can't be aimed at another account
	if custID := channel.OptionalCustomerID(ctx); custID != nil && o.CustomerID != nil && *o.CustomerID != *custID {
		return false, apperr.New("ORDER_NOT_FOUND", apperr.ClassNotFound, "order not found")
	}
	if o.PaymentStatus != order.PaymentUnpaid { // already settled - a webhook or a prior finalize won the race
		return true, nil
	}
	if o.PaymentProvider == "" {
		return false, nil
	}
	amountDue := o.AmountDue()
	if amountDue <= 0 {
		return true, nil
	}
	return r.PaymentSvc.FinalizeAuthorization(ctx, o.PaymentProvider, payment.VerifyAuthorizationRequest{
		OrderID:           o.ID,
		ChannelID:         o.ChannelID,
		ProviderReference: o.PaymentReference,
		AmountCents:       amountDue,
		CurrencyCode:      o.CurrencyCode,
	})
}

// Re-adds at CURRENT catalog prices - AddToCart re-resolves pricing, so the order's snapshot price is ignored
// A line that no longer validates (discontinued SKU, dead bundle option) goes to `skipped` and the rest still land in the cart; bundle/configured lines rebuild from the order-line snapshot minus FILE options - see reorderConfiguredInput
func (r *mutationResolver) Reorder(ctx context.Context, orderID string) (*model1.ReorderResult, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	oid, err := uuid.Parse(orderID)
	if err != nil {
		return nil, invalidID("order")
	}
	o, err := r.OrderSvc.GetOrderForCustomer(ctx, claims.CustomerID, oid)
	if err != nil {
		return nil, err
	}
	cartObj, err := r.CartSvc.GetOrCreateCart(ctx, claims.CustomerID, claims.ChannelID)
	if err != nil {
		return nil, err
	}
	cid := cartObj.ID

	added := int32(0)
	var skipped []*model1.ReorderSkippedLine
	for _, line := range o.Lines {
		var addErr error
		switch {
		case len(line.Components) > 0:
			_, addErr = r.CartSvc.AddBundleToCart(ctx, cid, &claims.CustomerID, reorderBundleInput(line))
		case len(line.Options) > 0:
			_, addErr = r.CartSvc.AddConfiguredToCart(ctx, cid, &claims.CustomerID, reorderConfiguredInput(line))
		default:
			_, addErr = r.CartSvc.AddToCart(ctx, cid, &claims.CustomerID, line.ProductSku, line.Quantity)
		}
		if addErr != nil {
			skipped = append(skipped, &model1.ReorderSkippedLine{
				Name:   line.ProductName,
				Sku:    line.ProductSku,
				Reason: "This item could not be re-added (it may be unavailable or have changed).",
			})
			continue
		}
		added++
	}

	final, err := r.CartSvc.GetCart(ctx, cid, &claims.CustomerID)
	if err != nil {
		return nil, err
	}
	m := toCartModel(final)
	r.HydrateCartLineNames(ctx, m)
	return &model1.ReorderResult{Cart: m, Added: added, Skipped: skipped}, nil
}

// The customer never picks restock/refund target - it's always restock + refund-to-original, and an operator drives approve/receive/refund from there
func (r *mutationResolver) RequestReturn(ctx context.Context, input model.RequestReturnInput) (*model.OrderReturn, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	oid, err := uuid.Parse(input.OrderID)
	if err != nil {
		return nil, invalidID("order")
	}
	if _, err := r.OrderSvc.GetOrderForCustomer(ctx, claims.CustomerID, oid); err != nil { // the only ownership gate - RequestReturn itself doesn't check the caller
		return nil, err
	}
	lines := make([]order.ReturnLineInput, 0, len(input.Lines))
	for _, l := range input.Lines {
		olID, err := uuid.Parse(l.OrderLineID)
		if err != nil {
			return nil, invalidID("order line")
		}
		lines = append(lines, order.ReturnLineInput{
			OrderLineID: olID,
			Quantity:    l.Quantity,
			Reason:      ptr.Deref(l.Reason),
		})
	}
	ret, err := r.OrderSvc.RequestReturn(ctx, order.RequestReturnInput{
		OrderID:      oid,
		Lines:        lines,
		Reason:       ptr.Deref(input.Reason),
		Restock:      true,
		RefundTarget: order.RefundTargetOriginal,
		Resolution:   ptr.Deref(input.Resolution),
	})
	if err != nil {
		return nil, err
	}
	return toOrderReturnModel(ret), nil
}

// Guest and authenticated checkout share this path - the customer claim comes off the auth context, email is required either way, and addresses arrive as address-book IDs or inline payloads (guests use inline, signed-in customers may too for a one-off)
func (r *mutationResolver) PlaceOrder(ctx context.Context, cartID *string, paymentToken *string, savedPaymentMethodID *string) (*model.Order, error) {
	c, err := r.checkoutCart(ctx, cartID)
	if err != nil {
		return nil, err
	}
	v := r.checkoutView(ctx, c)
	slug := c.Checkout.PaymentProviderSlug
	paymentOK := r.paymentMethodOffered(ctx, c.ChannelID, slug, paymentCountry(c.Checkout))
	if slug != "" && !paymentOK {
		return nil, errPaymentMethodOffered
	}
	if err := r.checkoutComplete(ctx, v); err != nil {
		return nil, err
	}
	input := r.placeOrderInput(ctx, v, paymentOK)
	input.CustomerID = channel.OptionalCustomerID(ctx)
	if paymentToken != nil && *paymentToken != "" {
		input.PaymentToken = paymentToken
	}
	if savedPaymentMethodID != nil && *savedPaymentMethodID != "" { // resolves to a vaulted token under an ownership check, and the provider slugs must line up
		if input.CustomerID == nil {
			return nil, apperr.New("UNAUTHENTICATED", apperr.ClassAuth, "sign in to pay with a saved method")
		}
		mid, err := uuid.Parse(*savedPaymentMethodID)
		if err != nil {
			return nil, invalidID("payment method")
		}
		m, err := r.PaymentVaultSvc.Get(ctx, *input.CustomerID, mid)
		if err != nil {
			return nil, err
		}
		if input.PaymentProviderSlug == nil || *input.PaymentProviderSlug != m.ProviderSlug {
			return nil, apperr.New("PAYMENT_METHOD_MISMATCH", apperr.ClassValidation, "saved method belongs to a different payment provider")
		}
		input.VaultedPaymentToken = m.Token
	}

	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &c.ChannelID}
	// sales/checkout/allow_guest - a nil CustomerID IS the guest predicate, same value the input carries above Only this resolver reaches OrderSvc.PlaceOrder; admin drafts, B2B quotes and subscription renewals take other service methods entirely, so an operator disabling guest checkout can't strand them
	if input.CustomerID == nil {
		if allowGuest, gerr := r.ConfigSvc.GetBool(ctx, "sales/checkout/allow_guest", scope); gerr == nil && !allowGuest {
			return nil, apperr.New("GUEST_CHECKOUT_DISABLED", apperr.ClassAuth, "sign in or create an account to place this order")
		}
	}
	// sales/checkout/min_order in cents, channel-scoped, checked against the cart's line subtotal
	if minOrder, _ := r.ConfigSvc.GetInt(ctx, "sales/checkout/min_order", scope); minOrder > 0 {
		var subtotal int64
		for _, l := range c.Lines {
			subtotal += money.LineTotal(l.UnitPrice, l.Quantity)
		}
		if subtotal < int64(minOrder) {
			return nil, apperr.New("ORDER_MINIMUM_NOT_MET", apperr.ClassValidation, fmt.Sprintf("order minimum not met: subtotal must be at least %d cents", minOrder))
		}
	}

	o, err := r.OrderSvc.PlaceOrder(ctx, input)
	if err != nil {
		return nil, err
	}
	return toOrderModel(o), nil
}

func (r *mutationResolver) SavePaymentMethod(ctx context.Context, providerSlug string, token string) (*model1.SavedPaymentMethod, error) {
	customerID := channel.OptionalCustomerID(ctx)
	if customerID == nil {
		return nil, apperr.New("UNAUTHENTICATED", apperr.ClassAuth, "sign in to save payment methods")
	}
	chID := channelIDFromContext(ctx)
	if chID == nil {
		return nil, storeRequired()
	}
	c, err := r.CustomerSvc.GetByID(ctx, *customerID)
	if err != nil {
		return nil, err
	}
	m, err := r.PaymentVaultSvc.Save(ctx, *customerID, c.Email, providerSlug, token, *chID)
	if err != nil {
		return nil, err
	}
	return toSavedPaymentMethod(m), nil
}

func (r *mutationResolver) DeleteMyPaymentMethod(ctx context.Context, id string) (bool, error) {
	customerID := channel.OptionalCustomerID(ctx)
	if customerID == nil {
		return false, apperr.New("UNAUTHENTICATED", apperr.ClassAuth, "sign in to manage payment methods")
	}
	chID := channelIDFromContext(ctx)
	if chID == nil {
		return false, storeRequired()
	}
	mid, err := uuid.Parse(id)
	if err != nil {
		return false, invalidID("payment method")
	}
	if err := r.PaymentVaultSvc.Delete(ctx, *customerID, mid, *chID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *orderResolver) PaymentInstructions(ctx context.Context, obj *model.Order) (*string, error) {
	// "how to pay" text for an order still awaiting an offline payment - bank details, "pay the courier", and the like
	if obj.PaymentStatus != model.PaymentStatusUnpaid || obj.PaymentProvider == "" {
		return nil, nil
	}
	// The order's own channel, so instructions match where it was placed rather than the storefront
	// currently rendering it
	scope, ok := channelConfigScope(obj.ChannelID)
	if !ok {
		scope = requestConfigScope(ctx)
	}
	if s := r.paymentInstructions(ctx, obj.PaymentProvider, scope); s != "" {
		return &s, nil
	}
	return nil, nil
}

// Best-effort live lookup through the shipping.Tracker capability - unsupported carriers, unshipped orders and gateway hiccups all yield null rather than erroring the whole order query
func (r *orderResolver) TrackingURL(ctx context.Context, obj *model.Order) (*string, error) {
	if obj.TrackingNumber == "" {
		return nil, nil
	}
	orderID, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, nil
	}
	o, err := r.OrderSvc.GetByID(ctx, orderID)
	if err != nil || o.ShippingCarrierSlug == nil || *o.ShippingCarrierSlug == "" {
		return nil, nil
	}
	res, err := r.ShippingSvc.Track(ctx, *o.ShippingCarrierSlug, obj.TrackingNumber)
	if err != nil || res.URL == "" {
		return nil, nil
	}
	return &res.URL, nil
}

func (r *orderResolver) AmountDue(ctx context.Context, obj *model.Order) (int32, error) {
	return max(obj.Total-obj.GiftCardAmount-obj.StoreCreditAmount, 0), nil
}

// No gate here - the parent Order is only reachable through the ownership-checked myOrder / myOrders / guestOrder paths
func (r *orderResolver) Returns(ctx context.Context, obj *model.Order) ([]*model.OrderReturn, error) {
	oid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, invalidID("order")
	}
	rets, err := r.OrderSvc.ListReturns(ctx, oid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.OrderReturn, len(rets))
	for i, rr := range rets {
		out[i] = toOrderReturnModel(rr)
	}
	return out, nil
}

// Per-line remainder of shipped minus already-returned, and lines with nothing left are dropped - an empty list means the order isn't returnable yet
func (r *orderResolver) ReturnableLines(ctx context.Context, obj *model.Order) ([]*model1.ReturnableLine, error) {
	oid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, invalidID("order")
	}
	rem, err := r.OrderSvc.ReturnableByOrderLine(ctx, oid)
	if err != nil {
		return nil, err
	}
	out := make([]*model1.ReturnableLine, 0, len(rem))
	for lineID, qty := range rem {
		if qty > 0 {
			out = append(out, &model1.ReturnableLine{OrderLineID: lineID.String(), Quantity: qty})
		}
	}
	return out, nil
}

func (r *queryResolver) GiftCardBalance(ctx context.Context, code string) (*model1.GiftCardBalance, error) {
	if r.GiftcardSvc == nil {
		return nil, nil
	}
	if err := rateByIP(ctx, r.RateLimiter, "giftcard-balance"); err != nil {
		return nil, err
	}
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return nil, nil
	}
	card, err := r.GiftcardSvc.GetByCode(ctx, strings.TrimSpace(code))
	if err != nil || !card.SpendsOn(*storeID) {
		return nil, nil // null not an error - enumerators shouldn't be able to tell unknown from disabled or another store's, and a typo isn't an internal fault
	}
	return &model1.GiftCardBalance{
		BalanceCents: int32(card.Balance),
		CurrencyCode: card.CurrencyCode,
		Active:       card.Status == "active",
	}, nil
}

func (r *queryResolver) MyOrders(ctx context.Context) ([]*model.Order, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	orderList, err := r.OrderSvc.ListOrdersByCustomer(ctx, claims.CustomerID)
	if err != nil {
		return nil, err
	}
	// Drafts are admin scaffolding (an exchange RMA makes one) and DRAFT isn't in the storefront's OrderStatus enum, so emitting one breaks a generated client outright - other channels' orders aren't this store's business either
	result := make([]*model.Order, 0, len(orderList))
	for _, o := range orderList {
		if o.Status == order.OrderStatusDraft || o.ChannelID != claims.ChannelID {
			continue
		}
		result = append(result, toOrderModel(o))
	}
	return result, nil
}

func (r *queryResolver) MyOrder(ctx context.Context, id string) (*model.Order, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	orderID, err := uuid.Parse(id)
	if err != nil {
		return nil, invalidID("order")
	}
	o, err := r.OrderSvc.GetOrderForCustomer(ctx, claims.CustomerID, orderID)
	if err != nil {
		return nil, err
	}
	return toOrderModel(o), nil
}

// No auth - the (id, email) pair IS the bearer credential; customer-owned orders go through myOrder instead and return null here
func (r *queryResolver) GuestOrder(ctx context.Context, id string, email string) (*model.Order, error) {
	orderID, err := uuid.Parse(id)
	if err != nil {
		return nil, nil
	}
	o, err := r.OrderSvc.GetGuestOrder(ctx, orderID, email)
	if err != nil || !orderOnCurrentStore(ctx, o) {
		return nil, nil
	}
	return toOrderModel(o), nil
}

func (r *queryResolver) MyPaymentMethods(ctx context.Context) ([]*model1.SavedPaymentMethod, error) {
	customerID := channel.OptionalCustomerID(ctx)
	if customerID == nil {
		return []*model1.SavedPaymentMethod{}, nil
	}
	methods, err := r.PaymentVaultSvc.List(ctx, *customerID)
	if err != nil {
		return nil, err
	}
	out := make([]*model1.SavedPaymentMethod, 0, len(methods))
	for _, m := range methods {
		out = append(out, toSavedPaymentMethod(m))
	}
	return out, nil
}

func (r *queryResolver) ValidateAddress(ctx context.Context, street1 string, street2 *string, city string, state *string, postalCode string, countryCode string) (*model1.AddressValidationResult, error) {
	if err := r.checkRate(ctx, "validate_address"); err != nil {
		return nil, err
	}
	chID := channelIDFromContext(ctx)
	if chID == nil {
		return nil, storeRequired()
	}
	res, err := r.AddressSvc.Validate(ctx, *chID, address.Input{
		Street1:     street1,
		Street2:     ptr.Deref(street2),
		City:        city,
		State:       ptr.Deref(state),
		PostalCode:  postalCode,
		CountryCode: countryCode,
	})
	if err != nil {
		return nil, err
	}
	out := &model1.AddressValidationResult{
		Valid:    res.Valid,
		Messages: res.Messages,
		Strict:   r.AddressSvc.Strict(ctx, *chID),
	}
	if out.Messages == nil {
		out.Messages = []string{}
	}
	if res.Normalized != nil {
		out.Normalized = &model1.AddressInput2{
			Street1:     res.Normalized.Street1,
			Street2:     res.Normalized.Street2,
			City:        res.Normalized.City,
			State:       res.Normalized.State,
			PostalCode:  res.Normalized.PostalCode,
			CountryCode: res.Normalized.CountryCode,
		}
	}
	return out, nil
}

func (r *Resolver) Order() OrderResolver { return &orderResolver{r} }

type orderResolver struct{ *Resolver }
