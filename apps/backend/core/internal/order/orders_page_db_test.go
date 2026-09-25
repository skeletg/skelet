package order

import (
	"context"
	"testing"

	"github.com/google/uuid"

	dbsqlc "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestTheOrderPageReadsOnlyTheScopedChannelsAndAllOfThemUnscoped(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	chans := make([]uuid.UUID, 2)
	search := "PAGE-" + uuid.NewString()[:8]
	for i := range chans {
		if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'pg', 'USD') RETURNING id`, search+string(rune('a'+i))).Scan(&chans[i]); err != nil {
			t.Fatal(err)
		}
		for n := range 2 {
			if _, err := conn.Exec(`INSERT INTO orders (channel_id, currency_code, number) VALUES ($1, 'USD', $2)`, chans[i], search+"-"+string(rune('a'+i))+string(rune('0'+n))); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Cleanup(func() {
		for _, c := range chans {
			_, _ = conn.Exec(`DELETE FROM orders WHERE channel_id = $1`, c)
			_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, c)
		}
	})
	repo := NewRepository(dbsqlc.New(conn))

	scoped, total, err := repo.ListOrdersPage(ctx, ListPageFilter{ChannelIDs: []uuid.UUID{chans[1]}, Search: search, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(scoped) != 2 {
		t.Fatalf("scoped page = %d rows of %d, want the one channel's 2", len(scoped), total)
	}
	for _, o := range scoped {
		if o.ChannelID != chans[1] {
			t.Fatalf("the scoped page carried channel %s's order", o.ChannelID)
		}
	}
	all, total, err := repo.ListOrdersPage(ctx, ListPageFilter{Search: search, Limit: 10})
	if err != nil || total != 4 || len(all) != 4 {
		t.Fatalf("unscoped page = %d rows of %d, %v, want all 4", len(all), total, err)
	}
}

func TestOrdersCarryTheTimeIndexesTheListsAndReportsRead(t *testing.T) {
	conn := testdb.Require(t)
	for _, name := range []string{"idx_orders_channel_created", "idx_orders_created", "idx_orders_payment_sync"} {
		var n int
		if err := conn.QueryRow(`SELECT count(*) FROM pg_indexes WHERE tablename = 'orders' AND indexname = $1`, name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("orders has no %s, so the list or report behind it scans the table", name)
		}
	}
}
