package analytics

import (
	"testing"
	"time"
)

// These walkers are what keeps Retention/Flow/Insights byte-identical across the Postgres and ClickHouse drivers, so they get always-on synthetic-event tests - the driver-level checks only run against live databases

func at(day, hour int) time.Time {
	return time.Date(2026, 8, day, hour, 0, 0, 0, time.UTC)
}

func TestRetentionFromEvents(t *testing.T) {
	unit := 24 * time.Hour
	now := at(11, 0)
	events := []SpineActorEvent{
		{Actor: "alice", EventName: "page_view", OccurredAt: at(1, 9)}, // alice starts day 1, returns day 2 and day 4
		{Actor: "alice", EventName: "purchase", OccurredAt: at(2, 9)},
		{Actor: "alice", EventName: "purchase", OccurredAt: at(4, 9)},
		{Actor: "bob", EventName: "purchase", OccurredAt: at(1, 8)}, // bob starts day 1 and never returns - this purchase is BEFORE his start and must not count
		{Actor: "bob", EventName: "page_view", OccurredAt: at(1, 9)},
		{Actor: "carol", EventName: "page_view", OccurredAt: at(3, 9)}, // carol starts day 3, returns in the same period (period 0)
		{Actor: "carol", EventName: "purchase", OccurredAt: at(3, 20)},
		{Actor: "dave", EventName: "purchase", OccurredAt: at(2, 9)}, // dave never does the start event at all
	}
	r := RetentionFromEvents(events, "page_view", "purchase", now, 4, unit)

	if r.Cohort != 3 {
		t.Fatalf("Cohort = %d, want 3 (alice, bob, carol)", r.Cohort)
	}
	wantCounts := []int64{1, 1, 0, 1} // p0 carol, p1 alice, p3 alice
	for i, w := range wantCounts {
		if r.Buckets[i].Count != w {
			t.Errorf("Buckets[%d].Count = %d, want %d", i, r.Buckets[i].Count, w)
		}
	}
	for i := range r.Buckets { // at now = day 11 every cohort is old enough for every period
		if r.Buckets[i].Eligible != 3 {
			t.Errorf("Buckets[%d].Eligible = %d, want 3", i, r.Buckets[i].Eligible)
		}
	}
	if len(r.Cohorts) != 2 {
		t.Fatalf("Cohorts = %d rows, want 2 (day-1 and day-3)", len(r.Cohorts))
	}
	if r.Cohorts[0].Size != 2 || r.Cohorts[1].Size != 1 {
		t.Errorf("cohort sizes = %d, %d, want 2, 1", r.Cohorts[0].Size, r.Cohorts[1].Size)
	}
}

// A cohort too young for later periods must not deflate them - day-10 starters can't have returned in period 3 yet, so they aren't eligible for it
func TestRetentionEligibilityExcludesImmatureCohorts(t *testing.T) {
	unit := 24 * time.Hour
	now := at(11, 12)
	events := []SpineActorEvent{
		{Actor: "old", EventName: "start", OccurredAt: at(1, 9)},
		{Actor: "young", EventName: "start", OccurredAt: at(10, 9)},
	}
	r := RetentionFromEvents(events, "start", "start", now, 4, unit)
	if r.Buckets[0].Eligible != 2 {
		t.Errorf("period 0 eligible = %d, want 2", r.Buckets[0].Eligible)
	}
	if r.Buckets[3].Eligible != 1 {
		t.Errorf("period 3 eligible = %d, want only the day-1 starter", r.Buckets[3].Eligible)
	}
}

func TestFlowFromEvents(t *testing.T) {
	events := []SpineActorEvent{
		{Actor: "alice", EventName: "page_view", OccurredAt: at(1, 1)}, // alice: focus -> cart_add -> cart_add (consecutive dupe collapses) -> checkout
		{Actor: "alice", EventName: "cart_add", OccurredAt: at(1, 2)},
		{Actor: "alice", EventName: "cart_add", OccurredAt: at(1, 3)},
		{Actor: "alice", EventName: "checkout", OccurredAt: at(1, 4)},
		{Actor: "bob", EventName: "checkout", OccurredAt: at(1, 1)}, // bob: pre-focus noise, then focus -> cart_add
		{Actor: "bob", EventName: "page_view", OccurredAt: at(1, 2)},
		{Actor: "bob", EventName: "cart_add", OccurredAt: at(1, 3)},
		{Actor: "carol", EventName: "page_view", OccurredAt: at(1, 1)}, // carol: focus then nothing
	}
	f := FlowFromEvents(events, "page_view", 3, 5)

	if f.Cohort != 3 {
		t.Fatalf("Cohort = %d, want 3", f.Cohort)
	}
	if f.Continued != 2 {
		t.Fatalf("Continued = %d, want 2 (carol stopped at the focus)", f.Continued)
	}
	if len(f.Nodes) == 0 || f.Nodes[0].Event != "cart_add" || f.Nodes[0].Count != 2 || f.Nodes[0].Depth != 1 { // first hop is cart_add, with both actors
		t.Fatalf("Nodes[0] = %+v, want cart_add count=2 depth=1", f.Nodes)
	}
	if want := (2.0 + 1.0) / 2.0; f.AvgSteps != want {
		t.Errorf("AvgSteps = %v, want %v (alice 2 collapsed steps, bob 1)", f.AvgSteps, want)
	}
	if len(f.TopPaths) != 2 {
		t.Fatalf("TopPaths = %d, want 2 distinct journeys", len(f.TopPaths))
	}
}

// depth caps journey length and topK prunes to the busiest children per node
func TestFlowFromEventsDepthAndTopK(t *testing.T) {
	events := []SpineActorEvent{
		{Actor: "a", EventName: "f", OccurredAt: at(1, 1)},
		{Actor: "a", EventName: "s1", OccurredAt: at(1, 2)},
		{Actor: "a", EventName: "s2", OccurredAt: at(1, 3)},
		{Actor: "a", EventName: "s3", OccurredAt: at(1, 4)},
		{Actor: "b", EventName: "f", OccurredAt: at(1, 1)},
		{Actor: "b", EventName: "s1", OccurredAt: at(1, 2)},
		{Actor: "c", EventName: "f", OccurredAt: at(1, 1)},
		{Actor: "c", EventName: "other", OccurredAt: at(1, 2)},
	}
	f := FlowFromEvents(events, "f", 2, 1)
	for _, n := range f.Nodes {
		if n.Depth > 2 {
			t.Errorf("node %q at depth %d, want the depth-2 cap to hold", n.Event, n.Depth)
		}
		if n.Depth == 1 && n.Event != "s1" {
			t.Errorf("depth-1 node %q survived topK=1, want only the busiest (s1)", n.Event)
		}
	}
}

func TestRollupInsightsSeries(t *testing.T) {
	day := at(1, 0)
	rows := []InsightsSeriesInput{
		{Bucket: day, Breakdown: "big", Total: 100, Uniques: 10},
		{Bucket: day, Breakdown: "small-1", Total: 1, Uniques: 1},
		{Bucket: day, Breakdown: "small-2", Total: 2, Uniques: 1},
		{Bucket: day, Breakdown: "small-3", Total: 3, Uniques: 1},
		{Bucket: day, Breakdown: "small-4", Total: 4, Uniques: 1},
		{Bucket: day, Breakdown: "small-5", Total: 5, Uniques: 1},
		{Bucket: day, Breakdown: "small-6", Total: 6, Uniques: 1},
		{Bucket: day, Breakdown: "small-7", Total: 7, Uniques: 1},
		{Bucket: day, Breakdown: "small-8", Total: 8, Uniques: 1},
		{Bucket: day, Breakdown: "small-9", Total: 9, Uniques: 1},
	}
	points, series := RollupInsightsSeries(rows, false)

	if len(series) != insightsTopSeries+1 {
		t.Fatalf("series = %d, want the top %d plus (other)", len(series), insightsTopSeries)
	}
	if series[0].Key != "big" || series[0].Total != 100 {
		t.Fatalf("series[0] = %+v, want big=100 ranked first", series[0])
	}
	var other int64
	for _, s := range series {
		if s.Key == "(other)" {
			other = s.Total
		}
	}
	if other != 3 { // small-1 + small-2 fold (the two weakest of ten)
		t.Errorf("(other) total = %d, want 3", other)
	}
	sum := int64(0)
	for _, p := range points {
		sum += p.Value
	}
	if sum != 145 {
		t.Errorf("points sum = %d, want every input total preserved (145)", sum)
	}
}
