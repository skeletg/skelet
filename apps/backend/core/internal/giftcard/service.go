package giftcard

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type Service struct {
	repo   Repository
	tx     *txrunner.Manager
	now    func() time.Time
	orders OrderGiftcardLookup
	bus    *events.Bus
}

func NewService(repo Repository, txMgr *txrunner.Manager) *Service {
	return &Service{repo: repo, tx: txMgr, now: time.Now}
}

// Wires the bus so gift-card lifecycle webhooks are published - optional and nil-safe
func (s *Service) SetEventBus(b *events.Bus) { s.bus = b }

func (s *Service) publish(ctx context.Context, e events.Event) {
	if s.bus != nil {
		s.bus.Publish(ctx, e)
	}
}

// Wires the order-to-giftcard bridge the OrderPlaced subscriber uses to find which lines of a placed order are gift cards - optional, and without it issuance-on-purchase is disabled
func (s *Service) SetOrderLookup(o OrderGiftcardLookup) { s.orders = o }

// Creates a card with a freshly-generated unique code and an opening 'issue' ledger row, all in one transaction
func (s *Service) Issue(ctx context.Context, in IssueInput) (GiftCard, error) {
	if in.Amount <= 0 {
		return GiftCard{}, ErrInvalidAmount
	}
	var card GiftCard
	err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		var err error
		card, err = issueTx(ctx, r, in)
		return err
	})
	if err != nil {
		return GiftCard{}, err
	}
	s.publish(ctx, &Issued{
		GiftCardID: card.ID,
		Code:       card.Code,
		Currency:   card.CurrencyCode,
		Amount:     card.InitialBalance,
		CustomerID: card.CustomerID,
		OrderID:    card.IssuedOrderID,
	})
	return card, nil
}

// Applies `amount` cents of the card to an order, debiting the balance under a row lock so concurrent redemptions can't overdraw - a non-empty currency must match the card
func (s *Service) Redeem(ctx context.Context, code string, amount int64, currency string, orderID *uuid.UUID) (GiftCard, error) {
	var card GiftCard
	err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		var err error
		card, err = redeemTx(ctx, r, code, amount, currency, s.now(), orderID)
		return err
	})
	if err != nil {
		return GiftCard{}, err
	}
	s.publish(ctx, &Redeemed{
		GiftCardID: card.ID,
		Code:       card.Code,
		Currency:   card.CurrencyCode,
		Amount:     amount,
		Balance:    card.Balance,
		OrderID:    orderID,
	})
	return card, nil
}

// Applies the card as a TENDER against an order, joining the caller's placement transaction - it spends min(balance, amountDue), so a gift card pays what it can and the remainder is owed elsewhere
// Validates active state, expiry and currency, and applies nothing with no ledger row when the card is empty or nothing is due
func (s *Service) RedeemUpToTx(ctx context.Context, tx *sql.Tx, code string, channelID uuid.UUID, amountDue int64, currency string, orderID uuid.UUID) (int64, error) {
	return redeemUpToTx(ctx, s.repo.WithTx(tx), code, channelID, amountDue, currency, s.now(), &orderID)
}

func (s *Service) GetByCode(ctx context.Context, code string) (GiftCard, error) {
	return s.repo.GetByCode(ctx, code)
}

func (s *Service) Balance(ctx context.Context, code string) (int64, error) {
	c, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return 0, err
	}
	return c.Balance, nil
}

// Marks a card inactive so it can no longer be redeemed, for loss or fraud - idempotent, so disabling an already-disabled card is a no-op
func (s *Service) Disable(ctx context.Context, id uuid.UUID) (GiftCard, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return GiftCard{}, err
	}
	if c.Status == StatusDisabled {
		return c, nil
	}
	return s.repo.UpdateBalance(ctx, id, c.Balance, StatusDisabled)
}

// Applies a signed balance correction for an admin top-up, goodwill or chargeback with an 'adjust' ledger row - rejects anything that would drive the balance negative, and the update and ledger are atomic
func (s *Service) Adjust(ctx context.Context, id uuid.UUID, delta int64, note string) (GiftCard, error) {
	var out GiftCard
	err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		c, err := r.GetByID(ctx, id)
		if err != nil {
			return err
		}
		newBalance := c.Balance + delta
		if newBalance < 0 {
			return ErrInsufficient
		}
		status := c.Status
		switch {
		case status == StatusDepleted && newBalance > 0:
			status = StatusActive
		case status == StatusActive && newBalance == 0:
			status = StatusDepleted
		}
		out, err = r.UpdateBalance(ctx, id, newBalance, status)
		if err != nil {
			return err
		}
		_, err = r.InsertTransaction(ctx, Transaction{
			GiftCardID: id, Amount: delta, Reason: ReasonAdjust, Note: note,
		})
		return err
	})
	if err != nil {
		return GiftCard{}, err
	}
	s.publish(ctx, &Adjusted{
		GiftCardID: out.ID,
		Code:       out.Code,
		Currency:   out.CurrencyCode,
		Delta:      delta,
		Balance:    out.Balance,
		Note:       note,
	})
	return out, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (GiftCard, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context, limit, offset int32) ([]GiftCard, error) {
	return s.ListForChannels(ctx, nil, limit, offset)
}

func (s *Service) ListForChannels(ctx context.Context, channelIDs []uuid.UUID, limit, offset int32) ([]GiftCard, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if channelIDs != nil && len(channelIDs) == 0 {
		return []GiftCard{}, nil
	}
	return s.repo.List(ctx, channelIDs, limit, offset)
}

func (s *Service) Transactions(ctx context.Context, id uuid.UUID) ([]Transaction, error) {
	return s.repo.ListTransactions(ctx, id)
}

// Issuance against a transaction-bound repository - factored out so it's testable with a fake repository, no txrunner.Manager
func issueTx(ctx context.Context, r Repository, in IssueInput) (GiftCard, error) {
	// A keyed issue is idempotent: refunds mint store value between their pending ledger row and its
	// finalize, so without it a replay in that window hands the customer a second card
	if in.IssueRef != "" {
		if existing, err := r.GetByIssueRef(ctx, in.IssueRef); err == nil {
			return existing, nil
		} else if !errors.Is(err, ErrNotFound) {
			return GiftCard{}, err
		}
	}
	code, err := generateUniqueCode(ctx, r)
	if err != nil {
		return GiftCard{}, err
	}
	card, err := r.CreateGiftCard(ctx, code, in)
	if err != nil {
		if in.IssueRef != "" && errors.Is(err, ErrCodeTaken) { // a concurrent replay won the unique index - its card is the one card
			if existing, refErr := r.GetByIssueRef(ctx, in.IssueRef); refErr == nil {
				return existing, nil
			}
		}
		return GiftCard{}, err
	}
	if _, err := r.InsertTransaction(ctx, Transaction{
		GiftCardID: card.ID,
		Amount:     in.Amount,
		Reason:     ReasonIssue,
		OrderID:    in.IssuedOrderID,
	}); err != nil {
		return GiftCard{}, err
	}
	return card, nil
}

// Redemption against a transaction-bound repository - factored out so the lock, validate, debit and ledger sequence is testable with a fake
func redeemTx(ctx context.Context, r Repository, code string, amount int64, currency string, now time.Time, orderID *uuid.UUID) (GiftCard, error) {
	c, err := r.GetByCodeForUpdate(ctx, code)
	if err != nil {
		return GiftCard{}, err
	}
	newBalance, newStatus, err := applyRedemption(c, amount, currency, now)
	if err != nil {
		return GiftCard{}, err
	}
	card, err := r.UpdateBalance(ctx, c.ID, newBalance, newStatus)
	if err != nil {
		return GiftCard{}, err
	}
	if _, err := r.InsertTransaction(ctx, Transaction{
		GiftCardID: c.ID,
		Amount:     -amount,
		Reason:     ReasonRedeem,
		OrderID:    orderID,
	}); err != nil {
		return GiftCard{}, err
	}
	return card, nil
}

// A tender redemption against a transaction-bound repository, spending min(balance, amountDue) - factored out so it's testable with a fake
func redeemUpToTx(ctx context.Context, r Repository, code string, channelID uuid.UUID, amountDue int64, currency string, now time.Time, orderID *uuid.UUID) (int64, error) {
	c, err := r.GetByCodeForUpdate(ctx, code)
	if err != nil {
		return 0, err
	}
	if !c.SpendsOn(channelID) {
		return 0, ErrWrongChannel
	}
	applied, newBalance, newStatus, err := tenderAmount(c, amountDue, currency, now)
	if err != nil {
		return 0, err
	}
	if applied == 0 {
		return 0, nil // empty card or nothing due - no-op, no ledger row
	}
	if _, err := r.UpdateBalance(ctx, c.ID, newBalance, newStatus); err != nil {
		return 0, err
	}
	if _, err := r.InsertTransaction(ctx, Transaction{
		GiftCardID: c.ID,
		Amount:     -applied,
		Reason:     ReasonRedeem,
		OrderID:    orderID,
	}); err != nil {
		return 0, err
	}
	return applied, nil
}

// Validates a card as a tender and computes how much of amountDue it covers, plus the resulting balance and status - pure, no I/O
// Unlike applyRedemption, which takes a fixed amount and errors when short, a tender never errors on insufficient balance and just applies what it has
func tenderAmount(card GiftCard, amountDue int64, currency string, now time.Time) (applied, newBalance int64, newStatus string, err error) {
	if card.Status != StatusActive {
		return 0, 0, "", ErrInactive
	}
	if card.ExpiresAt != nil && !card.ExpiresAt.After(now) {
		return 0, 0, "", ErrExpired
	}
	if currency != "" && card.CurrencyCode != currency {
		return 0, 0, "", ErrCurrencyMismatch
	}
	if amountDue <= 0 {
		return 0, card.Balance, card.Status, nil
	}
	applied = min(amountDue, card.Balance)
	newBalance = card.Balance - applied
	newStatus = StatusActive
	if newBalance == 0 && applied > 0 {
		newStatus = StatusDepleted
	}
	return applied, newBalance, newStatus, nil
}

// Validates a redemption and computes the resulting balance and status - pure, so the rules are unit-tested directly, with now as the expiry reference
func applyRedemption(card GiftCard, amount int64, currency string, now time.Time) (newBalance int64, newStatus string, err error) {
	if amount <= 0 {
		return 0, "", ErrInvalidAmount
	}
	if card.Status != StatusActive {
		return 0, "", ErrInactive
	}
	if card.ExpiresAt != nil && !card.ExpiresAt.After(now) {
		return 0, "", ErrExpired
	}
	if currency != "" && card.CurrencyCode != currency {
		return 0, "", ErrCurrencyMismatch
	}
	if amount > card.Balance {
		return 0, "", ErrInsufficient
	}
	newBalance = card.Balance - amount
	newStatus = StatusActive
	if newBalance == 0 {
		newStatus = StatusDepleted
	}
	return newBalance, newStatus, nil
}

// Excludes visually ambiguous characters - I, O, 0, 1
const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// Mints codes until one is free, giving up after a few tries - collisions are astronomically unlikely at 12 chars over 32 symbols
func generateUniqueCode(ctx context.Context, r Repository) (string, error) {
	for range 10 {
		code, err := newCode()
		if err != nil {
			return "", err
		}
		_, err = r.GetByCode(ctx, code)
		if errors.Is(err, ErrNotFound) {
			return code, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("gift card: could not generate a unique code")
}

// A grouped 12-symbol code, e.g. "GC-A7K2-9MTX-PQ4R"
func newCode() (string, error) {
	raw := make([]byte, 12)
	if _, err := crand.Read(raw); err != nil {
		return "", fmt.Errorf("gift card: read random: %w", err)
	}
	out := make([]byte, 0, 18)
	out = append(out, 'G', 'C', '-')
	for i, b := range raw {
		out = append(out, codeAlphabet[int(b)%len(codeAlphabet)])
		if i%4 == 3 && i != len(raw)-1 {
			out = append(out, '-')
		}
	}
	return string(out), nil
}
