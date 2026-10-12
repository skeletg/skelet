package order

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	dbsqlc "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

// Hands back the return as first read for every later read - what two admins acting on the same screen both saw
type frozenReturn struct {
	Repository
	once sync.Once
	ret  OrderReturn
	conn *sql.DB
}

func (f *frozenReturn) GetReturn(ctx context.Context, id uuid.UUID) (OrderReturn, error) {
	var err error
	f.once.Do(func() {
		f.ret, err = f.Repository.GetReturn(ctx, id)
		_, _ = f.conn.Exec(`UPDATE order_returns SET status = 'received' WHERE id = $1`, id)
	})
	return f.ret, err
}

// Receive racing Reject left a restocked return rejected, which made its units returnable - and refundable - again
func TestAReturnCannotBeRejectedAfterItWasReceived(t *testing.T) {
	conn := statusTestDB(t)
	orderID := seedConfirmedOrder(t, conn)
	var returnID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO order_returns (order_id, status) VALUES ($1, 'approved') RETURNING id`, orderID).Scan(&returnID); err != nil {
		t.Fatal(err)
	}
	svc := &Service{repo: &frozenReturn{Repository: NewRepository(dbsqlc.New(conn)), conn: conn}, tx: moneySvc(conn).tx}
	if _, err := svc.RejectReturn(context.Background(), returnID); conflictCode(err) != "RETURN_STATUS_RACED" {
		t.Fatalf("rejecting a return received meanwhile = %v, want RETURN_STATUS_RACED", err)
	}
	var status string
	if err := conn.QueryRow(`SELECT status FROM order_returns WHERE id = $1`, returnID).Scan(&status); err != nil || status != ReturnReceived {
		t.Fatalf("return is %q (%v), want still received", status, err)
	}
}

func waitLockWaiters(t *testing.T, conn *sql.DB, want int) {
	t.Helper()
	for range 300 {
		var n int
		if err := conn.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fewer than %d writers queued", want)
}

// Two shipments planned against the same shipped quantity, and partially_fulfilled -> partially_fulfilled passes the status guard - a line of 3 shipped 4
func TestTwoShipmentsCannotOverShipALine(t *testing.T) {
	conn := statusTestDB(t)
	svc := moneySvc(conn)
	ctx := context.Background()
	orderID, lineID := seedPaidOrder(t, conn, OrderStatusProcessing, PaymentPaid, 0)
	if _, err := conn.Exec(`UPDATE order_lines SET quantity = 3 WHERE id = $1`, lineID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM order_fulfillments WHERE order_id = $1`, orderID) })
	ship := func(qty float64) error {
		_, err := svc.CreateFulfillment(ctx, CreateFulfillmentInput{OrderID: orderID, Lines: []FulfillmentLineInput{{OrderLineID: lineID, Quantity: qty}}})
		return err
	}
	if err := ship(1); err != nil {
		t.Fatal(err)
	}

	hold, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = hold.Rollback() }()
	if _, err := hold.Exec(`SELECT 1 FROM orders WHERE id = $1 FOR UPDATE`, orderID); err != nil {
		t.Fatal(err)
	}
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { first <- ship(1) }()
	waitLockWaiters(t, conn, 1)
	go func() { second <- ship(2) }()
	waitLockWaiters(t, conn, 2)
	if err := hold.Commit(); err != nil {
		t.Fatal(err)
	}
	<-first
	<-second
	var shipped float64
	if err := conn.QueryRow(`SELECT COALESCE(SUM(fl.quantity), 0) FROM order_fulfillment_lines fl JOIN order_fulfillments f ON f.id = fl.fulfillment_id WHERE f.order_id = $1`, orderID).Scan(&shipped); err != nil {
		t.Fatal(err)
	}
	if shipped > 3 {
		t.Fatalf("%g units shipped against 3 ordered", shipped)
	}
}

// An edit planned from an unlocked read went ahead over a cancel, or over another edit, committed meanwhile
func TestAnEditRechecksTheOrderUnderTheLock(t *testing.T) {
	conn := statusTestDB(t)
	repo := NewRepository(dbsqlc.New(conn))
	ctx := context.Background()
	orderID := seedConfirmedOrder(t, conn)
	read, err := repo.GetOrderByID(ctx, orderID)
	if err != nil {
		t.Fatal(err)
	}
	inTx := func(fn func(r Repository) error) error {
		tx, err := conn.Begin()
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		return fn(repo.WithTx(tx))
	}
	if _, err := conn.Exec(`UPDATE orders SET updated_at = updated_at + interval '1 second' WHERE id = $1`, orderID); err != nil {
		t.Fatal(err)
	}
	if err := inTx(func(r Repository) error { return lockForEdit(ctx, r, read) }); conflictCode(err) != "ORDER_CHANGED" {
		t.Fatalf("editing over a concurrent change = %v, want ORDER_CHANGED", err)
	}
	if _, err := conn.Exec(`UPDATE orders SET status = 'cancelled' WHERE id = $1`, orderID); err != nil {
		t.Fatal(err)
	}
	if err := inTx(func(r Repository) error { return lockForEdit(ctx, r, read) }); conflictCode(err) != "ORDER_NOT_EDITABLE" {
		t.Fatalf("editing a cancelled order = %v, want ORDER_NOT_EDITABLE", err)
	}
}

type capturedOutbox struct {
	mu     sync.Mutex
	events []any
}

func (c *capturedOutbox) Enqueue(_ context.Context, _ *sql.Tx, e events.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
	return nil
}

// The shipped share was read before the cancel took the order row - a shipment landing meanwhile on a partially fulfilled order was missed, so the tender reversals refunded goods that had just left
func TestACancelScalesReversalsByWhatHadShippedWhenItCommitted(t *testing.T) {
	conn := statusTestDB(t)
	ctx := context.Background()
	orderID, lineID := seedPaidOrder(t, conn, OrderStatusProcessing, PaymentPaid, 0)
	if _, err := conn.Exec(`UPDATE order_lines SET quantity = 4, total_price = 4000 WHERE id = $1`, lineID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM order_fulfillments WHERE order_id = $1`, orderID) })
	out := &capturedOutbox{}
	svc := moneySvc(conn)
	svc.outbox = out
	if _, err := svc.CreateFulfillment(ctx, CreateFulfillmentInput{OrderID: orderID, Lines: []FulfillmentLineInput{{OrderLineID: lineID, Quantity: 1}}}); err != nil {
		t.Fatal(err)
	}

	hold, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = hold.Rollback() }()
	if _, err := hold.Exec(`SELECT 1 FROM orders WHERE id = $1 FOR UPDATE`, orderID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := svc.UpdateStatus(ctx, orderID, OrderStatusCancelled)
		done <- err
	}()
	waitLockWaiters(t, conn, 1)
	// A second shipment commits while the cancel waits
	var fid uuid.UUID
	if err := hold.QueryRow(`INSERT INTO order_fulfillments (order_id, status, sequence) VALUES ($1, 'shipped', 2) RETURNING id`, orderID).Scan(&fid); err != nil {
		t.Fatal(err)
	}
	if _, err := hold.Exec(`INSERT INTO order_fulfillment_lines (fulfillment_id, order_line_id, quantity) VALUES ($1, $2, 1)`, fid, lineID); err != nil {
		t.Fatal(err)
	}
	if err := hold.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var fraction float64 = -1
	for _, e := range out.events {
		if c, ok := e.(*OrderCancelled); ok {
			fraction = c.RefundableFraction
		}
	}
	if fraction != 0.5 {
		t.Fatalf("cancel reversed a %g share, want 0.5 - two of four units had shipped when it committed", fraction)
	}
}
