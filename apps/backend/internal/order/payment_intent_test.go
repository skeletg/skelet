package order

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/tx"
)

// The over-capture and over-refund guards, exercised THROUGH recordPaymentIntent
//
// payment_ledger_test.go already tables these cases, but it restates the comparison inside the test
// rather than calling the service - so deleting both guards from payment.go leaves that file
// passing. These call the real thing, so the arithmetic and the guard are pinned together

type intentRepo struct {
	Repository
	totals   PaymentTotals
	inserted []PaymentTransaction
	locked   []uuid.UUID
}

func (r *intentRepo) WithTx(*sql.Tx) Repository { return r }

func (r *intentRepo) LockOrderForPayment(_ context.Context, orderID uuid.UUID) error {
	r.locked = append(r.locked, orderID)
	return nil
}

func (r *intentRepo) PaymentTotalsActiveByKind(context.Context, uuid.UUID) (PaymentTotals, error) {
	return r.totals, nil
}

func (r *intentRepo) InsertPaymentTransaction(_ context.Context, t PaymentTransaction) (PaymentTransaction, error) {
	t.ID = uuid.New()
	r.inserted = append(r.inserted, t)
	return t, nil
}

func newIntentSvc(t *testing.T, totals PaymentTotals) (*Service, *intentRepo) {
	t.Helper()
	repo := &intentRepo{totals: totals}
	return &Service{repo: repo, tx: newOrderNoopTxManager(t)}, repo
}

// A refund may never exceed what was actually captured - over-refunding hands back money the
// merchant never took, and the gateway will happily do it
func TestRefundIntentGuard(t *testing.T) {
	cases := []struct {
		name   string
		totals PaymentTotals
		refund int64
		wantOK bool
	}{
		{"nothing captured rejects any refund", PaymentTotals{Authorized: 10000}, 1, false},
		{"refund up to captured", PaymentTotals{Captured: 10000}, 10000, true},
		{"one cent past captured rejected", PaymentTotals{Captured: 10000}, 10001, false},
		{"partial refund inside the remainder", PaymentTotals{Captured: 10000, Refunded: 9000}, 1000, true},
		{"partial refund past the remainder rejected", PaymentTotals{Captured: 10000, Refunded: 9000}, 1001, false},
		// pending rows count toward Refunded, which is what stops two concurrent full refunds
		{"an in-flight refund blocks a duplicate", PaymentTotals{Captured: 10000, Refunded: 10000}, 10000, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, repo := newIntentSvc(t, c.totals)
			_, err := svc.recordPaymentIntent(context.Background(), uuid.New(), PaymentTxnRefund, c.refund, "stripe", "test")

			if c.wantOK {
				if err != nil {
					t.Fatalf("refund %d against %+v was refused: %v", c.refund, c.totals, err)
				}
				if len(repo.inserted) != 1 || repo.inserted[0].Kind != PaymentTxnRefund {
					t.Fatalf("no refund row written: %+v", repo.inserted)
				}
				return
			}
			if err == nil {
				t.Fatalf("refund %d against %+v was ALLOWED - money was returned that was never taken", c.refund, c.totals)
			}
			if !strings.Contains(err.Error(), "captured remaining") {
				t.Errorf("error = %v, want the over-refund guard", err)
			}
			if len(repo.inserted) != 0 {
				t.Errorf("a refused refund still wrote %d ledger rows", len(repo.inserted))
			}
		})
	}
}

// A capture may never exceed what was authorized - the inverse failure, and the one that shipped:
// with no authorize row the FIRST capture was rejected and goods went out unpaid
func TestCaptureIntentGuard(t *testing.T) {
	cases := []struct {
		name    string
		totals  PaymentTotals
		capture int64
		wantOK  bool
	}{
		{"no authorize row rejects any capture", PaymentTotals{}, 1, false},
		{"capture the full authorization", PaymentTotals{Authorized: 10000}, 10000, true},
		{"one cent past authorized rejected", PaymentTotals{Authorized: 10000}, 10001, false},
		{"second capture inside the remainder", PaymentTotals{Authorized: 10000, Captured: 6000}, 4000, true},
		{"second capture past the remainder rejected", PaymentTotals{Authorized: 10000, Captured: 6000}, 4001, false},
		{"an in-flight capture blocks a duplicate", PaymentTotals{Authorized: 10000, Captured: 10000}, 10000, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, repo := newIntentSvc(t, c.totals)
			_, err := svc.RecordCaptureIntent(context.Background(), uuid.New(), c.capture, "stripe", "test")

			if c.wantOK {
				if err != nil {
					t.Fatalf("capture %d against %+v was refused: %v", c.capture, c.totals, err)
				}
				if len(repo.inserted) != 1 || repo.inserted[0].Kind != PaymentTxnCapture {
					t.Fatalf("no capture row written: %+v", repo.inserted)
				}
				return
			}
			if err == nil {
				t.Fatalf("capture %d against %+v was ALLOWED - the customer is charged past their authorization", c.capture, c.totals)
			}
			if !strings.Contains(err.Error(), "authorized remaining") {
				t.Errorf("error = %v, want the over-capture guard", err)
			}
			if len(repo.inserted) != 0 {
				t.Errorf("a refused capture still wrote %d ledger rows", len(repo.inserted))
			}
		})
	}
}

// Zero and negative amounts are refused before any total is consulted, so a bug upstream can't
// write a meaningless ledger row
func TestPaymentIntentRejectsNonPositiveAmounts(t *testing.T) {
	for _, bad := range []int64{0, -1, -10000} {
		svc, repo := newIntentSvc(t, PaymentTotals{Authorized: 10000, Captured: 10000})
		if _, err := svc.recordPaymentIntent(context.Background(), uuid.New(), PaymentTxnRefund, bad, "stripe", "test"); err == nil {
			t.Errorf("refund intent for %d was allowed", bad)
		}
		if _, err := svc.RecordCaptureIntent(context.Background(), uuid.New(), bad, "stripe", "test"); err == nil {
			t.Errorf("RecordCaptureIntent(%d) was allowed", bad)
		}
		if len(repo.inserted) != 0 {
			t.Errorf("a non-positive amount wrote %d rows", len(repo.inserted))
		}
	}
}

// The order row is locked BEFORE the totals are read - without it two concurrent intents read the
// same totals, both pass, and both reach the gateway
func TestPaymentIntentLocksTheOrderFirst(t *testing.T) {
	svc, repo := newIntentSvc(t, PaymentTotals{Authorized: 10000})
	orderID := uuid.New()

	if _, err := svc.RecordCaptureIntent(context.Background(), orderID, 5000, "stripe", "test"); err != nil {
		t.Fatalf("capture: %v", err)
	}
	if len(repo.locked) != 1 || repo.locked[0] != orderID {
		t.Fatalf("locked %v, want exactly the order being paid", repo.locked)
	}
}

// Each intent carries its own idempotency key, which is what stops a retried gateway call charging
// twice
func TestPaymentIntentWritesAnIdempotencyKey(t *testing.T) {
	svc, repo := newIntentSvc(t, PaymentTotals{Authorized: 10000})
	for range 2 {
		if _, err := svc.RecordCaptureIntent(context.Background(), uuid.New(), 1000, "stripe", "test"); err != nil {
			t.Fatalf("capture: %v", err)
		}
	}
	if len(repo.inserted) != 2 {
		t.Fatalf("wrote %d rows, want 2", len(repo.inserted))
	}
	a, b := repo.inserted[0].IdempotencyKey, repo.inserted[1].IdempotencyKey
	if a == "" || b == "" {
		t.Fatal("an intent was written with no idempotency key - a retry could double-charge")
	}
	if a == b {
		t.Errorf("two intents shared idempotency key %q - the gateway would drop the second", a)
	}
	if repo.inserted[0].Status != PaymentTxnPending {
		t.Errorf("intent status = %q, want pending", repo.inserted[0].Status)
	}
}

// --- in-memory tx manager, mirroring internal/inventory's ---

type orderNoopDriver struct{}
type orderNoopConn struct{}
type orderNoopTx struct{}

func (orderNoopDriver) Open(string) (driver.Conn, error)  { return orderNoopConn{}, nil }
func (orderNoopConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (orderNoopConn) Close() error                        { return nil }
func (orderNoopConn) Begin() (driver.Tx, error)           { return orderNoopTx{}, nil }
func (orderNoopTx) Commit() error                         { return nil }
func (orderNoopTx) Rollback() error                       { return nil }

var registerOrderNoopDriver sync.Once

func newOrderNoopTxManager(t *testing.T) *tx.Manager {
	t.Helper()
	registerOrderNoopDriver.Do(func() { sql.Register("order-noop", orderNoopDriver{}) })
	db, err := sql.Open("order-noop", "")
	if err != nil {
		t.Fatalf("open noop db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return tx.New(db)
}
