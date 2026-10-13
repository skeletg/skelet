package order

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"

	dbsqlc "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func moneySvc(conn *sql.DB) *Service {
	return &Service{repo: NewRepository(dbsqlc.New(conn)), tx: txrunner.New(conn)}
}

// A confirmed order with one line of qty 2 at 1000 and the given payment status, totalled 2000
func seedPaidOrder(t *testing.T, conn *sql.DB, status, paymentStatus string, captured int64) (uuid.UUID, uuid.UUID) {
	t.Helper()
	orderID := seedConfirmedOrder(t, conn)
	if _, err := conn.Exec(`UPDATE orders SET status = $2, payment_status = $3, subtotal_amount = 2000, total_amount = 2000, payment_provider = 'gw', payment_reference = 'auth-1' WHERE id = $1`,
		orderID, status, paymentStatus); err != nil {
		t.Fatal(err)
	}
	var lineID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO order_lines (order_id, product_name, product_sku, unit_price, quantity, total_price)
		VALUES ($1, 'fixture', 'MP-1', 1000, 2, 2000) RETURNING id`, orderID).Scan(&lineID); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{PaymentTxnAuthorize, PaymentTxnCapture} {
		if captured == 0 {
			break
		}
		if _, err := conn.Exec(`INSERT INTO order_payment_transactions (order_id, kind, amount_cents, status, provider_slug, idempotency_key, reason)
			VALUES ($1, $2, $3, 'succeeded', 'gw', $4, 'fixture')`, orderID, kind, captured, kind+":"+uuid.NewString()); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM order_payment_transactions WHERE order_id = $1`, orderID)
		_, _ = conn.Exec(`DELETE FROM credit_memos WHERE order_id = $1`, orderID)
	})
	return orderID, lineID
}

func conflictCode(err error) string {
	if ae, ok := apperr.AsAppError(err); ok {
		return ae.Code
	}
	return ""
}

// The memo was written before the refund moved money - a declined refund left it counting as credited, so the same units could never be refunded again
func TestADeclinedRefundFreesItsUnits(t *testing.T) {
	conn := statusTestDB(t)
	svc := moneySvc(conn)
	ctx := context.Background()
	orderID, lineID := seedPaidOrder(t, conn, OrderStatusFulfilled, PaymentPaid, 2000)
	in := IssueCreditMemoInput{OrderID: orderID, Lines: []CreditMemoLineInput{{OrderLineID: lineID, Quantity: 2}}}

	intent, err := svc.RecordCreditMemoIntent(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.FinalizeCreditMemo(ctx, intent.Txn.ID, false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordCreditMemoIntent(ctx, in); err != nil {
		t.Fatalf("retrying the declined refund = %v, want the units still creditable", err)
	}
	memos, err := svc.repo.ListCreditMemos(ctx, orderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(memos) != 1 {
		t.Errorf("the order lists %d memos, want only the live retry - the failed one is no credit", len(memos))
	}
}

// A second finalize used to flip a succeeded refund to failed, which freed its ceiling for the same money to go out again
func TestASettledTransactionCannotBeFlipped(t *testing.T) {
	conn := statusTestDB(t)
	svc := moneySvc(conn)
	ctx := context.Background()
	orderID, lineID := seedPaidOrder(t, conn, OrderStatusFulfilled, PaymentPaid, 2000)
	intent, err := svc.RecordCreditMemoIntent(ctx, IssueCreditMemoInput{OrderID: orderID, Lines: []CreditMemoLineInput{{OrderLineID: lineID, Quantity: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.FinalizeCreditMemo(ctx, intent.Txn.ID, true, "re-1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.FinalizeCreditMemo(ctx, intent.Txn.ID, false, ""); conflictCode(err) != "PAYMENT_ALREADY_SETTLED" {
		t.Fatalf("failing a succeeded refund = %v, want PAYMENT_ALREADY_SETTLED", err)
	}
	if _, _, err := svc.FinalizeCreditMemo(ctx, intent.Txn.ID, true, "re-1"); err != nil {
		t.Fatalf("a repeated success = %v, want a no-op", err)
	}
	var status string
	if err := conn.QueryRow(`SELECT status FROM order_payment_transactions WHERE id = $1`, intent.Txn.ID).Scan(&status); err != nil || status != PaymentTxnSucceeded {
		t.Fatalf("the refund row is %q (%v), want still succeeded", status, err)
	}
}

// Reads the order as the caller saw it, then lets a webhook settle it before the caller writes
type staleRead struct {
	Repository
	conn  *sql.DB
	fired bool
}

func (s *staleRead) GetOrderByID(ctx context.Context, id uuid.UUID) (Order, error) {
	o, err := s.Repository.GetOrderByID(ctx, id)
	if !s.fired {
		s.fired = true
		_, _ = s.conn.Exec(`UPDATE orders SET payment_status = 'paid' WHERE id = $1`, id)
	}
	return o, err
}

// The sync sweep and a webhook settled the same order at once - the slower one wrote authorized over paid
func TestASlowSettleDoesNotDowngradeAPaidOrder(t *testing.T) {
	conn := statusTestDB(t)
	orderID, _ := seedPaidOrder(t, conn, OrderStatusConfirmed, PaymentUnpaid, 0)
	svc := &Service{repo: &staleRead{Repository: NewRepository(dbsqlc.New(conn)), conn: conn}}
	got, err := svc.SettlePayment(context.Background(), orderID, PaymentAuthorized, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.PaymentStatus != PaymentPaid {
		t.Fatalf("payment status = %q, want paid kept", got.PaymentStatus)
	}
}

func TestACancelledOrderTakesNoCash(t *testing.T) {
	conn := statusTestDB(t)
	orderID, _ := seedPaidOrder(t, conn, OrderStatusCancelled, PaymentUnpaid, 0)
	_, err := moneySvc(conn).RecordOfflinePayment(context.Background(), RecordOfflinePaymentInput{OrderID: orderID, Method: "cod"})
	if conflictCode(err) != "PAYMENT_NOT_COLLECTABLE" {
		t.Fatalf("cash on a cancelled order = %v, want PAYMENT_NOT_COLLECTABLE", err)
	}
}

// A per-order key made a re-authorization after an expired hold a replay of the first, so the new hold was never recorded
func TestAReauthorizationIsRecorded(t *testing.T) {
	conn := statusTestDB(t)
	svc := moneySvc(conn)
	ctx := context.Background()
	orderID, _ := seedPaidOrder(t, conn, OrderStatusConfirmed, PaymentUnpaid, 0)
	for _, ref := range []string{"auth-1", "auth-1", "auth-2"} {
		if _, err := svc.RecordAuthorization(ctx, orderID, 2000, "gw", ref); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM order_payment_transactions WHERE order_id = $1 AND kind = 'authorize'`, orderID).Scan(&n); err != nil || n != 2 {
		t.Fatalf("%d authorize rows (%v), want one per gateway hold - the replay deduped, the re-authorization kept", n, err)
	}
}

// Any pending refund used to count as the in-flight twin of a gateway echo - an unrelated refund typed into the dashboard was dropped
func TestADashboardRefundIsNotSwallowedByAnUnrelatedPendingOne(t *testing.T) {
	conn := statusTestDB(t)
	svc := moneySvc(conn)
	ctx := context.Background()
	orderID, _ := seedPaidOrder(t, conn, OrderStatusFulfilled, PaymentPaid, 2000)
	if _, err := svc.recordPaymentIntent(ctx, orderID, PaymentTxnRefund, 500, "gw", "memo refund", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SettleGatewayAmount(ctx, orderID, PaymentTxnRefund, 300, "gw", "dash-1"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM order_payment_transactions WHERE order_id = $1 AND provider_reference = 'dash-1'`, orderID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("the dashboard refund was recorded %d times (%v), want once", n, err)
	}
	if _, err := svc.SettleGatewayAmount(ctx, orderID, PaymentTxnRefund, 500, "gw", "echo-1"); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT count(*) FROM order_payment_transactions WHERE order_id = $1 AND provider_reference = 'echo-1'`, orderID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("the pending refund's own echo was recorded %d times, want 0 - it's the same money", n)
	}
}

// Parking a 3DS handle wrote back the status read at placement, over a webhook's
func TestParkingAReferenceLeavesTheStatus(t *testing.T) {
	conn := statusTestDB(t)
	orderID, _ := seedPaidOrder(t, conn, OrderStatusConfirmed, PaymentPaid, 0)
	got, err := NewRepository(dbsqlc.New(conn)).SetPaymentReference(context.Background(), orderID, "pi_3ds")
	if err != nil || got.PaymentStatus != PaymentPaid || got.PaymentReference != "pi_3ds" {
		t.Fatalf("after parking: %q %q (%v)", got.PaymentStatus, got.PaymentReference, err)
	}
}

// A replayed capture minted a fresh intent each time, and the still-pending first made every later one an over-capture
func TestAKeyedCaptureIntentResumesThePendingOne(t *testing.T) {
	conn := statusTestDB(t)
	svc := moneySvc(conn)
	ctx := context.Background()
	orderID, _ := seedPaidOrder(t, conn, OrderStatusProcessing, PaymentAuthorized, 0)
	if _, err := svc.RecordAuthorization(ctx, orderID, 2000, "gw", "auth-1"); err != nil {
		t.Fatal(err)
	}
	key := "capture:fulfill:" + orderID.String()
	first, err := svc.RecordCaptureIntentKeyed(ctx, orderID, 2000, "gw", "capture on fulfillment", key)
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.RecordCaptureIntentKeyed(ctx, orderID, 2000, "gw", "capture on fulfillment", key)
	if err != nil {
		t.Fatalf("the replay = %v, want the pending intent back", err)
	}
	if again.ID != first.ID || again.Status != PaymentTxnPending {
		t.Fatalf("replay returned %s (%s), want the first intent %s still pending", again.ID, again.Status, first.ID)
	}
}
