package giftcard

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type lookup struct {
	mu    sync.Mutex
	state OrderGiftcards
}

func (l *lookup) GiftcardLines(context.Context, uuid.UUID) (OrderGiftcards, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state, nil
}

func (l *lookup) set(status, payment string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.state.Status, l.state.PaymentStatus = status, payment
}

type rig struct {
	conn    *sql.DB
	svc     *Service
	orders  *lookup
	orderID uuid.UUID
	channel uuid.UUID
}

func newRig(t *testing.T) rig {
	t.Helper()
	conn := testdb.Require(t)
	var orderID, channelID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO orders (channel_id, currency_code) SELECT id, 'USD' FROM channels ORDER BY created_at LIMIT 1 RETURNING id, channel_id`).Scan(&orderID, &channelID); err != nil {
		t.Skipf("no channel to place an order on: %v", err)
	}
	l := &lookup{state: OrderGiftcards{Status: "pending", PaymentStatus: order.PaymentUnpaid, Email: "buyer@example.test", ChannelID: channelID,
		Currency: "USD", Lines: []IssuableLine{{UnitPrice: 2500, Quantity: 2}}}}
	svc := NewService(NewRepository(db.New(conn)), txrunner.New(conn))
	svc.SetOrderLookup(l)
	return rig{conn: conn, svc: svc, orders: l, orderID: orderID, channel: channelID}
}

func (r rig) purchased(t *testing.T) []GiftCard {
	t.Helper()
	cards, err := r.svc.repo.ListByIssuedOrder(context.Background(), r.orderID)
	if err != nil {
		t.Fatal(err)
	}
	var out []GiftCard
	for _, c := range cards {
		if c.Purchased() {
			out = append(out, c)
		}
	}
	return out
}

func (r rig) payment(t *testing.T, status string) {
	t.Helper()
	if err := r.svc.onPaymentStatusChanged(context.Background(), &order.OrderPaymentStatusChanged{OrderID: r.orderID, Status: status}); err != nil {
		t.Fatal(err)
	}
}

// Cash on delivery places unpaid - a card minted then was spendable before anyone paid for it
func TestAnUnpaidOrderMintsNoCardUntilPaymentIsSecured(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if err := r.svc.onOrderPlaced(ctx, &order.OrderPlaced{OrderID: r.orderID}); err != nil {
		t.Fatal(err)
	}
	if n := len(r.purchased(t)); n != 0 {
		t.Fatalf("an unpaid order minted %d cards", n)
	}
	r.orders.set("pending", order.PaymentPartiallyCaptured)
	r.payment(t, order.PaymentPartiallyCaptured)
	if n := len(r.purchased(t)); n != 0 {
		t.Fatalf("a deposit minted %d cards", n)
	}
	r.orders.set("pending", order.PaymentPaid)
	r.payment(t, order.PaymentPaid)
	if err := r.svc.onOrderPlaced(ctx, &order.OrderPlaced{OrderID: r.orderID}); err != nil {
		t.Fatal(err)
	}
	cards := r.purchased(t)
	if len(cards) != 2 || cards[0].Balance != 2500 || cards[0].RecipientEmail != "buyer@example.test" {
		t.Fatalf("cards after payment and a replay: %+v, want two 2500 cards", cards)
	}
}

// The pre-check reads before it writes, so two deliveries can both pass it - the per-card issue ref makes them mint the same cards
func TestRacingIssuancesMintEachCardOnce(t *testing.T) {
	r := newRig(t)
	r.orders.set("pending", order.PaymentAuthorized)
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() { _ = r.svc.issueForOrder(context.Background(), r.orderID) }) // losers may error and replay
	}
	wg.Wait()
	if err := r.svc.issueForOrder(context.Background(), r.orderID); err != nil {
		t.Fatal(err)
	}
	if n := len(r.purchased(t)); n != 2 {
		t.Fatalf("%d cards from racing deliveries, want 2", n)
	}
}

func (r rig) refundCard(t *testing.T) GiftCard {
	t.Helper()
	c, err := r.svc.Issue(context.Background(), IssueInput{CurrencyCode: "USD", Amount: 900, IssuedOrderID: &r.orderID, IssueRef: "memo:" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Cancelling a purchase takes its cards back - but a refund paid out as a gift card carries the same order, and must survive
func TestCancellingTheOrderRevokesItsCardsButNotItsRefundCard(t *testing.T) {
	for _, tc := range []struct {
		name   string
		revoke func(r rig) error
	}{
		{"cancelled", func(r rig) error {
			return r.svc.onOrderCancelled(context.Background(), &order.OrderCancelled{OrderID: r.orderID, RefundableFraction: 1})
		}},
		{"refunded", func(r rig) error {
			return r.svc.onPaymentStatusChanged(context.Background(), &order.OrderPaymentStatusChanged{OrderID: r.orderID, Status: order.PaymentRefunded})
		}},
		{"voided", func(r rig) error {
			return r.svc.onPaymentStatusChanged(context.Background(), &order.OrderPaymentStatusChanged{OrderID: r.orderID, Status: order.PaymentVoided})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.orders.set("pending", order.PaymentPaid)
			r.payment(t, order.PaymentPaid)
			refund := r.refundCard(t)
			if err := tc.revoke(r); err != nil {
				t.Fatal(err)
			}
			if err := tc.revoke(r); err != nil { // replay
				t.Fatal(err)
			}
			for _, c := range r.purchased(t) {
				if c.Balance != 0 || c.Status != StatusDisabled {
					t.Fatalf("a %s order's card is %s with %d left", tc.name, c.Status, c.Balance)
				}
				txs, _ := r.svc.Transactions(context.Background(), c.ID)
				if len(txs) != 2 || txs[1].Amount != -2500 || txs[1].Reason != ReasonAdjust {
					t.Fatalf("ledger %+v, want the issue and one revoking adjust", txs)
				}
			}
			if got, _ := r.svc.GetByID(context.Background(), refund.ID); got.Balance != 900 || got.Status != StatusActive {
				t.Fatalf("the refund card was revoked with the purchase: %s, %d left", got.Status, got.Balance)
			}
		})
	}
}

// A late payment after a cancel must not hand fresh cards out
func TestALatePaymentMintsNothingForACancelledOrder(t *testing.T) {
	r := newRig(t)
	r.orders.set(order.OrderStatusCancelled, order.PaymentPaid)
	r.payment(t, order.PaymentPaid)
	if n := len(r.purchased(t)); n != 0 {
		t.Fatalf("a cancelled order minted %d cards", n)
	}
}

// A checkout holds the card's row lock while it spends - an admin action reading around that lock wrote the old balance back
func TestAdminActionsWaitForASpendInFlight(t *testing.T) {
	for _, tc := range []struct {
		name   string
		act    func(s *Service, id uuid.UUID) error
		status string
		want   int64
	}{
		{"adjust", func(s *Service, id uuid.UUID) error {
			_, err := s.Adjust(context.Background(), id, 100, "goodwill")
			return err
		}, StatusActive, 500},
		{"disable", func(s *Service, id uuid.UUID) error { _, err := s.Disable(context.Background(), id); return err }, StatusDisabled, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			ctx := context.Background()
			card, err := r.svc.Issue(ctx, IssueInput{CurrencyCode: "USD", Amount: 1000})
			if err != nil {
				t.Fatal(err)
			}
			tx, err := r.conn.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.svc.RedeemUpToTx(ctx, tx, card.Code, r.channel, 600, "USD", r.orderID); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- tc.act(r.svc, card.ID) }()
			time.Sleep(150 * time.Millisecond) // the admin action reaches the card while the spend still holds it
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			got, _ := r.svc.GetByID(ctx, card.ID)
			if got.Balance != tc.want || got.Status != tc.status {
				t.Fatalf("after a 600 spend and an admin %s: %s with %d, want %s with %d", tc.name, got.Status, got.Balance, tc.status, tc.want)
			}
		})
	}
}

func TestACodeIsFoundHoweverItIsTyped(t *testing.T) {
	r := newRig(t)
	card, err := r.svc.Issue(context.Background(), IssueInput{CurrencyCode: "USD", Amount: 1000})
	if err != nil {
		t.Fatal(err)
	}
	bare := strings.ReplaceAll(card.Code, "-", "")
	for _, typed := range []string{card.Code, strings.ToLower(card.Code), " " + bare + " ", strings.ToLower(bare[:6]) + " " + bare[6:]} {
		if got, err := r.svc.GetByCode(context.Background(), typed); err != nil || got.ID != card.ID {
			t.Errorf("GetByCode(%q) = %v, %v", typed, got.ID, err)
		}
	}
}

// The gift card list pages on the server, so search has to reach the database and the total has to follow it
func TestPageSearchesCodeAndRecipient(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	email := "page-" + uuid.NewString()[:8] + "@example.test"
	card, err := r.svc.Issue(ctx, IssueInput{CurrencyCode: "USD", Amount: 1000, RecipientEmail: email, ChannelID: &r.channel})
	if err != nil {
		t.Fatal(err)
	}
	for _, search := range []string{email[:13], strings.ToLower(card.Code)} {
		got, total, err := r.svc.Page(ctx, nil, search, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || len(got) != 1 || got[0].ID != card.ID {
			t.Errorf("Page(%q) = %d rows of %d, want just the card", search, len(got), total)
		}
	}
	if got, total, _ := r.svc.Page(ctx, []uuid.UUID{}, "", 10, 0); len(got) != 0 || total != 0 {
		t.Errorf("no permitted channels paged %d cards of %d", len(got), total)
	}
}
