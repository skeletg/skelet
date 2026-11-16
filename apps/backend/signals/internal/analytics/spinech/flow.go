package spinech

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

func (s *Store) Flow(ctx context.Context, channelID uuid.UUID, q analytics.FlowQuery, since time.Time) (analytics.FlowResult, error) {
	focus, focusFilters, excl, depth, topK := q.Focus, q.Filters, q.NotThen, q.Depth, q.TopK
	// Custom events surface by their real name, and the tracker's own $-events (experiments, vitals, rage and dead clicks)
	// are dropped from the paths unless one is the focus - each journey is cut to what the walker uses: from the first focus event, repeats collapsed, depth steps at most
	if depth < 1 {
		depth = 1
	}
	// Identical journeys are counted in ClickHouse, so Go gets one row per distinct path rather than one per person, and
	// only the flowMaxPaths most common - the headline totals come from every path, before the cap
	// The people the flow follows: who did the focus (held to its filters), less - for a drop-off - those who went on to
	// the excluded event within the window after their first focus
	ff, ffArgs, err := filterSQL(focusFilters)
	if err != nil {
		return analytics.FlowResult{}, err
	}
	focusActors := `SELECT DISTINCT ` + resolvedActor + ` AS actor
			FROM analytics_events AS e FINAL
			WHERE e.channel_id = ? AND e.occurred_at >= ? AND ` + notBot + `
			  AND (e.event_name = ? OR (e.event_name = 'custom' AND e.custom_name = ?))` + ff + `
			  AND ` + resolvedActor + ` != ''`
	focusArgs := append([]any{channelID, since, focus, focus}, ffArgs...)
	if excl != nil && excl.Event != "" {
		focusActors = `SELECT f.actor FROM (
				SELECT ` + resolvedActor + ` AS actor, min(e.occurred_at) AS first_at
				FROM analytics_events AS e FINAL
				WHERE e.channel_id = ? AND e.occurred_at >= ? AND ` + notBot + `
				  AND (e.event_name = ? OR (e.event_name = 'custom' AND e.custom_name = ?))` + ff + `
				  AND ` + resolvedActor + ` != ''
				GROUP BY actor
			) AS f
			LEFT JOIN (
				SELECT ` + resolvedActor + ` AS actor, groupArray(e.occurred_at) AS ts
				FROM analytics_events AS e FINAL
				WHERE e.channel_id = ? AND e.occurred_at >= ? AND ` + notBot + `
				  AND (e.event_name = ? OR (e.event_name = 'custom' AND e.custom_name = ?))
				  AND ` + resolvedActor + ` != ''
				GROUP BY actor
			) AS x ON x.actor = f.actor
			WHERE NOT arrayExists(t -> t >= f.first_at AND (? = 0 OR t <= f.first_at + toIntervalSecond(?)), x.ts)`
		within := int64(excl.Within.Seconds())
		focusArgs = append(focusArgs, channelID, since, excl.Event, excl.Event, within, within)
	}
	rows, err := s.conn.Query(ctx, `
		SELECT path, n, sum(n) OVER () AS cohort, sum(if(length(path) > 1, n, 0)) OVER () AS continued,
		       sum(n * (length(path) - 1)) OVER () AS steps
		FROM (
		SELECT path, toInt64(count()) AS n FROM (
		WITH focus_actors AS (
			`+focusActors+`
		)
		SELECT actor, arraySlice(arrayFilter((x, i) -> i = 1 OR x != journey[i - 1], journey, arrayEnumerate(journey)), 1, ?) AS path
		FROM (
			SELECT actor, arraySlice(evs, indexOf(evs, ?)) AS journey
			FROM (
				SELECT `+resolvedActor+` AS actor,
				       arrayMap(t -> t.2, arraySort(t -> t.1, groupArray((e.occurred_at,
				           if(e.event_name = 'custom', if(e.custom_name != '', e.custom_name, 'custom'), e.event_name))))) AS evs
				FROM analytics_events AS e FINAL
				WHERE e.channel_id = ? AND e.occurred_at >= ? AND `+notBot+`
				  AND `+resolvedActor+` IN (SELECT actor FROM focus_actors)
				  AND NOT (e.event_name = 'custom' AND startsWith(e.custom_name, '$') AND e.custom_name != ?)
				  AND `+resolvedActor+` != ''
				GROUP BY actor
			)
			WHERE has(evs, ?)
		)
		) GROUP BY path
		)
		ORDER BY n DESC, path
		LIMIT ?`,
		append(append(focusArgs, uint64(depth+1), focus, channelID, since, focus, focus), uint64(flowMaxPaths))...)
	if err != nil {
		return analytics.FlowResult{}, err
	}
	defer rows.Close()
	var (
		paths  []analytics.FlowPathCount
		totals analytics.FlowTotals
	)
	for rows.Next() {
		var p analytics.FlowPathCount
		if err := rows.Scan(&p.Path, &p.People, &totals.Cohort, &totals.Continued, &totals.Steps); err != nil {
			return analytics.FlowResult{}, err
		}
		p.Path = p.Path[1:] // every path opens on the focus event
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return analytics.FlowResult{}, err
	}
	return analytics.FlowFromPaths(paths, totals, focus, topK), nil
}

// The distinct journeys a Flow read brings back - the tree only shows the top few children per step, so the rare tail past this changes nothing on screen
const flowMaxPaths = 10_000
