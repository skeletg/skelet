// Package storecredit is a per-customer credit ledger spent as a payment TENDER at checkout - like gift cards it reduces the amount owed to the provider, never the taxed order total
// The ledger is append-only, grants positive and redemptions negative, with the balance the signed sum - Grants come from admin adjustments and, later, loyalty and referral programs
package storecredit

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

var (
	ErrInvalidAmount = apperr.New("STORE_CREDIT_INVALID_AMOUNT", apperr.ClassValidation, "adjustment amount must be non-zero")
	// Rejects a deduction that would drive the balance negative
	ErrInsufficientBalance = apperr.New("STORE_CREDIT_INSUFFICIENT", apperr.ClassValidation, "adjustment would make the store credit balance negative")
)

type Entry struct {
	ID           uuid.UUID
	CustomerID   uuid.UUID
	OrderID      *uuid.UUID
	AmountCents  int64
	CurrencyCode string
	Reason       string
	AdminID      *uuid.UUID
	CreatedAt    time.Time
}

// Holds the ledger logic - all writes go through the queries layer, and redemption joins the order-placement transaction
type Service struct {
	queries *db.Queries
	tx      *txrunner.Manager
}

func NewService(queries *db.Queries, txMgr *txrunner.Manager) *Service {
	return &Service{queries: queries, tx: txMgr}
}

func (s *Service) Balance(ctx context.Context, customerID uuid.UUID, currency string) (int64, error) {
	bal, err := s.queries.StoreCreditBalance(ctx, db.StoreCreditBalanceParams{
		CustomerID:   customerID,
		CurrencyCode: currency,
	})
	if err != nil {
		return 0, fmt.Errorf("store credit balance: %w", err)
	}
	return bal, nil
}

func (s *Service) Entries(ctx context.Context, customerID uuid.UUID, limit int32) ([]Entry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.queries.ListStoreCreditEntries(ctx, db.ListStoreCreditEntriesParams{
		CustomerID: customerID,
		Limit:      limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list store credit entries: %w", err)
	}
	out := make([]Entry, 0, len(rows))
	for _, r := range rows {
		out = append(out, toEntry(r))
	}
	return out, nil
}

// Applies a signed admin correction, granting when positive and deducting when negative - deductions cap at the current balance so the ledger never goes negative, and lock, sum and insert run in one transaction
func (s *Service) Adjust(ctx context.Context, customerID uuid.UUID, amountCents int64, currency, reason string, adminID *uuid.UUID) (Entry, error) {
	if amountCents == 0 {
		return Entry{}, ErrInvalidAmount
	}
	var entry Entry
	err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := s.queries.WithTx(t)
		if err := q.LockCustomerForStoreCredit(ctx, customerID); err != nil {
			return fmt.Errorf("lock customer: %w", err)
		}
		if amountCents < 0 {
			bal, err := q.StoreCreditBalance(ctx, db.StoreCreditBalanceParams{
				CustomerID:   customerID,
				CurrencyCode: currency,
			})
			if err != nil {
				return fmt.Errorf("store credit balance: %w", err)
			}
			if bal+amountCents < 0 {
				return ErrInsufficientBalance
			}
		}
		row, err := q.InsertStoreCreditEntry(ctx, db.InsertStoreCreditEntryParams{
			CustomerID:   customerID,
			OrderID:      uuid.NullUUID{},
			AmountCents:  amountCents,
			CurrencyCode: currency,
			Reason:       reason,
			AdminID:      uuid.NullUUID{UUID: ptr.Deref(adminID), Valid: adminID != nil},
		})
		if err != nil {
			return fmt.Errorf("insert store credit entry: %w", err)
		}
		entry = toEntry(row)
		return nil
	})
	if err != nil {
		return Entry{}, err
	}
	return entry, nil
}

// Spends credit as a tender against an order, joining the caller's placement transaction - it applies min(balance, amountDue) and writes a negative ledger row linked to the order, or nothing when the balance is empty or nothing is due
// The customer row is locked first so concurrent checkouts can't overdraw
func (s *Service) RedeemUpToTx(ctx context.Context, sqlTx *sql.Tx, customerID uuid.UUID, amountDue int64, currency string, orderID uuid.UUID) (int64, error) {
	if amountDue <= 0 {
		return 0, nil
	}
	q := s.queries.WithTx(sqlTx)
	if err := q.LockCustomerForStoreCredit(ctx, customerID); err != nil {
		return 0, fmt.Errorf("lock customer: %w", err)
	}
	bal, err := q.StoreCreditBalance(ctx, db.StoreCreditBalanceParams{
		CustomerID:   customerID,
		CurrencyCode: currency,
	})
	if err != nil {
		return 0, fmt.Errorf("store credit balance: %w", err)
	}
	apply := min(bal, amountDue)
	if apply <= 0 {
		return 0, nil
	}
	if _, err := q.InsertStoreCreditEntry(ctx, db.InsertStoreCreditEntryParams{
		CustomerID:   customerID,
		OrderID:      uuid.NullUUID{UUID: orderID, Valid: true},
		AmountCents:  -apply,
		CurrencyCode: currency,
		Reason:       "order payment",
	}); err != nil {
		return 0, fmt.Errorf("insert store credit redemption: %w", err)
	}
	return apply, nil
}

// GrantTx inserts a positive entry joining the caller's transaction Used by sibling programs (loyalty redemption, referral rewards) that
// must grant credit atomically with their own ledger writes - The caller is responsible for holding the customer lock
func (s *Service) GrantTx(ctx context.Context, sqlTx *sql.Tx, customerID uuid.UUID, amountCents int64, currency, reason string) error {
	if amountCents <= 0 {
		return ErrInvalidAmount
	}
	if _, err := s.queries.WithTx(sqlTx).InsertStoreCreditEntry(ctx, db.InsertStoreCreditEntryParams{
		CustomerID:   customerID,
		AmountCents:  amountCents,
		CurrencyCode: currency,
		Reason:       reason,
	}); err != nil {
		return fmt.Errorf("insert store credit grant: %w", err)
	}
	return nil
}

// ClawbackTx removes up to amountCents of credit inside the caller's transaction, clamped at the current balance so the customer is never pushed into invisible debt - returns what was actually taken, which is less than asked when they already spent it
// Used when the thing that minted the credit is reversed, e.g. loyalty points revoked with a cancelled order
func (s *Service) ClawbackTx(ctx context.Context, sqlTx *sql.Tx, customerID uuid.UUID, amountCents int64, currency, reason string) (int64, error) {
	if amountCents <= 0 {
		return 0, nil
	}
	q := s.queries.WithTx(sqlTx)
	if err := q.LockCustomerForStoreCredit(ctx, customerID); err != nil {
		return 0, fmt.Errorf("lock customer: %w", err)
	}
	bal, err := q.StoreCreditBalance(ctx, db.StoreCreditBalanceParams{CustomerID: customerID, CurrencyCode: currency})
	if err != nil {
		return 0, fmt.Errorf("store credit balance: %w", err)
	}
	take := min(bal, amountCents)
	if take <= 0 {
		return 0, nil
	}
	if _, err := q.InsertStoreCreditEntry(ctx, db.InsertStoreCreditEntryParams{
		CustomerID:   customerID,
		AmountCents:  -take,
		CurrencyCode: currency,
		Reason:       reason,
	}); err != nil {
		return 0, fmt.Errorf("insert store credit clawback: %w", err)
	}
	return take, nil
}

func toEntry(r db.StoreCreditEntry) Entry {
	e := Entry{
		ID:           r.ID,
		CustomerID:   r.CustomerID,
		AmountCents:  r.AmountCents,
		CurrencyCode: r.CurrencyCode,
		Reason:       r.Reason,
		CreatedAt:    r.CreatedAt,
	}
	if r.OrderID.Valid {
		id := r.OrderID.UUID
		e.OrderID = &id
	}
	if r.AdminID.Valid {
		id := r.AdminID.UUID
		e.AdminID = &id
	}
	return e
}
