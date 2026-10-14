package cart

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/delivery"
)

// A cart's kind - a shopper cart is the one a customer builds and holds one of per store, an offer is a cart an admin builds for one recipient and sends as a link. An offer checks out like any cart, but is never the customer's active cart, never merged and never treated as abandoned
const (
	KindShopper = "shopper"
	KindOffer   = "offer"
)

// The parts of an offer the admin can fix - the recipient can't change a locked part, and the admin sets it up front
const (
	LockLines    = "lines"    // items and quantities
	LockBilling  = "billing"  // billing address
	LockShipping = "shipping" // shipping address and destination
	LockDelivery = "delivery" // the delivery option per group, and its pickup point
	LockCoupons  = "coupons"  // promotion codes, gift cards and store credit
)

// CustomLineSKUPrefix starts every custom line's SKU - the key that keeps the line unique in its cart, never a catalog product
const CustomLineSKUPrefix = "CUSTOM-"

// Every lock, in the order the console shows them
var OfferLocks = []string{LockLines, LockBilling, LockShipping, LockDelivery, LockCoupons}

// Locks that only mean something when the cart ships - a cart of digital and custom lines has no address or delivery to fix
var shippingLocks = []string{LockShipping, LockDelivery}

// Where an offer stands, derived from its timestamps and the cart's status - never stored, so it can't drift from them
type OfferState string

const (
	OfferStateDraft     OfferState = "draft"
	OfferStateSent      OfferState = "sent"
	OfferStateOpened    OfferState = "opened"
	OfferStateCompleted OfferState = "completed"
	OfferStateExpired   OfferState = "expired"
	OfferStateRevoked   OfferState = "revoked"
)

var (
	ErrOfferExpired           = errors.New("this offer has expired")
	ErrOfferRevoked           = errors.New("this offer is no longer available")
	ErrOfferLocked            = errors.New("this part of the offer can't be changed")
	ErrOfferLockNotApplicable = errors.New("shipping and delivery can't be locked on a cart that doesn't ship")
	ErrOfferLockUnknown       = errors.New("unknown offer lock")
	ErrNotOffer               = errors.New("this cart is not an offer")
	ErrOfferExpiryInvalid     = errors.New("the offer must expire in the future")
	ErrOfferRecipientRequired = errors.New("an offer needs a customer or an email")
	ErrOfferMessageTooLong    = errors.New("the offer message is too long")
	ErrOfferLinePriceInvalid  = errors.New("the price can't be negative")
	ErrCustomLineNameRequired = errors.New("a custom line needs a name")
	ErrOfferPaymentNotAllowed = errors.New("this payment method isn't available for this offer")
)

// Offer is the offer half of a cart - nil on a shopper cart
type Offer struct {
	ExpiresAt     time.Time
	Message       string // a note to the recipient, shown in the email and on the offer page
	ShowInAccount bool   // also reachable from the customer's account, not just the link - customer offers only
	Locks         []string
	// The payment methods the recipient may choose from, by provider slug - empty leaves every method the store offers
	PaymentMethods []string
	LinkVersion    int32 // part of every link handed out - bumping it kills them all
	SentAt         *time.Time
	SendCount      int32
	OpenedAt       *time.Time
	RevokedAt      *time.Time
	CreatedBy      *uuid.UUID
	OrderID        *uuid.UUID // the order it became
}

func (o *Offer) Locked(lock string) bool { return o != nil && slices.Contains(o.Locks, lock) }

// PaymentAllowed says whether the recipient may pay this way - every method when the admin narrowed none
func (o *Offer) PaymentAllowed(slug string) bool {
	return o == nil || len(o.PaymentMethods) == 0 || slices.Contains(o.PaymentMethods, slug)
}

func (c Cart) IsOffer() bool { return c.Kind == KindOffer && c.Offer != nil }

// Locked reports whether the recipient is barred from a part of the cart - always false on a shopper cart
func (c Cart) Locked(lock string) bool { return c.IsOffer() && c.Offer.Locked(lock) }

// OfferState reads where the offer stands at now - a finished cart is completed whatever its dates say
func (c Cart) OfferState(now time.Time) OfferState {
	o := c.Offer
	switch {
	case o == nil:
		return ""
	case c.Status == "converted":
		return OfferStateCompleted
	case o.RevokedAt != nil || c.Status != "active":
		return OfferStateRevoked
	case !now.Before(o.ExpiresAt):
		return OfferStateExpired
	case o.OpenedAt != nil:
		return OfferStateOpened
	case o.SentAt != nil:
		return OfferStateSent
	default:
		return OfferStateDraft
	}
}

// OfferGrant is a verified offer link the request carries - the cart it opens and the link version it was signed for
type OfferGrant struct {
	CartID      uuid.UUID
	LinkVersion int32
}

type offerGrantKey struct{}

func WithOfferGrant(ctx context.Context, g OfferGrant) context.Context {
	return context.WithValue(ctx, offerGrantKey{}, g)
}

func OfferGrantFrom(ctx context.Context) (OfferGrant, bool) {
	g, ok := ctx.Value(offerGrantKey{}).(OfferGrant)
	return g, ok
}

// Whether the request holds a live link to this offer - a regenerated link leaves the old version, and every link signed for it, behind
func hasOfferGrant(ctx context.Context, c Cart) bool {
	g, ok := OfferGrantFrom(ctx)
	return ok && g.CartID == c.ID && g.LinkVersion == c.Offer.LinkVersion
}

// The offer side of checkCartAccess - its id alone opens nothing. A guest offer needs its link, a customer offer its owner signed in and either the link or the admin's leave to list it in the account. Ownership is answered before the dates, so a stranger learns nothing about an offer from its id
func checkOfferAccess(ctx context.Context, c Cart, callerID *uuid.UUID) error {
	if c.Offer == nil {
		return ErrCartForbidden
	}
	granted := hasOfferGrant(ctx, c)
	if c.CustomerID == nil {
		if !granted {
			return ErrCartForbidden
		}
	} else if callerID == nil || *callerID != *c.CustomerID || (!granted && !c.Offer.ShowInAccount) {
		return ErrCartForbidden
	}
	switch c.OfferState(time.Now()) {
	case OfferStateRevoked:
		return ErrOfferRevoked
	case OfferStateExpired:
		return ErrOfferExpired
	}
	return nil
}

// Refuses a checkout step that moved a locked part - one before/after diff covers every step setter, including ones added later
func checkOfferLocks(before, after Cart) error {
	if !before.IsOffer() {
		return nil
	}
	b, a := before.Checkout, after.Checkout
	changed := func(lock string, same bool) error {
		if !same && before.Locked(lock) {
			return lockedErr(lock)
		}
		return nil
	}
	billB, billIDB := b.EffectiveBilling()
	billA, billIDA := a.EffectiveBilling()
	var payment error
	if a.PaymentProviderSlug != b.PaymentProviderSlug && a.PaymentProviderSlug != "" && !before.Offer.PaymentAllowed(a.PaymentProviderSlug) {
		payment = ErrOfferPaymentNotAllowed
	}
	return errors.Join(
		payment,
		changed(LockBilling, sameAddress(billB, billA) && sameID(billIDB, billIDA)),
		changed(LockShipping, sameAddress(b.ShippingAddress, a.ShippingAddress) && sameID(b.ShippingAddressID, a.ShippingAddressID) && b.Destination == a.Destination),
		changed(LockDelivery, sameDeliveries(b, a)),
		changed(LockCoupons, slices.Equal(b.GiftCardCodes, a.GiftCardCodes) && b.UseStoreCredit == a.UseStoreCredit),
	)
}

func lockedErr(lock string) error { return &LockedError{Lock: lock} }

// LockedError names the locked part a change touched, so the storefront can say which
type LockedError struct{ Lock string }

func (e *LockedError) Error() string { return ErrOfferLocked.Error() + ": " + e.Lock }
func (e *LockedError) Unwrap() error { return ErrOfferLocked }

func sameAddress(a, b *Address) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.FirstName == b.FirstName && a.LastName == b.LastName && a.Street1 == b.Street1 && a.City == b.City &&
		a.PostalCode == b.PostalCode && a.CountryCode == b.CountryCode && sameStr(a.Company, b.Company) && sameStr(a.Street2, b.Street2) && sameStr(a.State, b.State) && sameStr(a.Phone, b.Phone)
}

func sameStr(a, b *string) bool {
	if a == nil || b == nil {
		return (a == nil || *a == "") && (b == nil || *b == "")
	}
	return *a == *b
}

func sameID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameDeliveries(b, a Checkout) bool {
	if b.SplitDelivery != a.SplitDelivery || b.ShippingCarrierSlug != a.ShippingCarrierSlug || b.ShippingMethodCode != a.ShippingMethodCode {
		return false
	}
	if len(b.Deliveries) != len(a.Deliveries) {
		return false
	}
	for k, d := range b.Deliveries {
		o, ok := a.Deliveries[k]
		if !ok || d.OptionID != o.OptionID || d.CarrierSlug != o.CarrierSlug || d.MethodCode != o.MethodCode || !samePoint(d, o) {
			return false
		}
	}
	return true
}

func samePoint(a, b DeliverySelection) bool {
	if a.Point == nil || b.Point == nil {
		return a.Point == b.Point
	}
	return a.Point.ID == b.Point.ID
}

// NormalizeOfferLocks dedupes and orders the locks, refusing an unknown one, and refuses the shipping locks when the cart doesn't ship
func NormalizeOfferLocks(locks []string, ships bool) ([]string, error) {
	out := make([]string, 0, len(locks))
	for _, l := range OfferLocks {
		if slices.Contains(locks, l) {
			out = append(out, l)
		}
	}
	for _, l := range locks {
		if !slices.Contains(OfferLocks, l) {
			return nil, errors.Join(ErrOfferLockUnknown, errors.New(l))
		}
	}
	if !ships {
		for _, l := range shippingLocks {
			if slices.Contains(out, l) {
				return nil, ErrOfferLockNotApplicable
			}
		}
	}
	return out, nil
}

// DeliveryProfiles adds what delivery planning should know of the cart's custom lines to the catalog's profiles - they never ship, and no catalog product backs them, so left out they'd plan as physical
func DeliveryProfiles(lines []CartLine, catalog map[string]delivery.LineProfile) map[string]delivery.LineProfile {
	out := catalog
	for _, l := range lines {
		if !l.IsCustom {
			continue
		}
		if out == nil {
			out = map[string]delivery.LineProfile{}
		}
		out[l.ProductSku] = delivery.LineProfile{SKU: l.ProductSku, RequiresShipping: false}
	}
	return out
}

// DeliveryLines are the cart's lines as delivery planning sees them
func DeliveryLines(lines []CartLine) []delivery.Line {
	out := make([]delivery.Line, 0, len(lines))
	for _, l := range lines {
		out = append(out, delivery.Line{ID: l.ID, SKU: l.ProductSku, Name: l.ProductName, Quantity: l.Quantity, UnitPrice: l.UnitPrice})
	}
	return out
}

// Unlocked refuses when an offer locks this part from its recipient - for the changes made outside the cart service, like promotion codes
func (c Cart) Unlocked(lock string) error {
	if c.Locked(lock) {
		return lockedErr(lock)
	}
	return nil
}

// OfferDeliveryGroup is one delivery group of an offer as the console fills it in - the choices are the storefront checkout's own, so what the admin picks is what the recipient's checkout offers
type OfferDeliveryGroup struct {
	Key       string
	LineNames []string
	Choices   []OfferDeliveryChoice
}

type OfferDeliveryChoice struct {
	OptionID    uuid.UUID
	OptionName  string
	Kind        string
	CarrierSlug string
	MethodCode  string
	MethodName  string
	PriceCents  int64
	NeedsPoint  bool
}

// OfferDeliveryInput is the admin's delivery for one group - the point only for a kind that delivers to one
type OfferDeliveryInput struct {
	GroupKey    string
	OptionID    uuid.UUID
	CarrierSlug string
	MethodCode  string
	PointID     string
}
