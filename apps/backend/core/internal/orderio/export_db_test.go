package orderio

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

type seeded struct {
	channel uuid.UUID
	numbers map[string]time.Time
}

func seedOrders(t *testing.T) seeded {
	t.Helper()
	conn := testdb.Require(t)
	tag := "OIO-" + uuid.NewString()[:8]
	var ch uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'oio', 'USD') RETURNING id`, tag).Scan(&ch); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM order_lines WHERE order_id IN (SELECT id FROM orders WHERE channel_id = $1)`, ch)
		_, _ = conn.Exec(`DELETE FROM orders WHERE channel_id = $1`, ch)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch)
	})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	out := seeded{channel: ch, numbers: map[string]time.Time{}}
	for i, at := range []time.Time{day.Add(9 * time.Hour), day.Add(9 * time.Hour), day.Add(23*time.Hour + 59*time.Minute), day.Add(24 * time.Hour), day.Add(30 * time.Hour)} {
		number := tag + "-" + string(rune('a'+i))
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO orders (channel_id, currency_code, number, created_at) VALUES ($1, 'USD', $2, $3) RETURNING id`, ch, number, at).Scan(&id); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if _, err := conn.Exec(`INSERT INTO order_lines (order_id, product_name, product_sku, unit_price, quantity, total_price) VALUES ($1, 'p', 'SKU', 100, 1, 100)`, id); err != nil {
				t.Fatal(err)
			}
		}
		out.numbers[number] = at
	}
	return out
}

func export(t *testing.T, s seeded, vals map[string]string, pageSize int) map[string]int {
	t.Helper()
	o := dataio.NewOpts(vals)
	o.ChannelID = s.channel
	exp, err := newExporter(db.New(testdb.Require(t)), o, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	exp.pageSize = pageSize
	lines := map[string]int{}
	if err := exp.Stream(context.Background(), func(r *dataio.Row) error {
		lines[r.Str(colOrderNumber)]++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return lines
}

func TestADateOnlyToIncludesThatDayAndNotTheNext(t *testing.T) {
	s := seedOrders(t)
	lines := export(t, s, map[string]string{optStartDate: "2026-03-01", optEndDate: "2026-03-01"}, exportPageSize)
	for number, at := range s.numbers {
		in := at.Before(time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC))
		if in && lines[number] != 2 {
			t.Errorf("%s placed %v exported %d lines, want both - the To day is inclusive", number, at, lines[number])
		}
		if !in && lines[number] != 0 {
			t.Errorf("%s placed %v on the next day was exported", number, at)
		}
	}
}

func TestTheExportPagesEveryLineExactlyOnce(t *testing.T) {
	s := seedOrders(t)
	lines := export(t, s, nil, 3)
	if len(lines) != len(s.numbers) {
		t.Fatalf("exported %d orders, want %d", len(lines), len(s.numbers))
	}
	for number, n := range lines {
		if n != 2 {
			t.Errorf("%s exported %d lines, want 2", number, n)
		}
	}
}

func TestACancelledExportStopsReading(t *testing.T) {
	s := seedOrders(t)
	o := dataio.NewOpts(nil)
	o.ChannelID = s.channel
	exp, err := newExporter(db.New(testdb.Require(t)), o, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := exp.Stream(ctx, func(*dataio.Row) error { return nil }); err == nil {
		t.Fatal("a cancelled export kept reading")
	}
}
