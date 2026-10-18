package ordercapture

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

type fakeOrders struct {
	o        order.Order
	getErr   error
	intent   order.PaymentTransaction
	intentEr error

	intents   int
	key       string
	finalized []bool
	failures  int
}

func (f *fakeOrders) GetByID(context.Context, uuid.UUID) (order.Order, error) {
	return f.o, f.getErr
}
func (f *fakeOrders) RecordCaptureIntentKeyed(_ context.Context, _ uuid.UUID, _ int64, _, _, key string) (order.PaymentTransaction, error) {
	f.intents++
	f.key = key
	return f.intent, f.intentEr
}
func (f *fakeOrders) FinalizePaymentTransaction(_ context.Context, _ uuid.UUID, ok bool, _ string) (order.Order, error) {
	f.finalized = append(f.finalized, ok)
	return f.o, nil
}
func (f *fakeOrders) RecordCaptureFailure(context.Context, uuid.UUID, string, error) { f.failures++ }

type fakePayments struct {
	err      error
	captures int
	lastReq  payment.CaptureRequest
}

func (f *fakePayments) Capture(_ context.Context, _ string, req payment.CaptureRequest) (payment.CaptureResult, error) {
	f.captures++
	f.lastReq = req
	if f.err != nil {
		return payment.CaptureResult{}, f.err
	}
	return payment.CaptureResult{ProviderReference: "cap_1"}, nil
}

func authorized() order.Order {
	return order.Order{
		ID:               uuid.New(),
		PaymentStatus:    order.PaymentAuthorized,
		PaymentProvider:  "stripe",
		PaymentReference: "pi_1",
		TotalAmount:      5000,
	}
}

func fire(t *testing.T, o *fakeOrders, p *fakePayments) error {
	t.Helper()
	bus := events.NewBus()
	Register(bus, o, p)
	return bus.DispatchAsyncNow(context.Background(), &order.OrderFulfillmentCreated{OrderID: uuid.New()})
}

// Only an authorized order with a live provider reference may be captured -
// anything else and we would be charging against a hold that isn't there
func TestCaptureGates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*order.Order)
	}{
		{"not authorized", func(o *order.Order) { o.PaymentStatus = order.PaymentPaid }},
		{"no provider", func(o *order.Order) { o.PaymentProvider = "" }},
		{"no provider reference", func(o *order.Order) { o.PaymentReference = "" }},
		{"nothing due", func(o *order.Order) { o.TotalAmount = 0 }},
		{"fully paid by tenders", func(o *order.Order) { o.GiftCardAmount, o.StoreCreditAmount = 3000, 2000 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := authorized()
			tc.mutate(&o)
			orders, payments := &fakeOrders{o: o}, &fakePayments{}
			_ = fire(t, orders, payments)
			if payments.captures != 0 {
				t.Errorf("captured %d times, want 0", payments.captures)
			}
			if orders.intents != 0 {
				t.Errorf("recorded %d intents, want 0", orders.intents)
			}
		})
	}
}

// A failed order load must not capture - we have no amount and no reference to
// capture against
func TestLoadFailureDoesNotCapture(t *testing.T) {
	orders := &fakeOrders{getErr: errors.New("gone")}
	payments := &fakePayments{}
	_ = fire(t, orders, payments)
	if payments.captures != 0 || orders.intents != 0 {
		t.Fatal("a failed order load must not reach the gateway")
	}
}

// The idempotency key comes from the intent row, so a redelivered fulfillment
// event cannot double-charge
func TestCapturePassesIdempotencyKeyFromIntent(t *testing.T) {
	o := authorized()
	o.GiftCardAmount, o.StoreCreditAmount = 1000, 500
	orders := &fakeOrders{o: o, intent: order.PaymentTransaction{ID: uuid.New(), IdempotencyKey: "idem_42"}}
	payments := &fakePayments{}
	_ = fire(t, orders, payments)
	if payments.captures != 1 {
		t.Fatalf("captures = %d, want 1", payments.captures)
	}
	if payments.lastReq.IdempotencyKey != "idem_42" {
		t.Errorf("idempotency key = %q, want the one from the intent row", payments.lastReq.IdempotencyKey)
	}
	if payments.lastReq.AmountCents != 3500 {
		t.Errorf("captured %d, want 5000 - 1000 gift card - 500 store credit = 3500", payments.lastReq.AmountCents)
	}
	if payments.lastReq.AmountCents != o.AmountDue() {
		t.Errorf("captured %d, want AmountDue() %d - AmountDue nets BOTH tenders", payments.lastReq.AmountCents, o.AmountDue())
	}
	if len(orders.finalized) != 1 || !orders.finalized[0] {
		t.Errorf("finalized = %v, want one success", orders.finalized)
	}
	if orders.failures != 0 {
		t.Errorf("recorded %d failures on a successful capture", orders.failures)
	}
}

// Both failure paths must land on the order timeline - a capture that fails after the goods shipped is money the store
// never collects, and a log line is not a signal anyone sees
func TestFailurePathsRecordOnTheOrder(t *testing.T) {
	t.Run("intent refused", func(t *testing.T) {
		orders := &fakeOrders{o: authorized(), intentEr: apperr.New("PAYMENT_OVER_CAPTURE", apperr.ClassConflict, "capture exceeds the authorized remaining")}
		payments := &fakePayments{}
		if err := fire(t, orders, payments); err != nil {
			t.Errorf("a refusal asked for a replay (%v) - it gets the same answer every time", err)
		}
		if orders.failures != 1 {
			t.Errorf("failures = %d, want 1", orders.failures)
		}
		if payments.captures != 0 {
			t.Error("must not reach the gateway without an intent row")
		}
	})
	t.Run("intent fails on infrastructure", func(t *testing.T) {
		orders := &fakeOrders{o: authorized(), intentEr: errors.New("db down")}
		payments := &fakePayments{}
		if err := fire(t, orders, payments); err == nil {
			t.Error("a database failure returned nil, so the outbox never retried the capture")
		}
		if orders.failures != 0 {
			t.Errorf("a retryable failure wrote %d timeline entries - one per replay", orders.failures)
		}
	})
	t.Run("gateway declines", func(t *testing.T) {
		orders := &fakeOrders{o: authorized(), intent: order.PaymentTransaction{ID: uuid.New(), IdempotencyKey: "k"}}
		payments := &fakePayments{err: errors.New("declined")}
		_ = fire(t, orders, payments)
		if orders.failures != 1 {
			t.Errorf("failures = %d, want 1", orders.failures)
		}
		if len(orders.finalized) != 1 || orders.finalized[0] {
			t.Errorf("finalized = %v, want one failure - the txn must still be closed out", orders.finalized)
		}
	})
}

// One key per order - a replay finds the intent it started and resumes it, rather than minting a second one the pending first blocks
func TestAReplayResumesItsOwnIntent(t *testing.T) {
	o := authorized()
	orders := &fakeOrders{o: o, intent: order.PaymentTransaction{ID: uuid.New(), IdempotencyKey: "capture:fulfill:x", Status: order.PaymentTxnPending}}
	payments := &fakePayments{}
	if err := fire(t, orders, payments); err != nil {
		t.Fatal(err)
	}
	if orders.key != "capture:fulfill:"+o.ID.String() {
		t.Errorf("intent key = %q, want the per-order capture key", orders.key)
	}
	if payments.captures != 1 || payments.lastReq.IdempotencyKey != "capture:fulfill:x" {
		t.Errorf("captures %d with key %q, want the pending intent resumed under its own key", payments.captures, payments.lastReq.IdempotencyKey)
	}

	done := &fakeOrders{o: o, intent: order.PaymentTransaction{ID: uuid.New(), Status: order.PaymentTxnSucceeded}}
	again := &fakePayments{}
	if err := fire(t, done, again); err != nil {
		t.Fatal(err)
	}
	if again.captures != 0 || len(done.finalized) != 0 {
		t.Error("a capture an earlier delivery finished was charged or finalized again")
	}
}
