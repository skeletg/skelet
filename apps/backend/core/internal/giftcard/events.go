package giftcard

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
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

// What minting needs of an order - its gift-card lines, who bought them and whether they're paid for - declared locally so giftcard doesn't import catalog, with the adapter in wiring resolving each line's product kind
type OrderGiftcards struct {
	Status        string
	PaymentStatus string
	Email         string
	CustomerID    *uuid.UUID
	ChannelID     uuid.UUID
	Currency      string
	Lines         []IssuableLine
}

type OrderGiftcardLookup interface {
	GiftcardLines(ctx context.Context, orderID uuid.UUID) (OrderGiftcards, error)
}

// Wires the order lifecycle, durable through the outbox and at-least-once: placement or a payment advance mints a paid-for order's cards, and
// cancellation, a void or a full refund revokes them - cancellation also restores what the order spent from cards
func (s *Service) SubscribeToOrderEvents(bus *events.Bus) {
	if bus == nil {
		return
	}
	bus.Subscribe(order.EventPlaced, s.onOrderPlaced)
	bus.Subscribe(order.EventPaymentStatusChanged, s.onPaymentStatusChanged)
	bus.Subscribe(order.EventCancelled, s.onOrderCancelled)
}

func (s *Service) onOrderPlaced(ctx context.Context, e events.Event) error {
	placed, ok := e.(*order.OrderPlaced)
	if !ok {
		return nil
	}
	return s.issueForOrder(ctx, placed.OrderID)
}

func (s *Service) onPaymentStatusChanged(ctx context.Context, e events.Event) error {
	changed, ok := e.(*order.OrderPaymentStatusChanged)
	if !ok {
		return nil
	}
	switch {
	case order.PaidFor(changed.Status):
		return s.issueForOrder(ctx, changed.OrderID)
	case changed.Status == order.PaymentVoided || changed.Status == order.PaymentRefunded:
		return s.revokeIssuedForOrder(ctx, changed.OrderID)
	}
	return nil
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
	return s.revokeIssuedForOrder(ctx, cancelled.OrderID)
}

// Zeroes and disables the cards an order bought, with an adjust row each so the ledger shows where the balance went - idempotent, since a revoked card
// has nothing left to take. What was already spent from a card can't be taken back here, so it's logged for an admin to settle
func (s *Service) revokeIssuedForOrder(ctx context.Context, orderID uuid.UUID) error {
	type revocation struct {
		card  GiftCard
		taken int64
	}
	var revoked []revocation
	err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		issued, err := r.ListByIssuedOrder(ctx, orderID)
		if err != nil {
			return fmt.Errorf("list gift cards issued by order: %w", err)
		}
		sort.Slice(issued, func(i, j int) bool { return issued[i].ID.String() < issued[j].ID.String() })
		revoked = revoked[:0]
		for _, c := range issued {
			if !c.Purchased() {
				continue
			}
			card, err := r.GetByIDForUpdate(ctx, c.ID)
			if err != nil {
				return fmt.Errorf("lock gift card %s: %w", c.ID, err)
			}
			if card.Status == StatusDisabled && card.Balance == 0 {
				continue
			}
			if spent := card.InitialBalance - card.Balance; spent > 0 {
				log.Printf("[giftcard] card %s from revoked order %s had %d already spent - settle it by hand", card.ID, orderID, spent)
			}
			out, err := r.UpdateBalance(ctx, card.ID, 0, StatusDisabled)
			if err != nil {
				return err
			}
			if card.Balance > 0 {
				if _, err := r.InsertTransaction(ctx, Transaction{
					GiftCardID: card.ID, Amount: -card.Balance, Reason: ReasonAdjust, OrderID: &orderID,
					Note: "purchasing order cancelled or refunded - card revoked",
				}); err != nil {
					return err
				}
			}
			revoked = append(revoked, revocation{out, card.Balance})
		}
		return nil
	})
	if err != nil {
		log.Printf("[giftcard] revoke cards issued by order %s: %v", orderID, err)
		return err
	}
	for _, rv := range revoked {
		s.publish(ctx, &Adjusted{GiftCardID: rv.card.ID, Code: rv.card.Code, Currency: rv.card.CurrencyCode, Delta: -rv.taken, Balance: 0, Note: "revoked"})
	}
	return nil
}

// restoreForOrder puts back the balance an order spent when that order is cancelled. Gift cards were the only tender with no cancel path - store credit, B2B credit and loyalty all reverse - so a cancelled order left the customer's card depleted with no refund row and no admin remedy short of a manual adjustment. Idempotent: a positive order-linked row means the restore already ran. fraction restores only the UNSHIPPED share of what the order spent
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

// Mints one card per unit of each gift-card line once the order is paid for and still standing - idempotent, the pre-check skipping an order
// that has its cards and the per-card issue ref collapsing two deliveries racing past it onto the same cards
func (s *Service) issueForOrder(ctx context.Context, orderID uuid.UUID) error {
	if s.orders == nil {
		return nil
	}
	o, err := s.orders.GiftcardLines(ctx, orderID)
	if err != nil {
		log.Printf("[giftcard] resolve gift-card lines for order %s: %v", orderID, err)
		return err
	}
	if len(o.Lines) == 0 || o.Status == order.OrderStatusCancelled || !order.PaidFor(o.PaymentStatus) {
		return nil
	}
	existing, err := s.repo.ListByIssuedOrder(ctx, orderID)
	if err != nil {
		log.Printf("[giftcard] check issued for order %s: %v", orderID, err)
		return err
	}
	for _, c := range existing {
		if c.Purchased() {
			return nil // a refund card on the order says nothing about whether its purchases were minted
		}
	}
	// Issue every card for the order in one transaction, so a mid-way crash rolls back cleanly and the replay re-issues the whole set rather than leaving a partial one the idempotency check would then skip
	var issued []GiftCard
	err = s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		issued = issued[:0]
		n := 0
		for _, l := range o.Lines {
			for range max(int(l.Quantity), 1) {
				n++
				card, err := issueTx(ctx, r, IssueInput{
					CurrencyCode:   o.Currency,
					Amount:         l.UnitPrice,
					RecipientEmail: o.Email,
					CustomerID:     o.CustomerID,
					IssuedOrderID:  &orderID,
					ChannelID:      &o.ChannelID,
					IssueRef:       fmt.Sprintf("%s%s:%d", purchaseRefPrefix, orderID, n),
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
		log.Printf("[giftcard] issue for order %s failed (will retry on replay): %v", orderID, err)
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
