package aiusage

import (
	"context"
	"math"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestTheReportAddsUpTurnsAndCreditsOnlyAssistedOrders(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	var ch, other uuid.UUID
	for _, row := range []struct {
		id   *uuid.UUID
		slug string
	}{{&ch, "usage-" + suffix}, {&other, "other-" + suffix}} {
		if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'usage', 'EUR') RETURNING id`, row.slug).Scan(row.id); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(db.New(conn), fakeSettings{priceInputPath: "1", priceOutputPath: "10"})
	for _, turn := range []Turn{
		{CartChanged: true, Usage: ai.Usage{Input: 100, Output: 10, CacheRead: 1000, CacheWrite: 50}},
		{CartChanged: true, Checkout: true, Usage: ai.Usage{Input: 200, Output: 20}},
		{Failed: true, Unanswered: true, Usage: ai.Usage{Input: 30}},
		{Unanswered: true},
	} {
		if err := svc.RecordTurn(ctx, ch, turn); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.RecordTurn(ctx, other, Turn{Usage: ai.Usage{Input: 999}}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := svc.RecordOverview(ctx, ch, ai.Usage{Input: 500, Output: 40}, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.RecordOverview(ctx, ch, ai.Usage{}, false); err != nil { // a failed one costs nothing and isn't an overview
		t.Fatal(err)
	}
	if spent, err := svc.TokensToday(ctx, ch); err != nil || spent != 1490 {
		t.Errorf("tokens today = %d, %v - want the overviews' counted toward the cap", spent, err)
	}
	order := func(store uuid.UUID, currency, status, by string, total int64, age string) {
		t.Helper()
		if _, err := conn.Exec(`INSERT INTO orders (channel_id, currency_code, status, assisted_by, total_amount, created_at) VALUES ($1, $2, $3, $4, $5, now() - $6::interval)`,
			store, currency, status, by, total, age); err != nil {
			t.Fatal(err)
		}
	}
	order(ch, "EUR", "pending", "assistant", 5000, "1 minute")
	order(ch, "EUR", "completed", "agent", 3000, "2 days")
	order(ch, "EUR", "pending", "", 1000, "1 minute")
	order(ch, "USD", "pending", "assistant", 700, "1 minute")
	order(ch, "EUR", "cancelled", "assistant", 9000, "1 minute")
	order(ch, "EUR", "pending", "assistant", 9000, "60 days")
	order(other, "EUR", "pending", "assistant", 9000, "1 minute")

	rep, err := svc.Report(ctx, ch, 30)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Turns != 4 || rep.Overviews != 2 || rep.FailedTurns != 1 || rep.UnansweredTurns != 2 || rep.CartTurns != 2 || rep.CheckoutTurns != 1 {
		t.Errorf("turns = %+v", rep)
	}
	if n := len(rep.Daily); n != 30 {
		t.Fatalf("daily = %d days, want the whole window", n)
	}
	if today, earlier := rep.Daily[29], rep.Daily[27]; today.Turns != 4 || today.UnansweredTurns != 2 || today.FailedTurns != 1 || today.AssistantOrders != 2 || earlier.AgentOrders != 1 || earlier.Turns != 0 {
		t.Errorf("today = %+v, two days ago = %+v", today, earlier)
	}
	if want := (ai.Usage{Input: 1330, Output: 110, CacheRead: 1000, CacheWrite: 50}); rep.Usage != want {
		t.Errorf("usage = %+v, want %+v", rep.Usage, want)
	}
	if rep.CostUSD == nil || math.Abs(*rep.CostUSD-(1330+110*10+1000+50)/1e6) > 1e-12 {
		t.Errorf("cost = %v", rep.CostUSD)
	}
	want := map[string]Orders{
		"EUR": {CurrencyCode: "EUR", All: 3, Assistant: 1, AssistantRevenue: 5000, Agent: 1, AgentRevenue: 3000},
		"USD": {CurrencyCode: "USD", All: 1, Assistant: 1, AssistantRevenue: 700},
	}
	if len(rep.Orders) != len(want) {
		t.Fatalf("orders = %+v", rep.Orders)
	}
	for _, o := range rep.Orders {
		if o != want[o.CurrencyCode] {
			t.Errorf("%s orders = %+v, want %+v", o.CurrencyCode, o, want[o.CurrencyCode])
		}
	}
	// Just after midnight UTC the minute-old orders fall on yesterday and the window holds none, which is still right
	if day, err := svc.Report(ctx, ch, 1); err != nil || slices.ContainsFunc(day.Orders, func(o Orders) bool { return o.Agent != 0 }) {
		t.Errorf("a one-day window kept the two-day-old order: %+v, %v", day.Orders, err)
	}
}
