// Package giftcard owns stored-value gift cards - issuing them, where a kind=giftcard product purchase generates one, tracking balance through an append-only signed-amount ledger, and redeeming them as a tender at checkout
// Balance is a cached column kept in lock-step with the ledger, and redeem and adjust lock the card row so concurrent spends can't overdraw
package giftcard

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	StatusActive   = "active"
	StatusDepleted = "depleted"
	StatusDisabled = "disabled"
	StatusExpired  = "expired"
)

const (
	ReasonIssue  = "issue"
	ReasonRedeem = "redeem"
	ReasonRefund = "refund"
	ReasonAdjust = "adjust"
)

// A stored-value instrument - Balance is current while InitialBalance is the face value at issue, both in minor units
type GiftCard struct {
	ID             uuid.UUID
	Code           string
	CurrencyCode   string
	InitialBalance int64
	Balance        int64
	Status         string
	RecipientEmail string
	Message        string
	IssuedOrderID  *uuid.UUID
	CustomerID     *uuid.UUID
	ChannelID      *uuid.UUID
	ExpiresAt      *time.Time
	IssueRef       string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Whether the issuing order bought this card - a refund paid out as a gift card also carries the order, keyed "memo:" or "return:",
// and cancelling or refunding the order must never take that one back. Purchase cards are keyed "order:", or unkeyed from before the key existed
func (c GiftCard) Purchased() bool {
	return c.IssuedOrderID != nil && (c.IssueRef == "" || strings.HasPrefix(c.IssueRef, purchaseRefPrefix))
}

const purchaseRefPrefix = "order:"

func (c GiftCard) SpendsOn(channelID uuid.UUID) bool {
	return c.ChannelID == nil || *c.ChannelID == channelID
}

type Transaction struct {
	ID         uuid.UUID
	GiftCardID uuid.UUID
	Amount     int64
	Reason     string
	OrderID    *uuid.UUID
	Note       string
	CreatedAt  time.Time
}

// Requests a new gift card - Amount is the face value, and the card opens with Balance equal to it
type IssueInput struct {
	CurrencyCode   string
	Amount         int64
	RecipientEmail string
	Message        string
	IssuedOrderID  *uuid.UUID
	CustomerID     *uuid.UUID
	ExpiresAt      *time.Time
	// nil takes the issuing order's channel, or every channel when there is no order
	ChannelID *uuid.UUID
	// IssueRef makes an issue idempotent for callers that mint store value between a pending ledger row and its finalize ("memo:<id>", "return:<id>") - a replay under the same ref returns the first card instead of minting a second - empty means no key, the plain issue path
	IssueRef string
}

var (
	ErrNotFound         = errors.New("gift card not found")
	ErrCodeTaken        = errors.New("gift card code already exists")
	ErrInvalidAmount    = errors.New("gift card amount must be positive")
	ErrInactive         = errors.New("gift card is not active")
	ErrExpired          = errors.New("gift card has expired")
	ErrCurrencyMismatch = errors.New("gift card currency does not match the order")
	ErrWrongChannel     = errors.New("gift card is not valid on this store")
	ErrInsufficient     = errors.New("gift card balance is insufficient")
)

type Repository interface {
	WithTx(tx *sql.Tx) Repository
	CreateGiftCard(ctx context.Context, code string, in IssueInput) (GiftCard, error)
	GetByID(ctx context.Context, id uuid.UUID) (GiftCard, error)
	// The card a keyed issue already minted, ErrNotFound when this ref hasn't been used
	GetByIssueRef(ctx context.Context, ref string) (GiftCard, error)
	GetByCode(ctx context.Context, code string) (GiftCard, error)
	// Row-locks the card for the caller's transaction, so concurrent redemptions serialize
	GetByCodeForUpdate(ctx context.Context, code string) (GiftCard, error)
	// The cancel-restore path knows ids, not codes - it locks before checking whether the restore already ran
	GetByIDForUpdate(ctx context.Context, id uuid.UUID) (GiftCard, error)
	UpdateBalance(ctx context.Context, id uuid.UUID, balance int64, status string) (GiftCard, error)
	// search matches the code or recipient email; the total counts every match, not just this page
	List(ctx context.Context, channelIDs []uuid.UUID, search string, limit, offset int32) ([]GiftCard, int32, error)
	// Cards issued by a given order - used for issuance idempotency under at-least-once OrderPlaced delivery
	ListByIssuedOrder(ctx context.Context, orderID uuid.UUID) ([]GiftCard, error)
	InsertTransaction(ctx context.Context, t Transaction) (Transaction, error)
	ListTransactions(ctx context.Context, giftCardID uuid.UUID) ([]Transaction, error)
	// Every ledger row tied to an order - the basis for reversing redemptions on cancellation
	ListTransactionsByOrder(ctx context.Context, orderID uuid.UUID) ([]Transaction, error)
}
