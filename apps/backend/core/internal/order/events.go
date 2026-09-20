package order

import (
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
)

const (
	EventCheckoutValidating   = "checkout.validating"
	EventPlacing              = "order.placing"
	EventTendering            = "order.tendering"
	EventPlaced               = "order.placed"
	EventCancelling           = "order.cancelling"
	EventCancelled            = "order.cancelled"
	EventRefunded             = "order.refunded"
	EventFulfilling           = "order.fulfilling"
	EventFulfilled            = "order.fulfilled"
	EventFulfillmentCreated   = "order.fulfillment_created"
	EventStatusChanged        = "order.status_changed"
	EventPaymentStatusChanged = "order.payment_status_changed"
	EventReturnReceived       = "order.return_received"
	EventReturnRequested      = "order.return_requested"
	EventReturnApproved       = "order.return_approved"
	EventReturnRejected       = "order.return_rejected"
	EventEdited               = "order.edited"
)

// Fires BEFORE the placement transaction opens - the pre-transaction twin of order.placing, same veto power but no transaction cost, so a slow external validator delays the shopper without pinning a DB transaction Prefer it for fraud, address and B2B gates, and use order.placing only when the handler must write inside the placement tx Mutations seed the OrderPlacing that follows: DiscountCents accumulates, RiskScore takes the max, Tags append
type CheckoutValidating struct {
	OrderID    uuid.UUID
	CustomerID *uuid.UUID
	Email      string
	ChannelID  uuid.UUID
	Cart       cart.Cart
	Subtotal   int64
	// EngineDiscountCents is the promotion engine's discount, informational so validators see realistic totals
	EngineDiscountCents int64
	PaymentProvider     string
	BillingAddress      OrderAddress
	ShippingAddress     OrderAddress
	DiscountCents       int64
	RiskScore           int
	Tags                []string
}

func (*CheckoutValidating) Type() string { return EventCheckoutValidating }

func (e *CheckoutValidating) EventChannelID() uuid.UUID { return e.ChannelID }

func (e *CheckoutValidating) ApplyWebhookResponse(body []byte) (bool, string, error) {
	if len(body) == 0 {
		return false, "", nil
	}
	var resp struct {
		Reject        bool     `json:"reject"`
		Reason        string   `json:"reason"`
		DiscountCents int64    `json:"discountCents"`
		RiskScore     int      `json:"riskScore"`
		Tags          []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	e.DiscountCents += resp.DiscountCents
	if resp.RiskScore > e.RiskScore {
		e.RiskScore = resp.RiskScore
	}
	if len(resp.Tags) > 0 {
		e.Tags = append(e.Tags, resp.Tags...)
	}
	return false, "", nil
}

// Fires INSIDE the placement transaction before order rows are written - sync handlers may abort or enrich, and DB writers must join through the Tx handle so their writes commit or roll back with the order
// Mutation contract: DiscountCents, RiskScore and Tags are by-value additive, each subscriber adding on top, while TaxCents, ShippingCents and TotalCents are pointers where nil means the core default and a plugin allocates and writes through to override
type OrderPlacing struct {
	// OrderID is pre-allocated so listeners can link work (inventory reservations) before the order row exists
	OrderID uuid.UUID
	// The placement transaction - DB writers join with WithTx(e.Tx) so a failed placement rolls their writes back - Nil only outside a tx, which no core emitter does - JSON-excluded
	Tx *sql.Tx `json:"-"`
	// CustomerID nil for guests, Email always present (the receipt address)
	CustomerID    *uuid.UUID
	Email         string
	ChannelID     uuid.UUID
	Cart          cart.Cart
	Subtotal      int64
	DiscountCents int64
	RiskScore     int
	Tags          []string
	// Pointer seams - plugins write through to override core's defaults (0 for tax/shipping, subtotal-discount for total). nil = core default
	TaxCents      *int64
	ShippingCents *int64
	TotalCents    *int64
}

func (*OrderPlacing) Type() string { return EventPlacing }

func (o *OrderPlacing) EventChannelID() uuid.UUID { return o.ChannelID }

// Fires inside the placement tx after core's own tenders, with RemainingCents the balance still owed. Fires for every placement carrying one - match on Provider to decide if it's yours. Returning an error rolls the placement back; sdk.UserError tells the shopper why. Not a mutation seam: the totals are already written, so record against the order rather than reprice it
type OrderTendering struct {
	// The placement transaction - a subscriber's writes commit or roll back with the order - JSON-excluded
	Tx *sql.Tx `json:"-"`

	OrderID     uuid.UUID
	OrderNumber string
	ChannelID   uuid.UUID
	// nil for a guest checkout - a tender keyed on who is buying must refuse rather than assume
	CustomerID   *uuid.UUID
	Email        string
	Provider     string
	CurrencyCode string
	TotalCents   int64
	// What is still owed after gift cards and store credit, always > 0 - core doesn't publish for a
	// fully covered order, since there is nothing left to tender
	RemainingCents int64
}

func (*OrderTendering) Type() string { return EventTendering }

func (e *OrderTendering) EventChannelID() uuid.UUID { return e.ChannelID }

// Accessors, because a plugin reaches this through sdk.SubscribeSync's `any` and cannot name the concrete type - a structural interface assertion sees METHODS, never fields, so the fields above are for core and these are the plugin contract (sdk.Tendering). Renaming one is a breaking SDK change
func (e *OrderTendering) PlacementTx() *sql.Tx         { return e.Tx }
func (e *OrderTendering) TenderOrderID() uuid.UUID     { return e.OrderID }
func (e *OrderTendering) TenderOrderNumber() string    { return e.OrderNumber }
func (e *OrderTendering) TenderChannelID() uuid.UUID   { return e.ChannelID }
func (e *OrderTendering) TenderCustomerID() *uuid.UUID { return e.CustomerID }
func (e *OrderTendering) TenderEmail() string          { return e.Email }
func (e *OrderTendering) TenderProvider() string       { return e.Provider }
func (e *OrderTendering) TenderCurrency() string       { return e.CurrencyCode }
func (e *OrderTendering) TenderTotalCents() int64      { return e.TotalCents }
func (e *OrderTendering) TenderRemainingCents() int64  { return e.RemainingCents }

// Response {reject, reason, discountCents, riskScore, tags} - across multiple webhooks discountCents accumulates, riskScore takes the max, tags append
// Tax, shipping and total are NOT settable here, since the remote providers and pointer seams own them and there should be one override path each - An empty body accepts, malformed JSON aborts
func (o *OrderPlacing) ApplyWebhookResponse(body []byte) (bool, string, error) {
	if len(body) == 0 {
		return false, "", nil
	}
	var resp struct {
		Reject        bool     `json:"reject"`
		Reason        string   `json:"reason"`
		DiscountCents int64    `json:"discountCents"`
		RiskScore     int      `json:"riskScore"`
		Tags          []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	o.DiscountCents += resp.DiscountCents
	if resp.RiskScore > o.RiskScore {
		o.RiskScore = resp.RiskScore
	}
	if len(resp.Tags) > 0 {
		o.Tags = append(o.Tags, resp.Tags...)
	}
	return false, "", nil
}

// Fires AFTER the placement transaction commits, async - with the outbox wired it's enqueued in the tx and delivered at-least-once, so subscribers must tolerate replays; without it, best-effort fire-and-forget
type OrderPlaced struct {
	OrderID     uuid.UUID
	OrderNumber string
	// CustomerID nil for guests, Email always present (email subscribers use Email, not CustomerID)
	CustomerID *uuid.UUID
	Email      string
	// AnonID is the visitor's lens_aid - lets a GUEST order attach to their browsing on the Synapse profile - Empty for admin/draft
	AnonID     string
	ChannelID  uuid.UUID
	TotalCents int64
	Currency   string
	Tags       []string
}

func (*OrderPlaced) Type() string { return EventPlaced }

func (o *OrderPlaced) EventChannelID() uuid.UUID { return o.ChannelID }

// OrderStatusChanged fires AFTER any non-placement status transition commits - Async - Inspect FromStatus + ToStatus
type OrderStatusChanged struct {
	OrderID    uuid.UUID
	ChannelID  uuid.UUID
	FromStatus string
	ToStatus   string
}

func (*OrderStatusChanged) Type() string                { return EventStatusChanged }
func (o *OrderStatusChanged) EventChannelID() uuid.UUID { return o.ChannelID }

// OrderPaymentStatusChanged fires AFTER payment_status advances, driven by gateway events - Async
type OrderPaymentStatusChanged struct {
	OrderID   uuid.UUID
	ChannelID uuid.UUID
	Status    string
	Reference string
}

func (*OrderPaymentStatusChanged) Type() string                { return EventPaymentStatusChanged }
func (o *OrderPaymentStatusChanged) EventChannelID() uuid.UUID { return o.ChannelID }

// Fires BEFORE a cancel commits - sync handlers may abort or enrich the audit note - Response {reject, reason, note}
type OrderCancelling struct {
	OrderID    uuid.UUID
	ChannelID  uuid.UUID
	FromStatus string
	Note       string
}

func (*OrderCancelling) Type() string                { return EventCancelling }
func (o *OrderCancelling) EventChannelID() uuid.UUID { return o.ChannelID }

func (o *OrderCancelling) ApplyWebhookResponse(body []byte) (bool, string, error) {
	if len(body) == 0 {
		return false, "", nil
	}
	var resp struct {
		Reject bool   `json:"reject"`
		Reason string `json:"reason"`
		Note   string `json:"note"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	if resp.Note != "" {
		o.Note = strings.TrimSpace(o.Note + " " + resp.Note)
	}
	return false, "", nil
}

// OrderCancelled fires after a cancel commits - the specific alternative to filtering OrderStatusChanged
type OrderCancelled struct {
	OrderID    uuid.UUID
	ChannelID  uuid.UUID
	FromStatus string
	// The share of the order that was NOT shipped, 0..1. Cancelling a partially-fulfilled order must not restore every tender in full, or a
	// customer could keep the shipped half and get all their money back - each compensating subsystem scales its reversal by this instead
	RefundableFraction float64
}

func (*OrderCancelled) Type() string                { return EventCancelled }
func (o *OrderCancelled) EventChannelID() uuid.UUID { return o.ChannelID }

// Carries REWARD reversals only, unlike OrderCancelled - a refund already hands the money back, so restoring the tenders it consumed would pay the customer twice
type OrderRefunded struct {
	OrderID      uuid.UUID
	ChannelID    uuid.UUID
	CreditMemoID uuid.UUID
	// Cumulative merchandise share across every memo, 0..1 - subsystems revoke toward it, so partial refunds add up and a replay is a no-op
	MerchandiseFraction float64
	FullyRefunded       bool // all-or-nothing reversals (a referral payout) key off this rather than the fraction
}

func (*OrderRefunded) Type() string                { return EventRefunded }
func (o *OrderRefunded) EventChannelID() uuid.UUID { return o.ChannelID }

// Fires BEFORE a fulfilled transition commits - sync handlers may abort or enrich carrier and tracking - Response {reject, reason, carrier, trackingNumber}
type OrderFulfilling struct {
	OrderID        uuid.UUID
	ChannelID      uuid.UUID
	FromStatus     string
	Carrier        string
	TrackingNumber string
}

func (*OrderFulfilling) Type() string                { return EventFulfilling }
func (o *OrderFulfilling) EventChannelID() uuid.UUID { return o.ChannelID }

func (o *OrderFulfilling) ApplyWebhookResponse(body []byte) (bool, string, error) {
	if len(body) == 0 {
		return false, "", nil
	}
	var resp struct {
		Reject         bool   `json:"reject"`
		Reason         string `json:"reason"`
		Carrier        string `json:"carrier"`
		TrackingNumber string `json:"trackingNumber"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	if resp.Carrier != "" {
		o.Carrier = resp.Carrier
	}
	if resp.TrackingNumber != "" {
		o.TrackingNumber = resp.TrackingNumber
	}
	return false, "", nil
}

// OrderFulfilled fires after a fulfilled-status transition commits
type OrderFulfilled struct {
	OrderID    uuid.UUID
	ChannelID  uuid.UUID
	FromStatus string
}

func (*OrderFulfilled) Type() string                { return EventFulfilled }
func (o *OrderFulfilled) EventChannelID() uuid.UUID { return o.ChannelID }

// One shipped (order line, quantity) on OrderFulfillmentCreated - enough for inventory to finalize per line without importing the order line model
type FulfillmentLineQty struct {
	OrderLineID uuid.UUID
	Quantity    float64
}

// Fires post-commit through the outbox when a shipment is recorded - inventory decrements the shipped quantities, and the payment bridge captures on the first
type OrderFulfillmentCreated struct {
	OrderID       uuid.UUID
	ChannelID     uuid.UUID
	FulfillmentID uuid.UUID
	Lines         []FulfillmentLineQty
	FullyShipped  bool
}

func (*OrderFulfillmentCreated) Type() string                { return EventFulfillmentCreated }
func (o *OrderFulfillmentCreated) EventChannelID() uuid.UUID { return o.ChannelID }

// ReturnLineQty is one returned (order line, quantity) on OrderReturnReceived, enough for inventory to restock per line
type ReturnLineQty struct {
	OrderLineID uuid.UUID
	Quantity    float64
}

// Fires post-commit through the outbox when a restock-flagged return's goods are received, so inventory adds the quantities back - non-restock returns don't emit it
type OrderReturnReceived struct {
	OrderID   uuid.UUID
	ChannelID uuid.UUID
	ReturnID  uuid.UUID
	Lines     []ReturnLineQty
}

func (*OrderReturnReceived) Type() string                { return EventReturnReceived }
func (o *OrderReturnReceived) EventChannelID() uuid.UUID { return o.ChannelID }

// Fires post-commit after a pre-fulfillment edit changes lines and totals - it carries the old and new amount owed so subscribers can react to payment drift
type OrderEdited struct {
	OrderID      uuid.UUID
	ChannelID    uuid.UUID
	OldAmountDue int64
	NewAmountDue int64
}

func (*OrderEdited) Type() string                { return EventEdited }
func (o *OrderEdited) EventChannelID() uuid.UUID { return o.ChannelID }

// OrderReturnRequested / Approved / Rejected fire post-commit through the early RMA lifecycle - unlike OrderReturnReceived they carry no quantities, being timeline signals rather than inventory triggers
type OrderReturnRequested struct {
	OrderID   uuid.UUID
	ChannelID uuid.UUID
	ReturnID  uuid.UUID
}

func (*OrderReturnRequested) Type() string                { return EventReturnRequested }
func (o *OrderReturnRequested) EventChannelID() uuid.UUID { return o.ChannelID }

type OrderReturnApproved struct {
	OrderID   uuid.UUID
	ChannelID uuid.UUID
	ReturnID  uuid.UUID
}

func (*OrderReturnApproved) Type() string                { return EventReturnApproved }
func (o *OrderReturnApproved) EventChannelID() uuid.UUID { return o.ChannelID }

type OrderReturnRejected struct {
	OrderID   uuid.UUID
	ChannelID uuid.UUID
	ReturnID  uuid.UUID
}

func (*OrderReturnRejected) Type() string                { return EventReturnRejected }
func (o *OrderReturnRejected) EventChannelID() uuid.UUID { return o.ChannelID }
