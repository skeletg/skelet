package spinech

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

func (s *Store) Retention(ctx context.Context, channelID uuid.UUID, q analytics.RetentionQuery, since, now time.Time) (analytics.Retention, error) {
	startEvent, startFilters, returnEvent, returnFilters, periods, unit := q.StartEvent, q.StartFilters, q.ReturnEvent, q.ReturnFilters, q.Periods, q.Unit
	out := analytics.Retention{Buckets: make([]analytics.RetentionBucket, periods)}
	for i := range out.Buckets {
		out.Buckets[i].Period = i
	}
	if periods <= 0 || unit <= 0 {
		return out, nil
	}
	// Cohort starts round down from Go's zero time like time.Truncate - Unix-epoch weeks would start on a Thursday
	unitMs := unit.Milliseconds()
	// The cohort and the returns are held to their own filters: "bought X" to start, "any order" to come back
	sf, sfArgs, err := filterSQL(startFilters)
	if err != nil {
		return out, err
	}
	rf, rfArgs, err := filterSQL(returnFilters)
	if err != nil {
		return out, err
	}
	// Each person's return periods are worked out in ClickHouse and kept distinct, so no array grows past the period count
	// however often someone came back - the earlier read carried every return's timestamp per person into the aggregate
	rows, err := s.conn.Query(ctx, `
		WITH anchors AS (
			SELECT actor, anchor_ms, anchor_ms - ((anchor_ms + 62135596800000) % ?) AS cohort_ms
			FROM (
				SELECT `+resolvedActor+` AS actor, toInt64(toUnixTimestamp64Milli(min(e.occurred_at))) AS anchor_ms
				FROM analytics_events AS e FINAL
				WHERE e.channel_id = ? AND e.occurred_at >= ? AND `+notBot+` AND e.event_name = ?`+sf+` AND `+resolvedActor+` != ''
				GROUP BY actor
			)
		),
		-- one row per person per period, its latest return: cohorts sit on the same period grid, so any return in a period
		-- lands in the same bucket, and the latest is the one that can clear the anchor inside the anchor's own period
		returns AS (
			SELECT `+resolvedActor+` AS actor, max(toInt64(toUnixTimestamp64Milli(e.occurred_at))) AS ts_ms
			FROM analytics_events AS e FINAL
			WHERE e.channel_id = ? AND e.occurred_at >= ? AND `+notBot+` AND e.event_name = ?`+rf+` AND `+resolvedActor+` != ''
			GROUP BY actor, intDiv(toInt64(toUnixTimestamp64Milli(e.occurred_at)) + 62135596800000, ?)
		),
		bucketed AS (
			SELECT a.actor, any(a.cohort_ms) AS cohort_ms,
			       groupUniqArrayIf(intDiv(r.ts_ms - a.cohort_ms, ?), r.ts_ms >= a.anchor_ms AND intDiv(r.ts_ms - a.cohort_ms, ?) < ?) AS buckets
			FROM anchors AS a LEFT JOIN returns AS r ON r.actor = a.actor
			GROUP BY a.actor
		)
		SELECT cohort_ms, toInt64(count()) AS size,
		       sumForEach(arrayMap(b -> toInt64(has(buckets, b)), range(?))) AS counts
		FROM bucketed GROUP BY cohort_ms`,
		append(append(append(append(append([]any{unitMs, channelID, since, startEvent}, sfArgs...), channelID, since, returnEvent), rfArgs...),
			unitMs, unitMs, unitMs, int64(periods)), uint64(periods))...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	var cohorts []analytics.RetentionCohortRow
	for rows.Next() {
		var (
			startMs int64
			c       analytics.RetentionCohortRow
		)
		if err := rows.Scan(&startMs, &c.Size, &c.Counts); err != nil {
			return out, err
		}
		c.Start = time.UnixMilli(startMs).UTC()
		cohorts = append(cohorts, c)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return analytics.RetentionFromCohorts(cohorts, now, periods, unit), nil
}
