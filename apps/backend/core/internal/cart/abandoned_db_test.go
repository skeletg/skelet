package cart

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func seedAbandonChannel(t *testing.T, conn *sql.DB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'Abandon','USD',true)`, id, "abandon-"+id.String()[:8]); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, id) })
	return id
}

// A guest cart with one line, both last written at the given time
func seedIdleCart(t *testing.T, conn *sql.DB, channelID uuid.UUID, at time.Time) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := conn.QueryRow(`INSERT INTO carts (channel_id, email, updated_at, last_activity_at) VALUES ($1, 'idle@example.test', $2, $2) RETURNING id`, channelID, at).Scan(&id); err != nil {
		t.Fatalf("seed cart: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO cart_lines (cart_id, product_name, product_sku, unit_price, quantity, updated_at) VALUES ($1, 'Thing', $2, 100, 1, $3)`, id, "SKU-"+id.String()[:8], at); err != nil {
		t.Fatalf("seed line: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM carts WHERE id = $1`, id) })
	return id
}

func TestAbandonedCartsAreListedPerChannelFromTheirLastActivity(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	repo := NewRepository(db.New(conn))
	a, b := seedAbandonChannel(t, conn), seedAbandonChannel(t, conn)
	idle := time.Now().Add(-2 * time.Hour)
	mine := seedIdleCart(t, conn, a, idle)
	seedIdleCart(t, conn, b, idle)
	touched := seedIdleCart(t, conn, a, idle)
	if _, err := conn.Exec(`UPDATE cart_lines SET quantity = 2, updated_at = now() WHERE cart_id = $1`, touched); err != nil {
		t.Fatal(err)
	}

	got, err := repo.ListAbandoned(ctx, a, time.Now().Add(-time.Hour), time.Now().Add(-72*time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != mine {
		t.Fatalf("channel a listed %v, want only its idle cart - not channel b's, not the one whose line was just edited", got)
	}
	if err := repo.MarkAbandonedNotified(ctx, []uuid.UUID{mine}); err != nil {
		t.Fatal(err)
	}
	if again, _ := repo.ListAbandoned(ctx, a, time.Now().Add(-time.Hour), time.Now().Add(-72*time.Hour), 100); len(again) != 0 {
		t.Fatalf("a notified cart came back: %v", again)
	}
}
