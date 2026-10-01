// Package order is order logic - placement, lifecycle transitions, admin workflows - Orders are post-purchase records, and pre-purchase state lives in cart
package order

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// A confirmed order, prices in cents - CustomerID is nil for guests, and Email is both the receipt address and the second half of the (id, email) bearer credential for unauthenticated detail lookups
type Order struct {
	ID              uuid.UUID
	CustomerID      *uuid.UUID
	Email           string
	ChannelID       uuid.UUID
	Number          string
	Status          string
	PaymentStatus   string
	PaymentProvider string
	// The gateway's handle (a Stripe PaymentIntent id, say), set at authorize and used to capture, refund, void and correlate webhooks - empty until authorized
	PaymentReference  string
	BillingAddress    OrderAddress
	ShippingAddress   OrderAddress
	Lines             []OrderLine
	Events            []OrderEvent
	SubtotalAmount    int64
	ShippingAmount    int64
	DiscountAmount    int64
	TaxAmount         int64
	ShippingTaxAmount int64 // the part of TaxAmount charged on shipping
	TotalAmount       int64
	AnonID            string // the visitor id it was placed under - a guest order's later events reach the profile its placement did
	// TRANSIENT and never persisted - set on the placement response for a client-side 3DS step and consumed by the storefront immediately
	PaymentActionKind   string // "", "redirect" or "challenge" - what the client must do with PaymentActionSecret
	PaymentActionSecret string
	// The cents of TotalAmount covered by gift cards - it reduces what's owed to the provider, not TotalAmount, since tax is on the full price
	GiftCardAmount    int64
	StoreCreditAmount int64
	CurrencyCode      string
	CustomerNote      string
	PONumber          string
	BusinessIDs       []BusinessID // the buyer's company identifiers, empty on personal purchases
	// Stamped at fulfillment, from admin args or an OrderFulfilling subscriber, and empty otherwise
	Carrier        string
	TrackingNumber string
	// The built-in carrier's method uuid that priced the order, its re-price key - nil for external carriers, and for free or promo shipping
	ShippingMethodID *uuid.UUID
	// Record the chosen carrier and method so a pre-fulfillment edit can re-price authoritatively - nil when shipping was free or promotional
	ShippingCarrierSlug *string
	ShippingMethodCode  *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// The cents owed to the provider - taxed total minus the gift-card and store-credit tenders
func (o Order) AmountDue() int64 {
	return o.TotalAmount - o.GiftCardAmount - o.StoreCreditAmount
}

type OrderAddress struct {
	FirstName   string
	LastName    string
	Company     *string
	Street1     string
	Street2     *string
	City        string
	State       *string
	PostalCode  string
	CountryCode string
	Phone       *string
}

// BusinessID is one company identifier as the order records it - the label travels with the value, so an invoice reads
// "PIB 123456789" even after the country's fields are edited
type BusinessID struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
}

type OrderAmounts struct {
	Subtotal, Shipping, Discount, Tax, ShippingTax, Total int64
}

// One line item, prices in cents - a bundle line carries the bundle total with picks in Components, and simple lines leave it empty
type OrderLine struct {
	ID          uuid.UUID
	OrderID     uuid.UUID
	ProductName string
	ProductSku  string
	UnitPrice   int64
	Quantity    float64
	TotalPrice  int64
	// The unit COGS snapshot at placement - nil means unknown, i.e. no catalog cost at the time
	CostCents *int64
	// A promotion free gift granted by core.add_free_gift - zero-priced, stock reserved like any other line, and excluded from pro-rata refunds since nothing was paid for it
	IsGift     bool
	Components []OrderLineComponent
	// What the line took stock from besides or instead of itself, per line unit, snapshotted at placement
	StockParts []OrderLineStockPart
	Options    []OrderLineOption
	CreatedAt  time.Time
}

type OrderLineStockPart struct {
	ProductID uuid.UUID
	Quantity  float64
	Source    string // cart.StockPartKit or cart.StockPartOption
}

// One custom-option pick snapshot copied from the cart at placement - PriceDelta is already folded into the line's UnitPrice
type OrderLineOption struct {
	ID              uuid.UUID
	OrderLineID     uuid.UUID
	ProductOptionID uuid.UUID
	OptionValueID   *uuid.UUID
	InputType       string
	OptionTitle     string
	ValueLabel      string
	TextValue       string
	PriceDelta      int64
	Quantity        float64
	SortOrder       int32
	FileURL         string
	CreatedAt       time.Time
}

// One picked component under a bundle line - Quantity is the total, already multiplied by the parent quantity, and the option and selection ids are snapshots rather than FKs
type OrderLineComponent struct {
	ID                uuid.UUID
	OrderLineID       uuid.UUID
	BundleOptionID    uuid.UUID
	BundleSelectionID uuid.UUID
	ProductID         uuid.UUID
	ProductSku        string
	ProductName       string
	UnitPrice         int64
	Quantity          float64
	SortOrder         int32
	CreatedAt         time.Time
}

// One tax rule's contribution to an order - name and rate are snapshots, so any rule shape lands here without the
// schema knowing about it and the record survives the rule being edited or deleted
type TaxLine struct {
	ID          uuid.UUID
	RuleID      *uuid.UUID
	Name        string
	RateBPS     int32
	AmountCents int64
}

type OrderEvent struct {
	ID         uuid.UUID
	OrderID    uuid.UUID
	Type       string
	Message    string
	ActorEmail string
	CreatedAt  time.Time
}

// ListPageFilter scopes the admin orders page - empty ChannelIDs means every channel (superuser), matching the customers pager
type ListPageFilter struct {
	ChannelIDs      []uuid.UUID
	Search          string
	Statuses        []string
	PaymentStatuses []string
	Limit           int32
	Offset          int32
	SkipCount       bool // the total is left 0, for callers that never show it
}

type Repository interface {
	CreateOrder(ctx context.Context, o Order) (Order, error)
	AddOrderLine(ctx context.Context, line OrderLine) (OrderLine, error)
	AddOrderLineComponent(ctx context.Context, component OrderLineComponent) (OrderLineComponent, error)
	AddOrderLineStockParts(ctx context.Context, orderLineID uuid.UUID, parts []OrderLineStockPart) error
	AddOrderLineOption(ctx context.Context, option OrderLineOption) (OrderLineOption, error)
	AddOrderEvent(ctx context.Context, event OrderEvent) (OrderEvent, error)
	GetOrderByID(ctx context.Context, id uuid.UUID) (Order, error)
	ListOrderLinesByOrder(ctx context.Context, orderID uuid.UUID) ([]OrderLine, error)
	ListOrderEventsByOrder(ctx context.Context, orderID uuid.UUID) ([]OrderEvent, error)
	ListOrdersPage(ctx context.Context, f ListPageFilter) ([]Order, int64, error)
	ListOrdersByCustomer(ctx context.Context, customerID uuid.UUID) ([]Order, error)
	GetGuestOrder(ctx context.Context, id uuid.UUID, email string) (Order, error)
	// Attaches guest orders on channelID under email to customerID and returns the count, where zero is valid - so a new customer's first /account/orders visit shows pre-registration history
	ClaimGuestOrdersByEmail(ctx context.Context, customerID uuid.UUID, email string, channelID uuid.UUID) (int64, error)
	// fromStatus is the status the caller validated the transition against - no row matches when it already moved, so concurrent callers can't both commit
	UpdateOrderStatusFrom(ctx context.Context, id uuid.UUID, fromStatus, status string) (Order, error)
	UpdateOrderLineQuantity(ctx context.Context, lineID uuid.UUID, quantity float64, totalPrice int64) (OrderLine, error)
	SoftDeleteOrderLine(ctx context.Context, lineID uuid.UUID) (OrderLine, error)
	UpdateOrderAmounts(ctx context.Context, id uuid.UUID, a OrderAmounts) (Order, error)
	UpdateOrderAddresses(ctx context.Context, id uuid.UUID, billing, shipping OrderAddress) (Order, error)
	SetReturnExchangeOrder(ctx context.Context, returnID, orderID uuid.UUID) (OrderReturn, error)
	// Stamps carrier and tracking inside the fulfill tx, so status and tracking commit atomically
	UpdateOrderTracking(ctx context.Context, id uuid.UUID, carrier, trackingNumber string) (Order, error)
	SetStoreCreditTender(ctx context.Context, id uuid.UUID, storeCreditAmount int64, paymentStatus string) (Order, error)
	SetGiftCardTender(ctx context.Context, id uuid.UUID, giftCardAmount int64, paymentStatus string) (Order, error)
	SetPaymentState(ctx context.Context, id uuid.UUID, paymentStatus, paymentReference string) (Order, error)
	// SetPaymentState only while the status is still from - false when someone moved it first
	SetPaymentStateFrom(ctx context.Context, id uuid.UUID, from, paymentStatus, paymentReference string) (Order, bool, error)
	SetPaymentReference(ctx context.Context, id uuid.UUID, paymentReference string) (Order, error)
	// The payment-sync sweep's worklist - unpaid gateway orders holding a provider reference, bounded so ancient ones stop polling
	ListOrdersAwaitingPaymentSync(ctx context.Context, notBefore, olderThan time.Time, maxRows int32) ([]Order, error)

	// The payment ledger - a durable record of every authorize, capture, refund and void - It's the source of truth for amounts, and payment_status derives from the succeeded sums
	InsertPaymentTransaction(ctx context.Context, t PaymentTransaction) (PaymentTransaction, error)
	LockOrderForPayment(ctx context.Context, orderID uuid.UUID) error
	// The same row lock, for the paths that plan against the order's lines - shipping and editing
	LockOrder(ctx context.Context, orderID uuid.UUID) error
	PaymentTotalsActiveByKind(ctx context.Context, orderID uuid.UUID) (PaymentTotals, error)
	// Settles a pending row - false with the row as it stands when it was already final
	MarkPaymentTransaction(ctx context.Context, id uuid.UUID, status, providerReference string) (PaymentTransaction, bool, error)
	MarkCreditMemoFailed(ctx context.Context, id uuid.UUID) error
	ListPaymentTransactions(ctx context.Context, orderID uuid.UUID) ([]PaymentTransaction, error)
	PaymentTotalsByKind(ctx context.Context, orderID uuid.UUID) (PaymentTotals, error)
	FindPaymentTransactionByKey(ctx context.Context, idempotencyKey string) (PaymentTransaction, bool, error)

	// Partial-shipment records - ShippedByOrderLine sums non-cancelled shipped qty for the over-ship guard
	CreateFulfillment(ctx context.Context, f Fulfillment) (Fulfillment, error)
	AddFulfillmentLine(ctx context.Context, fulfillmentID uuid.UUID, line FulfillmentLine) (FulfillmentLine, error)
	ListFulfillments(ctx context.Context, orderID uuid.UUID) ([]Fulfillment, error)
	GetFulfillment(ctx context.Context, id uuid.UUID) (Fulfillment, error)
	ShippedByOrderLine(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]float64, error)
	CountFulfillments(ctx context.Context, orderID uuid.UUID) (int64, error)

	CreateShippingLabel(ctx context.Context, l ShippingLabel) (ShippingLabel, error)
	GetShippingLabel(ctx context.Context, id uuid.UUID) (ShippingLabel, error)

	// Returns / RMA, requested to approved to received to refunded, or rejected
	// ReturnedByOrderLine sums non-rejected returned qty for the "return <= shipped minus already-returned" guard
	CreateReturn(ctx context.Context, r OrderReturn) (OrderReturn, error)
	AddReturnLine(ctx context.Context, returnID uuid.UUID, line OrderReturnLine) (OrderReturnLine, error)
	GetReturn(ctx context.Context, id uuid.UUID) (OrderReturn, error)
	ListReturns(ctx context.Context, orderID uuid.UUID) ([]OrderReturn, error)
	// Moves a return from one status to another - RETURN_STATUS_RACED when it already left from
	UpdateReturnStatus(ctx context.Context, id uuid.UUID, from, status string) (OrderReturn, error)
	MarkReturnRefunded(ctx context.Context, id uuid.UUID, refundAmount int64, giftCardID *uuid.UUID) (OrderReturn, error)
	ReturnedByOrderLine(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]float64, error)

	// Tax breakdown - replaced wholesale on an edit, since a recompute invalidates the old rows
	InsertTaxLine(ctx context.Context, orderID uuid.UUID, line TaxLine) error
	ListTaxLines(ctx context.Context, orderID uuid.UUID) ([]TaxLine, error)
	DeleteTaxLines(ctx context.Context, orderID uuid.UUID) error

	// CreditedByOrderLine sums credited qty across every memo, bounding a direct memo the way shipped-minus-returned bounds an RMA
	CreateCreditMemo(ctx context.Context, m CreditMemo) (CreditMemo, error)
	AddCreditMemoLine(ctx context.Context, memoID uuid.UUID, line CreditMemoLine) (CreditMemoLine, error)
	GetCreditMemo(ctx context.Context, id uuid.UUID) (CreditMemo, error)
	GetCreditMemoByReturn(ctx context.Context, returnID uuid.UUID) (CreditMemo, bool, error)
	ListCreditMemos(ctx context.Context, orderID uuid.UUID) ([]CreditMemo, error)
	CreditedByOrderLine(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]float64, error)
	LinkPaymentTransactionToMemo(ctx context.Context, txnID, memoID uuid.UUID) error

	// Lossless hold round-trip - releasing restores the status held from instead of always confirming
	HoldOrderFrom(ctx context.Context, orderID uuid.UUID, fromStatus string) (Order, error)
	ReleaseOrderToHeldFrom(ctx context.Context, orderID uuid.UUID) (Order, error)

	WithTx(tx *sql.Tx) Repository
}

const (
	FulfillmentPending   = "pending"
	FulfillmentShipped   = "shipped"
	FulfillmentCancelled = "cancelled"
)

type Fulfillment struct {
	ID             uuid.UUID
	OrderID        uuid.UUID
	Status         string
	Carrier        string
	TrackingNumber string
	Sequence       int32
	// The declared insurance value, 0 for uninsured, passed to the carrier's label call
	InsuredValueCents int64
	ShippedAt         *time.Time
	Lines             []FulfillmentLine
	Labels            []ShippingLabel
	CreatedAt         time.Time
}

type ShippingLabel struct {
	ID             uuid.UUID
	FulfillmentID  uuid.UUID
	CarrierSlug    string
	Format         string // "pdf" / "png" / "zpl" / "txt"
	TrackingNumber string
	Doc            []byte
	CreatedAt      time.Time
}

type FulfillmentLine struct {
	ID            uuid.UUID
	FulfillmentID uuid.UUID
	OrderLineID   uuid.UUID
	Quantity      float64
}

const (
	PaymentTxnAuthorize = "authorize"
	PaymentTxnCapture   = "capture"
	PaymentTxnRefund    = "refund"
	PaymentTxnVoid      = "void"

	PaymentTxnPending   = "pending"
	PaymentTxnSucceeded = "succeeded"
	PaymentTxnFailed    = "failed"
)

type PaymentTransaction struct {
	ID                uuid.UUID
	OrderID           uuid.UUID
	Kind              string
	AmountCents       int64
	Status            string
	ProviderSlug      string
	ProviderReference string
	IdempotencyKey    string
	Reason            string
	// Links a refund row back to the return that issued it, nil for non-return actions
	ReturnID     *uuid.UUID
	CreditMemoID *uuid.UUID // every refund gets one, from an RMA or issued directly
	// Backdates the row on write, for money that landed before anyone recorded it - nil keeps NOW()
	OccurredAt *time.Time
	CreatedAt  time.Time
}

// The succeeded sums per kind - the basis for payment status and the over-capture and over-refund guards
type PaymentTotals struct {
	Authorized int64
	Captured   int64
	Refunded   int64
	Voided     int64
}

const (
	ReturnRequested = "requested"
	ReturnApproved  = "approved"
	ReturnRejected  = "rejected"
	ReturnReceived  = "received"
	ReturnRefunded  = "refunded"
)

const (
	RefundTargetOriginal = "original"
	RefundTargetGiftCard = "gift_card"
)

// An RMA - a request to send back shipped quantity, with optional restock and a refund to the original tender or a new gift card
type OrderReturn struct {
	ID                uuid.UUID
	OrderID           uuid.UUID
	RMANumber         string
	Status            string
	Reason            string
	Restock           bool
	RefundTarget      string
	RefundAmountCents int64
	// "refund" (the default) issues money after receipt, "exchange" opens a zero-priced replacement draft
	Resolution       string
	ExchangeOrderID  *uuid.UUID
	IssuedGiftCardID *uuid.UUID
	AdminID          *uuid.UUID
	Lines            []OrderReturnLine
	CreditMemoNumber string // set once the refund is issued
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// One line and quantity in a return - RefundUnitCents snapshots the discounted per-unit basis at request time, so the refund survives later order edits
type OrderReturnLine struct {
	ID              uuid.UUID
	ReturnID        uuid.UUID
	OrderLineID     uuid.UUID
	Quantity        float64
	Reason          string
	RefundUnitCents int64
}
