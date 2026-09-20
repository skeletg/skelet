// Package subscription implements recurring billing - a kind=subscription product starts a Subscription when purchased, and a background scheduler advances each through billing periods, minting one append-only Invoice per elapsed period
// This engine owns the recurrence - scheduling, period math, lifecycle, idempotency - not the charge: the RenewalCharger seam does the collection, and without it invoices just stay 'due'
package subscription

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Status values mirror the CHECK constraint on subscriptions.status
const (
	StatusActive   = "active"
	StatusPaused   = "paused"
	StatusCanceled = "canceled"
	StatusExpired  = "expired"
)

const (
	InvoiceDue  = "due"
	InvoicePaid = "paid"
	InvoiceVoid = "void"
	// Marks a subscription whose renewal charge failed and is inside the dunning retry window
	StatusPastDue = "past_due"
)

// One customer's recurring commitment to a product - billing terms are snapshotted at purchase, so later edits to the product's terms don't retroactively change it
type Subscription struct {
	ID                 uuid.UUID
	CustomerID         *uuid.UUID // nil for guest purchases
	ProductID          uuid.UUID
	OrderID            *uuid.UUID // originating order
	Status             string
	IntervalUnit       string
	IntervalCount      int32
	Price              int64 // per-cycle charge, cents
	CurrencyCode       string
	Quantity           float64
	SKU                string
	Title              string
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	NextBillingAt      time.Time
	// The saved payment method renewals charge, customer-chosen - nil means renewals can't collect and dun immediately
	PaymentMethodID *uuid.UUID
	CanceledAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	// The owning channel, from the originating order - nil on rows whose order or customer link is gone, which then need an unscoped orders grant to administer
	ChannelID *uuid.UUID
}

type Invoice struct {
	ID             uuid.UUID
	SubscriptionID uuid.UUID
	PeriodStart    time.Time
	PeriodEnd      time.Time
	Amount         int64
	CurrencyCode   string
	Status         string
	Sequence       int32
	CreatedAt      time.Time
	// Dunning bookkeeping - attempts so far, when the next charge runs (nil for never attempted, or parked after exhausting retries), the last gateway failure, and payment proof once collected
	AttemptCount     int32
	NextAttemptAt    *time.Time
	FailureReason    string
	PaidAt           *time.Time
	PaymentReference string
}

// Creates a Subscription - period fields are computed by the caller, the OrderPlaced subscriber, from the terms plus purchase time
type StartInput struct {
	CustomerID         *uuid.UUID
	ProductID          uuid.UUID
	OrderID            *uuid.UUID
	IntervalUnit       string
	IntervalCount      int32
	Price              int64
	CurrencyCode       string
	Quantity           float64
	SKU                string
	Title              string
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	NextBillingAt      time.Time
	ChannelID          *uuid.UUID
}

// The persistence boundary - WithTx rebinds it to a transaction so the scheduler can lock and advance a subscription atomically
type Repository interface {
	WithTx(tx *sql.Tx) Repository

	Create(ctx context.Context, in StartInput) (Subscription, error)
	GetByID(ctx context.Context, id uuid.UUID) (Subscription, error)
	GetForUpdate(ctx context.Context, id uuid.UUID) (Subscription, error)
	ListByOrder(ctx context.Context, orderID uuid.UUID) ([]Subscription, error)
	ListByCustomer(ctx context.Context, customerID uuid.UUID) ([]Subscription, error)
	List(ctx context.Context, status string, limit, offset int32) ([]Subscription, error)
	ListDue(ctx context.Context, now time.Time, limit int32) ([]Subscription, error)
	AdvancePeriod(ctx context.Context, id uuid.UUID, start, end, next time.Time) (Subscription, error)
	SetStatus(ctx context.Context, id uuid.UUID, status string, canceledAt *time.Time) (Subscription, error)

	CreateInvoice(ctx context.Context, subID uuid.UUID, periodStart, periodEnd time.Time, amount int64, currency string, sequence int32) (bool, error)
	CountInvoices(ctx context.Context, subID uuid.UUID) (int64, error)
	ListInvoices(ctx context.Context, subID uuid.UUID) ([]Invoice, error)
	ListChargeableInvoices(ctx context.Context, now time.Time, limit int32) ([]Invoice, error)
	MarkInvoicePaid(ctx context.Context, id uuid.UUID, reference string) error
	RecordInvoiceAttempt(ctx context.Context, id uuid.UUID, nextAttemptAt *time.Time, failureReason string) (Invoice, error)
	SetPaymentMethod(ctx context.Context, id uuid.UUID, paymentMethodID *uuid.UUID) (Subscription, error)
}

var (
	ErrNotFound  = errors.New("subscription not found")
	ErrNotOwned  = errors.New("subscription does not belong to this customer")
	ErrNotActive = errors.New("subscription is not active")
)
