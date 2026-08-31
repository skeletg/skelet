package giftcard

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/internal/db"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

type postgresRepository struct {
	queries *db.Queries
}

// NewRepository builds a postgres-backed gift card repository
func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) WithTx(tx *sql.Tx) Repository {
	return &postgresRepository{queries: r.queries.WithTx(tx)}
}

func (r *postgresRepository) CreateGiftCard(ctx context.Context, code string, in IssueInput) (GiftCard, error) {
	row, err := r.queries.CreateGiftCard(ctx, db.CreateGiftCardParams{
		Code:           code,
		CurrencyCode:   in.CurrencyCode,
		InitialBalance: in.Amount,
		RecipientEmail: in.RecipientEmail,
		Message:        in.Message,
		IssuedOrderID:  nullableUUID(in.IssuedOrderID),
		CustomerID:     nullableUUID(in.CustomerID),
		ExpiresAt:      nullableTime(in.ExpiresAt),
		IssueRef:       in.IssueRef,
	})
	if err != nil {
		if pgdb.IsUniqueViolation(err) {
			return GiftCard{}, ErrCodeTaken // the code collided, or a concurrent replay already claimed this issue_ref - issueTx re-reads by ref either way
		}
		return GiftCard{}, fmt.Errorf("create gift card: %w", err)
	}
	return toGiftCard(row), nil
}

func (r *postgresRepository) GetByIssueRef(ctx context.Context, ref string) (GiftCard, error) {
	row, err := r.queries.GetGiftCardByIssueRef(ctx, ref)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GiftCard{}, ErrNotFound
		}
		return GiftCard{}, fmt.Errorf("get gift card by issue ref: %w", err)
	}
	return toGiftCard(row), nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (GiftCard, error) {
	row, err := r.queries.GetGiftCardByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GiftCard{}, ErrNotFound
		}
		return GiftCard{}, fmt.Errorf("get gift card: %w", err)
	}
	return toGiftCard(row), nil
}

func (r *postgresRepository) GetByCode(ctx context.Context, code string) (GiftCard, error) {
	row, err := r.queries.GetGiftCardByCode(ctx, code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GiftCard{}, ErrNotFound
		}
		return GiftCard{}, fmt.Errorf("get gift card by code: %w", err)
	}
	return toGiftCard(row), nil
}

func (r *postgresRepository) GetByCodeForUpdate(ctx context.Context, code string) (GiftCard, error) {
	row, err := r.queries.GetGiftCardByCodeForUpdate(ctx, code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GiftCard{}, ErrNotFound
		}
		return GiftCard{}, fmt.Errorf("get gift card for update: %w", err)
	}
	return toGiftCard(row), nil
}

func (r *postgresRepository) GetByIDForUpdate(ctx context.Context, id uuid.UUID) (GiftCard, error) {
	row, err := r.queries.GetGiftCardByIDForUpdate(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GiftCard{}, ErrNotFound
		}
		return GiftCard{}, fmt.Errorf("get gift card by id for update: %w", err)
	}
	return toGiftCard(row), nil
}

func (r *postgresRepository) UpdateBalance(ctx context.Context, id uuid.UUID, balance int64, status string) (GiftCard, error) {
	row, err := r.queries.UpdateGiftCardBalance(ctx, db.UpdateGiftCardBalanceParams{
		ID:      id,
		Balance: balance,
		Status:  status,
	})
	if err != nil {
		return GiftCard{}, fmt.Errorf("update gift card balance: %w", err)
	}
	return toGiftCard(row), nil
}

func (r *postgresRepository) List(ctx context.Context, limit, offset int32) ([]GiftCard, error) {
	rows, err := r.queries.ListGiftCards(ctx, db.ListGiftCardsParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, fmt.Errorf("list gift cards: %w", err)
	}
	out := make([]GiftCard, len(rows))
	for i, row := range rows {
		out[i] = toGiftCard(row)
	}
	return out, nil
}

func (r *postgresRepository) ListByIssuedOrder(ctx context.Context, orderID uuid.UUID) ([]GiftCard, error) {
	rows, err := r.queries.ListGiftCardsByIssuedOrder(ctx, uuid.NullUUID{UUID: orderID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list gift cards by issued order: %w", err)
	}
	out := make([]GiftCard, len(rows))
	for i, row := range rows {
		out[i] = toGiftCard(row)
	}
	return out, nil
}

func (r *postgresRepository) ListTransactionsByOrder(ctx context.Context, orderID uuid.UUID) ([]Transaction, error) {
	rows, err := r.queries.ListGiftCardTransactionsByOrder(ctx, uuid.NullUUID{UUID: orderID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list gift card transactions by order: %w", err)
	}
	out := make([]Transaction, len(rows))
	for i, row := range rows {
		out[i] = toTransaction(row)
	}
	return out, nil
}

func (r *postgresRepository) InsertTransaction(ctx context.Context, t Transaction) (Transaction, error) {
	row, err := r.queries.InsertGiftCardTransaction(ctx, db.InsertGiftCardTransactionParams{
		GiftCardID: t.GiftCardID,
		Amount:     t.Amount,
		Reason:     t.Reason,
		OrderID:    nullableUUID(t.OrderID),
		Note:       t.Note,
	})
	if err != nil {
		return Transaction{}, fmt.Errorf("insert gift card transaction: %w", err)
	}
	return toTransaction(row), nil
}

func (r *postgresRepository) ListTransactions(ctx context.Context, giftCardID uuid.UUID) ([]Transaction, error) {
	rows, err := r.queries.ListGiftCardTransactions(ctx, giftCardID)
	if err != nil {
		return nil, fmt.Errorf("list gift card transactions: %w", err)
	}
	out := make([]Transaction, len(rows))
	for i, row := range rows {
		out[i] = toTransaction(row)
	}
	return out, nil
}

// --- mappers ---

func toGiftCard(row db.GiftCard) GiftCard {
	g := GiftCard{
		ID:             row.ID,
		Code:           row.Code,
		CurrencyCode:   row.CurrencyCode,
		InitialBalance: row.InitialBalance,
		Balance:        row.Balance,
		Status:         row.Status,
		RecipientEmail: row.RecipientEmail,
		Message:        row.Message,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
	if row.IssuedOrderID.Valid {
		id := row.IssuedOrderID.UUID
		g.IssuedOrderID = &id
	}
	if row.CustomerID.Valid {
		id := row.CustomerID.UUID
		g.CustomerID = &id
	}
	if row.ExpiresAt.Valid {
		t := row.ExpiresAt.Time
		g.ExpiresAt = &t
	}
	return g
}

func toTransaction(row db.GiftCardTransaction) Transaction {
	t := Transaction{
		ID:         row.ID,
		GiftCardID: row.GiftCardID,
		Amount:     row.Amount,
		Reason:     row.Reason,
		Note:       row.Note,
		CreatedAt:  row.CreatedAt,
	}
	if row.OrderID.Valid {
		id := row.OrderID.UUID
		t.OrderID = &id
	}
	return t
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
