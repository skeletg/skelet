package spinech

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// Aggregate events count in event totals and are nobody everywhere people are counted
func TestAggregateEventsCountInTotalsButAreNobody(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	ch := uuid.New()
	now := time.Now().UTC().Truncate(time.Millisecond)
	since := now.AddDate(0, 0, -7)
	at := now.Add(-time.Hour)
	add := func(name, anon, source string) {
		t.Helper()
		e := analytics.SpineEvent{EventName: name, OccurredAt: at, ChannelID: &ch, AnonID: anon, Source: source, DedupeKey: uuid.NewString()}
		at = at.Add(time.Second)
		if _, err := st.AppendEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	for _, anon := range []string{"anon-a", "anon-b"} { // two people, each adding and buying
		add("add_to_cart", anon, analytics.SourceCommerce)
		add("purchase", anon, analytics.SourceCommerce)
	}
	for range 3 { // three aggregate adds and orders - from one person or three, nobody can tell
		add("add_to_cart", "", analytics.SourceAggregate)
		add("purchase", "", analytics.SourceAggregate)
	}
	if err := st.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	ins, err := st.Insights(ctx, ch, "add_to_cart", "", "total", "day", since, now)
	if err != nil {
		t.Fatal(err)
	}
	if ins.Total != 5 || ins.Uniques != 2 || ins.Aggregate != 3 {
		t.Errorf("insights total %d, uniques %d, aggregate %d - want every add counted, only the two people as uniques, and the three named", ins.Total, ins.Uniques, ins.Aggregate)
	}
	res, err := st.Funnel(ctx, ch, analytics.FunnelQuery{Steps: []string{"add_to_cart", "purchase"}}, since)
	steps := res.Steps
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || steps[0].Count != 2 || steps[1].Count != 2 {
		t.Errorf("funnel %+v - aggregate events must not walk it as a phantom person", steps)
	}
	sticky, err := st.Stickiness(ctx, ch, "", since, now, 7)
	if err != nil {
		t.Fatal(err)
	}
	if sticky.ActiveNow != 2 {
		t.Errorf("active %d, want the two people", sticky.ActiveNow)
	}
}
