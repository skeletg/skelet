package order

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	dbsqlc "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/testdb"
)

// validTransition runs against a read taken outside the transaction, and for a cancel the
// pre-commit hooks (webhooks, carrier calls) widen that window further - so the guarantee
// has to come from the UPDATE itself, not the check
//
// DB-backed because the whole point is what two real transactions do to one row

func statusTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Require(t)
}

// A confirmed order on whatever channel the database already has - dropped on cleanup
func seedConfirmedOrder(t *testing.T, conn *sql.DB) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var channelID uuid.UUID
	if err := conn.QueryRowContext(ctx, `SELECT id FROM channels LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel to seed against: %v", err)
	}
	var orderID uuid.UUID
	err := conn.QueryRowContext(ctx,
		`INSERT INTO orders (channel_id, currency_code, status) VALUES ($1, 'USD', $2) RETURNING id`,
		channelID, OrderStatusConfirmed).Scan(&orderID)
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.ExecContext(context.Background(), `DELETE FROM orders WHERE id = $1`, orderID)
	})
	return orderID
}

// Two admins cancelling at once both passed validTransition against the same read and both
// committed, so the outbox got two order.cancelled rows and every tender reversed twice
func TestConcurrentCancelCommitsExactlyOnce(t *testing.T) {
	conn := statusTestDB(t)
	orderID := seedConfirmedOrder(t, conn)
	repo := NewRepository(dbsqlc.New(conn))

	const racers = 8
	var wg sync.WaitGroup
	results := make([]error, racers)
	start := make(chan struct{})
	for i := range racers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, results[i] = repo.UpdateOrderStatusFrom(
				context.Background(), orderID, OrderStatusConfirmed, OrderStatusCancelled)
		}(i)
	}
	close(start)
	wg.Wait()

	won := 0
	for i, err := range results {
		if err == nil {
			won++
			continue
		}
		if !isStatusConflict(err) {
			t.Errorf("racer %d failed with %v, want the status conflict", i, err)
		}
	}
	if won != 1 {
		t.Fatalf("%d of %d cancels committed, want exactly 1 - the rest must lose the race", won, racers)
	}

	var status string
	if err := conn.QueryRow(`SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != OrderStatusCancelled {
		t.Errorf("order status = %q, want %q", status, OrderStatusCancelled)
	}
}

// Releasing is guarded too - an unguarded second release re-runs the COALESCE and can hand back a status the order has already left
func TestReleaseFromHoldCommitsExactlyOnce(t *testing.T) {
	conn := statusTestDB(t)
	orderID := seedConfirmedOrder(t, conn)
	repo := NewRepository(dbsqlc.New(conn))
	ctx := context.Background()

	if _, err := repo.HoldOrderFrom(ctx, orderID, OrderStatusConfirmed); err != nil {
		t.Fatalf("hold: %v", err)
	}
	// An unguarded racing second hold stamps held_from_status = 'on_hold' and wedges the release
	if _, err := repo.HoldOrderFrom(ctx, orderID, OrderStatusConfirmed); !isStatusConflict(err) {
		t.Errorf("second hold returned %v, want the status conflict", err)
	}
	if _, err := repo.ReleaseOrderToHeldFrom(ctx, orderID); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := repo.ReleaseOrderToHeldFrom(ctx, orderID); !isStatusConflict(err) {
		t.Errorf("second release returned %v, want the status conflict", err)
	}

	var status string
	if err := conn.QueryRow(`SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != OrderStatusConfirmed {
		t.Errorf("order status = %q, want %q - the hold must put it back where it came from", status, OrderStatusConfirmed)
	}
}

func isStatusConflict(err error) bool {
	var appErr *apperr.Error
	return errors.As(err, &appErr) && appErr.Code == "ORDER_STATUS_RACED"
}
