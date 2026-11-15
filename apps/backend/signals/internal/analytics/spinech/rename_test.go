package spinech

import (
	"context"
	"io/fs"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/migfile"
	"github.com/skeletg/skelet/apps/backend/signals/internal/chtest"
)

// The GA4 rename on the spine: server and promoted names are rewritten, a custom event's name in its properties too,
// a browser's purchase goes, and a merchant's own event that shares an old server name is left alone
func TestTheRenameLeavesNoOldNameOnTheSpine(t *testing.T) {
	opts, err := clickhouse.ParseDSN(chtest.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := clickhouse.Open(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx := context.Background()
	ch := uuid.New()
	for _, r := range [][3]string{
		{"cart_add", "commerce", `{}`},
		{"order_placed", "commerce", `{"revenue_cents":1500}`},
		{"registered", "account", `{}`},
		{"purchase", "web", `{"name":"purchase"}`},
		{"product_filter", "web", `{"name":"product_filter","filters":"red"}`},
		{"custom", "web", `{"name":"$search","query":"mug"}`},
		{"custom", "web", `{"name":"search_overview_click","position":2}`},
		{"custom", "web", `{"name":"lead"}`},
	} {
		if err := conn.Exec(ctx, `INSERT INTO analytics_events (event_name, occurred_at, channel_id, source, properties, dedupe_key) VALUES (?, now64(3), ?, ?, ?, ?)`,
			r[0], ch, r[1], r[2], uuid.NewString()); err != nil {
			t.Fatal(err)
		}
	}

	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	files, err := migfile.ListFS(sub)
	if err != nil {
		t.Fatal(err)
	}
	ran := false
	for _, f := range files {
		if f.Version == "20261115221700" {
			if err := chtest.ApplyUp(ctx, conn, f); err != nil {
				t.Fatal(err)
			}
			ran = true
		}
	}
	if !ran {
		t.Fatal("the rename migration wasn't found")
	}

	rows, err := conn.Query(ctx, `SELECT event_name, source, custom_name FROM analytics_events FINAL WHERE channel_id = ? ORDER BY event_name, custom_name`, ch)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name, source, custom string
		if err := rows.Scan(&name, &source, &custom); err != nil {
			t.Fatal(err)
		}
		got = append(got, name+"/"+source+"/"+custom)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"add_to_cart/commerce/", "custom/web/lead", "custom/web/search", "custom/web/select_overview_item",
		"filter_items/web/filter_items", "purchase/commerce/", "sign_up/account/",
	}
	if len(got) != len(want) {
		t.Fatalf("spine after the rename: %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("spine after the rename: %v, want %v", got, want)
		}
	}
}
