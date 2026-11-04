package lens

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

func TestDataHealthCountsWhatIngestStripped(t *testing.T) {
	conn := lensDB(t)
	q := signalsdbNew(conn)
	rollup := analytics.NewTrafficRollup(q)
	svc := NewService(q, analytics.NewRecorder(nil, &fakeSpine{}), nil, rollup)
	ch := uuid.New()
	ctx := context.Background()
	for range 2 {
		svc.Ingest(ctx, ch, "anon-1", analytics.Hit{}, Batch{Events: []Event{
			{ID: uuid.NewString(), Type: analytics.EventPageView, PageKey: "/", Props: map[string]any{"revenue_cents": 1.0, "coupon": "X"}},
			{ID: uuid.NewString(), Type: analytics.EventCustom, PageKey: "/", Name: "$search", Props: map[string]any{"query": "tee", "results": "many"}},
			{ID: uuid.NewString(), Type: analytics.EventCustom, PageKey: "/", Name: "$made.up"},
		}}, uuid.Nil)
	}
	if err := rollup.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	ov, err := svc.Overview(ctx, ch, time.Now().AddDate(0, 0, -7))
	if err != nil {
		t.Fatal(err)
	}
	got := map[PlanIssue]bool{}
	for _, i := range ov.PlanIssues {
		got[i] = true
	}
	for _, want := range []PlanIssue{
		{Kind: analytics.IssueServerProp, Event: "page_view", Prop: "revenue_cents", Count: 2},
		{Kind: analytics.IssueUnknownProp, Event: "page_view", Prop: "coupon", Count: 2},
		{Kind: analytics.IssueBadType, Event: "$search", Prop: "results", Count: 2},
		{Kind: analytics.IssueUnknownEvent, Event: "$made.up", Count: 2},
	} {
		if !got[want] {
			t.Errorf("missing %+v in %+v", want, ov.PlanIssues)
		}
	}
	if len(ov.PlanIssues) != 4 {
		t.Errorf("%d issues, want 4: %+v", len(ov.PlanIssues), ov.PlanIssues)
	}
}
