package sdk

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"time"

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
	EventCartLineAdding       = "cart.line.adding"
	EventCartBundleAdding     = "cart.bundle.adding"
	EventCartLineUpdating     = "cart.line.updating"
	EventCartLineRemoving     = "cart.line.removing"
	EventCheckoutValidating   = "checkout.validating"
	EventOrderPlacing         = "order.placing"
	EventOrderCancelling      = "order.cancelling"
	EventOrderFulfilling      = "order.fulfilling"
	EventOrderTransitioning   = "order.transitioning"
	EventInventoryAllocating  = "inventory.allocating"
	EventReviewSubmitting     = "review.submitting"
	EventCustomerRegistering  = "customer.registering"
	EventCustomerDeleting     = "customer.deleting"
	EventLoyaltyEarning       = "loyalty.earning"
	EventSubscriptionRenewing = "subscription.renewing"
	EventProductSaving        = "product.saving"
	EventCustomerSaving       = "customer.saving"
)

// A line about to be added to a cart - what arrives is what will be written, so a price or quantity set here is the
// line's
//
// A price you set is a set price, and core treats it the way it treats one an admin set. It holds: checkout doesn't
// price the line again from the catalog. It is for the quantity you priced - the shopper may lower the quantity, but
// more of the item reaches you again (here, or as a CartLineUpdating), and is refused if you don't price it then.
// Coupons and promotions leave the line alone unless you say they apply. The line and the order line it becomes
// record that your plugin set the price, and the catalog price it replaced. For a price that should behave like
// the catalog's own - repriced, discounted by promotions - register a PriceResolver instead
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
	// Refuses a negative price, and one past what a line can cost
	SetLineUnitPrice(cents int64) error
	// Lets coupons and promotions lower a line whose price you set - they don't unless this says so
	SetLinePromotionsApply(apply bool)
	// Refuses anything not above zero - removing a line is its own operation
	SetLineQuantity(qty float64) error
}

// A cart line's quantity about to change - the quantity is the absolute new one, not a delta. The price is the unit
// price the line will have at it: set it to price the line at the new quantity, with the same meaning as on an add
type CartLineUpdating interface {
	LineCartID() uuid.UUID
	LineChannelID() uuid.UUID
	LineCustomerID() *uuid.UUID
	CartLineID() uuid.UUID
	// Empty when core couldn't resolve the line's product
	LineSKU() string
	LineQuantity() float64
	SetLineQuantity(qty float64) error
	LineUnitPriceCents() int64
	// Ignored on a bundle line, which keeps its own price, and on a line whose price an admin set
	SetLineUnitPrice(cents int64) error
	SetLinePromotionsApply(apply bool)
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

// On checkout.validating and order.placing - false for a field left empty or no longer asked
type CheckoutFieldAnswers interface {
	CheckoutFieldValue(key string) (string, bool)
}

// On order.placing, into the order's metadata - Plugin.SetOrderMetadata keeps a key under your slug, and "tracking" is
// core's
type OrderPlacingMetadata interface {
	SetPlacingMetadata(key, value string) error
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

// An order about to change status or stage - refuse-only. It fires for the moves someone asks for (an admin's, a
// customer's cancel, a shipment, a hold, a plugin's stage), not the ones placement makes itself
type OrderTransitioning interface {
	TransitioningOrderID() uuid.UUID
	TransitioningChannelID() uuid.UUID
	TransitioningFromStatus() string
	TransitioningToStatus() string // the same as the from status when only the stage moves
	TransitioningFromStage() string
	TransitioningToStage() string // empty when the order is left without one
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

// Points about to be earned on a paid-for order, the channel's rate already applied - change them for a rule of your
// own, like double points on a category or none on free gifts. It can arrive more than once for an order, so compute
// the points from the event alone rather than counting what you've seen
type LoyaltyEarning interface {
	EarningOrderID() uuid.UUID
	EarningChannelID() uuid.UUID
	EarningCustomerID() uuid.UUID
	EarningCurrencyCode() string
	EarningUnitCents() int64 // cents in one whole currency unit, what the rate earns per
	EarningPointsPerUnit() int64
	EarningMerchandiseCents() int64 // the subtotal less discounts, what core's points came from
	EarningLineCount() int
	EarningLine(i int) (sku string, quantity float64, totalCents int64)
	EarningLineProduct(i int) (productID uuid.UUID, categoryIDs []uuid.UUID) // uuid.Nil for a line with no catalog product
	EarningLineUnitCents(i int) int64                                        // what one unit sold for
	EarningLineGift(i int) bool                                              // a promotion's free gift
	EarningPoints() int64
	SetEarningPoints(points int64) error // refuses a negative number, or one past 100,000,000 - 0 earns nothing
	AddEarningNote(note string)          // shown in the points history beside the order number - five at most, 200 characters each
}

// A subscription's next period about to be invoiced - change what it costs, skip it, or move it later. It runs before
// the subscription is locked for billing and can arrive more than once for a period, so decide from the event alone;
// an error leaves the period for a later run, a minute on and longer each time
type SubscriptionRenewing interface {
	RenewalSubscriptionID() uuid.UUID
	RenewalChannelID() uuid.UUID
	RenewalCustomerID() *uuid.UUID // nil for a guest's
	RenewalProductID() uuid.UUID
	RenewalSKU() string
	RenewalQuantity() float64
	RenewalSequence() int // which invoice this would be, 1 for the first renewal
	RenewalPeriodStart() time.Time
	RenewalPeriodEnd() time.Time
	RenewalCurrencyCode() string
	RenewalAmountCents() int64
	SetRenewalAmount(cents int64) error // refuses a negative amount, or one past 10,000,000,000 - 0 bills nothing
	SkipRenewal()                       // the period passes unbilled, and the next comes as usual
	DelayRenewal(until time.Time) error // refuses a time not ahead of now - later periods count from it, and it wins over a skip
}

// A product or customer being saved, after its row is written and before the transaction commits - with what the
// save's input carried for plugins under `extensions`, each under its owner's slug. Write your rows through SavingTx
// so they commit with the record's; return an error to refuse the whole save, an sdk.UserError when the admin should
// read why. Read your part with pl.Extension - it only arrives once you've declared it with RegisterRecordExtension,
// which is also where you name the permission sending it takes. Keep the handler to your own writes: the record's row
// is locked until it returns
//
//	p.SubscribeSync(sdk.EventProductSaving, 100, func(ctx context.Context, e any) error {
//	    save, ok := e.(sdk.RecordSaving)
//	    var warranty struct{ Months int `json:"months"` }
//	    if !ok {
//	        return nil
//	    }
//	    if given, err := p.Extension(save, &warranty); !given || err != nil {
//	        return err
//	    }
//	    _, err := save.SavingTx().ExecContext(ctx, `INSERT INTO acme_warranties ...`, save.SavingRecordID(), warranty.Months)
//	    return err
//	})
type RecordSaving interface {
	SavingRecord() string // "product" or "customer"
	SavingRecordID() uuid.UUID
	SavingCreated() bool
	SavingExtension(key string) (json.RawMessage, bool)
	SavingTx() *sql.Tx
}

// Extension decodes this plugin's part of a save's extensions into v - false when the save carried none for it, null
// included. A part that doesn't decode is the admin's mistake, so it comes back as a UserError they read
func (pl *Plugin) Extension(save RecordSaving, v any) (bool, error) {
	raw, ok := save.SavingExtension(pl.slug)
	if !ok || string(bytes.TrimSpace(raw)) == "null" {
		return false, nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return true, Invalid("%s's part of the save doesn't read: %v", pl.slug, err)
	}
	return true, nil
}
