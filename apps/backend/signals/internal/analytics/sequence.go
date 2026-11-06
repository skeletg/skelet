package analytics

import (
	"slices"
	"sort"
	"strings"
	"time"
)

// What the sequence walkers consume - the walkers stay the reference the ClickHouse reads are tested against
type SpineActorEvent struct {
	Actor      string
	EventName  string
	OccurredAt time.Time // rows arrive ordered by actor, then this
}

// Classic cohort retention: cohorts by the period bucket of each person's first start event, returns relative to that bucket, and per-bucket eligible denominators - a cohort only counts toward period N once it's old enough to have reached it
func RetentionFromEvents(events []SpineActorEvent, startEvent, returnEvent string, now time.Time, periods int, unit time.Duration) Retention {
	out := Retention{Buckets: make([]RetentionBucket, periods)}
	for i := range out.Buckets {
		out.Buckets[i].Period = i
	}
	if periods <= 0 || unit <= 0 {
		return out
	}
	type cohortAcc struct {
		start time.Time
		size  int64
		count []int64
	}
	cohorts := map[int64]*cohortAcc{}
	curActor := ""
	have := false
	var anchor, cohortStart time.Time
	anchored := false
	seen := make([]bool, periods)
	finish := func() {
		if !anchored {
			return
		}
		key := cohortStart.UnixNano()
		c := cohorts[key]
		if c == nil {
			c = &cohortAcc{start: cohortStart, count: make([]int64, periods)}
			cohorts[key] = c
		}
		c.size++
		for b := range periods {
			if seen[b] {
				c.count[b]++
			}
		}
	}
	reset := func(actor string) {
		curActor = actor
		anchored = false
		for b := range seen {
			seen[b] = false
		}
	}
	for _, ev := range events {
		if !have || ev.Actor != curActor {
			if have {
				finish()
			}
			reset(ev.Actor)
			have = true
		}
		if !anchored && ev.EventName == startEvent {
			anchor, anchored = ev.OccurredAt, true
			cohortStart = anchor.Truncate(unit)
		}
		if anchored && ev.EventName == returnEvent {
			b := int(ev.OccurredAt.Sub(cohortStart) / unit)
			if b >= 0 && b < periods {
				seen[b] = true
			}
		}
	}
	if have {
		finish()
	}

	list := make([]RetentionCohortRow, 0, len(cohorts))
	for _, c := range cohorts {
		list = append(list, RetentionCohortRow{Start: c.start, Size: c.size, Counts: c.count})
	}
	return RetentionFromCohorts(list, now, periods, unit)
}

// One cohort as the store aggregates it - its start bucket, its size and its returns per period
type RetentionCohortRow struct {
	Start  time.Time
	Size   int64
	Counts []int64 // indexed by period, len periods
}

// The finish both retention paths share
func RetentionFromCohorts(cohorts []RetentionCohortRow, now time.Time, periods int, unit time.Duration) Retention {
	out := Retention{Buckets: make([]RetentionBucket, periods)}
	for i := range out.Buckets {
		out.Buckets[i].Period = i
	}
	if periods <= 0 || unit <= 0 {
		return out
	}
	for _, c := range cohorts {
		out.Cohort += c.Size
		maxP := int(now.Sub(c.Start) / unit)
		for b := range periods {
			if b <= maxP {
				out.Buckets[b].Eligible += c.Size
			}
			out.Buckets[b].Count += c.Counts[b]
		}
	}

	list := slices.Clone(cohorts)
	sort.Slice(list, func(a, b int) bool { return list[a].Start.Before(list[b].Start) })
	const maxCohorts = 12
	if len(list) > maxCohorts {
		list = list[len(list)-maxCohorts:]
	}
	out.Cohorts = make([]RetentionCohort, 0, len(list))
	for _, c := range list {
		maxP := int(now.Sub(c.Start) / unit)
		rc := RetentionCohort{StartAt: c.Start, Size: c.Size, Buckets: make([]RetentionBucket, periods)}
		for b := range periods {
			rc.Buckets[b].Period = b
			rc.Buckets[b].Count = c.Counts[b]
			if b <= maxP {
				rc.Buckets[b].Eligible = c.Size
			}
		}
		out.Cohorts = append(out.Cohorts, rc)
	}
	return out
}

// One journey after the focus event and how many people took it - repeats collapsed, focus excluded, depth steps at most
type FlowPathCount struct {
	Path   []string
	People int64
}

// A flow's headline numbers - who reached the focus event, who took a step after it, and how many steps those took in all
type FlowTotals struct {
	Cohort, Continued, Steps int64
}

// The reference walker - names must already be resolved (custom events under their real name, the $experiment ping
// dropped): each actor's journey from their first focus event, then the tree FlowFromPaths builds for a store
func FlowFromEvents(events []SpineActorEvent, focus string, depth, topK int) FlowResult {
	depth = max(depth, 1)
	var (
		paths   []FlowPathCount
		totals  FlowTotals
		actor   string
		reached bool
		steps   []string
		prev    string // the last event kept, so a repeat collapses
	)
	finalize := func() {
		if !reached {
			return
		}
		paths = append(paths, FlowPathCount{Path: steps, People: 1})
		totals.Cohort++
		if len(steps) > 0 {
			totals.Continued++
			totals.Steps += int64(len(steps))
		}
	}
	for i, ev := range events {
		if i == 0 || ev.Actor != actor {
			finalize()
			actor, reached, steps = ev.Actor, false, nil
		}
		switch {
		case !reached:
			reached, prev = ev.EventName == focus, focus
		case len(steps) < depth && ev.EventName != prev:
			steps, prev = append(steps, ev.EventName), ev.EventName
		}
	}
	finalize()
	return FlowFromPaths(paths, totals, focus, topK)
}

// A forward path tree from grouped journeys - topK keeps the busiest children per node. The totals come separately
// because a store may hand over only its most common journeys
func FlowFromPaths(paths []FlowPathCount, totals FlowTotals, focus string, topK int) FlowResult {
	topK = max(topK, 1)
	edges := map[string]map[string]int64{} // parent path → child event → people
	ended := map[string]int64{}            // complete path → people who took it exactly
	for _, p := range paths {
		prefix := focus
		for _, ev := range p.Path {
			if edges[prefix] == nil {
				edges[prefix] = map[string]int64{}
			}
			edges[prefix][ev] += p.People
			prefix += flowSep + ev
		}
		if len(p.Path) > 0 {
			ended[prefix] += p.People
		}
	}

	type qi struct {
		prefix string
		depth  int
	}
	var nodes []FlowNode
	queue := []qi{{focus, 0}}
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		for _, rk := range ranked(edges[it.prefix], topK) {
			childPrefix := it.prefix + flowSep + rk.key
			nodes = append(nodes, FlowNode{ID: childPrefix, ParentID: it.prefix, Event: rk.key, Depth: it.depth + 1, Count: rk.count})
			queue = append(queue, qi{childPrefix, it.depth + 1})
		}
	}

	const maxPaths = 6
	topPaths := []FlowPath{}
	for _, rk := range ranked(ended, maxPaths) {
		topPaths = append(topPaths, FlowPath{Events: strings.Split(rk.key, flowSep)[1:], Count: rk.count})
	}
	avg := 0.0
	if totals.Continued > 0 {
		avg = float64(totals.Steps) / float64(totals.Continued)
	}
	return FlowResult{Cohort: totals.Cohort, Nodes: nodes, Continued: totals.Continued, AvgSteps: avg, TopPaths: topPaths}
}

type keyCount struct {
	key   string
	count int64
}

// The n largest counts, ties by key
func ranked(m map[string]int64, n int) []keyCount {
	out := make([]keyCount, 0, len(m))
	for k, c := range m {
		out = append(out, keyCount{k, c})
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].count != out[b].count {
			return out[a].count > out[b].count
		}
		return out[a].key < out[b].key
	})
	return out[:min(n, len(out))]
}

type InsightsSeriesInput struct { // one cell from the per-engine InsightsSeries query - what the shared rollup below folds
	Bucket    time.Time
	Breakdown string
	Total     int64
	Uniques   int64
}

// Ranks breakdown values by window total and folds all but the top insightsTopSeries into "(other)" - unique picks the uniques measure over the total
func RollupInsightsSeries(rows []InsightsSeriesInput, unique bool) ([]InsightPoint, []InsightSeriesTotal) {
	val := func(r InsightsSeriesInput) int64 {
		if unique {
			return r.Uniques
		}
		return r.Total
	}
	totals := map[string]int64{}
	for _, row := range rows {
		totals[row.Breakdown] += val(row)
	}
	keep := topKeys(totals, insightsTopSeries)
	label := func(k string) string {
		if keep[k] {
			return k
		}
		return "(other)"
	}
	type cell struct {
		bucket time.Time
		key    string
	}
	pointAgg := map[cell]int64{}
	seriesAgg := map[string]int64{}
	for _, row := range rows {
		k := label(row.Breakdown)
		pointAgg[cell{row.Bucket, k}] += val(row)
		seriesAgg[k] += val(row)
	}
	var points []InsightPoint
	for c, v := range pointAgg {
		points = append(points, InsightPoint{Bucket: c.bucket, Key: c.key, Value: v})
	}
	sort.Slice(points, func(a, b int) bool {
		if !points[a].Bucket.Equal(points[b].Bucket) {
			return points[a].Bucket.Before(points[b].Bucket)
		}
		return points[a].Key < points[b].Key
	})
	var series []InsightSeriesTotal
	for k, t := range seriesAgg {
		series = append(series, InsightSeriesTotal{Key: k, Total: t})
	}
	sort.Slice(series, func(a, b int) bool {
		if series[a].Total != series[b].Total {
			return series[a].Total > series[b].Total
		}
		return series[a].Key < series[b].Key
	})
	return points, series
}
