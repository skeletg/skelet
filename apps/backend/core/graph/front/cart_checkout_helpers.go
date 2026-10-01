package front

// Checkout steps save to the cart one at a time. The cart package only stores them - shipping, payment, address and gift-card checks live here because the resolver layer is where those domains meet

import (
	"context"
	"errors"
	"log"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	frontmodel "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/address"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/country"
	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/phone"
)

var (
	errCheckoutNeedsAddress     = apperr.New("CHECKOUT_SHIPPING_ADDRESS_REQUIRED", apperr.ClassValidation, "enter a shipping address")
	errCheckoutNeedsDestination = apperr.New("CHECKOUT_DESTINATION_REQUIRED", apperr.ClassValidation, "enter where the order is going first")
	errCheckoutNeedsBilling     = apperr.New("CHECKOUT_BILLING_ADDRESS_REQUIRED", apperr.ClassValidation, "enter a billing address")
	errCheckoutNeedsEmail       = apperr.New("CHECKOUT_EMAIL_REQUIRED", apperr.ClassValidation, "enter an email address")
	errCheckoutNeedsPhone       = apperr.New("CHECKOUT_PHONE_REQUIRED", apperr.ClassValidation, "enter a phone number")
	errCheckoutNeedsCompany     = apperr.New("CHECKOUT_COMPANY_REQUIRED", apperr.ClassValidation, "enter the company name")
	errCheckoutNeedsCollector   = apperr.New("CHECKOUT_PICKUP_CONTACT_REQUIRED", apperr.ClassValidation, "enter who will collect the order")
	errCheckoutPostalCode       = apperr.New("CHECKOUT_POSTAL_CODE_REQUIRED", apperr.ClassValidation, "enter a postcode")
	errShippingMethodOffered    = apperr.New("SHIPPING_METHOD_UNAVAILABLE", apperr.ClassValidation, "that shipping method isn't available for this address")
	errPaymentMethodOffered     = apperr.New("PAYMENT_METHOD_UNAVAILABLE", apperr.ClassValidation, "that payment method isn't available for this address")
	errGiftCardInvalid          = apperr.New("GIFT_CARD_INVALID", apperr.ClassValidation, "that gift card can't be used")
	errCheckoutAddressChoice    = apperr.New("CHECKOUT_ADDRESS_CHOICE", apperr.ClassValidation, "give either a saved address or a new one")
	errCheckoutAddressMissing   = apperr.New("ADDRESS_INVALID", apperr.ClassValidation, "first name, last name, street, city, postal code and country are required")
)

// checkoutStep is the shape of every step mutation - the cart must be on this store, the step runs under the cart's row lock, and the whole cart comes back so the storefront renders from one response
func (r *Resolver) checkoutStep(ctx context.Context, cartID string, step func(c cart.Cart, co *cart.Checkout) error) (*frontmodel.Cart, error) {
	return r.checkoutWrite(ctx, cartID, func(c *cart.Cart) error { return step(*c, &c.Checkout) })
}

// checkoutWrite is checkoutStep for a step that also sets the cart's email
func (r *Resolver) checkoutWrite(ctx context.Context, cartID string, step func(c *cart.Cart) error) (*frontmodel.Cart, error) {
	cid, err := uuid.Parse(cartID)
	if err != nil {
		return nil, invalidID("cart")
	}
	if err := r.assertCartOnCurrentStore(ctx, cid); err != nil {
		return nil, err
	}
	callerID := channel.OptionalCustomerID(ctx)
	if err := r.CartSvc.UpdateCheckout(ctx, cid, callerID, step); err != nil {
		return nil, err
	}
	c, err := r.CartSvc.GetCart(ctx, cid, callerID)
	if err != nil {
		return nil, err
	}
	return r.fullCartModel(ctx, c), nil
}

// cartAddress resolves a step's address - a saved one (the cart's owner only) is copied as a snapshot, a typed one is checked for the fields placement requires - and both must be allowed on this store
func (r *Resolver) cartAddress(ctx context.Context, c cart.Cart, addressID *string, in *model.AddressInput, which string) (*cart.Address, *uuid.UUID, error) {
	hasID := addressID != nil && *addressID != ""
	if hasID == (in != nil) {
		return nil, nil, errCheckoutAddressChoice
	}
	var (
		a      cart.Address
		source *uuid.UUID
	)
	if hasID {
		aid, err := uuid.Parse(*addressID)
		if err != nil {
			return nil, nil, invalidID("address")
		}
		callerID := channel.OptionalCustomerID(ctx)
		// Saved addresses belong on the customer's own cart - a guest cart may be held by anyone with its id, and merge assumes it carries no address ids
		if callerID == nil || c.CustomerID == nil || *c.CustomerID != *callerID {
			return nil, nil, order.ErrAddressNotFound
		}
		saved, err := r.CustomerSvc.GetAddressByID(ctx, aid)
		if err != nil || saved.CustomerID != *callerID {
			return nil, nil, order.ErrAddressNotFound
		}
		a = cart.Address{
			FirstName: saved.FirstName, LastName: saved.LastName, Company: saved.Company,
			Street1: saved.Street1, Street2: saved.Street2, City: saved.City, State: saved.State,
			PostalCode: saved.PostalCode, CountryCode: saved.CountryCode, Phone: saved.Phone,
		}
		source = &aid
	} else {
		a = cart.Address{
			FirstName: strings.TrimSpace(in.FirstName), LastName: strings.TrimSpace(in.LastName),
			Company: trimmedOrNil(in.Company), Street1: strings.TrimSpace(in.Street1), Street2: trimmedOrNil(in.Street2),
			City: strings.TrimSpace(in.City), State: trimmedOrNil(in.State), PostalCode: strings.TrimSpace(in.PostalCode),
			CountryCode: strings.ToUpper(strings.TrimSpace(in.CountryCode)), Phone: trimmedOrNil(in.Phone),
		}
		if a.FirstName == "" || a.LastName == "" || a.Street1 == "" || a.City == "" || a.PostalCode == "" || a.CountryCode == "" {
			return nil, nil, errCheckoutAddressMissing
		}
		if a.Phone != nil {
			p, err := r.phoneRules(ctx, c.ChannelID).Normalize(*a.Phone, a.CountryCode)
			if err != nil {
				return nil, nil, err
			}
			a.Phone = &p
		}
	}
	if r.CountrySvc != nil {
		if err := r.CountrySvc.ValidateAddressForChannel(ctx, c.ChannelID, a.CountryCode, a.State); err != nil {
			return nil, nil, err
		}
	}
	if err := r.strictAddressCheck(ctx, c.ChannelID, which, a); err != nil {
		return nil, nil, err
	}
	return &a, source, nil
}

// The same rule placement applies (order.checkStrictAddress), run at the step so the shopper hears about it on the address form rather than at "Place order"
func (r *Resolver) strictAddressCheck(ctx context.Context, channelID uuid.UUID, which string, a cart.Address) error {
	if r.AddressSvc == nil || !r.AddressSvc.Strict(ctx, channelID) {
		return nil
	}
	res, err := r.AddressSvc.Validate(ctx, channelID, address.Input{
		Street1: a.Street1, Street2: ptr.Deref(a.Street2), City: a.City, State: ptr.Deref(a.State),
		PostalCode: a.PostalCode, CountryCode: a.CountryCode,
	})
	if errors.Is(err, address.ErrUnavailable) {
		return err
	}
	if err != nil || res.Valid {
		return nil
	}
	msg := "Please check the " + which + " address"
	if len(res.Messages) > 0 {
		msg += ": " + strings.Join(res.Messages, "; ")
	}
	return apperr.New("ADDRESS_INVALID", apperr.ClassValidation, msg+".")
}

// offeredShippingMethod is the chosen method as currently quoted for the cart at its destination - nil when nothing is chosen or it is no longer offered
func (r *Resolver) offeredShippingMethod(ctx context.Context, c cart.Cart, co cart.Checkout) *shipping.RatedQuote {
	dest := co.QuoteDestination()
	if co.ShippingCarrierSlug == "" || dest.IsZero() {
		return nil
	}
	quotes := r.quotesFor(ctx, c, dest)
	for i, q := range quotes {
		if q.CarrierSlug == co.ShippingCarrierSlug && q.Method.Code == co.ShippingMethodCode {
			return &quotes[i]
		}
	}
	return nil
}

func (r *Resolver) quotesFor(ctx context.Context, c cart.Cart, d cart.Destination) []shipping.RatedQuote {
	sv, ok := channel.FromContext(ctx)
	if r.ShippingSvc == nil || !ok || sv == nil || d.IsZero() {
		return nil
	}
	req, _ := r.cartRateRequest(ctx, sv, c, d.CountryCode, d.Region, d.PostalCode)
	quotes, err := r.ShippingSvc.QuoteAll(ctx, req)
	if err != nil {
		return nil
	}
	return quotes
}

func (r *Resolver) paymentMethodOffered(ctx context.Context, channelID uuid.UUID, slug, country string) bool {
	if slug == "" || country == "" || r.PaymentSvc == nil {
		return false
	}
	methods, err := r.PaymentSvc.ListEligiblePaymentMethods(ctx, channelID, country)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(methods, func(m payment.PaymentMethod) bool { return m.ProviderSlug == slug })
}

// dropUnsupportedSelections runs after the destination changes - a method or payment option the new address doesn't support is cleared, so the cart never holds a combination placement would refuse
func (r *Resolver) dropUnsupportedSelections(ctx context.Context, c cart.Cart, co *cart.Checkout) {
	if co.ShippingCarrierSlug != "" && r.offeredShippingMethod(ctx, c, *co) == nil {
		co.ShippingCarrierSlug, co.ShippingMethodCode = "", ""
	}
	if co.PaymentProviderSlug != "" && !r.paymentMethodOffered(ctx, c.ChannelID, co.PaymentProviderSlug, co.ShippingCountry()) {
		co.PaymentProviderSlug = ""
	}
}

// usableGiftCard is a card that exists, is active, spends on this store and has something left
func (r *Resolver) usableGiftCard(ctx context.Context, channelID uuid.UUID, code string) (giftcard.GiftCard, bool) {
	if r.GiftcardSvc == nil || code == "" {
		return giftcard.GiftCard{}, false
	}
	card, err := r.GiftcardSvc.GetByCode(ctx, code)
	if err != nil || !card.SpendsOn(channelID) || card.Status != giftcard.StatusActive || card.Balance <= 0 {
		return giftcard.GiftCard{}, false
	}
	return card, true
}

// checkoutEmail is the receipt address - the one entered at checkout, else a signed-in customer's account email
func (r *Resolver) checkoutEmail(ctx context.Context, c cart.Cart) string {
	if c.Email != "" {
		return c.Email
	}
	if c.CustomerID != nil && r.CustomerSvc != nil {
		if cu, err := r.CustomerSvc.GetByID(ctx, *c.CustomerID); err == nil {
			return cu.Email
		}
	}
	return ""
}

// placeOrderInput is the cart's checkout as a placement request. Selections the cart holds but the store no longer offers are left out, so a quote falls back the way placement would and placeOrder refuses rather than guessing
func (r *Resolver) placeOrderInput(ctx context.Context, c cart.Cart, method *shipping.RatedQuote, paymentOK bool) order.PlaceOrderInput {
	co := c.Checkout
	in := order.PlaceOrderInput{
		CustomerID:     c.CustomerID,
		Email:          r.checkoutEmail(ctx, c),
		CartID:         &c.ID,
		CustomerNote:   c.Note,
		GiftCardCodes:  co.GiftCardCodes,
		UseStoreCredit: co.UseStoreCredit && c.CustomerID != nil,
	}
	business := co.PurchaseType == cart.PurchaseBusiness
	if business {
		in.PONumber = co.PONumber
		for _, f := range r.businessIDFieldsFor(ctx, companyCountry(co), r.checkoutSettings(ctx, c.ChannelID)) {
			if v := co.BusinessIDs[f.Key]; v != "" {
				in.BusinessIDs = append(in.BusinessIDs, order.BusinessID{Key: f.Key, Label: f.Label, Value: v})
			}
		}
	}
	var ship *order.OrderAddress
	switch {
	case co.IsPickup():
		ship = r.pickupOrderAddress(ctx, co)
	case co.ShippingAddress != nil:
		a := cartAddressToOrderAddress(*co.ShippingAddress)
		ship = &a
	case !co.Destination.IsZero():
		// Only the destination so far - enough for a quote to price shipping and tax, and placement refuses it
		ship = &order.OrderAddress{CountryCode: co.Destination.CountryCode, PostalCode: co.Destination.PostalCode, State: nilIfEmpty(co.Destination.Region)}
	}
	if ship != nil {
		if ship.Phone == nil && co.ContactPhone != "" {
			ship.Phone = &co.ContactPhone
		}
		bill := *ship
		if b, _ := co.EffectiveBilling(); b != nil {
			bill = cartAddressToOrderAddress(*b)
		}
		if bill.Phone == nil && co.ContactPhone != "" {
			bill.Phone = &co.ContactPhone
		}
		if business && co.Company != "" {
			bill.Company = &co.Company
		}
		in.ShippingAddress, in.BillingAddress = ship, &bill
	}
	if method != nil {
		in.ShippingCarrierSlug, in.ShippingMethodCode = &method.CarrierSlug, &method.Method.Code
	}
	if paymentOK {
		in.PaymentProviderSlug = &co.PaymentProviderSlug
	}
	return in
}

// pickupOrderAddress is where a pickup order ships - the store itself, addressed to whoever collects it. Nil when the store stopped offering pickup, so a quote falls back to the subtotal and placement refuses
func (r *Resolver) pickupOrderAddress(ctx context.Context, co cart.Checkout) *order.OrderAddress {
	if r.InventorySvc == nil {
		return nil
	}
	locs, err := r.InventorySvc.PickupLocations(ctx)
	if err != nil {
		return nil
	}
	slug := strings.TrimPrefix(co.ShippingMethodCode, "loc-")
	for _, l := range locs {
		if l.Slug != slug {
			continue
		}
		a := order.OrderAddress{
			Company: nilIfEmpty(l.Name), Street1: l.Line1, Street2: nilIfEmpty(l.Line2),
			City: l.City, State: nilIfEmpty(l.Region), PostalCode: l.PostalCode, CountryCode: l.CountryCode,
		}
		switch b, _ := co.EffectiveBilling(); {
		case co.PickupContact != nil:
			a.FirstName, a.LastName = co.PickupContact.FirstName, co.PickupContact.LastName
		case b != nil:
			a.FirstName, a.LastName = b.FirstName, b.LastName
		}
		return &a
	}
	return nil
}

// checkoutSettings are the store's sales/checkout switches for what the details step asks - read per request, so an admin change applies to checkouts already in progress
type checkoutSettings struct {
	Phone              string // hidden, optional or required
	Business           bool
	TaxID              string // hidden, optional or required - the generic company ID of a country with no fields of its own
	PONumber           bool
	PickupSomeoneElse  bool
	PickupNeedsBilling bool
}

func (r *Resolver) checkoutSettings(ctx context.Context, channelID uuid.UUID) checkoutSettings {
	s := checkoutSettings{Phone: "optional", TaxID: "optional", PickupSomeoneElse: true}
	if r.ConfigSvc == nil {
		return s
	}
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
	str := func(path string, into *string) {
		if v, err := r.ConfigSvc.GetString(ctx, path, scope); err == nil && v != "" {
			*into = v
		}
	}
	flag := func(path string, into *bool) {
		if v, err := r.ConfigSvc.GetBool(ctx, path, scope); err == nil {
			*into = v
		}
	}
	str("sales/checkout/phone_field", &s.Phone)
	str("sales/checkout/tax_id_field", &s.TaxID)
	flag("sales/checkout/business_purchase", &s.Business)
	flag("sales/checkout/po_number_field", &s.PONumber)
	flag("sales/checkout/pickup_by_someone_else", &s.PickupSomeoneElse)
	flag("sales/checkout/pickup_requires_billing_address", &s.PickupNeedsBilling)
	return s
}

// pickupNeedsBilling - a pickup has no shipping address to name the payer, so one is asked for when the store wants it on every invoice or someone else collects
func (s checkoutSettings) pickupNeedsBilling(co cart.Checkout) bool {
	return s.PickupNeedsBilling || (co.PickupContact != nil && co.PickupContact.SomeoneElse)
}

// checkoutModel resolves Cart.checkout - selections are re-checked against today's quotes and payment options, and totals come from the placement pricing path
func (r *Resolver) checkoutModel(ctx context.Context, c cart.Cart) *frontmodel.CartCheckout {
	co := c.Checkout
	m := &frontmodel.CartCheckout{
		Email:                 r.checkoutEmail(ctx, c),
		IsPickup:              co.IsPickup(),
		Phone:                 co.ContactPhone,
		PurchaseType:          frontmodel.PurchaseTypePersonal,
		Company:               co.Company,
		BusinessIds:           toBusinessIDValues(co.BusinessIDs),
		PoNumber:              co.PONumber,
		ShippingAddress:       toCartAddressModel(co.ShippingAddress, co.ShippingAddressID),
		BillingSameAsShipping: co.BillingSameAsShipping,
		GiftCards:             []*frontmodel.CartGiftCard{},
		UseStoreCredit:        co.UseStoreCredit && c.CustomerID != nil,
	}
	if co.PurchaseType == cart.PurchaseBusiness {
		m.PurchaseType = frontmodel.PurchaseTypeBusiness
	}
	if d := co.QuoteDestination(); !d.IsZero() {
		m.Destination = &frontmodel.CheckoutDestination{CountryCode: d.CountryCode, RegionCode: nilIfEmpty(d.Region), PostalCode: d.PostalCode}
	}
	if p := co.PickupContact; p != nil {
		m.PickupContact = &frontmodel.PickupContact{FirstName: p.FirstName, LastName: p.LastName, SomeoneElse: p.SomeoneElse}
	}
	billing, billingID := co.EffectiveBilling()
	m.BillingAddress = toCartAddressModel(billing, billingID)

	method := r.offeredShippingMethod(ctx, c, co)
	if method != nil {
		m.ShippingMethod = toShippingQuoteModel(*method)
	}
	paymentOK := r.paymentMethodOffered(ctx, c.ChannelID, co.PaymentProviderSlug, co.ShippingCountry())
	if paymentOK {
		m.PaymentProviderSlug = &co.PaymentProviderSlug
	}

	m.Totals = r.checkoutTotals(ctx, c, method, paymentOK)

	// Tenders in placement's order: gift cards as added, then store credit against what they leave
	remaining := int64(m.Totals.TotalCents)
	for _, code := range co.GiftCardCodes {
		card, ok := r.usableGiftCard(ctx, c.ChannelID, code)
		if !ok {
			continue
		}
		applied := min(card.Balance, max(remaining, 0))
		remaining -= applied
		m.Totals.GiftCardCents += int32(applied)
		m.GiftCards = append(m.GiftCards, &frontmodel.CartGiftCard{Code: code, AppliedCents: int32(applied), BalanceCents: int32(card.Balance)})
	}
	if m.UseStoreCredit && r.StoreCreditSvc != nil && remaining > 0 {
		if sv, ok := channel.FromContext(ctx); ok && sv != nil {
			if bal, err := r.StoreCreditSvc.Balance(ctx, *c.CustomerID, sv.CurrencyCode); err == nil && bal > 0 {
				applied := min(bal, remaining)
				remaining -= applied
				m.Totals.StoreCreditCents = int32(applied)
			}
		}
	}
	m.Totals.AmountDueCents = int32(max(remaining, 0))
	return m
}

func (r *Resolver) checkoutTotals(ctx context.Context, c cart.Cart, method *shipping.RatedQuote, paymentOK bool) *frontmodel.CheckoutTotals {
	if !c.Checkout.QuoteDestination().IsZero() && r.OrderSvc != nil && len(c.Lines) > 0 {
		q, err := r.OrderSvc.QuoteCheckout(ctx, r.placeOrderInput(ctx, c, method, paymentOK))
		if err == nil {
			return &frontmodel.CheckoutTotals{
				SubtotalCents: int32(q.SubtotalCents), DiscountCents: int32(q.DiscountCents),
				ShippingCents: int32(q.ShippingCents), TaxCents: int32(q.TaxCents), TotalCents: int32(q.TotalCents),
			}
		}
		log.Printf("[checkout] quote for cart %s failed, showing the subtotal only: %v", c.ID, err)
	}
	// No destination yet, so no shipping or tax - the discounted subtotal, the same figure the cart page shows
	m := toCartModel(c)
	r.HydrateCartPromotions(ctx, m, c, c.Checkout.ShippingCountry())
	return &frontmodel.CheckoutTotals{
		SubtotalCents: m.Subtotal,
		DiscountCents: m.TotalDiscount,
		TotalCents:    max(m.Subtotal-m.TotalDiscount, 0),
	}
}

func toShippingQuoteModel(q shipping.RatedQuote) *frontmodel.ShippingQuote {
	m := &frontmodel.ShippingQuote{
		CarrierSlug: q.CarrierSlug, CarrierName: q.CarrierName,
		Code: q.Method.Code, Name: q.Method.Name, PriceCents: int32(q.Method.PriceCents),
	}
	if q.Method.EstimatedDays > 0 {
		d := int32(q.Method.EstimatedDays)
		m.EstimatedDays = &d
	}
	return m
}

func toCartAddressModel(a *cart.Address, sourceID *uuid.UUID) *frontmodel.CartAddress {
	if a == nil {
		return nil
	}
	out := &frontmodel.CartAddress{
		FirstName: a.FirstName, LastName: a.LastName, Company: a.Company,
		Street1: a.Street1, Street2: a.Street2, City: a.City, State: a.State,
		PostalCode: a.PostalCode, CountryCode: a.CountryCode, Phone: a.Phone,
	}
	if sourceID != nil {
		s := sourceID.String()
		out.SourceAddressID = &s
	}
	return out
}

func cartAddressToOrderAddress(a cart.Address) order.OrderAddress {
	return order.OrderAddress{
		FirstName: a.FirstName, LastName: a.LastName, Company: a.Company,
		Street1: a.Street1, Street2: a.Street2, City: a.City, State: a.State,
		PostalCode: a.PostalCode, CountryCode: a.CountryCode, Phone: a.Phone,
	}
}

func trimmedOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

func nilIfEmpty(s string) *string {
	return trimmedOrNil(&s)
}

// checkoutDestination validates the first step - a country this store sells to, and a postcode to quote from
func (r *Resolver) checkoutDestination(ctx context.Context, channelID uuid.UUID, country, region, postal string) (cart.Destination, error) {
	d := cart.Destination{
		CountryCode: strings.ToUpper(strings.TrimSpace(country)),
		Region:      strings.TrimSpace(region),
		PostalCode:  strings.ToUpper(strings.TrimSpace(postal)),
	}
	if d.CountryCode == "" {
		return d, errCheckoutNeedsDestination
	}
	if d.PostalCode == "" {
		return d, errCheckoutPostalCode
	}
	if r.CountrySvc != nil {
		if err := r.CountrySvc.ValidateAddressForChannel(ctx, channelID, d.CountryCode, nilIfEmpty(d.Region)); err != nil {
			return d, err
		}
	}
	return d, nil
}

// samePostalCode ignores case and spacing - "sw1a 1aa" and "SW1A1AA" are one postcode
func samePostalCode(a, b string) bool {
	norm := func(s string) string { return strings.ToUpper(strings.ReplaceAll(s, " ", "")) }
	return norm(a) == norm(b)
}

// applyCheckoutDetails saves the details step - contact, business details, then the addresses a delivery needs or the contact a pickup needs. What is asked for and what is required follow the store's settings, the same rules detailsComplete re-checks at placement
func (r *Resolver) applyCheckoutDetails(ctx context.Context, c *cart.Cart, in frontmodel.CheckoutDetailsInput) error {
	co := &c.Checkout
	set := r.checkoutSettings(ctx, c.ChannelID)
	if co.QuoteDestination().IsZero() {
		return errCheckoutNeedsDestination
	}

	if email := strings.TrimSpace(ptr.Deref(in.Email)); email != "" {
		c.Email = email
	}
	if r.checkoutEmail(ctx, *c) == "" {
		return errCheckoutNeedsEmail
	}
	co.ContactPhone = ""
	if set.Phone != "hidden" {
		p, err := r.phoneRules(ctx, c.ChannelID).Normalize(ptr.Deref(in.Phone), co.QuoteDestination().CountryCode)
		if err != nil {
			return err
		}
		co.ContactPhone = p
	}
	if set.Phone == "required" && co.ContactPhone == "" {
		return errCheckoutNeedsPhone
	}

	co.PurchaseType, co.Company, co.BusinessIDs, co.PONumber = cart.PurchasePersonal, "", nil, ""
	business := set.Business && in.PurchaseType == frontmodel.PurchaseTypeBusiness
	if business {
		co.PurchaseType = cart.PurchaseBusiness
		if co.Company = strings.TrimSpace(ptr.Deref(in.Company)); co.Company == "" {
			return errCheckoutNeedsCompany
		}
		if set.PONumber {
			co.PONumber = strings.TrimSpace(ptr.Deref(in.PoNumber))
		}
	}

	var err error
	if co.IsPickup() {
		err = r.applyPickupDetails(ctx, *c, co, set, in)
	} else {
		err = r.applyDeliveryDetails(ctx, c, co, in)
	}
	if err != nil || !business {
		return err
	}
	// Last, since which identifiers are owed follows the company's country - its billing address, else the destination
	co.BusinessIDs, err = r.businessIDs(ctx, *co, set, in.BusinessIds)
	return err
}

func (r *Resolver) applyDeliveryDetails(ctx context.Context, c *cart.Cart, co *cart.Checkout, in frontmodel.CheckoutDetailsInput) error {
	a, source, err := r.cartAddress(ctx, *c, in.ShippingAddressID, in.ShippingAddress, "shipping")
	if err != nil {
		return err
	}
	co.ShippingAddress, co.ShippingAddressID, co.PickupContact = a, source, nil
	// The address is now the destination - a different country or postcode can drop the method, and the shopper picks again
	co.Destination = co.QuoteDestination()
	r.dropUnsupportedSelections(ctx, *c, co)
	if in.BillingSameAsShipping {
		co.BillingSameAsShipping, co.BillingAddress, co.BillingAddressID = true, nil, nil
		return nil
	}
	b, bsource, err := r.cartAddress(ctx, *c, in.BillingAddressID, in.BillingAddress, "billing")
	if err != nil {
		return err
	}
	co.BillingSameAsShipping, co.BillingAddress, co.BillingAddressID = false, b, bsource
	return nil
}

func (r *Resolver) applyPickupDetails(ctx context.Context, c cart.Cart, co *cart.Checkout, set checkoutSettings, in frontmodel.CheckoutDetailsInput) error {
	p := in.PickupContact
	if p == nil || strings.TrimSpace(p.FirstName) == "" || strings.TrimSpace(p.LastName) == "" {
		return errCheckoutNeedsCollector
	}
	co.PickupContact = &cart.PickupContact{
		FirstName:   strings.TrimSpace(p.FirstName),
		LastName:    strings.TrimSpace(p.LastName),
		SomeoneElse: p.SomeoneElse && set.PickupSomeoneElse,
	}
	// A pickup has no shipping address to mirror, so billing is its own address or none at all
	co.BillingSameAsShipping, co.BillingAddress, co.BillingAddressID = false, nil, nil
	hasBilling := (in.BillingAddressID != nil && *in.BillingAddressID != "") || in.BillingAddress != nil
	if !hasBilling {
		if set.pickupNeedsBilling(*co) {
			return errCheckoutNeedsBilling
		}
		return nil
	}
	b, source, err := r.cartAddress(ctx, c, in.BillingAddressID, in.BillingAddress, "billing")
	if err != nil {
		return err
	}
	co.BillingAddress, co.BillingAddressID = b, source
	return nil
}

// companyCountry is the country a business purchase's identifiers follow - where the company is billed, else where the
// order goes
func companyCountry(co cart.Checkout) string {
	if b, _ := co.EffectiveBilling(); b != nil {
		return b.CountryCode
	}
	return co.QuoteDestination().CountryCode
}

// businessIDFieldsFor is what a business purchase from the country asks for - the fields the country defines in the
// dashboard, else one generic tax ID as sales/checkout/tax_id_field says
func (r *Resolver) businessIDFieldsFor(ctx context.Context, countryCode string, set checkoutSettings) []country.BusinessIDField {
	if countryCode != "" && r.CountrySvc != nil {
		if c, err := r.CountrySvc.GetByCode(ctx, countryCode); err == nil && len(c.BusinessIDFields) > 0 {
			return c.BusinessIDFields
		}
	}
	if set.TaxID == "hidden" {
		return nil
	}
	return []country.BusinessIDField{{Key: genericTaxIDKey, Label: "Tax ID", Required: set.TaxID == "required"}}
}

const genericTaxIDKey = "tax_id"

// businessIDs keeps the values the company's country asks for and refuses a missing required one or one that doesn't
// match its pattern - keys the country doesn't ask for are dropped
func (r *Resolver) businessIDs(ctx context.Context, co cart.Checkout, set checkoutSettings, in []*frontmodel.BusinessIDValueInput) (map[string]string, error) {
	given := make(map[string]string, len(in))
	for _, v := range in {
		if v != nil {
			given[v.Key] = strings.TrimSpace(v.Value)
		}
	}
	out := map[string]string{}
	for _, f := range r.businessIDFieldsFor(ctx, companyCountry(co), set) {
		v := given[f.Key]
		switch {
		case v == "" && f.Required:
			return nil, apperr.New("CHECKOUT_BUSINESS_ID_REQUIRED", apperr.ClassValidation, "enter the "+f.Label)
		case v != "" && !f.Matches(v):
			return nil, apperr.New("CHECKOUT_BUSINESS_ID_INVALID", apperr.ClassValidation, "check the "+f.Label)
		case v != "":
			out[f.Key] = v
		}
	}
	return out, nil
}

// missingBusinessID reports whether a required identifier is absent - a setting or the country's fields may have
// changed since the details step was saved
func (r *Resolver) missingBusinessID(ctx context.Context, co cart.Checkout, set checkoutSettings) bool {
	for _, f := range r.businessIDFieldsFor(ctx, companyCountry(co), set) {
		if f.Required && co.BusinessIDs[f.Key] == "" {
			return true
		}
	}
	return false
}

func toBusinessIDValues(ids map[string]string) []*frontmodel.BusinessIDValue {
	keys := make([]string, 0, len(ids))
	for k := range ids {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]*frontmodel.BusinessIDValue, len(keys))
	for i, k := range keys {
		out[i] = &frontmodel.BusinessIDValue{Key: k, Value: ids[k]}
	}
	return out
}

// The store's phone rules - without the country domain (some tests), a phone is read in its address's country alone
func (r *Resolver) phoneRules(ctx context.Context, channelID uuid.UUID) phone.Rules {
	if r.CountrySvc == nil {
		return phone.Rules{}
	}
	return r.CountrySvc.PhoneRulesForChannel(ctx, channelID)
}
