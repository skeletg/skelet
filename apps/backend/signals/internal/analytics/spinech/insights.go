package spinech

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

func (s *Store) Insights(ctx context.Context, channelID uuid.UUID, q analytics.InsightsQuery, since, now time.Time) (analytics.Insights, error) {
	if err := q.Validate(); err != nil {
		return analytics.Insights{}, err
	}
	breakdown, measure, unit := q.Breakdown, q.Measure, q.Unit
	bucket := "toStartOfDay(e.occurred_at)"
	if unit == "week" {
		bucket = "toStartOfWeek(e.occurred_at, 1)" // Monday, matching date_trunc('week')
	}
	// breakdown: no key, or the property missing or empty -> '(none)'
	bdCol, bdArgs := "'(none)'", []any(nil)
	if breakdown != "" {
		bdCol, bdArgs = propLabel(breakdown)
	}
	evCond, evArg, err := eventCond(q.Events, q.Filters)
	if err != nil {
		return analytics.Insights{}, err
	}
	from := "FROM analytics_events AS e FINAL"
	uniq := "toInt64(uniqExactIf(" + resolvedActor + ", " + resolvedActor + " != ''))" // distinct non-null resolved
	// A sum adds an integer property up - revenue in cents, units - where a count counts events
	total, totalArgs := "toInt64(count())", []any(nil)
	if measure == "sum" {
		total, totalArgs = "toInt64(sum(JSONExtractInt(e.properties, ?)))", []any{q.SumKey}
	}

	seriesArgs := append(append(append([]any{}, bdArgs...), totalArgs...), channelID, since)
	seriesArgs = append(seriesArgs, evArg...)
	rows, err := s.conn.Query(ctx,
		"SELECT "+bucket+" AS bucket, "+bdCol+" AS breakdown, "+total+" AS total, "+uniq+" AS uniques "+
			from+" WHERE e.channel_id = ? AND e.occurred_at >= ? AND "+notBot+evCond+
			" GROUP BY bucket, breakdown ORDER BY bucket, breakdown", seriesArgs...)
	if err != nil {
		return analytics.Insights{}, err
	}
	var in []analytics.InsightsSeriesInput
	for rows.Next() {
		var r analytics.InsightsSeriesInput
		if err := rows.Scan(&r.Bucket, &r.Breakdown, &r.Total, &r.Uniques); err != nil {
			rows.Close()
			return analytics.Insights{}, err
		}
		in = append(in, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return analytics.Insights{}, err
	}

	var out analytics.Insights
	out.Points, out.Series = analytics.RollupInsightsSeries(in, measure == "unique")

	// window-level measures (headline + prior equal-length window) - errors are swallowed, since the series is the primary result
	wm := "SELECT " + total + ", " + uniq + ", toInt64(countIf(e.source = 'aggregate')) " + from + " WHERE e.channel_id = ? AND " + inWindow + " AND " + notBot + evCond
	window := func(lo, hi time.Time) []any {
		return append(append(append([]any{}, totalArgs...), channelID, lo.UnixMilli(), hi.UnixMilli()), evArg...)
	}
	_ = s.conn.QueryRow(ctx, wm, window(since, now)...).Scan(&out.Total, &out.Uniques, &out.Aggregate)
	prevStart := since.Add(-now.Sub(since))
	var prevAggregate int64
	_ = s.conn.QueryRow(ctx, wm, window(prevStart, since)...).Scan(&out.PrevTotal, &out.PrevUniques, &prevAggregate)
	return out, nil
}
