package cart

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"

	"github.com/skeletg/skelet/apps/backend/core/internal/syncveto"
)

// hookPrice is how a hook prices a line through its event. Whoever runs a handler names itself first (ActingAs - the
// plugin host and the webhook dispatcher do), and a price set through the setter or a webhook's answer records who
// was acting. A price a hook sets is a set price: it holds, is for the quantity it priced, and coupons and promotions
// leave it alone unless the hook says they apply
type hookPrice struct {
	actor           string
	priced          bool
	pricedBy        string
	promotionsApply bool
}

// ActingAs names who the next handler is, "plugin:<slug>" or "webhook:<name>" - empty once it has run
func (h *hookPrice) ActingAs(actor string) { h.actor = actor }

func (h *hookPrice) setPriced() { h.priced, h.pricedBy = true, h.actor }

// The price the hook leaves on a line, once the event's price is known - ok is false when no hook moved it off what
// core would have charged
func (h *hookPrice) linePrice(unit, coreUnit int64) (LinePrice, bool) {
	if !h.priced && unit == coreUnit {
		return LinePrice{}, false
	}
	return LinePrice{UnitPrice: unit, Source: PriceHook, Ref: h.pricedBy, ListUnitPrice: &coreUnit, NoPromotions: !h.promotionsApply}, true
}

type cartLineResponse struct { // shared sync-webhook response for the "-ing" events - reject/veto plus the price and quantity overrides the in-process seams offer, and across multiple webhooks the last writer wins as in the plugin handler chain
	syncveto.Response
	UnitPriceCents *int64   `json:"unitPriceCents"`
	Quantity       *float64 `json:"quantity"`
	// Lets coupons and promotions lower a line whose price this answer set - they don't unless it says so
	PromotionsApply *bool `json:"promotionsApply"`
}

func parseCartLineResponse(body []byte) (cartLineResponse, error) {
	var resp cartLineResponse
	if len(body) == 0 {
		return resp, nil
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return resp, err
	}
	if resp.UnitPriceCents != nil && (*resp.UnitPriceCents < 0 || *resp.UnitPriceCents > MaxLinePriceCents) {
		return resp, fmt.Errorf("webhook response unitPriceCents must be from 0 to %d, got %d", int64(MaxLinePriceCents), *resp.UnitPriceCents)
	}
	if resp.Quantity != nil && *resp.Quantity <= 0 {
		return resp, fmt.Errorf("webhook response quantity must be > 0, got %v", *resp.Quantity)
	}
	return resp, nil
}

const ( // subscribe by these, e.g. bus.SubscribeSync(cart.EventLineAdding, 100, stockGuard)
	EventLineAdding   = "cart.line.adding"
	EventLineAdded    = "cart.line.added"
	EventLineUpdating = "cart.line.updating"
	EventLineUpdated  = "cart.line.updated"
	EventLineRemoving = "cart.line.removing"
	EventLineRemoved  = "cart.line.removed"
	EventCleared      = "cart.cleared"
	EventMerging      = "cart.merging"
	EventMerged       = "cart.merged"
	EventBundleAdding = "cart.bundle.adding"
	EventBundleAdded  = "cart.bundle.added"
	EventOfferSent    = "cart.offer.sent"
	EventOfferOpened  = "cart.offer.opened"
	EventOfferExpired = "cart.offer.expired"
	// A shopper's checkout step picked a different delivery or payment method
	EventCheckoutChosen = "cart.checkout.chosen"
)

type CheckoutChosen struct { // fires AFTER the step is saved, async - at least one of Shipping and Payment is set
	CartID     uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	AnonID     string
	Shipping   string // the shipping methods it changed to, comma-joined per delivery group - empty when unchanged
	Payment    string // the payment provider's slug it changed to - empty when unchanged
	Lines      []CartLine
	Shopper    *auth.Shopper // the visitor as their consent stood, nil for a caller with no banner
}

func (*CheckoutChosen) Type() string                { return EventCheckoutChosen }
func (e *CheckoutChosen) EventChannelID() uuid.UUID { return e.ChannelID }

type OfferSent struct { // fires AFTER an admin emails an offer, async
	CartID     uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	Email      string
	ExpiresAt  time.Time
}

func (*OfferSent) Type() string                { return EventOfferSent }
func (e *OfferSent) EventChannelID() uuid.UUID { return e.ChannelID }

type OfferOpened struct { // fires AFTER an offer link is first followed by its recipient, async
	CartID     uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
}

func (*OfferOpened) Type() string                { return EventOfferOpened }
func (e *OfferOpened) EventChannelID() uuid.UUID { return e.ChannelID }

type OfferExpired struct { // fires once, AFTER an offer runs out unfinished, async
	CartID     uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	Email      string
	ExpiresAt  time.Time
}

func (*OfferExpired) Type() string                { return EventOfferExpired }
func (e *OfferExpired) EventChannelID() uuid.UUID { return e.ChannelID }

type LineAdding struct { // fires BEFORE a simple line is upserted - sync handlers abort with a non-nil error or enrich through the pointer fields, which the service reads back before hitting the repository
	CartID      uuid.UUID
	ChannelID   uuid.UUID
	CustomerID  *uuid.UUID
	ProductSku  string
	ProductName string
	UnitPrice   *int64   // mutable: nil = "use input price"
	Quantity    *float64 // mutable: nil = "use input quantity"
	// Linked option sources the line also takes, per line unit, so the stock guard can check them
	StockParts []CartLineStockPart `json:"-"`
	hookPrice
}

func (*LineAdding) Type() string                { return EventLineAdding }
func (e *LineAdding) EventChannelID() uuid.UUID { return e.ChannelID }

func (e *LineAdding) ApplyWebhookResponse(body []byte) (bool, string, error) { // response {reject, reason, unitPriceCents, quantity} - the two overrides replace the line being added, omitting them leaves the input alone
	resp, err := parseCartLineResponse(body)
	if err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	if resp.UnitPriceCents != nil {
		e.UnitPrice = resp.UnitPriceCents
		e.setPriced()
	}
	if resp.Quantity != nil {
		e.Quantity = resp.Quantity
	}
	if resp.PromotionsApply != nil {
		e.promotionsApply = *resp.PromotionsApply
	}
	return false, "", nil
}

type LineAdded struct { // fires AFTER the upsert, async
	CartID     uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	AnonID     string // visitor's lens_aid (guest attribution); empty when unknown
	LineID     uuid.UUID
	ProductSku string
	UnitPrice  int64
	Quantity   float64
	Shopper    *auth.Shopper // the visitor as their consent stood, nil for a caller with no banner
}

func (*LineAdded) Type() string                { return EventLineAdded }
func (e *LineAdded) EventChannelID() uuid.UUID { return e.ChannelID }

type BundleAdding struct { // fires BEFORE a bundle line is upserted, like LineAdding, so a plugin can reject a composition or override the bundle unit price
	CartID      uuid.UUID
	ChannelID   uuid.UUID
	CustomerID  *uuid.UUID
	ProductSku  string
	ProductName string
	UnitPrice   *int64   // mutable: bundle's summed component price; plugin can override
	Quantity    *float64 // mutable
	// The components the bundle takes, per bundle unit, so the stock guard can check them - the bundle itself holds no stock
	StockParts []CartLineStockPart `json:"-"`
	// A hook that prices the bundle may let coupons and promotions lower it - they don't unless it says so
	promotionsApply bool
}

func (*BundleAdding) Type() string                { return EventBundleAdding }
func (e *BundleAdding) EventChannelID() uuid.UUID { return e.ChannelID }

func (e *BundleAdding) ApplyWebhookResponse(body []byte) (bool, string, error) { // same contract as LineAdding, unitPriceCents overrides the bundle price
	resp, err := parseCartLineResponse(body)
	if err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	if resp.UnitPriceCents != nil {
		e.UnitPrice = resp.UnitPriceCents
	}
	if resp.Quantity != nil {
		e.Quantity = resp.Quantity
	}
	if resp.PromotionsApply != nil {
		e.promotionsApply = *resp.PromotionsApply
	}
	return false, "", nil
}

type BundleAdded struct { // fires AFTER the upsert, async
	CartID     uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	AnonID     string
	LineID     uuid.UUID
	ProductSku string
	UnitPrice  int64
	Quantity   float64
	Shopper    *auth.Shopper // the visitor as their consent stood, nil for a caller with no banner
}

func (*BundleAdded) Type() string                { return EventBundleAdded }
func (e *BundleAdded) EventChannelID() uuid.UUID { return e.ChannelID }

type LineUpdating struct { // fires BEFORE a line's quantity changes, sync - a plugin can reject, override the new quantity, or price the line at it
	CartID     uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	LineID     uuid.UUID
	ProductSku string   // the line's SKU, so a stock guard can resolve the product; empty when the emitter couldn't hydrate it
	Quantity   *float64 // mutable; absolute new line quantity
	// mutable: the unit price the line will have at the new quantity - a hook that sets it prices the line, as on an add
	UnitPrice  *int64
	StockParts []CartLineStockPart `json:"-"`
	hookPrice
}

func (*LineUpdating) Type() string                { return EventLineUpdating }
func (e *LineUpdating) EventChannelID() uuid.UUID { return e.ChannelID }

func (e *LineUpdating) ApplyWebhookResponse(body []byte) (bool, string, error) { // response {reject, reason, quantity, unitPriceCents, promotionsApply} - quantity overrides the absolute new line quantity, unitPriceCents prices the line at it
	resp, err := parseCartLineResponse(body)
	if err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	if resp.Quantity != nil {
		e.Quantity = resp.Quantity
	}
	if resp.UnitPriceCents != nil {
		e.UnitPrice = resp.UnitPriceCents
		e.setPriced()
	}
	if resp.PromotionsApply != nil {
		e.promotionsApply = *resp.PromotionsApply
	}
	return false, "", nil
}

type LineUpdated struct { // fires AFTER the quantity is written, async
	CartID       uuid.UUID
	ChannelID    uuid.UUID
	CustomerID   *uuid.UUID
	AnonID       string
	LineID       uuid.UUID
	ProductSku   string
	UnitPrice    int64 // after the edit, which may have crossed a price tier
	Quantity     float64
	PrevQuantity float64
	Shopper      *auth.Shopper // the visitor as their consent stood, nil for a caller with no banner
}

func (*LineUpdated) Type() string                { return EventLineUpdated }
func (e *LineUpdated) EventChannelID() uuid.UUID { return e.ChannelID }

type LineRemoving struct { // fires BEFORE a line is removed - sync handlers may reject removal, e.g. a free gift locked while the cart still qualifies for a promotion
	CartID     uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	LineID     uuid.UUID
}

func (*LineRemoving) Type() string                { return EventLineRemoving }
func (e *LineRemoving) EventChannelID() uuid.UUID { return e.ChannelID }

type LineRemoved struct { // fires AFTER the removal, async
	CartID     uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	AnonID     string
	LineID     uuid.UUID
	Shopper    *auth.Shopper // the visitor as their consent stood, nil for a caller with no banner
	ProductSku string        // what the line held, for the platforms that report a removal by item
	UnitPrice  int64
	Quantity   float64
}

func (*LineRemoved) Type() string                { return EventLineRemoved }
func (e *LineRemoved) EventChannelID() uuid.UUID { return e.ChannelID }

type Cleared struct { // fires AFTER all lines are removed, async
	CartID     uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	AnonID     string
	Shopper    *auth.Shopper // the visitor as their consent stood, nil for a caller with no banner
}

func (*Cleared) Type() string                { return EventCleared }
func (e *Cleared) EventChannelID() uuid.UUID { return e.ChannelID }

type Merging struct { // fires BEFORE a guest cart merges into a customer cart on login, sync - a plugin can reject, e.g. when the guest cart holds items the customer can't buy
	GuestCartID    uuid.UUID
	CustomerCartID uuid.UUID
	CustomerID     uuid.UUID
	ChannelID      uuid.UUID
}

func (*Merging) Type() string                { return EventMerging }
func (e *Merging) EventChannelID() uuid.UUID { return e.ChannelID }

type Merged struct { // fires AFTER the merge commits, async
	CustomerCartID uuid.UUID
	GuestCartID    uuid.UUID
	CustomerID     uuid.UUID
	ChannelID      uuid.UUID
	Shopper        *auth.Shopper // the visitor as their consent stood, nil for a caller with no banner
}

func (*Merged) Type() string                { return EventMerged }
func (e *Merged) EventChannelID() uuid.UUID { return e.ChannelID }
