package spinech

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

func (s *Store) Stickiness(ctx context.Context, channelID uuid.UUID, q analytics.StickinessQuery, since, now time.Time) (analytics.Stickiness, error) {
	event, filters, windowDays := q.Event, q.Filters, q.WindowDays
	if windowDays < 1 {
		windowDays = 30
	}
	out := analytics.Stickiness{WindowDays: windowDays}
	// optional event filter (""=any activity); actor resolved, empty actor dropped
	var events []string
	if event != "" {
		events = []string{event}
	}
	evCond, evArg, err := eventCond(events, filters)
	if err != nil {
		return out, err
	}
	from := "FROM analytics_events AS e FINAL"
	where := " WHERE e.channel_id = ? AND e.occurred_at >= ? AND " + notBot + evCond + " AND " + resolvedActor + " != ''"

	// daily-active series
	dayArgs := append([]any{channelID, since}, evArg...)
	days, err := s.conn.Query(ctx, "SELECT toStartOfDay(e.occurred_at) AS d, toInt64(uniqExact("+resolvedActor+")) AS c "+from+where+" GROUP BY d ORDER BY d", dayArgs...)
	if err != nil {
		return out, err
	}
	for days.Next() {
		var p analytics.ActivePoint
		if err := days.Scan(&p.Day, &p.Count); err != nil {
			days.Close()
			return out, err
		}
		out.Days = append(out.Days, p)
	}
	days.Close()
	if err := days.Err(); err != nil {
		return out, err
	}

	// WAU / MAU (trailing 7 / 30 days)
	active := "SELECT toInt64(uniqExact(" + resolvedActor + ")) " + from + where
	if err := s.conn.QueryRow(ctx, active, append([]any{channelID, now.AddDate(0, 0, -7)}, evArg...)...).Scan(&out.WAU); err != nil {
		return out, err
	}
	if err := s.conn.QueryRow(ctx, active, append([]any{channelID, now.AddDate(0, 0, -30)}, evArg...)...).Scan(&out.MAU); err != nil {
		return out, err
	}

	// return window: prior = [now-2W, now-W), current = [now-W, now)
	win := func(lo, hi time.Time) (string, []any) {
		w := " WHERE e.channel_id = ? AND " + inWindow + " AND " + notBot + evCond + " AND " + resolvedActor + " != ''"
		return "SELECT DISTINCT " + resolvedActor + " AS a " + from + w, append([]any{channelID, lo.UnixMilli(), hi.UnixMilli()}, evArg...)
	}
	prevSQL, prevArgs := win(now.AddDate(0, 0, -2*windowDays), now.AddDate(0, 0, -windowDays))
	curSQL, curArgs := win(now.AddDate(0, 0, -windowDays), now)
	rw := "WITH prev AS (" + prevSQL + "), cur AS (" + curSQL + ") " +
		"SELECT toInt64((SELECT count() FROM prev)), toInt64((SELECT count() FROM cur)), toInt64((SELECT count() FROM cur WHERE a IN (SELECT a FROM prev)))"
	if err := s.conn.QueryRow(ctx, rw, append(prevArgs, curArgs...)...).Scan(&out.PriorActive, &out.ActiveNow, &out.Returning); err != nil {
		return out, err
	}
	if out.PriorActive > 0 {
		out.ReturnRate = float64(out.Returning) / float64(out.PriorActive) * 100
	}
	return out, nil
}
