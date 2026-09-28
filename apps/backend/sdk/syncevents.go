package sdk

import (
	"database/sql"

	"github.com/google/uuid"
)

// The before-events a plugin may change or refuse, each seen through an interface of its own like Tendering - subscribe
// with SubscribeSync and type-assert the `any` to the interface, since the concrete event types live in core's internal
// packages, which a plugin module cannot import
//
// Refusing is the same everywhere: return an error, a UserError when the shopper should read why. What a plugin may
// change is what a sync webhook on the same event may change, so an in-process plugin and an external app have the
// same powers
//
// Accessors carry the event's own prefix because a Go method can't share a field's name, and core's events keep
// their fields
const (
	EventCartLineAdding      = "cart.line.adding"
	EventCartBundleAdding    = "cart.bundle.adding"
	EventCartLineUpdating    = "cart.line.updating"
	EventCartLineRemoving    = "cart.line.removing"
	EventCheckoutValidating  = "checkout.validating"
	EventOrderPlacing        = "order.placing"
	EventOrderCancelling     = "order.cancelling"
	EventOrderFulfilling     = "order.fulfilling"
	EventInventoryAllocating = "inventory.allocating"
	EventReviewSubmitting    = "review.submitting"
	EventCustomerRegistering = "customer.registering"
	EventCustomerDeleting    = "customer.deleting"
)

// A line about to be added to a cart - what arrives is what will be written, so a price or quantity set here is the
// line's
//
// cart.bundle.adding arrives as one of these too, the price being the bundle's, so one handler can register for both
//
//	p.SubscribeSync(sdk.EventCartLineAdding, 100, func(ctx context.Context, e any) error {
//	    line, ok := e.(sdk.CartLineAdding)
//	    if !ok {
//	        return nil
//	    }
//	    if cents, ok := staffPrice(line.LineSKU()); ok {
//	        return line.SetLineUnitPrice(cents)
//	    }
//	    return nil
//	})
type CartLineAdding interface {
	LineCartID() uuid.UUID
	LineChannelID() uuid.UUID
	// nil for a guest cart
	LineCustomerID() *uuid.UUID
	LineSKU() string
	LineName() string
	LineQuantity() float64
	LineUnitPriceCents() int64
	// Refuses a negative price
	SetLineUnitPrice(cents int64) error
	// Refuses anything not above zero - removing a line is its own operation
	SetLineQuantity(qty float64) error
}

// A cart line's quantity about to change - the quantity is the absolute new one, not a delta
type CartLineUpdating interface {
	LineCartID() uuid.UUID
	LineChannelID() uuid.UUID
	LineCustomerID() *uuid.UUID
	CartLineID() uuid.UUID
	// Empty when core couldn't resolve the line's product
	LineSKU() string
	LineQuantity() float64
	SetLineQuantity(qty float64) error
}

// A cart line about to be removed - refuse-only, for a line a plugin put there on purpose, like a free gift the cart
// still qualifies for
type CartLineRemoving interface {
	RemovingCartID() uuid.UUID
	RemovingChannelID() uuid.UUID
	// nil for a guest cart
	RemovingCustomerID() *uuid.UUID
	RemovingLineID() uuid.UUID
}

// What checkout.validating and order.placing share, so one handler can review both - a fraud screen, say
//
// The three changes accumulate across every subscriber, in the webhook's rules: a discount adds on top, a risk score
// only ever rises, tags append
type CheckoutReview interface {
	CheckoutOrderID() uuid.UUID // pre-allocated, the id the order will have
	CheckoutChannelID() uuid.UUID
	// nil for a guest checkout
	CheckoutCustomerID() *uuid.UUID
	CheckoutEmail() string
	CheckoutCartID() uuid.UUID
	CheckoutSubtotalCents() int64
	// What subscribers so far have added - on order.placing it includes the promotion engine's discount
	CheckoutDiscountCents() int64
	CheckoutRiskScore() int
	CheckoutTags() []string

	AddCheckoutDiscount(cents int64)
	RaiseCheckoutRiskScore(score int)
	AddCheckoutTags(tags ...string)
}

// Before the placement transaction opens - the place for gates that call out, since a slow one delays the shopper
// without holding a transaction. What it adds seeds the order.placing that follows
type CheckoutValidating interface {
	CheckoutReview
	CheckoutPaymentProvider() string
	// The promotion engine's discount, informational so a validator sees the realistic total - not yet in CheckoutDiscountCents
	CheckoutEngineDiscountCents() int64
}

// Inside the placement transaction, before the order is written - join PlacementTx() for writes that must commit or
// roll back with the order, and keep the handler fast, since the transaction stays open while it runs
//
// Tax and shipping can be set outright, for a plugin that computes them itself; unset, core prices them. The total is
// not settable - it is derived from its parts, and overriding it would let them disagree
type OrderPlacing interface {
	CheckoutReview
	PlacementTx() *sql.Tx
	// Refuses a negative amount
	SetPlacingTaxCents(cents int64) error
	SetPlacingShippingCents(cents int64) error
}

// A shopper's self-registration, before the account exists - only the storefront's, never an admin-created customer,
// who is vouched for. Tags ride through to customer.registered for async subscribers to act on
//
// A plugin can route the account into a customer group - corporate email domains into a B2B group, say - and decide
// marketing consent where the shopper was shown no checkbox. The shopper's own answer always wins over a plugin's, and
// a plugin's is recorded with the source "plugin"; a group that no longer exists falls back to the default rather than
// refusing the signup
type CustomerRegistering interface {
	RegisteringChannelID() uuid.UUID
	RegisteringEmail() string
	RegisteringFirstName() string
	RegisteringLastName() string
	RegisteringTags() []string
	// nil until a subscriber picks one
	RegisteringGroupID() *uuid.UUID
	// nil until a subscriber decides
	RegisteringMarketingConsent() *bool

	AddRegisteringTags(tags ...string)
	SetRegisteringGroup(groupID uuid.UUID)
	SetRegisteringMarketingConsent(optedIn bool)
}

// An order about to be cancelled - refuse it (an ERP that knows it already shipped) or add to the note that lands on
// the order's timeline
type OrderCancelling interface {
	CancellingOrderID() uuid.UUID
	CancellingChannelID() uuid.UUID
	CancellingFromStatus() string
	CancellingNote() string
	// Appends, so several subscribers each add their own line
	AddCancellingNote(note string)
}

// An order about to be marked fulfilled - the seam a warehouse or 3PL integration stamps its shipment on
type OrderFulfilling interface {
	FulfillingOrderID() uuid.UUID
	FulfillingChannelID() uuid.UUID
	FulfillingFromStatus() string
	FulfillingCarrier() string
	FulfillingTrackingNumber() string
	// Both refuse an empty value - there is nothing to clear
	SetFulfillingCarrier(carrier string) error
	SetFulfillingTrackingNumber(number string) error
}

// Stock about to be reserved for an order, line by line - a plugin routes a line to one stock location, and core
// reserves it only there. A location outside the order's channel fails the placement, so route to the channel's own
type InventoryAllocating interface {
	AllocatingOrderID() uuid.UUID
	AllocatingChannelID() uuid.UUID
	AllocatingLineIDs() []uuid.UUID
	// uuid.Nil and 0 for a line id not in this allocation
	AllocatingLineProductID(lineID uuid.UUID) uuid.UUID
	AllocatingLineQuantity(lineID uuid.UUID) float64
	// The location a subscriber routed the line to, if any
	AllocatingRoute(lineID uuid.UUID) (locationID uuid.UUID, ok bool)
	// Later routes win over earlier ones; refuses a line id not in this allocation
	RouteAllocatingLine(lineID, locationID uuid.UUID) error
}

// The moderation statuses a review can be submitted into
const (
	ReviewStatusPending  = "pending"
	ReviewStatusApproved = "approved"
	ReviewStatusRejected = "rejected"
)

// A product review about to be stored - a spam or profanity filter sets its status, which skips the moderation queue
// when it isn't pending, and tags it
type ReviewSubmitting interface {
	ReviewProductID() uuid.UUID
	ReviewChannelID() uuid.UUID
	// nil for a guest review, which carries ReviewGuestEmail instead
	ReviewCustomerID() *uuid.UUID
	ReviewGuestEmail() string
	ReviewLocale() string
	ReviewTitle() string
	ReviewBody() string
	ReviewStatus() string
	ReviewTags() []string
	// Refuses anything but the ReviewStatus* values
	SetReviewStatus(status string) error
	AddReviewTags(tags ...string)
}

// A customer about to be deleted - refuse-only, for a plugin whose own records can't lose their customer, like a
// company's only admin
type CustomerDeleting interface {
	DeletingCustomerID() uuid.UUID
	DeletingChannelID() uuid.UUID
	DeletingEmail() string
}
