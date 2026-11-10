package lens

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

func TestASearchOnlyMeaningAnsweredIsReportedApartFromTheZeros(t *testing.T) {
	conn := lensDB(t)
	ctx := context.Background()
	q := signalsdbNew(conn)
	rollup := analytics.NewTrafficRollup(q)
	svc := NewService(q, analytics.NewRecorder(nil, &fakeSpine{}), nil, rollup)
	ch := uuid.New()
	now := time.Now().UnixMilli()
	search := func(id, query string, props map[string]any) Event {
		props["query"] = query
		return Event{ID: id, Type: analytics.EventCustom, Name: analytics.EventSearch, PageKey: "/search", TS: now, Props: props}
	}
	svc.Ingest(ctx, ch, "anon-1", analytics.Hit{}, Batch{Events: []Event{
		search("e1", "linen shirt", map[string]any{"results": 12.0}),
		search("e2", "zxqv", map[string]any{"results": 0.0}),
		search("e3", "something breezy for a wedding", map[string]any{"results": 9.0, "byMeaningOnly": true}),
	}}, uuid.Nil)
	if err := rollup.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	sa, err := svc.SearchAnalytics(ctx, ch, time.Now().AddDate(0, 0, -1))
	if err != nil {
		t.Fatal(err)
	}
	terms := func(list []TermCount) []string {
		var out []string
		for _, c := range list {
			out = append(out, c.Term)
		}
		return out
	}
	if sa.TotalSearches != 3 || sa.ZeroResultSearches != 1 || sa.MeaningOnlySearches != 1 {
		t.Errorf("totals %d searches, %d zero, %d by meaning - want 3, 1, 1", sa.TotalSearches, sa.ZeroResultSearches, sa.MeaningOnlySearches)
	}
	if !slices.Equal(terms(sa.ZeroResult), []string{"zxqv"}) || !slices.Equal(terms(sa.MeaningOnly), []string{"something breezy for a wedding"}) {
		t.Errorf("zero %v, by meaning %v - want each gap in its own list", terms(sa.ZeroResult), terms(sa.MeaningOnly))
	}
	if len(sa.Daily) != 1 || sa.Daily[0].MeaningOnly != 1 || sa.Daily[0].ZeroResults != 1 {
		t.Errorf("daily %+v - want today's one zero and one by meaning", sa.Daily)
	}
}

func TestTheAIOverviewAndTheAIsEmptySearchesJoinTheSearchReport(t *testing.T) {
	conn := lensDB(t)
	ctx := context.Background()
	q := signalsdbNew(conn)
	rollup := analytics.NewTrafficRollup(q)
	svc := NewService(q, analytics.NewRecorder(nil, &fakeSpine{}), nil, rollup)
	ch := uuid.New()
	now := time.Now().UnixMilli()
	overview := func(id, name, query string) Event {
		return Event{ID: id, Type: analytics.EventCustom, Name: name, PageKey: "/search", TS: now, Props: map[string]any{"query": query}}
	}
	svc.Ingest(ctx, ch, "anon-1", analytics.Hit{}, Batch{Events: []Event{
		overview("o1", evOverviewShown, "Something warm for a hike"),
		overview("o2", evOverviewShown, "something warm  for a hike"),
		overview("o3", evOverviewClick, "something warm for a hike"),
		overview("o4", evOverviewChat, "something warm for a hike"),
		overview("o5", evOverviewShown, "a dress for a wedding"),
		overview("o6", evOverviewShown, ""),
	}}, uuid.Nil)
	if err := rollup.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RecordAIActivity(ctx, signalsdbRecordAI(ch, "assistant", "", "t1", "wide fit boots", 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RecordAIActivity(ctx, signalsdbRecordAI(ch, "agent", "Claude", "c1", "wide fit boots", 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RecordAIActivity(ctx, signalsdbRecordAI(ch, "agent", "Claude", "c2", "linen shirt", 3)); err != nil {
		t.Fatal(err)
	}

	sa, err := svc.SearchAnalytics(ctx, ch, time.Now().AddDate(0, 0, -1))
	if err != nil {
		t.Fatal(err)
	}
	if sa.OverviewsShown != 3 || sa.OverviewClicks != 1 || sa.OverviewChats != 1 || sa.TotalSearches != 0 {
		t.Errorf("overviews %d shown, %d clicked, %d chats, %d searches - want 3, 1, 1 and none counted as a search", sa.OverviewsShown, sa.OverviewClicks, sa.OverviewChats, sa.TotalSearches)
	}
	if len(sa.Overviews) != 2 || sa.Overviews[0] != (OverviewTerm{Term: "something warm for a hike", Shown: 2, Clicks: 1, Chats: 1}) {
		t.Errorf("per search %+v - want the hike first, its two spellings as one", sa.Overviews)
	}
	if len(sa.AIGaps) != 1 || sa.AIGaps[0] != (AIGap{Term: "wide fit boots", Assistant: 1, Agents: 1}) {
		t.Errorf("AI gaps %+v - want only the empty search, from both", sa.AIGaps)
	}
}
