package notification

import (
	"context"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// The dedupe key used to be permanent - a product restocked and sold out again never notified a second time while the first notice sat in the inbox
func TestARecurringStockOutNotifiesAgainNextDay(t *testing.T) {
	conn := testdb.Require(t)
	svc := New(db.New(conn), nil)
	day := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return day }
	n := NewNotifier(svc)
	ev := &inventory.StockOut{InventoryItemID: uuid.New(), LocationID: uuid.New(), ProductID: uuid.New()}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM admin_notifications WHERE entity_id = $1`, ev.ProductID.String())
	})
	count := func() int {
		var c int
		if err := conn.QueryRow(`SELECT count(*) FROM admin_notifications WHERE entity_id = $1`, ev.ProductID.String()).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	ctx := context.Background()
	for range 2 {
		if err := n.onStockOut(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	if got := count(); got != 1 {
		t.Fatalf("one stock-out delivered twice made %d notices, want 1", got)
	}
	day = day.Add(48 * time.Hour)
	if err := n.onStockOut(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 2 {
		t.Fatalf("a stock-out two days later made %d notices in all, want 2", got)
	}
}

// A dropped write used to return nil, so the outbox never retried and the notice was gone
func TestAFailedNoticeReturnsItsError(t *testing.T) {
	conn := testdb.Require(t)
	n := NewNotifier(New(db.New(conn), nil))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := n.onStockLow(ctx, &inventory.StockLow{InventoryItemID: uuid.New(), ProductID: uuid.New()}); err == nil {
		t.Fatal("a notice that could not be written reported success")
	}
}

func TestTruncateKeepsWholeCharacters(t *testing.T) {
	got := truncate("Neuspešno ćirilica žž", 10)
	if !utf8.ValidString(got) || got != "Neuspešno "+"…" {
		t.Fatalf("truncate = %q", got)
	}
	if truncate("short", 10) != "short" {
		t.Fatal("a short string was cut")
	}
}
