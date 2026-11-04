package spinech

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// A small spine with known answers for the Synapse engines: four visitors walk a funnel to different depths, one
// does the steps out of order, a bot does all of them, one visitor signs in half way, and one visit is counted as nobody
type engineSpine struct {
	st  *Store
	ch  uuid.UUID
	now time.Time
	t   *testing.T
}

func newEngineSpine(t *testing.T) *engineSpine {
	t.Helper()
	return &engineSpine{st: testStore(t), ch: uuid.New(), now: time.Now().UTC(), t: t}
}

func (e *engineSpine) add(name, anon string, cust *uuid.UUID, ago time.Duration, props map[string]any) {
	e.t.Helper()
	src := "web"
	if anon == "" && cust == nil {
		src = analytics.SourceAggregate
	}
	if props == nil {
		props = map[string]any{}
	}
	if _, err := e.st.AppendEvent(context.Background(), analytics.SpineEvent{
		EventName: name, OccurredAt: e.now.Add(-ago), ChannelID: &e.ch, AnonID: anon, CustomerID: cust,
		Source: src, Props: props, DedupeKey: "eng:" + uuid.NewString(),
	}); err != nil {
		e.t.Fatal(err)
	}
}

func (e *engineSpine) flush() {
	e.t.Helper()
	if err := e.st.flush(context.Background()); err != nil {
		e.t.Fatal(err)
	}
	e.st.RefreshIdentities(context.Background())
}

func seedEngines(t *testing.T) *engineSpine {
	e := newEngineSpine(t)
	h := time.Hour
	mobile, desktop := map[string]any{"device": "mobile"}, map[string]any{"device": "desktop"}
	e.add("page_view", "a", nil, 50*h, mobile) // a: all three steps
	e.add("view_item", "a", nil, 49*h, nil)
	e.add("cart_add", "a", nil, 48*h, nil)
	e.add("page_view", "b", nil, 30*h, desktop) // b: two steps
	e.add("view_item", "b", nil, 29*h, nil)
	e.add("cart_add", "c", nil, 20*h, nil) // c: the steps out of order, so only the first counts
	e.add("page_view", "c", nil, 19*h, mobile)
	e.add("page_view", "d", nil, 10*h, mobile) // d: signs in, then finishes the funnel as the customer
	cust := uuid.New()
	if err := e.st.UpsertIdentity(context.Background(), "d", cust, e.ch); err != nil {
		t.Fatal(err)
	}
	e.add("view_item", "", &cust, 9*h, nil)
	e.add("cart_add", "", &cust, 8*h, nil)
	for _, n := range []string{"page_view", "view_item", "cart_add"} { // a bot, everywhere
		e.add(n, "bot", nil, 5*h, map[string]any{"is_bot": true})
	}
	e.add("page_view", "", nil, 2*h, nil)          // a visitor counted as nobody
	e.add("page_view", "a", nil, 10*24*h, desktop) // a, in the prior window
	e.flush()
	return e
}

func TestFunnelCountsPeopleInOrderWithoutBots(t *testing.T) {
	e := seedEngines(t)
	defer e.st.Close()
	steps, err := e.st.Funnel(context.Background(), e.ch, []string{"page_view", "view_item", "cart_add"}, e.now.Add(-3*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	got := []int64{steps[0].Count, steps[1].Count, steps[2].Count}
	if want := []int64{4, 3, 2}; got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("funnel = %v, want %v: a, b, c and d start; a, b and the signed-in d view; a and d add", got, want)
	}
}

func TestInsightsTotalsUniquesAndBreakdown(t *testing.T) {
	e := seedEngines(t)
	defer e.st.Close()
	since := e.now.Add(-3 * 24 * time.Hour)
	ins, err := e.st.Insights(context.Background(), e.ch, "page_view", "device", "total", "day", since, e.now)
	if err != nil {
		t.Fatal(err)
	}
	// a, b, c, d and the nobody visit - the bot is out, and the nobody visit counts in the total but has no person
	if ins.Total != 5 || ins.Uniques != 4 || ins.Aggregate != 1 {
		t.Fatalf("total %d uniques %d aggregate %d, want 5, 4 and 1", ins.Total, ins.Uniques, ins.Aggregate)
	}
	if ins.PrevTotal != 0 { // a's older visit is 10 days back, outside the prior 3-day window
		t.Errorf("prev total = %d, want 0", ins.PrevTotal)
	}
	series := map[string]int64{}
	for _, s := range ins.Series {
		series[s.Key] = s.Total
	}
	if series["mobile"] != 3 || series["desktop"] != 1 || series["(none)"] != 1 {
		t.Fatalf("breakdown = %v, want mobile 3, desktop 1, (none) 1", series)
	}
}

func TestStickinessActiveAndReturning(t *testing.T) {
	e := seedEngines(t)
	defer e.st.Close()
	st, err := e.st.Stickiness(context.Background(), e.ch, "page_view", e.now.Add(-30*24*time.Hour), e.now, 7)
	if err != nil {
		t.Fatal(err)
	}
	// active this week: a, b, c, d - and only a was also active the week before
	if st.WAU != 4 || st.MAU != 4 || st.ActiveNow != 4 || st.PriorActive != 1 || st.Returning != 1 {
		t.Fatalf("wau %d mau %d now %d prior %d returning %d, want 4, 4, 4, 1, 1", st.WAU, st.MAU, st.ActiveNow, st.PriorActive, st.Returning)
	}
}
