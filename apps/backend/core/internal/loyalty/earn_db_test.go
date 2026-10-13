package loyalty

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

type orderState struct {
	mu  sync.Mutex
	o   order.Order
	err error
}

func (s *orderState) GetByID(context.Context, uuid.UUID) (order.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.o, s.err
}

func (s *orderState) set(status, payment string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.o.Status, s.o.PaymentStatus = status, payment
}

func earnFixture(t *testing.T, currency string, subtotal int64) (*Service, *orderState, uuid.UUID, uuid.UUID) {
	t.Helper()
	conn := testDB(t)
	svc, _, customerID, channelID := newFixture(t, conn)
	setProgram(t, conn, channelID, true, 1)
	orderID := seedOrder(t, svc, channelID)
	st := &orderState{o: order.Order{
		ID: orderID, CustomerID: &customerID, ChannelID: channelID, Number: "1001",
		Status: "pending", PaymentStatus: order.PaymentUnpaid, SubtotalAmount: subtotal, CurrencyCode: currency,
	}}
	svc.SetOrderLookup(st)
	return svc, st, customerID, orderID
}

func placed(svc *Service, orderID uuid.UUID) error {
	return svc.onOrderPlaced(context.Background(), &order.OrderPlaced{OrderID: orderID})
}

func paid(svc *Service, orderID uuid.UUID, status string) error {
	return svc.onPaymentStatusChanged(context.Background(), &order.OrderPaymentStatusChanged{OrderID: orderID, Status: status})
}

// Earned at placement, the points redeemed into credit before anyone paid - spend it, never pay, and the cancel clawback could only take back what was left
func TestPointsWaitForThePayment(t *testing.T) {
	svc, st, _, orderID := earnFixture(t, "USD", 2500)
	if err := placed(svc, orderID); err != nil {
		t.Fatal(err)
	}
	if got := orderPoints(t, svc, orderID); got != 0 {
		t.Fatalf("an unpaid order earned %d points at placement", got)
	}
	st.set("pending", order.PaymentPartiallyCaptured)
	if err := paid(svc, orderID, order.PaymentPartiallyCaptured); err != nil {
		t.Fatal(err)
	}
	if got := orderPoints(t, svc, orderID); got != 0 {
		t.Fatalf("a deposit earned %d points", got)
	}
	st.set("pending", order.PaymentPaid)
	for range 3 {
		if err := paid(svc, orderID, order.PaymentPaid); err != nil {
			t.Fatal(err)
		}
	}
	if err := placed(svc, orderID); err != nil {
		t.Fatal(err)
	}
	if got := orderPoints(t, svc, orderID); got != 25 {
		t.Fatalf("after payment and replays: %d points, want 25 once", got)
	}
}

func TestALatePaymentDoesNotEarnOnACancelledOrder(t *testing.T) {
	svc, st, _, orderID := earnFixture(t, "USD", 2500)
	st.set(order.OrderStatusCancelled, order.PaymentPaid)
	if err := paid(svc, orderID, order.PaymentPaid); err != nil {
		t.Fatal(err)
	}
	if got := orderPoints(t, svc, orderID); got != 0 {
		t.Fatalf("a cancelled order earned %d points", got)
	}
}

// Deliveries are dispatched concurrently - the unlocked "already earned?" read let each of them mint
func TestRacingDeliveriesEarnOnce(t *testing.T) {
	svc, st, _, orderID := earnFixture(t, "USD", 2500)
	st.set("pending", order.PaymentAuthorized)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			<-start
			if err := paid(svc, orderID, order.PaymentAuthorized); err != nil {
				t.Error(err)
			}
		})
	}
	close(start)
	wg.Wait()
	if got := orderPoints(t, svc, orderID); got != 25 {
		t.Fatalf("eight racing deliveries earned %d points, want 25", got)
	}
}

// Points are per whole unit of the order's own currency - a flat /100 earned yen at a hundredth of the rate and dinars at ten times it
func TestEarningCountsTheCurrencysOwnUnits(t *testing.T) {
	for _, tc := range []struct {
		currency string
		subtotal int64
		want     int64
	}{
		{"USD", 1599, 15},
		{"JPY", 1500, 1500},
		{"KWD", 2500, 2},
	} {
		t.Run(tc.currency, func(t *testing.T) {
			svc, st, _, orderID := earnFixture(t, tc.currency, tc.subtotal)
			st.set("pending", order.PaymentPaid)
			if err := paid(svc, orderID, order.PaymentPaid); err != nil {
				t.Fatal(err)
			}
			if got := orderPoints(t, svc, orderID); got != tc.want {
				t.Errorf("%s %d: %d points, want %d", tc.currency, tc.subtotal, got, tc.want)
			}
		})
	}
}

func TestAVoidedAuthorizationTakesThePointsBack(t *testing.T) {
	svc, st, _, orderID := earnFixture(t, "USD", 2500)
	st.set("pending", order.PaymentAuthorized)
	if err := paid(svc, orderID, order.PaymentAuthorized); err != nil {
		t.Fatal(err)
	}
	st.set("pending", order.PaymentVoided)
	if err := paid(svc, orderID, order.PaymentVoided); err != nil {
		t.Fatal(err)
	}
	if got := orderPoints(t, svc, orderID); got != 0 {
		t.Fatalf("after the void the order still nets %d points", got)
	}
}

// The clawback of already-redeemed points is the guard against minting credit - a failed lookup must retry, not skip it
func TestARevokeWhoseOrderLookupFailsIsRetried(t *testing.T) {
	svc, st, customerID, orderID := earnFixture(t, "USD", 2500)
	seedEarn(t, svc, customerID, orderID, 25)
	st.err = errors.New("db blip")
	if err := svc.onOrderCancelled(context.Background(), &order.OrderCancelled{OrderID: orderID, RefundableFraction: 1}); err == nil {
		t.Fatal("the revoke reported success without its lookup")
	}
	if got := orderPoints(t, svc, orderID); got != 25 {
		t.Fatalf("a failed revoke still moved the ledger to %d", got)
	}
}
