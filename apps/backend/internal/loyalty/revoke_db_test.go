package loyalty

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

// revokeForOrder revokes toward a TARGET share of what the order earned - bailing on the first negative row means only the first partial refund ever deducted
// DB-backed because the behaviour is "read the existing entries, insert the difference"

// loyalty_point_entries.order_id is a real FK, so the earn needs an order to hang off
func seedOrder(t *testing.T, s *Service, channelID uuid.UUID) uuid.UUID {
	t.Helper()
	conn := testDB(t)
	var id uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO orders (channel_id, currency_code) VALUES ($1, 'USD') RETURNING id`,
		channelID).Scan(&id); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM loyalty_point_entries WHERE order_id = $1`, id)
		_, _ = conn.Exec(`DELETE FROM orders WHERE id = $1`, id)
	})
	return id
}

func seedEarn(t *testing.T, s *Service, customerID, orderID uuid.UUID, points int64) {
	t.Helper()
	if _, err := s.queries.InsertLoyaltyEntry(t.Context(), db.InsertLoyaltyEntryParams{
		CustomerID: customerID,
		OrderID:    uuid.NullUUID{UUID: orderID, Valid: true},
		Points:     points,
		Reason:     "test earn",
	}); err != nil {
		t.Fatalf("seed earn: %v", err)
	}
}

func orderPoints(t *testing.T, s *Service, orderID uuid.UUID) int64 {
	t.Helper()
	rows, err := s.queries.LoyaltyEntriesForOrder(t.Context(), uuid.NullUUID{UUID: orderID, Valid: true})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	var net int64
	for _, r := range rows {
		net += r.Points
	}
	return net
}

func TestRevokeForOrder_PartialRefundsAccumulate(t *testing.T) {
	conn := testDB(t)
	svc, _, customerID, channelID := newFixture(t, conn)
	orderID := seedOrder(t, svc, channelID)
	seedEarn(t, svc, customerID, orderID, 100)

	if err := svc.revokeForOrder(t.Context(), orderID, 0.25, "refund 1"); err != nil {
		t.Fatalf("first revoke: %v", err)
	}
	if got := orderPoints(t, svc, orderID); got != 75 {
		t.Fatalf("after a 25%% refund the order nets %d points, want 75", got)
	}

	// Cumulative share of a half - only the extra quarter may be taken
	if err := svc.revokeForOrder(t.Context(), orderID, 0.5, "refund 2"); err != nil {
		t.Fatalf("second revoke: %v", err)
	}
	if got := orderPoints(t, svc, orderID); got != 50 {
		t.Errorf("after a cumulative 50%% refund the order nets %d points, want 50", got)
	}

	if err := svc.revokeForOrder(t.Context(), orderID, 1, "refund 3"); err != nil {
		t.Fatalf("third revoke: %v", err)
	}
	if got := orderPoints(t, svc, orderID); got != 0 {
		t.Errorf("after a full refund the order nets %d points, want 0", got)
	}
}

// At-least-once delivery, so the same fraction arriving twice must be a no-op
func TestRevokeForOrder_ReplayIsANoop(t *testing.T) {
	conn := testDB(t)
	svc, _, customerID, channelID := newFixture(t, conn)
	orderID := seedOrder(t, svc, channelID)
	seedEarn(t, svc, customerID, orderID, 80)

	for range 3 {
		if err := svc.revokeForOrder(t.Context(), orderID, 0.5, "replayed refund"); err != nil {
			t.Fatalf("revoke: %v", err)
		}
	}
	if got := orderPoints(t, svc, orderID); got != 40 {
		t.Errorf("three deliveries of the same event net %d points, want 40 - a replay must not double-revoke", got)
	}
}

// The replay above passes with no lock at all and racing goroutines only sometimes hit the window - so assert the mechanism: summing what's revoked is only trustworthy while this tx owns the customer row
// Unlocked, two concurrent deliveries both read alreadyRevoked = 0 and both insert the full delta, and the negative balance claws back store credit never redeemed
func TestRevokeForOrder_WaitsForTheCustomerLock(t *testing.T) {
	conn := testDB(t)
	svc, _, customerID, channelID := newFixture(t, conn)
	orderID := seedOrder(t, svc, channelID)
	seedEarn(t, svc, customerID, orderID, 100)

	// Stand in for the delivery that got there first and hasn't committed yet
	holder, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin holder: %v", err)
	}
	if _, err := holder.Exec(`SELECT 1 FROM customers WHERE id = $1 FOR UPDATE`, customerID); err != nil {
		_ = holder.Rollback()
		t.Fatalf("hold customer row: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- svc.revokeForOrder(context.Background(), orderID, 1, "second delivery") }()

	// It's stuck either on the lock or on the FK check of its own insert, which is why "did it block?" alone can't tell the two apart
	select {
	case err := <-done:
		_ = holder.Rollback()
		t.Fatalf("revoke returned %v without ever waiting on the held customer row", err)
	case <-time.After(300 * time.Millisecond):
	}

	// The first delivery revokes the full 100 and commits - a revoke that read BEFORE waiting still thinks nothing is revoked and takes another 100
	if _, err := holder.Exec(
		`INSERT INTO loyalty_point_entries (customer_id, order_id, points, reason)
		 VALUES ($1, $2, -100, 'first delivery')`, customerID, orderID); err != nil {
		_ = holder.Rollback()
		t.Fatalf("first delivery revoke: %v", err)
	}
	if err := holder.Commit(); err != nil {
		t.Fatalf("release holder: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("revoke once the lock released: %v", err)
	}
	if got := orderPoints(t, svc, orderID); got != 0 {
		t.Errorf("order nets %d points, want 0 - the second delivery revoked what the first already had", got)
	}
}

// Shipping-only and goodwill memos return no goods, so a zero fraction must cost the customer nothing
func TestRevokeForOrder_ZeroFractionRevokesNothing(t *testing.T) {
	conn := testDB(t)
	svc, _, customerID, channelID := newFixture(t, conn)
	orderID := seedOrder(t, svc, channelID)
	seedEarn(t, svc, customerID, orderID, 60)

	if err := svc.revokeForOrder(t.Context(), orderID, 0, "shipping-only refund"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if got := orderPoints(t, svc, orderID); got != 60 {
		t.Errorf("net %d points, want 60 - no goods came back", got)
	}
}

// A cancel after a partial refund takes the remainder, not the whole earn again
func TestRevokeForOrder_CancelAfterPartialRefundTakesTheRemainder(t *testing.T) {
	conn := testDB(t)
	svc, _, customerID, channelID := newFixture(t, conn)
	orderID := seedOrder(t, svc, channelID)
	seedEarn(t, svc, customerID, orderID, 100)

	if err := svc.revokeForOrder(t.Context(), orderID, 0.3, "partial refund"); err != nil {
		t.Fatalf("refund revoke: %v", err)
	}
	if err := svc.revokeForOrder(t.Context(), orderID, 1, "order cancelled"); err != nil {
		t.Fatalf("cancel revoke: %v", err)
	}
	if got := orderPoints(t, svc, orderID); got != 0 {
		t.Errorf("net %d points, want 0", got)
	}
}
