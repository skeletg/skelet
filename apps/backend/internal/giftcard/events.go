package giftcard

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/money"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

const (
	EventIssued   = "giftcard.issued"
	EventRedeemed = "giftcard.redeemed"
	EventAdjusted = "giftcard.adjusted"
)

// Fires after a gift card is minted, whether an admin issue or one card per gift-card line on a placed order - async - Amount is the opening balance, and OrderID is set only for a purchase
type Issued struct {
	GiftCardID uuid.UUID
	Code       string
	Currency   string
	Amount     int64
	CustomerID *uuid.UUID
	OrderID    *uuid.UUID
}

func (*Issued) Type() string { return EventIssued }

// Fires after a gift card is spent through an admin or standalone redeem - Amount is what was applied and Balance what remains - Async
type Redeemed struct {
	GiftCardID uuid.UUID
	Code       string
	Currency   string
	Amount     int64
	Balance    int64
	OrderID    *uuid.UUID
}

func (*Redeemed) Type() string { return EventRedeemed }

// Fires after an admin balance correction - top-up, goodwill, chargeback - Delta is signed and Balance is post-adjustment - Async
type Adjusted struct {
	GiftCardID uuid.UUID
	Code       string
	Currency   string
	Delta      int64
	Balance    int64
	Note       string
}

func (*Adjusted) Type() string { return EventAdjusted }

// One gift-card line on a placed order - its face value, the balance to mint, and how many cards to issue, one per unit
type IssuableLine struct {
	UnitPrice int64
	Quantity  float64
}

// The gift-card lines of a placed order - declared locally so giftcard doesn't import catalog, with the adapter in main.go resolving each line's product kind
type OrderGiftcardLookup interface {
	GiftcardLines(ctx context.Context, orderID uuid.UUID) ([]IssuableLine, error)
}

// Wires issuance - on OrderPlaced, durable through the outbox and at-least-once, mint a gift card per gift-card line - Idempotent under replay, skipping orders that already have cards
func (s *Service) SubscribeToOrderEvents(bus *events.Bus) {
	if bus == nil {
		return
	}
	bus.Subscribe(order.EventPlaced, s.onOrderPlaced)
	bus.Subscribe(order.EventCancelled, s.onOrderCancelled)
}

func (s *Service) onOrderCancelled(ctx context.Context, e events.Event) error {
	cancelled, ok := e.(*order.OrderCancelled)
	if !ok {
		return nil
	}
	if err := s.restoreForOrder(ctx, cancelled.OrderID, cancelled.RefundableFraction); err != nil {
		log.Printf("[giftcard] restore redemptions for cancelled order %s: %v", cancelled.OrderID, err)
		return err
	}
	return nil
}

// restoreForOrder puts back the balance an order spent when that order is cancelled.
// Gift cards were the only tender with no cancel path - store credit, B2B credit and
// loyalty all reverse - so a cancelled order left the customer's card depleted with
// no refund row and no admin remedy short of a manual adjustment.
// Idempotent: a positive order-linked row means the restore already ran.
// fraction restores only the UNSHIPPED share of what the order spent
func (s *Service) restoreForOrder(ctx context.Context, orderID uuid.UUID, fraction float64) error {
	return s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		spent, err := spendByCard(ctx, r, orderID)
		if err != nil || len(spent) == 0 {
			return err
		}
		// Lock every card BEFORE deciding whether the restore already ran - the outbox dispatches a claimed batch concurrently, so two deliveries both reading "not restored" would both credit
		// Sorted, so two orders sharing a pair of cards can't take them in opposite orders and deadlock
		cardIDs := make([]uuid.UUID, 0, len(spent))
		for id := range spent {
			cardIDs = append(cardIDs, id)
		}
		sort.Slice(cardIDs, func(i, j int) bool { return cardIDs[i].String() < cardIDs[j].String() })
		cards := make(map[uuid.UUID]GiftCard, len(cardIDs))
		for _, cardID := range cardIDs {
			card, err := r.GetByIDForUpdate(ctx, cardID)
			if err != nil {
				return fmt.Errorf("lock gift card %s: %w", cardID, err)
			}
			cards[cardID] = card
		}
		spent, err = spendByCard(ctx, r, orderID) // re-read under the locks - whoever we queued behind has committed by now
		if err != nil || len(spent) == 0 {
			return err
		}
		for _, cardID := range cardIDs {
			amount := money.ScaleFraction(spent[cardID], fraction)
			if amount <= 0 {
				continue
			}
			card := cards[cardID]
			// Restoring revives a depleted card - it has spendable balance again
			status := card.Status
			if status == StatusDepleted {
				status = StatusActive
			}
			if _, err := r.UpdateBalance(ctx, cardID, card.Balance+amount, status); err != nil {
				return fmt.Errorf("restore gift card balance: %w", err)
			}
			if _, err := r.InsertTransaction(ctx, Transaction{
				GiftCardID: cardID,
				Amount:     amount,
				Reason:     ReasonRefund,
				OrderID:    &orderID,
				Note:       "order cancelled - redemption reversed",
			}); err != nil {
				return fmt.Errorf("record gift card restore: %w", err)
			}
		}
		return nil
	})
}

// Empty means nothing to do - either no card paid for this order, or a positive order-linked row says the restore already ran
func spendByCard(ctx context.Context, r Repository, orderID uuid.UUID) (map[uuid.UUID]int64, error) {
	rows, err := r.ListTransactionsByOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list gift card transactions for order: %w", err)
	}
	spent := map[uuid.UUID]int64{}
	for _, row := range rows {
		switch {
		case row.Reason == ReasonRedeem && row.Amount < 0:
			spent[row.GiftCardID] += -row.Amount
		case row.Reason == ReasonRefund && row.Amount > 0:
			return nil, nil // already restored
		}
	}
	return spent, nil
}

func (s *Service) onOrderPlaced(ctx context.Context, e events.Event) error {
	placed, ok := e.(*order.OrderPlaced)
	if !ok || s.orders == nil {
		return nil
	}
	// Idempotency - OrderPlaced is at-least-once, since the outbox relay can replay, so a card already referencing this order means it's done
	if existing, err := s.repo.ListByIssuedOrder(ctx, placed.OrderID); err != nil {
		log.Printf("[giftcard] check issued for order %s: %v", placed.OrderID, err)
		return err
	} else if len(existing) > 0 {
		return nil
	}
	lines, err := s.orders.GiftcardLines(ctx, placed.OrderID)
	if err != nil {
		log.Printf("[giftcard] resolve gift-card lines for order %s: %v", placed.OrderID, err)
		return err
	}
	if len(lines) == 0 {
		return nil
	}
	orderID := placed.OrderID
	// Issue every card for the order in one transaction, so a mid-way crash rolls back cleanly and the replay re-issues the whole set rather than leaving a partial one the idempotency check would then skip
	var issued []GiftCard
	err = s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		issued = issued[:0]
		for _, l := range lines {
			count := max(int(l.Quantity), 1)
			for range count {
				card, err := issueTx(ctx, r, IssueInput{
					CurrencyCode:   placed.Currency,
					Amount:         l.UnitPrice,
					RecipientEmail: placed.Email,
					CustomerID:     placed.CustomerID,
					IssuedOrderID:  &orderID,
				})
				if err != nil {
					return err
				}
				issued = append(issued, card)
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("[giftcard] issue for order %s failed (will retry on replay): %v", placed.OrderID, err)
		return err
	}
	// Emit one webhook per minted card, post-commit and never inside the tx, so a rolled-back issuance can't fire a phantom event
	for _, card := range issued {
		s.publish(ctx, &Issued{
			GiftCardID: card.ID,
			Code:       card.Code,
			Currency:   card.CurrencyCode,
			Amount:     card.InitialBalance,
			CustomerID: card.CustomerID,
			OrderID:    card.IssuedOrderID,
		})
	}
	return nil
}
