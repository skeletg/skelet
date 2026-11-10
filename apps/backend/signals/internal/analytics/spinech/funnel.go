package spinech

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

func (s *Store) Funnel(ctx context.Context, channelID uuid.UUID, q analytics.FunnelQuery, since time.Time) (analytics.FunnelResult, error) {
	steps := q.Steps
	out := analytics.FunnelResult{Steps: make([]analytics.FunnelStep, len(steps))}
	for i, st := range steps {
		out.Steps[i] = analytics.FunnelStep{EventName: st}
	}
	if len(steps) < 2 {
		return out, nil
	}
	// Native windowFunnel: per resolved person, the longest in-order chain of the step events (greedy earliest-match) within
	// the conversion window - unset is ~10 years (ms, matching toUnixTimestamp64Milli), no limit inside the query window -
	// and countIf(level >= k) is the people who reached step k
	window := int64(315360000000)
	if q.Window > 0 {
		window = q.Window.Milliseconds()
	}
	conds := make([]string, len(steps))
	condArgs := make([][]any, len(steps))
	for i, st := range steps {
		conds[i], condArgs[i] = "e.event_name = ?", []any{st}
		for _, f := range q.Filters {
			if f.Step == i {
				fs, fa, err := filterSQL([]analytics.PropertyFilter{f.PropertyFilter})
				if err != nil {
					return out, err
				}
				conds[i] += fs
				condArgs[i] = append(condArgs[i], fa...)
			}
		}
	}
	var args []any
	for _, a := range condArgs {
		args = append(args, a...)
	}
	// The breakdown is the property's value on a person's first step-one event - a person who never reached step one
	// lands in '' with nothing counted, and HAVING drops that group
	bd := "'(none)'"
	if q.Breakdown != "" {
		label, labelArgs := propLabel(q.Breakdown)
		bd = "argMinIf(" + label + ", e.occurred_at, " + conds[0] + ")"
		args = append(append(args, labelArgs...), condArgs[0]...)
	}
	args = append(args, channelID, since)
	cols := make([]string, len(steps))      // people per step and group
	names := make([]string, len(steps))     // the same, by name
	totals := make([]string, len(steps))    // people per step over every group, before the cap
	inHolders := make([]string, len(steps)) // event_name IN (...)
	for i, st := range steps {
		names[i] = fmt.Sprintf("c%d", i+1)
		cols[i] = fmt.Sprintf("toInt64(countIf(level >= %d)) AS %s", i+1, names[i])
		totals[i] = fmt.Sprintf("sum(%s) OVER ()", names[i])
		inHolders[i] = "?"
		args = append(args, st)
	}
	args = append(args, uint64(analytics.FunnelBreakdownMax))
	// the empty-actor guard - without it every unattributable event (zero customer, empty anon) merges into one phantom person whose combined stream walks the funnel
	sql := fmt.Sprintf(`
		SELECT bd, %s, %s
		FROM (
			SELECT bd, %s
			FROM (
				SELECT windowFunnel(%d)(toUInt64(toUnixTimestamp64Milli(e.occurred_at)), %s) AS level, %s AS bd
				FROM analytics_events AS e FINAL
				WHERE e.channel_id = ? AND e.occurred_at >= ? AND %s AND e.event_name IN (%s)
				  AND %s != ''
				GROUP BY %s
			)
			GROUP BY bd
			HAVING c1 > 0
		)
		ORDER BY c1 DESC, bd
		LIMIT ?`,
		strings.Join(names, ", "), strings.Join(totals, ", "), strings.Join(cols, ", "),
		window, strings.Join(conds, ", "), bd,
		notBot, strings.Join(inHolders, ", "), resolvedActor, resolvedActor)
	rows, err := s.conn.Query(ctx, sql, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		b := analytics.FunnelBreakdown{Counts: make([]int64, len(steps))}
		dest := []any{&b.Value}
		for i := range b.Counts {
			dest = append(dest, &b.Counts[i])
		}
		for i := range out.Steps {
			dest = append(dest, &out.Steps[i].Count)
		}
		if err := rows.Scan(dest...); err != nil {
			return out, err
		}
		if q.Breakdown != "" {
			out.Breakdown = append(out.Breakdown, b)
		}
	}
	return out, rows.Err()
}
