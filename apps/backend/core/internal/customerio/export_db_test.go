package customerio

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestTheExportPagesEveryCustomerExactlyOnce(t *testing.T) {
	conn := testdb.Require(t)
	var group, own uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM customer_groups LIMIT 1`).Scan(&group); err != nil {
		t.Skipf("no customer group: %v", err)
	}
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'cio', 'USD') RETURNING id`, "cio-"+uuid.NewString()[:8]).Scan(&own); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM customers WHERE channel_id = $1`, own)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, own)
	})
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	want := map[string]bool{}
	for i, at := range []time.Time{base, base.Add(time.Hour), base.Add(time.Hour), base.Add(2 * time.Hour), base.Add(3 * time.Hour)} {
		email := "cio-" + uuid.NewString()[:8] + "@example.invalid"
		if _, err := conn.Exec(`INSERT INTO customers (email, password_hash, customer_group_id, channel_id, created_at) VALUES ($1, 'x', $2, $3, $4)`,
			email, group, own, at); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
		want[email] = true
	}
	o := dataio.NewOpts(nil)
	o.ChannelID = own
	exp := &customerExporter{queries: db.New(conn), opts: o, pageSize: 2}
	seen := map[string]int{}
	if err := exp.Stream(context.Background(), func(r *dataio.Row) error {
		seen[r.Str(colEmail)]++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != len(want) {
		t.Fatalf("exported %d distinct customers, want %d", len(seen), len(want))
	}
	for email, n := range seen {
		if !want[email] || n != 1 {
			t.Fatalf("%s exported %d times, want once and only this channel's", email, n)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := exp.Stream(ctx, func(*dataio.Row) error { return nil }); err == nil {
		t.Fatal("a cancelled export kept reading")
	}
}
