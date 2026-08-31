package subscription

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) WithTx(tx *sql.Tx) Repository {
	return &postgresRepository{queries: r.queries.WithTx(tx)}
}

func (r *postgresRepository) Create(ctx context.Context, in StartInput) (Subscription, error) {
	row, err := r.queries.CreateSubscription(ctx, db.CreateSubscriptionParams{
		CustomerID:         nullableUUID(in.CustomerID),
		ProductID:          in.ProductID,
		OrderID:            nullableUUID(in.OrderID),
		IntervalUnit:       in.IntervalUnit,
		IntervalCount:      in.IntervalCount,
		Price:              in.Price,
		CurrencyCode:       in.CurrencyCode,
		Quantity:           in.Quantity,
		Sku:                in.SKU,
		Title:              in.Title,
		CurrentPeriodStart: in.CurrentPeriodStart,
		CurrentPeriodEnd:   in.CurrentPeriodEnd,
		NextBillingAt:      in.NextBillingAt,
		ChannelID:          nullableUUID(in.ChannelID),
	})
	if err != nil {
		return Subscription{}, fmt.Errorf("create subscription: %w", err)
	}
	return toSubscription(row), nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (Subscription, error) {
	row, err := r.queries.GetSubscriptionByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Subscription{}, ErrNotFound
		}
		return Subscription{}, fmt.Errorf("get subscription: %w", err)
	}
	return toSubscription(row), nil
}

func (r *postgresRepository) GetForUpdate(ctx context.Context, id uuid.UUID) (Subscription, error) {
	row, err := r.queries.GetSubscriptionForUpdate(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Subscription{}, ErrNotFound
		}
		return Subscription{}, fmt.Errorf("get subscription for update: %w", err)
	}
	return toSubscription(row), nil
}

func (r *postgresRepository) ListByOrder(ctx context.Context, orderID uuid.UUID) ([]Subscription, error) {
	rows, err := r.queries.ListSubscriptionsByOrder(ctx, uuid.NullUUID{UUID: orderID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list subscriptions by order: %w", err)
	}
	return toSubscriptions(rows), nil
}

func (r *postgresRepository) ListByCustomer(ctx context.Context, customerID uuid.UUID) ([]Subscription, error) {
	rows, err := r.queries.ListSubscriptionsByCustomer(ctx, uuid.NullUUID{UUID: customerID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list subscriptions by customer: %w", err)
	}
	return toSubscriptions(rows), nil
}

func (r *postgresRepository) List(ctx context.Context, status string, limit, offset int32) ([]Subscription, error) {
	var st sql.NullString
	if status != "" {
		st = sql.NullString{String: status, Valid: true}
	}
	rows, err := r.queries.ListSubscriptions(ctx, db.ListSubscriptionsParams{
		Limit:  limit,
		Offset: offset,
		Status: st,
	})
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	return toSubscriptions(rows), nil
}

func (r *postgresRepository) ListDue(ctx context.Context, now time.Time, limit int32) ([]Subscription, error) {
	rows, err := r.queries.ListDueSubscriptions(ctx, db.ListDueSubscriptionsParams{
		NextBillingAt: now,
		Limit:         limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list due subscriptions: %w", err)
	}
	return toSubscriptions(rows), nil
}

func (r *postgresRepository) AdvancePeriod(ctx context.Context, id uuid.UUID, start, end, next time.Time) (Subscription, error) {
	row, err := r.queries.AdvanceSubscriptionPeriod(ctx, db.AdvanceSubscriptionPeriodParams{
		ID:                 id,
		CurrentPeriodStart: start,
		CurrentPeriodEnd:   end,
		NextBillingAt:      next,
	})
	if err != nil {
		return Subscription{}, fmt.Errorf("advance subscription period: %w", err)
	}
	return toSubscription(row), nil
}

func (r *postgresRepository) SetStatus(ctx context.Context, id uuid.UUID, status string, canceledAt *time.Time) (Subscription, error) {
	row, err := r.queries.SetSubscriptionStatus(ctx, db.SetSubscriptionStatusParams{
		ID:         id,
		Status:     status,
		CanceledAt: nullableTime(canceledAt),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Subscription{}, ErrNotFound
		}
		return Subscription{}, fmt.Errorf("set subscription status: %w", err)
	}
	return toSubscription(row), nil
}

// CreateInvoice inserts a period's invoice, returning whether a new row was
// created (false = the period was already billed - ON CONFLICT DO NOTHING)
func (r *postgresRepository) CreateInvoice(ctx context.Context, subID uuid.UUID, periodStart, periodEnd time.Time, amount int64, currency string, sequence int32) (bool, error) {
	rows, err := r.queries.CreateSubscriptionInvoice(ctx, db.CreateSubscriptionInvoiceParams{
		SubscriptionID: subID,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		Amount:         amount,
		CurrencyCode:   currency,
		Sequence:       sequence,
	})
	if err != nil {
		return false, fmt.Errorf("create subscription invoice: %w", err)
	}
	return len(rows) > 0, nil
}

func (r *postgresRepository) CountInvoices(ctx context.Context, subID uuid.UUID) (int64, error) {
	n, err := r.queries.CountSubscriptionInvoices(ctx, subID)
	if err != nil {
		return 0, fmt.Errorf("count subscription invoices: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) ListInvoices(ctx context.Context, subID uuid.UUID) ([]Invoice, error) {
	rows, err := r.queries.ListSubscriptionInvoices(ctx, subID)
	if err != nil {
		return nil, fmt.Errorf("list subscription invoices: %w", err)
	}
	out := make([]Invoice, len(rows))
	for i, row := range rows {
		out[i] = toInvoice(row)
	}
	return out, nil
}

func toSubscriptions(rows []db.Subscription) []Subscription {
	out := make([]Subscription, len(rows))
	for i, row := range rows {
		out[i] = toSubscription(row)
	}
	return out
}

func toSubscription(row db.Subscription) Subscription {
	s := Subscription{
		ID:                 row.ID,
		ProductID:          row.ProductID,
		Status:             row.Status,
		IntervalUnit:       row.IntervalUnit,
		IntervalCount:      row.IntervalCount,
		Price:              row.Price,
		CurrencyCode:       row.CurrencyCode,
		Quantity:           row.Quantity,
		SKU:                row.Sku,
		Title:              row.Title,
		CurrentPeriodStart: row.CurrentPeriodStart,
		CurrentPeriodEnd:   row.CurrentPeriodEnd,
		NextBillingAt:      row.NextBillingAt,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
	if row.CustomerID.Valid {
		id := row.CustomerID.UUID
		s.CustomerID = &id
	}
	if row.OrderID.Valid {
		id := row.OrderID.UUID
		s.OrderID = &id
	}
	if row.ChannelID.Valid {
		id := row.ChannelID.UUID
		s.ChannelID = &id
	}
	if row.CanceledAt.Valid {
		t := row.CanceledAt.Time
		s.CanceledAt = &t
	}
	if row.PaymentMethodID.Valid {
		id := row.PaymentMethodID.UUID
		s.PaymentMethodID = &id
	}
	return s
}

func toInvoice(row db.SubscriptionInvoice) Invoice {
	return Invoice{
		ID:             row.ID,
		SubscriptionID: row.SubscriptionID,
		PeriodStart:    row.PeriodStart,
		PeriodEnd:      row.PeriodEnd,
		Amount:         row.Amount,
		CurrencyCode:   row.CurrencyCode,
		Status:         row.Status,
		Sequence:       row.Sequence,
		CreatedAt:      row.CreatedAt,

		AttemptCount:     row.AttemptCount,
		FailureReason:    row.FailureReason,
		PaymentReference: row.PaymentReference,
		NextAttemptAt:    nullTimePtr(row.NextAttemptAt),
		PaidAt:           nullTimePtr(row.PaidAt),
	}
}

func nullTimePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func (r *postgresRepository) ListChargeableInvoices(ctx context.Context, now time.Time, limit int32) ([]Invoice, error) {
	rows, err := r.queries.ListChargeableInvoices(ctx, db.ListChargeableInvoicesParams{Now: now, Batch: limit})
	if err != nil {
		return nil, fmt.Errorf("list chargeable invoices: %w", err)
	}
	out := make([]Invoice, len(rows))
	for i, row := range rows {
		out[i] = toInvoice(row)
	}
	return out, nil
}

func (r *postgresRepository) MarkInvoicePaid(ctx context.Context, id uuid.UUID, reference string) error {
	if err := r.queries.MarkInvoicePaid(ctx, db.MarkInvoicePaidParams{ID: id, PaymentReference: reference}); err != nil {
		return fmt.Errorf("mark invoice paid: %w", err)
	}
	return nil
}

func (r *postgresRepository) RecordInvoiceAttempt(ctx context.Context, id uuid.UUID, nextAttemptAt *time.Time, failureReason string) (Invoice, error) {
	next := sql.NullTime{}
	if nextAttemptAt != nil {
		next = sql.NullTime{Time: *nextAttemptAt, Valid: true}
	}
	row, err := r.queries.RecordInvoiceAttempt(ctx, db.RecordInvoiceAttemptParams{
		ID:            id,
		NextAttemptAt: next,
		FailureReason: failureReason,
	})
	if err != nil {
		return Invoice{}, fmt.Errorf("record invoice attempt: %w", err)
	}
	return toInvoice(row), nil
}

func (r *postgresRepository) SetPaymentMethod(ctx context.Context, id uuid.UUID, paymentMethodID *uuid.UUID) (Subscription, error) {
	row, err := r.queries.SetSubscriptionPaymentMethod(ctx, db.SetSubscriptionPaymentMethodParams{
		ID:              id,
		PaymentMethodID: nullableUUID(paymentMethodID),
	})
	if err != nil {
		return Subscription{}, fmt.Errorf("set subscription payment method: %w", err)
	}
	return toSubscription(row), nil
}

func nullableUUID(p *uuid.UUID) uuid.NullUUID {
	if p == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *p, Valid: true}
}

func nullableTime(p *time.Time) sql.NullTime {
	if p == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *p, Valid: true}
}
