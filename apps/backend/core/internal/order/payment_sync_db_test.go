package order

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	dbsqlc "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// The sweep's worklist has four gates (unpaid, provider set, reference set, not cancelled) plus a time
// window - each seeded row here violates exactly one of them, so a regression names itself

func seedSyncOrder(t *testing.T, conn *sql.DB, channelID uuid.UUID, paymentStatus, provider, reference, status string, age time.Duration) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := conn.QueryRowContext(context.Background(),
		`INSERT INTO orders (channel_id, currency_code, status, payment_status, payment_provider, payment_reference, updated_at)
		 VALUES ($1, 'USD', $2, $3, $4, $5, NOW() - $6::interval) RETURNING id`,
		channelID, status, paymentStatus, provider, reference, age.String()).Scan(&id)
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.ExecContext(context.Background(), `DELETE FROM orders WHERE id = $1`, id)
	})
	return id
}

func TestListOrdersAwaitingPaymentSync(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	var channelID uuid.UUID
	if err := conn.QueryRowContext(ctx, `SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel to seed against: %v", err)
	}

	want := seedSyncOrder(t, conn, channelID, PaymentUnpaid, "gw", "ref-1", OrderStatusConfirmed, time.Hour)
	seedSyncOrder(t, conn, channelID, PaymentPaid, "gw", "ref-2", OrderStatusConfirmed, time.Hour)         // already settled
	seedSyncOrder(t, conn, channelID, PaymentUnpaid, "", "ref-3", OrderStatusConfirmed, time.Hour)         // offline - no provider
	seedSyncOrder(t, conn, channelID, PaymentUnpaid, "gw", "", OrderStatusConfirmed, time.Hour)            // nothing to ask the gateway about
	seedSyncOrder(t, conn, channelID, PaymentUnpaid, "gw", "ref-5", OrderStatusCancelled, time.Hour)       // cancelled
	seedSyncOrder(t, conn, channelID, PaymentUnpaid, "gw", "ref-6", OrderStatusConfirmed, time.Minute)     // too fresh - probably mid-checkout
	seedSyncOrder(t, conn, channelID, PaymentUnpaid, "gw", "ref-7", OrderStatusConfirmed, 30*24*time.Hour) // abandoned past the window

	repo := NewRepository(dbsqlc.New(conn))
	now := time.Now()
	got, err := repo.ListOrdersAwaitingPaymentSync(ctx, now.Add(-7*24*time.Hour), now.Add(-30*time.Minute), 100)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[uuid.UUID]bool{}
	for _, o := range got {
		ids[o.ID] = true
	}
	if !ids[want] {
		t.Error("the qualifying order was not returned")
	}
	if len(ids) != 1 {
		t.Errorf("got %d orders, want exactly the 1 qualifying seed (query leaked a disqualified row)", len(ids))
	}
}
