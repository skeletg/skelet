package spinech

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// The web page_view filter every page-scoped Lens read shares
const lensPageView = "source = 'web' AND event_name = 'page_view' AND channel_id = ? AND page_key = ?"

// Runs a value/count rollup over one page's views
// since is the Lens retention edge - older rows are pruned, and the bound lets ClickHouse skip their partitions
func (s *Store) lensPageCounts(ctx context.Context, expr string, channelID uuid.UUID, pageKey string, since time.Time, order string) ([]analytics.ValueCount, error) {
	rows, err := s.conn.Query(ctx,
		"SELECT "+expr+" AS v, toInt64(count()) AS views FROM analytics_events FINAL WHERE "+lensPageView+
			" AND occurred_at >= ? GROUP BY v ORDER BY "+order, channelID, pageKey, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.ValueCount
	for rows.Next() {
		var vc analytics.ValueCount
		if err := rows.Scan(&vc.Value, &vc.Count); err != nil {
			return nil, err
		}
		out = append(out, vc)
	}
	return out, rows.Err()
}

func (s *Store) LensPageTotals(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) (int64, int64, error) {
	var views, visitors int64
	err := s.conn.QueryRow(ctx,
		"SELECT toInt64(count()), toInt64(uniqExact(anon_id)) FROM analytics_events FINAL WHERE "+lensPageView+" AND occurred_at >= ?",
		channelID, pageKey, since).Scan(&views, &visitors)
	return views, visitors, err
}

func (s *Store) LensPageReferrers(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) ([]analytics.ValueCount, error) {
	return s.lensPageCounts(ctx, "if(JSONExtractString(properties, 'referrer') != '', JSONExtractString(properties, 'referrer'), '(direct)')",
		channelID, pageKey, since, "views DESC LIMIT 10")
}

func (s *Store) LensPageClients(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) ([]analytics.ValueCount, error) {
	return s.lensPageCounts(ctx, "if(JSONExtractString(properties, 'client') != '', JSONExtractString(properties, 'client'), 'unknown')",
		channelID, pageKey, since, "views DESC LIMIT 10")
}

func (s *Store) LensPageDeviceSplit(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) ([]analytics.ValueCount, error) {
	return s.lensPageCounts(ctx, "if(JSONExtractString(properties, 'device') != '', JSONExtractString(properties, 'device'), 'unknown')",
		channelID, pageKey, since, "views DESC")
}

func (s *Store) LensPageTimeline(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) ([]analytics.LensDayCount, error) {
	rows, err := s.conn.Query(ctx,
		"SELECT formatDateTime(toStartOfDay(occurred_at), '%Y-%m-%d') AS d, toInt64(count()) AS views "+
			"FROM analytics_events FINAL WHERE "+lensPageView+" AND occurred_at >= ? GROUP BY d ORDER BY d",
		channelID, pageKey, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.LensDayCount
	for rows.Next() {
		var p analytics.LensDayCount
		if err := rows.Scan(&p.Date, &p.Views); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) LensListPages(ctx context.Context, channelID uuid.UUID, since time.Time) ([]analytics.ValueCount, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT page_key AS v, toInt64(count()) AS views FROM analytics_events FINAL
		WHERE source = 'web' AND event_name = 'page_view' AND channel_id = ? AND occurred_at >= ?
		GROUP BY v ORDER BY views DESC LIMIT 200`, channelID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.ValueCount
	for rows.Next() {
		var vc analytics.ValueCount
		if err := rows.Scan(&vc.Value, &vc.Count); err != nil {
			return nil, err
		}
		out = append(out, vc)
	}
	return out, rows.Err()
}

func (s *Store) LensTopRageElements(ctx context.Context, channelID uuid.UUID, since time.Time, limit int32) ([]analytics.LensRageElement, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT JSONExtractString(properties, 'selector') AS selector,
		       max(JSONExtractString(properties, 'text')) AS text,
		       toInt64(count()) AS c
		FROM analytics_events FINAL
		WHERE channel_id = ? AND event_name = 'custom' AND custom_name = '$rage_click'
		  AND occurred_at >= ? AND JSONExtractString(properties, 'selector') != ''
		GROUP BY selector ORDER BY c DESC LIMIT ?`, channelID, since, uint64(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.LensRageElement
	for rows.Next() {
		var e analytics.LensRageElement
		if err := rows.Scan(&e.Selector, &e.Text, &e.Count); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) LensTopErrors(ctx context.Context, channelID uuid.UUID, since time.Time, limit int32) ([]analytics.LensErrorStat, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT JSONExtractString(properties, 'message') AS message,
		       toInt64(count()) AS c, toInt64(uniqExact(page_key)) AS pages, max(occurred_at) AS last_seen
		FROM analytics_events FINAL
		WHERE channel_id = ? AND event_name = 'custom' AND custom_name = '$js_error' AND occurred_at >= ?
		GROUP BY message ORDER BY c DESC LIMIT ?`, channelID, since, uint64(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.LensErrorStat
	for rows.Next() {
		var e analytics.LensErrorStat
		if err := rows.Scan(&e.Message, &e.Count, &e.Pages, &e.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// JSON_VALUE, not JSONExtractString: the tracker sends value as a JSON NUMBER and JSONExtractString returns an empty string for non-strings, so every sample would fail the guard and the vitals dashboards would sit permanently empty - JSON_VALUE stringifies scalars
// The regex guard - a junk sample would otherwise coerce to 0 and drag the percentile
const lensVitalValue = "JSON_VALUE(properties, '$.value')"

const lensVitals = "channel_id = ? AND event_name = 'custom' AND custom_name = '$web_vitals' AND occurred_at >= ? " +
	"AND match(" + lensVitalValue + ", '^-?[0-9.]+$')"

func (s *Store) LensVitalsByPage(ctx context.Context, channelID uuid.UUID, since time.Time) ([]analytics.LensVital, error) {
	rows, err := s.conn.Query(ctx,
		"SELECT page_key, JSONExtractString(properties, 'metric') AS metric, "+
			"quantileExactInclusive(0.75)(toFloat64("+lensVitalValue+")) AS p75, "+
			"toInt64(count()) AS samples FROM analytics_events FINAL WHERE "+lensVitals+
			" GROUP BY page_key, metric ORDER BY page_key, metric", channelID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.LensVital
	for rows.Next() {
		var v analytics.LensVital
		if err := rows.Scan(&v.PageKey, &v.Metric, &v.P75, &v.Samples); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) LensVitalsOverall(ctx context.Context, channelID uuid.UUID, since time.Time) ([]analytics.LensVital, error) {
	rows, err := s.conn.Query(ctx,
		"SELECT JSONExtractString(properties, 'metric') AS metric, "+
			"quantileExactInclusive(0.75)(toFloat64("+lensVitalValue+")) AS p75, "+
			"toInt64(count()) AS samples FROM analytics_events FINAL WHERE "+lensVitals+
			" GROUP BY metric", channelID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.LensVital
	for rows.Next() {
		var v analytics.LensVital
		if err := rows.Scan(&v.Metric, &v.P75, &v.Samples); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) LensSessionSignals(ctx context.Context, channelID uuid.UUID, sessionID string, from, to time.Time) ([]analytics.LensSessionSignal, error) {
	// value rides JSON_VALUE for the same reason as the vitals reads - it's a JSON number
	rows, err := s.conn.Query(ctx, `
		SELECT custom_name, JSONExtractString(properties, 'message'), JSONExtractString(properties, 'metric'),
		       JSON_VALUE(properties, '$.value'), occurred_at
		FROM analytics_events FINAL
		WHERE channel_id = ? AND event_name = 'custom'
		  AND occurred_at >= ? AND occurred_at <= ?
		  AND JSONExtractString(properties, 'sessionId') = ?
		  AND custom_name IN ('$js_error', '$web_vitals')
		ORDER BY occurred_at`, channelID, from, to, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.LensSessionSignal
	for rows.Next() {
		var g analytics.LensSessionSignal
		if err := rows.Scan(&g.Name, &g.Message, &g.Metric, &g.Value, &g.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// --- event-derived heatmaps: grid increments append as raw events, aggregated at read ---

// page_key is the MATERIALIZED column, not a JSONExtract - the filter then reads one compressed column (and prunes via PREWHERE) instead of parsing the JSON blob of every lens_cell row the channel has ever kept
func (s *Store) LensCellsFromEvents(ctx context.Context, channelID uuid.UUID, pageKey, viewport, state, kind string, since time.Time) ([]analytics.LensCellCount, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT toInt32(JSONExtractInt(properties, 'x')) AS x, toInt32(JSONExtractInt(properties, 'y')) AS y,
		       toInt64(sum(JSONExtractInt(properties, 'n'))) AS n
		FROM analytics_events FINAL
		WHERE event_name = 'lens_cell' AND channel_id = ? AND page_key = ? AND occurred_at >= ?
		  AND JSONExtractString(properties, 'viewport') = ?
		  AND JSONExtractString(properties, 'state') = ? AND JSONExtractString(properties, 'kind') = ?
		GROUP BY x, y ORDER BY n DESC`, channelID, pageKey, since, viewport, state, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.LensCellCount
	for rows.Next() {
		var c analytics.LensCellCount
		if err := rows.Scan(&c.X, &c.Y, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) LensElementsFromEvents(ctx context.Context, channelID uuid.UUID, pageKey, viewport string, limit int32, since time.Time) ([]analytics.LensElementCount, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT JSONExtractString(properties, 'selector') AS sel,
		       argMax(JSONExtractString(properties, 'text'), occurred_at) AS label,
		       toInt64(sum(JSONExtractInt(properties, 'n'))) AS n
		FROM analytics_events FINAL
		WHERE event_name = 'lens_element' AND channel_id = ? AND page_key = ? AND occurred_at >= ?
		  AND JSONExtractString(properties, 'viewport') = ?
		GROUP BY sel ORDER BY n DESC LIMIT ?`, channelID, pageKey, since, viewport, uint64(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.LensElementCount
	for rows.Next() {
		var e analytics.LensElementCount
		if err := rows.Scan(&e.Selector, &e.Text, &e.Count); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) LensPageGridFromEvents(ctx context.Context, channelID uuid.UUID, pageKey string, since time.Time) (analytics.LensGridTotals, error) {
	var g analytics.LensGridTotals
	err := s.conn.QueryRow(ctx, `
		SELECT toInt64(sumIf(JSONExtractInt(properties, 'n'), JSONExtractString(properties, 'kind') = 'click')),
		       toInt64(sumIf(JSONExtractInt(properties, 'y') * JSONExtractInt(properties, 'n'), JSONExtractString(properties, 'kind') = 'scroll')),
		       toInt64(sumIf(JSONExtractInt(properties, 'n'), JSONExtractString(properties, 'kind') = 'scroll'))
		FROM analytics_events FINAL
		WHERE event_name = 'lens_cell' AND channel_id = ? AND page_key = ? AND occurred_at >= ?`,
		channelID, pageKey, since).Scan(&g.Clicks, &g.ScrollWeight, &g.ScrollTotal)
	return g, err
}

func (s *Store) LensPagesGridFromEvents(ctx context.Context, channelID uuid.UUID, since time.Time) (map[string]analytics.LensGridTotals, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT page_key,
		       toInt64(sumIf(JSONExtractInt(properties, 'n'), JSONExtractString(properties, 'kind') = 'click')),
		       toInt64(sumIf(JSONExtractInt(properties, 'y') * JSONExtractInt(properties, 'n'), JSONExtractString(properties, 'kind') = 'scroll')),
		       toInt64(sumIf(JSONExtractInt(properties, 'n'), JSONExtractString(properties, 'kind') = 'scroll'))
		FROM analytics_events FINAL
		WHERE event_name = 'lens_cell' AND channel_id = ? AND occurred_at >= ?
		GROUP BY page_key`, channelID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]analytics.LensGridTotals{}
	for rows.Next() {
		var page string
		var g analytics.LensGridTotals
		if err := rows.Scan(&page, &g.Clicks, &g.ScrollWeight, &g.ScrollTotal); err != nil {
			return nil, err
		}
		out[page] = g
	}
	return out, rows.Err()
}
