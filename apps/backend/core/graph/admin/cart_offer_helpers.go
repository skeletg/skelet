package admin

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/delivery"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// OfferCheckout is the storefront checkout seen from the console - the front resolver implements it, so an offer's delivery is picked from the very choices the recipient's checkout will offer
type OfferCheckout interface {
	OfferNeedsShipping(ctx context.Context, c cart.Cart) bool
	OfferDeliveryGroups(ctx context.Context, ch *channel.Channel, c cart.Cart) []cart.OfferDeliveryGroup
	OfferDeliveryPoints(ctx context.Context, ch *channel.Channel, c cart.Cart, groupKey string, optionID uuid.UUID, search string) ([]delivery.Point, error)
	ApplyOfferDeliveries(ctx context.Context, ch *channel.Channel, c *cart.Cart, in []cart.OfferDeliveryInput) error
	OfferPaymentOffered(ctx context.Context, c cart.Cart, slug string) bool
}

// OfferMail is everything an offer email says
type OfferMail struct {
	Cart     cart.Cart
	Channel  *channel.Channel
	To       string
	Name     string // the recipient's name, empty for a guest
	Link     string
	Currency string
}

// OfferMailer sends an offer's email - through a Signals flow when one takes it, else core's own template - and renders that template for the console's preview
type OfferMailer interface {
	SendCartOffer(ctx context.Context, m OfferMail) error
	PreviewCartOffer(ctx context.Context, m OfferMail) (OfferMailPreview, error)
}

type OfferMailPreview struct {
	Subject string
	HTML    string
	Text    string
}

var (
	errOfferNoLink    = apperr.New("CART_OFFER_NO_LINK", apperr.ClassValidation, "the channel has no public URL to link the offer to - set one on the channel first")
	errOfferNoMailer  = apperr.New("CART_OFFER_MAIL_UNAVAILABLE", apperr.ClassUnavailable, "offer emails aren't configured")
	errOfferNoAddress = apperr.New("CART_OFFER_NO_RECIPIENT", apperr.ClassValidation, "the offer has no email to send to")
	errOfferPayment   = apperr.New("PAYMENT_METHOD_UNAVAILABLE", apperr.ClassValidation, "that payment method isn't enabled on the channel")
)

// The lookup-first gate shared by the offer mutations - orders.update on the cart's channel, and the cart must be an offer
func (r *mutationResolver) offerForAdmin(ctx context.Context, idStr string) (cart.Cart, error) {
	id, err := r.cartForAdmin(ctx, idStr)
	if err != nil {
		return cart.Cart{}, err
	}
	c, err := r.CartSvc.GetCartByIDReadOnly(ctx, id)
	if err != nil {
		return cart.Cart{}, errNotFound("cart")
	}
	if !c.IsOffer() {
		return cart.Cart{}, cart.ErrNotOffer
	}
	return c, nil
}

func (r *Resolver) offerChannel(ctx context.Context, channelID uuid.UUID) (*channel.Channel, error) {
	if r.ChannelSvc == nil {
		return nil, errNotFound("channel")
	}
	ch, err := r.ChannelSvc.GetByID(ctx, channelID)
	if err != nil {
		return nil, errNotFound("channel")
	}
	return &ch, nil
}

// offerLinkPath is the link's path on the storefront - empty when no link resolver is wired
func (r *Resolver) offerLinkPath(c cart.Cart) string {
	links := r.CartSvc.OfferLinks()
	if links == nil || !c.IsOffer() {
		return ""
	}
	return cart.OfferLinkPath + c.Offer.LinkToken
}

// offerLink is the link the offer's email carries - empty when the channel has no public URL or no link resolver is wired
func (r *Resolver) offerLink(ch *channel.Channel, c cart.Cart) string {
	path := r.offerLinkPath(c)
	if ch == nil || path == "" || strings.TrimSpace(ch.PublicURL) == "" {
		return ""
	}
	return strings.TrimRight(ch.PublicURL, "/") + path
}

// The default expiry a new offer gets - sales/cart/offer_expiry_days from now, at least a day
func (r *Resolver) defaultOfferExpiry(ctx context.Context, channelID uuid.UUID) time.Time {
	days := 7
	if r.ConfigSvc != nil {
		if n, err := r.ConfigSvc.GetInt(ctx, "sales/cart/offer_expiry_days", config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}); err == nil && n > 0 {
			days = int(n)
		}
	}
	return time.Now().Add(time.Duration(days) * 24 * time.Hour)
}

func (r *Resolver) offerNeedsShipping(ctx context.Context, c cart.Cart) bool {
	if r.OfferCheckout == nil {
		return true
	}
	return r.OfferCheckout.OfferNeedsShipping(ctx, c)
}

func (r *Resolver) toAdminCartOfferModel(ctx context.Context, c cart.Cart) *model.AdminCartOffer {
	if !c.IsOffer() {
		return nil
	}
	o := c.Offer
	locks := make([]model.CartOfferLock, len(o.Locks))
	for i, l := range o.Locks {
		locks[i] = model.CartOfferLock(strings.ToUpper(l))
	}
	ch, _ := r.offerChannel(ctx, c.ChannelID)
	m := &model.AdminCartOffer{
		State:          model.CartOfferState(strings.ToUpper(string(c.OfferState(time.Now())))),
		ExpiresAt:      o.ExpiresAt,
		Message:        o.Message,
		ShowInAccount:  o.ShowInAccount,
		Locks:          locks,
		PaymentMethods: append([]string{}, o.PaymentMethods...),
		NeedsShipping:  r.offerNeedsShipping(ctx, c),
		SentAt:         o.SentAt,
		SendCount:      o.SendCount,
		OpenedAt:       o.OpenedAt,
		RevokedAt:      o.RevokedAt,
		Checkout:       toAdminOfferCheckoutModel(c.Checkout),
	}
	if auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.update", c.ChannelID, nil) == nil {
		m.Link = r.offerLink(ch, c)
		m.LinkPath = r.offerLinkPath(c)
	}
	if o.OrderID != nil {
		s := o.OrderID.String()
		m.OrderID = &s
	}
	return m
}

func toAdminOfferCheckoutModel(co cart.Checkout) *model.AdminCartOfferCheckout {
	m := &model.AdminCartOfferCheckout{
		ShippingAddress:       toAdminCartAddressModel(co.ShippingAddress),
		BillingSameAsShipping: co.BillingSameAsShipping,
		BillingAddress:        toAdminCartAddressModel(co.BillingAddress),
		PaymentProviderSlug:   co.PaymentProviderSlug,
		Deliveries:            []*model.AdminCartDeliverySelection{},
	}
	for key, d := range co.Deliveries {
		if d.OptionID == uuid.Nil {
			continue
		}
		sel := &model.AdminCartDeliverySelection{GroupKey: key, OptionID: d.OptionID.String(), CarrierSlug: d.CarrierSlug, MethodCode: d.MethodCode}
		if d.Point != nil {
			sel.PointID, sel.PointName = &d.Point.ID, &d.Point.Name
		}
		m.Deliveries = append(m.Deliveries, sel)
	}
	return m
}

func toAdminCartAddressModel(a *cart.Address) *model.AdminCartAddress {
	if a == nil {
		return nil
	}
	return &model.AdminCartAddress{
		FirstName: a.FirstName, LastName: a.LastName, Company: a.Company, Street1: a.Street1, Street2: a.Street2,
		City: a.City, State: a.State, PostalCode: a.PostalCode, CountryCode: a.CountryCode, Phone: a.Phone,
	}
}

func fromAdminCartAddressInput(in *model.AdminCartAddressInput) *cart.Address {
	if in == nil {
		return nil
	}
	return &cart.Address{
		FirstName: strings.TrimSpace(in.FirstName), LastName: strings.TrimSpace(in.LastName), Company: trimPtr(in.Company),
		Street1: strings.TrimSpace(in.Street1), Street2: trimPtr(in.Street2), City: strings.TrimSpace(in.City), State: trimPtr(in.State),
		PostalCode: strings.TrimSpace(in.PostalCode), CountryCode: strings.ToUpper(strings.TrimSpace(in.CountryCode)), Phone: trimPtr(in.Phone),
	}
}

func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

// The offer state a list row shows, from the columns the list carries
func listItemOfferState(it cart.CartListItem) *model.CartOfferState {
	if it.Kind != cart.KindOffer || it.OfferExpiresAt == nil {
		return nil
	}
	c := cart.Cart{Status: it.Status, Kind: it.Kind, Offer: &cart.Offer{ExpiresAt: *it.OfferExpiresAt, SentAt: it.OfferSentAt, OpenedAt: it.OfferOpenedAt, RevokedAt: it.OfferRevokedAt}}
	s := model.CartOfferState(strings.ToUpper(string(c.OfferState(time.Now()))))
	return &s
}

func cartKindModel(kind string) model.CartKind {
	if kind == cart.KindOffer {
		return model.CartKindOffer
	}
	return model.CartKindShopper
}

func lockSlugs(locks []model.CartOfferLock) []string {
	out := make([]string, len(locks))
	for i, l := range locks {
		out[i] = strings.ToLower(string(l))
	}
	return out
}

// The read gate of the offer queries - orders.view on the cart's channel
func (r *queryResolver) offerForView(ctx context.Context, idStr string) (cart.Cart, *channel.Channel, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "orders.view"); err != nil {
		return cart.Cart{}, nil, err
	}
	cartID, err := parseID(idStr, "cart")
	if err != nil {
		return cart.Cart{}, nil, err
	}
	c, err := r.CartSvc.GetCartByIDReadOnly(ctx, cartID)
	if err != nil {
		return cart.Cart{}, nil, errNotFound("cart")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.view", c.ChannelID, nil); err != nil {
		return cart.Cart{}, nil, err
	}
	if !c.IsOffer() {
		return cart.Cart{}, nil, cart.ErrNotOffer
	}
	ch, err := r.offerChannel(ctx, c.ChannelID)
	if err != nil {
		return cart.Cart{}, nil, err
	}
	return c, ch, nil
}

func (r *Resolver) reloadAdminCart(ctx context.Context, id uuid.UUID) (*model.AdminCart, error) {
	c, err := r.CartSvc.GetCartByIDReadOnly(ctx, id)
	if err != nil {
		return nil, err
	}
	return r.mapCart(ctx, c), nil
}

// applyOfferCheckout writes the admin's checkout onto the offer - the addresses as given, the payment method and deliveries only as the recipient's checkout would accept them. A cart with nothing to deliver keeps no shipping address or delivery
func (r *Resolver) applyOfferCheckout(ctx context.Context, ch *channel.Channel, next *cart.Cart, in model.AdminUpdateCartOfferInput, ships bool) error {
	co := &next.Checkout
	co.ShippingAddressID, co.BillingAddressID = nil, nil
	co.ShippingAddress = nil
	if ships {
		co.ShippingAddress = fromAdminCartAddressInput(in.ShippingAddress)
	}
	if err := validOfferAddress(co.ShippingAddress, "shipping"); err != nil {
		return err
	}
	if co.ShippingAddress != nil {
		a := co.ShippingAddress
		co.Destination = cart.Destination{CountryCode: a.CountryCode, Region: ptrStr(a.State), PostalCode: a.PostalCode}
	}
	co.BillingSameAsShipping = in.BillingSameAsShipping && co.ShippingAddress != nil
	co.BillingAddress = nil
	if !co.BillingSameAsShipping {
		co.BillingAddress = fromAdminCartAddressInput(in.BillingAddress)
	}
	if err := validOfferAddress(co.BillingAddress, "billing"); err != nil {
		return err
	}
	// A method the recipient already chose that the offer no longer allows is dropped, and a single allowed method is chosen for them
	allowed := &cart.Offer{PaymentMethods: in.PaymentMethods}
	if !allowed.PaymentAllowed(co.PaymentProviderSlug) {
		co.PaymentProviderSlug = ""
	}
	if len(in.PaymentMethods) == 1 {
		co.PaymentProviderSlug = in.PaymentMethods[0]
	}
	if err := lockedPartsFilled(*co, lockSlugs(in.Locks), ships, len(in.Deliveries) > 0); err != nil {
		return err
	}
	co.Deliveries = nil
	if !ships || len(in.Deliveries) == 0 {
		return nil
	}
	if r.OfferCheckout == nil {
		return apperr.New("CART_OFFER_DELIVERY_UNAVAILABLE", apperr.ClassUnavailable, "deliveries can't be chosen here")
	}
	sels := make([]cart.OfferDeliveryInput, 0, len(in.Deliveries))
	for _, d := range in.Deliveries {
		oid, err := parseID(d.OptionID, "delivery option")
		if err != nil {
			return err
		}
		sel := cart.OfferDeliveryInput{GroupKey: d.GroupKey, OptionID: oid, CarrierSlug: d.CarrierSlug, MethodCode: d.MethodCode}
		if d.PointID != nil {
			sel.PointID = *d.PointID
		}
		sels = append(sels, sel)
	}
	return r.OfferCheckout.ApplyOfferDeliveries(ctx, ch, next, sels)
}

// The fields every address needs - placement checks the rest against the store's countries and validators
func validOfferAddress(a *cart.Address, which string) error {
	if a == nil {
		return nil
	}
	if a.FirstName == "" || a.LastName == "" || a.Street1 == "" || a.City == "" || a.PostalCode == "" || len(a.CountryCode) != 2 {
		return apperr.New("CART_OFFER_ADDRESS_INCOMPLETE", apperr.ClassValidation, "the "+which+" address needs a name, street, city, postal code and country")
	}
	return nil
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func cartKindFilter(k *model.CartKind) *string {
	if k == nil {
		return nil
	}
	s := cart.KindShopper
	if *k == model.CartKindOffer {
		s = cart.KindOffer
	}
	return &s
}

func formatPointAddress(a delivery.Address) string {
	parts := []string{}
	for _, p := range []string{a.Street1, a.Street2, strings.TrimSpace(a.PostalCode + " " + a.City), a.CountryCode} {
		if strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

// A locked part the admin left empty would strand the recipient - they couldn't fill it in, and checkout can't finish without it
func lockedPartsFilled(co cart.Checkout, locks []string, ships, hasDeliveries bool) error {
	need := func(lock string, filled bool, what string) error {
		if !slices.Contains(locks, lock) || filled {
			return nil
		}
		return apperr.New("CART_OFFER_LOCK_EMPTY", apperr.ClassValidation, "fill in the "+what+" before locking it")
	}
	billing, _ := co.EffectiveBilling()
	return errors.Join(
		need(cart.LockShipping, !ships || co.ShippingAddress != nil, "shipping address"),
		need(cart.LockBilling, billing != nil, "billing address"),
		need(cart.LockDelivery, !ships || hasDeliveries, "delivery"),
	)
}

// offerMail is the mail an offer would go out as now - its recipient, and the link that opens it. A channel with no public URL has no link to send, so complete is false and Link is the path alone
func (r *Resolver) offerMail(ctx context.Context, c cart.Cart) (m OfferMail, complete bool, err error) {
	ch, err := r.offerChannel(ctx, c.ChannelID)
	if err != nil {
		return OfferMail{}, false, err
	}
	m = OfferMail{Cart: c, Channel: ch, To: c.Email, Link: r.offerLink(ch, c), Currency: r.channelCurrency(ctx, c.ChannelID)}
	if email, n := r.cartCustomerInfo(ctx, c.CustomerID); email != nil {
		m.To = *email
		if n != nil {
			m.Name = *n
		}
	}
	complete = m.Link != ""
	if !complete {
		m.Link = r.offerLinkPath(c)
	}
	return m, complete, nil
}

// offerPaymentMethods checks the admin's methods against what the channel has enabled - an offer can only narrow the store's own choice
func (r *Resolver) offerPaymentMethods(ctx context.Context, channelID uuid.UUID, slugs []string) ([]string, error) {
	if len(slugs) == 0 {
		return nil, nil
	}
	if r.PaymentSvc == nil {
		return nil, errOfferPayment
	}
	enabled, err := r.PaymentSvc.ListEnabledPaymentMethods(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(slugs))
	for _, s := range slugs {
		s = strings.TrimSpace(s)
		if !slices.ContainsFunc(enabled, func(m payment.PaymentMethod) bool { return m.ProviderSlug == s }) {
			return nil, errOfferPayment
		}
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out, nil
}
