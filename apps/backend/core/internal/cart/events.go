package cart

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/syncveto"
)

type cartLineResponse struct { // shared sync-webhook response for the "-ing" events - reject/veto plus the price and quantity overrides the in-process seams offer, and across multiple webhooks the last writer wins as in the plugin handler chain
	syncveto.Response
	UnitPriceCents *int64   `json:"unitPriceCents"`
	Quantity       *float64 `json:"quantity"`
}

func parseCartLineResponse(body []byte) (cartLineResponse, error) {
	var resp cartLineResponse
	if len(body) == 0 {
		return resp, nil
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return resp, err
	}
	if resp.UnitPriceCents != nil && *resp.UnitPriceCents < 0 {
		return resp, fmt.Errorf("webhook response unitPriceCents must be >= 0, got %d", *resp.UnitPriceCents)
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
)

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
	}
	if resp.Quantity != nil {
		e.Quantity = resp.Quantity
	}
	return false, "", nil
}

type LineAdded struct { // fires AFTER the upsert, async
	CartID           uuid.UUID
	ChannelID        uuid.UUID
	CustomerID       *uuid.UUID
	AnonID           string // visitor's lens_aid (guest attribution); empty when unknown
	LineID           uuid.UUID
	ProductSku       string
	UnitPrice        int64
	Quantity         float64
	AnalyticsRefused bool // the shopper refused analytics on the banner, so nothing may profile them from this
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
	return false, "", nil
}

type BundleAdded struct { // fires AFTER the upsert, async
	CartID           uuid.UUID
	ChannelID        uuid.UUID
	CustomerID       *uuid.UUID
	AnonID           string
	LineID           uuid.UUID
	ProductSku       string
	UnitPrice        int64
	Quantity         float64
	AnalyticsRefused bool // the shopper refused analytics on the banner, so nothing may profile them from this
}

func (*BundleAdded) Type() string                { return EventBundleAdded }
func (e *BundleAdded) EventChannelID() uuid.UUID { return e.ChannelID }

type LineUpdating struct { // fires BEFORE a line's quantity changes, sync - a plugin can reject or override the new quantity
	CartID     uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	LineID     uuid.UUID
	ProductSku string              // the line's SKU, so a stock guard can resolve the product; empty when the emitter couldn't hydrate it
	Quantity   *float64            // mutable; absolute new line quantity
	StockParts []CartLineStockPart `json:"-"`
}

func (*LineUpdating) Type() string                { return EventLineUpdating }
func (e *LineUpdating) EventChannelID() uuid.UUID { return e.ChannelID }

func (e *LineUpdating) ApplyWebhookResponse(body []byte) (bool, string, error) { // response {reject, reason, quantity} - quantity overrides the absolute new line quantity
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
	return false, "", nil
}

type LineUpdated struct { // fires AFTER the quantity is written, async
	CartID           uuid.UUID
	ChannelID        uuid.UUID
	CustomerID       *uuid.UUID
	AnonID           string
	LineID           uuid.UUID
	Quantity         float64
	AnalyticsRefused bool // the shopper refused analytics on the banner, so nothing may profile them from this
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
	CartID           uuid.UUID
	ChannelID        uuid.UUID
	CustomerID       *uuid.UUID
	AnonID           string
	LineID           uuid.UUID
	AnalyticsRefused bool // the shopper refused analytics on the banner, so nothing may profile them from this
}

func (*LineRemoved) Type() string                { return EventLineRemoved }
func (e *LineRemoved) EventChannelID() uuid.UUID { return e.ChannelID }

type Cleared struct { // fires AFTER all lines are removed, async
	CartID           uuid.UUID
	ChannelID        uuid.UUID
	CustomerID       *uuid.UUID
	AnonID           string
	AnalyticsRefused bool // the shopper refused analytics on the banner, so nothing may profile them from this
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
	CustomerCartID   uuid.UUID
	GuestCartID      uuid.UUID
	CustomerID       uuid.UUID
	ChannelID        uuid.UUID
	AnalyticsRefused bool // the shopper refused analytics on the banner, so nothing may profile them from this
}

func (*Merged) Type() string                { return EventMerged }
func (e *Merged) EventChannelID() uuid.UUID { return e.ChannelID }
