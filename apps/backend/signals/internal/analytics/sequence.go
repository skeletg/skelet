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

// A forward path tree from a focus event - names must already be resolved (custom events under their real name, the $experiment ping dropped), depth caps the journey length and topK keeps the busiest children per node
func FlowFromEvents(events []SpineActorEvent, focus string, depth, topK int) FlowResult {
	if depth < 1 {
		depth = 1
	}
	if topK < 1 {
		topK = 1
	}
	edges := map[string]map[string]int64{} // parent path → child event → count
	pathCounts := map[string]int64{}       // complete collapsed path → people
	var cohort, continued, totalSteps int64
	curActor := ""
	have := false
	var prefix, lastEvent string
	steps := 0
	hitFocus := false
	process := func(eventName string) {
		if !hitFocus {
			if eventName == focus {
				hitFocus, cohort, prefix, lastEvent = true, cohort+1, focus, focus
			}
			return
		}
		if eventName == lastEvent || steps >= depth {
			return
		}
		if edges[prefix] == nil {
			edges[prefix] = map[string]int64{}
		}
		edges[prefix][eventName]++
		prefix += flowSep + eventName
		lastEvent = eventName
		steps++
	}
	finalize := func() {
		if hitFocus && steps > 0 {
			pathCounts[prefix]++
			totalSteps += int64(steps)
			continued++
		}
	}
	for _, ev := range events {
		if !have {
			curActor, have = ev.Actor, true
		} else if ev.Actor != curActor {
			finalize()
			curActor, hitFocus, prefix, lastEvent, steps = ev.Actor, false, "", "", 0
		}
		process(ev.EventName)
	}
	if have {
		finalize()
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
		if it.depth >= depth {
			continue
		}
		children := edges[it.prefix]
		if len(children) == 0 {
			continue
		}
		type ec struct {
			event string
			count int64
		}
		ranked := make([]ec, 0, len(children))
		for ev, c := range children {
			ranked = append(ranked, ec{ev, c})
		}
		sort.Slice(ranked, func(a, b int) bool {
			if ranked[a].count != ranked[b].count {
				return ranked[a].count > ranked[b].count
			}
			return ranked[a].event < ranked[b].event
		})
		if len(ranked) > topK {
			ranked = ranked[:topK]
		}
		for _, rk := range ranked {
			childPrefix := it.prefix + flowSep + rk.event
			nodes = append(nodes, FlowNode{ID: childPrefix, ParentID: it.prefix, Event: rk.event, Depth: it.depth + 1, Count: rk.count})
			queue = append(queue, qi{childPrefix, it.depth + 1})
		}
	}

	type pc struct {
		path  string
		count int64
	}
	rankedPaths := make([]pc, 0, len(pathCounts))
	for p, c := range pathCounts {
		rankedPaths = append(rankedPaths, pc{p, c})
	}
	sort.Slice(rankedPaths, func(a, b int) bool {
		if rankedPaths[a].count != rankedPaths[b].count {
			return rankedPaths[a].count > rankedPaths[b].count
		}
		return rankedPaths[a].path < rankedPaths[b].path
	})
	const maxPaths = 6
	if len(rankedPaths) > maxPaths {
		rankedPaths = rankedPaths[:maxPaths]
	}
	topPaths := make([]FlowPath, 0, len(rankedPaths))
	for _, rk := range rankedPaths {
		seq := strings.Split(rk.path, flowSep)[1:]
		topPaths = append(topPaths, FlowPath{Events: seq, Count: rk.count})
	}
	avg := 0.0
	if continued > 0 {
		avg = float64(totalSteps) / float64(continued)
	}
	return FlowResult{Cohort: cohort, Nodes: nodes, Continued: continued, AvgSteps: avg, TopPaths: topPaths}
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
