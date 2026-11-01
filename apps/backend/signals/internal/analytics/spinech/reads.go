package spinech

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/ext"
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// zeroUUID is the "none" sentinel for optional UUID columns in the CH schema
const zeroUUID = "toUUID('00000000-0000-0000-0000-000000000000')"

// Identity resolution goes through the dictionary the driver creates at startup - an in-memory anon_id to customer_id map, so the eleven queries that need it stop re-running a full GROUP BY over analytics_identities and hash-joining it per query A miss returns the zero UUID, exactly what the LEFT JOIN produced resolvedCustomer is the customer uuid (zero = anonymous); resolvedActor is the person key as a string (customer, else anon id)
const (
	dictLookup = "dictGetUUID('" + identityDictName + "', 'customer_id', tuple(e.anon_id))"
	// Ask the dictionary whether it holds the key rather than comparing its result to the zero
	// sentinel - ClickHouse 26's analyzer reads that comparison as false even on a resolved hit
	dictHit          = "dictHas('" + identityDictName + "', tuple(e.anon_id))"
	resolvedCustomer = "if(e.customer_id != " + zeroUUID + ", e.customer_id, " + dictLookup + ")"
	resolvedActor    = "if(e.customer_id != " + zeroUUID + ", toString(e.customer_id), if(" + dictHit + ", toString(" + dictLookup + "), e.anon_id))"
	// "we know who this is" - the sentinel compare is safe on a plain column, unsafe on a dictGet
	hasCustomer = "(e.customer_id != " + zeroUUID + " OR " + dictHit + ")"
	notBot      = "NOT e.is_bot"
)

// The analytical reads, implemented with native ClickHouse functions (windowFunnel / uniqExact / ARRAY JOIN JSONExtractKeysAndValues / window functions) - the post-processing (Retention cohorts, Flow path-tree, Insights fold) lives in the analytics.*FromEvents helpers

func (s *Store) Funnel(ctx context.Context, channelID uuid.UUID, steps []string, since time.Time) ([]analytics.FunnelStep, error) {
	out := make([]analytics.FunnelStep, len(steps))
	for i, st := range steps {
		out[i] = analytics.FunnelStep{EventName: st}
	}
	if len(steps) < 2 {
		return out, nil
	}
	// Native windowFunnel: per resolved person, the longest in-order chain of the step events (greedy earliest-match) - a ~10-year window (ms, matching toUnixTimestamp64Milli) means no inter-step limit inside the query window, and countIf(level >= k) is the people who reached step k
	conds := make([]string, len(steps)) // windowFunnel conditions
	cols := make([]string, len(steps))  // countIf output columns
	inHolders := make([]string, len(steps))
	args := make([]any, 0, len(steps)*2+2)
	for i := range steps {
		conds[i] = "event_name = ?"
		cols[i] = fmt.Sprintf("toInt64(countIf(level >= %d))", i+1)
		inHolders[i] = "?"
	}
	for _, st := range steps { // windowFunnel condition args
		args = append(args, st)
	}
	args = append(args, channelID, since)
	for _, st := range steps { // event_name IN (...) args
		args = append(args, st)
	}
	// the empty-actor guard - without it every unattributable event (zero customer, empty anon) merges into one phantom person whose combined stream walks the funnel
	q := fmt.Sprintf(`
		SELECT %s
		FROM (
			SELECT windowFunnel(315360000000)(toUInt64(toUnixTimestamp64Milli(e.occurred_at)), %s) AS level
			FROM analytics_events AS e FINAL
			WHERE e.channel_id = ? AND e.occurred_at >= ? AND %s AND e.event_name IN (%s)
			  AND %s != ''
			GROUP BY %s
		)`,
		strings.Join(cols, ", "), strings.Join(conds, ", "),
		notBot, strings.Join(inHolders, ", "), resolvedActor, resolvedActor)

	counts := make([]int64, len(steps))
	dest := make([]any, len(steps))
	for i := range counts {
		dest[i] = &counts[i]
	}
	if err := s.conn.QueryRow(ctx, q, args...).Scan(dest...); err != nil {
		return out, err
	}
	for i := range steps {
		out[i].Count = counts[i]
	}
	return out, nil
}

func (s *Store) Insights(ctx context.Context, channelID uuid.UUID, event, breakdown, measure, unit string, since, now time.Time) (analytics.Insights, error) {
	bucket := "toStartOfDay(e.occurred_at)"
	if unit == "week" {
		bucket = "toStartOfWeek(e.occurred_at, 1)" // Monday, matching date_trunc('week')
	}
	// breakdown: no key -> '(none)'; else missing/'' -> '(none)' - non-string values stringify via JSONExtractRaw - bare JSONExtractString returns '' for numbers and booleans, folding them all into '(none)'
	bdCol, bdArgs := "'(none)'", []any(nil)
	if breakdown != "" {
		bdCol = "if((multiIf(NOT JSONHas(e.properties, ?), '', " +
			"JSONType(e.properties, ?) = 'String', JSONExtractString(e.properties, ?), " +
			"JSONType(e.properties, ?) = 'Null', '', JSONExtractRaw(e.properties, ?)) AS bdv) = '', '(none)', bdv)"
		bdArgs = []any{breakdown, breakdown, breakdown, breakdown, breakdown}
	}
	evCond, evArg := "", []any(nil)
	if event != "" {
		evCond, evArg = " AND e.event_name = ?", []any{event}
	}
	from := "FROM analytics_events AS e FINAL"
	uniq := "toInt64(uniqExactIf(" + resolvedActor + ", " + resolvedActor + " != ''))" // distinct non-null resolved

	seriesArgs := append(append([]any{}, bdArgs...), channelID, since)
	seriesArgs = append(seriesArgs, evArg...)
	rows, err := s.conn.Query(ctx,
		"SELECT "+bucket+" AS bucket, "+bdCol+" AS breakdown, toInt64(count()) AS total, "+uniq+" AS uniques "+
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
	wm := "SELECT toInt64(count()), " + uniq + ", toInt64(countIf(e.source = 'aggregate')) " + from + " WHERE e.channel_id = ? AND e.occurred_at >= ? AND e.occurred_at < ? AND " + notBot + evCond
	_ = s.conn.QueryRow(ctx, wm, append([]any{channelID, since, now}, evArg...)...).Scan(&out.Total, &out.Uniques, &out.Aggregate)
	prevStart := since.Add(-now.Sub(since))
	var prevAggregate int64
	_ = s.conn.QueryRow(ctx, wm, append([]any{channelID, prevStart, since}, evArg...)...).Scan(&out.PrevTotal, &out.PrevUniques, &prevAggregate)
	return out, nil
}

func (s *Store) Stickiness(ctx context.Context, channelID uuid.UUID, event string, since, now time.Time, windowDays int) (analytics.Stickiness, error) {
	if windowDays < 1 {
		windowDays = 30
	}
	out := analytics.Stickiness{WindowDays: windowDays}
	// optional event filter (""=any activity); actor resolved, empty actor dropped
	evCond, evArg := "", []any(nil)
	if event != "" {
		evCond, evArg = " AND e.event_name = ?", []any{event}
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
		w := " WHERE e.channel_id = ? AND e.occurred_at >= ? AND e.occurred_at < ? AND " + notBot + evCond + " AND " + resolvedActor + " != ''"
		return "SELECT DISTINCT " + resolvedActor + " AS a " + from + w, append([]any{channelID, lo, hi}, evArg...)
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

func (s *Store) Flow(ctx context.Context, channelID uuid.UUID, focus string, since time.Time, depth, topK int) (analytics.FlowResult, error) {
	// Custom events surface by their real name and $experiment is dropped - each journey is cut to what the walker uses: from the first focus event, repeats collapsed, depth steps at most
	if depth < 1 {
		depth = 1
	}
	rows, err := s.conn.Query(ctx, `
		WITH focus_actors AS (
			SELECT DISTINCT `+resolvedActor+` AS actor
			FROM analytics_events AS e FINAL
			WHERE e.channel_id = ? AND e.occurred_at >= ? AND `+notBot+`
			  AND (e.event_name = ? OR (e.event_name = 'custom' AND e.custom_name = ?))
			  AND `+resolvedActor+` != ''
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
				  AND NOT (e.event_name = 'custom' AND e.custom_name = '$experiment')
				  AND `+resolvedActor+` != ''
				GROUP BY actor
			)
			WHERE has(evs, ?)
		)`,
		channelID, since, focus, focus, uint64(depth+1), focus, channelID, since, focus)
	if err != nil {
		return analytics.FlowResult{}, err
	}
	defer rows.Close()
	var events []analytics.SpineActorEvent
	for rows.Next() {
		var (
			actor string
			path  []string
		)
		if err := rows.Scan(&actor, &path); err != nil {
			return analytics.FlowResult{}, err
		}
		for _, name := range path {
			events = append(events, analytics.SpineActorEvent{Actor: actor, EventName: name})
		}
	}
	if err := rows.Err(); err != nil {
		return analytics.FlowResult{}, err
	}
	return analytics.FlowFromEvents(events, focus, depth, topK), nil
}

func (s *Store) Retention(ctx context.Context, channelID uuid.UUID, startEvent, returnEvent string, since, now time.Time, periods int, unit time.Duration) (analytics.Retention, error) {
	out := analytics.Retention{Buckets: make([]analytics.RetentionBucket, periods)}
	for i := range out.Buckets {
		out.Buckets[i].Period = i
	}
	if periods <= 0 || unit <= 0 {
		return out, nil
	}
	// Cohort starts round down from Go's zero time like time.Truncate - Unix-epoch weeks would start on a Thursday
	unitMs := unit.Milliseconds()
	rows, err := s.conn.Query(ctx, `
		WITH per_actor AS (
			SELECT actor,
			       toInt64(toUnixTimestamp64Milli(minIf(ts, ev = ?))) AS anchor_ms,
			       groupArrayIf(toInt64(toUnixTimestamp64Milli(ts)), ev = ?) AS returns_ms
			FROM (
				SELECT `+resolvedActor+` AS actor, e.event_name AS ev, e.occurred_at AS ts
				FROM analytics_events AS e FINAL
				WHERE e.channel_id = ? AND e.occurred_at >= ? AND `+notBot+`
				  AND e.event_name IN (?, ?) AND `+resolvedActor+` != ''
			)
			GROUP BY actor
			HAVING countIf(ev = ?) > 0
		),
		bucketed AS (
			SELECT anchor_ms - ((anchor_ms + 62135596800000) % ?) AS cohort_ms,
			       arrayDistinct(arrayFilter(b -> b >= 0 AND b < ?,
			           arrayMap(r -> intDiv(r - cohort_ms, ?), arrayFilter(r -> r >= anchor_ms, returns_ms)))) AS buckets
			FROM per_actor
		)
		SELECT cohort_ms, toInt64(count()) AS size,
		       sumForEach(arrayMap(b -> toInt64(has(buckets, b)), range(?))) AS counts
		FROM bucketed GROUP BY cohort_ms`,
		startEvent, returnEvent, channelID, since, startEvent, returnEvent, startEvent,
		unitMs, int64(periods), unitMs, uint64(periods))
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

func (s *Store) Engagement(ctx context.Context, channelID uuid.UUID, since time.Time) (analytics.Engagement, error) {
	var out analytics.Engagement
	// human vs raw opens/clicks: machine is a JSON bool (JSONExtractBool = false when absent)
	if err := s.conn.QueryRow(ctx, `
		SELECT
			toInt64(countIf(event_name = 'email_opened'))                                          AS opens_raw,
			toInt64(countIf(event_name = 'email_opened'  AND NOT machine)) AS opens_human,
			toInt64(countIf(event_name = 'email_clicked' AND NOT machine)) AS clicks,
			toInt64(countIf(event_name = 'conversion'))                                            AS conversions
		FROM analytics_events FINAL
		WHERE channel_id = ? AND occurred_at >= ?`,
		channelID, since).Scan(&out.OpensRaw, &out.OpensHuman, &out.Clicks, &out.Conversions); err != nil {
		return analytics.Engagement{}, err
	}

	rev, err := s.conn.Query(ctx, `
		SELECT currency                     AS currency,
		       toInt64(sum(revenue_cents))     AS cents,
		       toInt64(count())                                              AS orders
		FROM analytics_events FINAL
		WHERE channel_id = ? AND event_name = 'conversion' AND occurred_at >= ?
		GROUP BY currency ORDER BY cents DESC`, channelID, since)
	if err != nil {
		return analytics.Engagement{}, err
	}
	for rev.Next() {
		var r analytics.RevenueByCurrency
		if err := rev.Scan(&r.Currency, &r.Cents, &r.Orders); err != nil {
			rev.Close()
			return analytics.Engagement{}, err
		}
		out.Revenue = append(out.Revenue, r)
	}
	rev.Close()
	if err := rev.Err(); err != nil {
		return analytics.Engagement{}, err
	}

	links, err := s.conn.Query(ctx, `
		SELECT JSONExtractString(properties, 'url') AS url, toInt64(count()) AS clicks
		FROM analytics_events FINAL
		WHERE channel_id = ? AND event_name = 'email_clicked'
		  AND NOT machine AND occurred_at >= ?
		GROUP BY url ORDER BY clicks DESC LIMIT 10`, channelID, since)
	if err != nil {
		return analytics.Engagement{}, err
	}
	defer links.Close()
	for links.Next() {
		var url string
		var clicks int64
		if err := links.Scan(&url, &clicks); err != nil {
			return analytics.Engagement{}, err
		}
		if url != "" {
			out.TopLinks = append(out.TopLinks, analytics.LinkClicks{URL: url, Clicks: clicks})
		}
	}
	return out, links.Err()
}

func (s *Store) SynapseOverview(ctx context.Context, channelID uuid.UUID, since time.Time) (analytics.SynapseOverview, error) {
	var o analytics.SynapseOverview
	notBot := "NOT is_bot"

	if err := s.conn.QueryRow(ctx, `
		SELECT toInt64(count()), toInt64(countIf(event_name = 'page_view')), toInt64(countIf(event_name = 'conversion'))
		FROM analytics_events FINAL WHERE channel_id = ? AND occurred_at >= ? AND `+notBot,
		channelID, since).Scan(&o.TotalEvents, &o.PageViews, &o.Conversions); err != nil {
		return o, fmt.Errorf("synapse overview: totals: %w", err)
	}

	if err := s.conn.QueryRow(ctx,
		`SELECT toInt64(uniqExact(customer_id)) FROM analytics_identities FINAL WHERE channel_id = ?`,
		channelID).Scan(&o.IdentifiedCustomers); err != nil {
		return o, fmt.Errorf("synapse overview: identified: %w", err)
	}

	ev, err := s.conn.Query(ctx, `
		SELECT event_name, toInt64(count()) AS c FROM analytics_events FINAL
		WHERE channel_id = ? AND occurred_at >= ? AND `+notBot+`
		GROUP BY event_name ORDER BY c DESC`, channelID, since)
	if err != nil {
		return o, fmt.Errorf("synapse overview: event counts: %w", err)
	}
	for ev.Next() {
		var e analytics.EventCount
		if err := ev.Scan(&e.Name, &e.Count); err != nil {
			ev.Close()
			return o, err
		}
		o.TopEvents = append(o.TopEvents, e)
	}
	ev.Close()
	if err := ev.Err(); err != nil {
		return o, err
	}

	rev, err := s.conn.Query(ctx, `
		SELECT currency, toInt64(sum(revenue_cents)), toInt64(count())
		FROM analytics_events FINAL WHERE channel_id = ? AND event_name = 'conversion' AND occurred_at >= ?
		GROUP BY 1 ORDER BY 2 DESC`, channelID, since)
	if err != nil {
		return o, fmt.Errorf("synapse overview: revenue: %w", err)
	}
	for rev.Next() {
		var r analytics.RevenueByCurrency
		if err := rev.Scan(&r.Currency, &r.Cents, &r.Orders); err != nil {
			rev.Close()
			return o, err
		}
		o.Revenue = append(o.Revenue, r)
	}
	rev.Close()
	if err := rev.Err(); err != nil {
		return o, err
	}

	// resolved active customers: events joined to the latest identity mapping, customer = the event's customer_id else the stitched one (zero = none)
	ac, err := s.conn.Query(ctx, `
		SELECT cust, toInt64(count()) AS c, max(e.occurred_at)
		FROM analytics_events AS e FINAL
		WHERE e.channel_id = ? AND e.occurred_at >= ? AND NOT e.is_bot
		  AND if(e.customer_id != toUUID('00000000-0000-0000-0000-000000000000'), e.customer_id, dictGetUUID('identity_map', 'customer_id', tuple(e.anon_id))) != toUUID('00000000-0000-0000-0000-000000000000')
		GROUP BY if(e.customer_id != toUUID('00000000-0000-0000-0000-000000000000'), e.customer_id, dictGetUUID('identity_map', 'customer_id', tuple(e.anon_id))) AS cust
		ORDER BY c DESC LIMIT 15`, channelID, since)
	if err != nil {
		return o, fmt.Errorf("synapse overview: active customers: %w", err)
	}
	for ac.Next() {
		var c analytics.ActiveCustomer
		if err := ac.Scan(&c.CustomerID, &c.Events, &c.LastSeen); err != nil {
			ac.Close()
			return o, err
		}
		o.ActiveCustomers = append(o.ActiveCustomers, c)
	}
	ac.Close()
	if err := ac.Err(); err != nil {
		return o, err
	}

	if err := s.conn.QueryRow(ctx, `
		SELECT toInt64(uniqExact(anon_id)) FROM analytics_events FINAL
		WHERE channel_id = ? AND occurred_at >= ? AND `+notBot+`
		  AND anon_id != '' AND NOT dictHas('`+identityDictName+`', tuple(anon_id))`,
		channelID, since).Scan(&o.AnonymousVisitors); err != nil {
		return o, fmt.Errorf("synapse overview: anonymous count: %w", err)
	}

	av, err := s.conn.Query(ctx, `
		SELECT anon_id, toInt64(count()) AS c, max(occurred_at),
		       argMaxIf(coalesce(nullIf(title, ''), page_key), occurred_at, event_name = 'page_view')
		FROM analytics_events FINAL
		WHERE channel_id = ? AND occurred_at >= ? AND `+notBot+`
		  AND anon_id != '' AND NOT dictHas('`+identityDictName+`', tuple(anon_id))
		GROUP BY anon_id ORDER BY c DESC LIMIT 15`, channelID, since)
	if err != nil {
		return o, fmt.Errorf("synapse overview: active visitors: %w", err)
	}
	defer av.Close()
	for av.Next() {
		var v analytics.Visitor
		if err := av.Scan(&v.AnonID, &v.Events, &v.LastSeen, &v.LastPage); err != nil {
			return o, err
		}
		o.ActiveVisitors = append(o.ActiveVisitors, v)
	}
	return o, av.Err()
}

func (s *Store) CustomerProfile(ctx context.Context, customerID uuid.UUID, since time.Time) (analytics.Profile, error) {
	anonIDs, err := s.AnonIDsForCustomer(ctx, customerID)
	if err != nil {
		return analytics.Profile{}, err
	}
	return s.profileFor(ctx, person{cid: customerID, anon: anonIDs}, since)
}

func (s *Store) VisitorProfile(ctx context.Context, channelID uuid.UUID, anonID string, since time.Time) (analytics.Profile, error) {
	if anonID == "" || channelID == uuid.Nil {
		return analytics.Profile{}, nil
	}
	return s.profileFor(ctx, person{channel: channelID, anon: []string{anonID}}, since)
}

// profileFor rolls the spine into a Profile for a customer (zero = anonymous visitor) and/or anon ids - the shared core of Customer/VisitorProfile
func (s *Store) profileFor(ctx context.Context, who person, since time.Time) (analytics.Profile, error) {
	pf, pfArgs := who.filter()
	base := func(extra ...any) []any { return append(append([]any{}, pfArgs...), append([]any{since}, extra...)...) }
	var p analytics.Profile

	// overview
	var total int64
	var first, last time.Time
	if err := s.conn.QueryRow(ctx, "SELECT toInt64(count()), min(occurred_at), max(occurred_at) FROM analytics_events FINAL WHERE "+pf+" AND occurred_at >= ?", base()...).Scan(&total, &first, &last); err != nil {
		return p, err
	}
	p.TotalEvents = total
	if total > 0 {
		p.FirstSeen, p.LastSeen = &first, &last
	}

	// event counts (+ page views / conversions)
	if rows, err := s.conn.Query(ctx, "SELECT event_name, toInt64(count()) c FROM analytics_events FINAL WHERE "+pf+" AND occurred_at >= ? GROUP BY event_name ORDER BY c DESC", base()...); err == nil {
		for rows.Next() {
			var c analytics.EventCount
			if rows.Scan(&c.Name, &c.Count) == nil {
				p.Counts = append(p.Counts, c)
				switch c.Name {
				case analytics.EventPageView:
					p.PageViews = c.Count
				case analytics.EventConversion:
					p.Conversions = c.Count
				}
			}
		}
		rows.Close()
	}

	// conversions -> revenue by currency
	if rows, err := s.conn.Query(ctx, "SELECT currency, toInt64(sum(revenue_cents)), toInt64(count()) FROM analytics_events FINAL WHERE event_name = 'conversion' AND "+pf+" AND occurred_at >= ? GROUP BY 1", base()...); err == nil {
		for rows.Next() {
			var r analytics.RevenueByCurrency
			if rows.Scan(&r.Currency, &r.Cents, &r.Orders) == nil {
				p.Revenue = append(p.Revenue, r)
			}
		}
		rows.Close()
	}

	// timeline (most recent 60)
	if rows, err := s.conn.Query(ctx, "SELECT event_name, occurred_at, source, properties FROM analytics_events FINAL WHERE "+pf+" AND occurred_at >= ? ORDER BY occurred_at DESC LIMIT 60", base()...); err == nil {
		for rows.Next() {
			var it analytics.ActivityItem
			var props string
			if rows.Scan(&it.EventName, &it.OccurredAt, &it.Source, &props) == nil {
				it.Props = json.RawMessage(props)
				it.Summary = analytics.SummarizeProps(it.EventName, it.Props)
				p.Timeline = append(p.Timeline, it)
			}
		}
		rows.Close()
	}

	// context dimensions (reuse topValues)
	p.Devices, _ = s.topValues(ctx, "device", who, since)
	p.Browsers, _ = s.topValues(ctx, "client", who, since)
	p.Referrers, _ = s.topValues(ctx, "referrer", who, since)
	p.IPs, _ = s.topValues(ctx, "ip", who, since)
	p.OSes, _ = s.topValues(ctx, "os", who, since)
	p.Screens, _ = s.topValues(ctx, "viewport", who, since)
	p.Countries, _ = s.topValues(ctx, "country", who, since)
	p.Sources, _ = s.topValues(ctx, "utm_source", who, since)
	p.Campaigns, _ = s.topValues(ctx, "utm_campaign", who, since)
	p.Searches, _ = s.topValues(ctx, "search_query", who, since)
	p.TopPages, p.PagesViewedTotal = s.topPages(ctx, pf, base())
	p.TopProducts = s.topProducts(ctx, pf, base())

	// session count: distinct visits, split on a >30-min gap between consecutive events (no stored session id). lagInFrame default -1 marks the first event
	if rows, err := s.conn.Query(ctx, `
		SELECT toInt64(count()) FROM (
			SELECT toUnixTimestamp64Milli(occurred_at) AS ts,
			       lagInFrame(toUnixTimestamp64Milli(occurred_at), 1, toInt64(-1)) OVER (ORDER BY occurred_at ROWS BETWEEN 1 PRECEDING AND CURRENT ROW) AS prev
			FROM analytics_events FINAL WHERE `+pf+` AND occurred_at >= ?
		) WHERE prev = -1 OR (ts - prev) > 1800000`, base()...); err == nil {
		if rows.Next() {
			_ = rows.Scan(&p.Sessions)
		}
		rows.Close()
	}
	return p, nil
}

// topPages: pages viewed + avg dwell (gap to the next event, capped 1..1800s) and the distinct-page total before the limit
func (s *Store) topPages(ctx context.Context, pf string, args []any) ([]analytics.PageView, int) {
	rows, err := s.conn.Query(ctx, `
		SELECT title, page_key, toInt64(count()) AS views,
		       toInt64(round(ifNotFinite(avgIf(dwell, dwell >= 1 AND dwell <= 1800), 0))) AS avg_dwell,
		       toInt64(count() OVER ()) AS total_pages
		FROM (
			SELECT title, page_key, event_name,
			       (leadInFrame(toUnixTimestamp64Milli(occurred_at)) OVER (PARTITION BY if(customer_id != `+zeroUUID+`, toString(customer_id), anon_id) ORDER BY occurred_at ROWS BETWEEN CURRENT ROW AND UNBOUNDED FOLLOWING) - toUnixTimestamp64Milli(occurred_at)) / 1000.0 AS dwell
			FROM analytics_events FINAL WHERE `+pf+` AND occurred_at >= ?
		) WHERE event_name = 'page_view' AND title != ''
		GROUP BY title, page_key ORDER BY views DESC, title LIMIT 12`, args...)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()
	var out []analytics.PageView
	total := 0
	for rows.Next() {
		var pv analytics.PageView
		var tp int64
		var views, dwell int64
		if err := rows.Scan(&pv.Title, &pv.PageKey, &views, &dwell, &tp); err != nil {
			return out, total
		}
		pv.Views, pv.AvgDwellSeconds, total = int(views), int(dwell), int(tp)
		out = append(out, pv)
	}
	return out, total
}

// topProducts: products the profile viewed, richest-first, with display metadata
func (s *Store) topProducts(ctx context.Context, pf string, args []any) []analytics.ProductView {
	rows, err := s.conn.Query(ctx, `
		SELECT JSONExtractString(properties,'entity_name') AS name,
		       max(JSONExtractString(properties,'entity_image')), max(JSONExtractString(properties,'entity_price')),
		       max(JSONExtractString(properties,'sku')), toInt64(count()) AS views
		FROM analytics_events FINAL
		WHERE `+pf+` AND event_name = 'page_view' AND JSONExtractString(properties,'entity') = 'product'
		  AND JSONExtractString(properties,'entity_name') != '' AND occurred_at >= ?
		GROUP BY name ORDER BY views DESC, name LIMIT 12`, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []analytics.ProductView
	for rows.Next() {
		var pv analytics.ProductView
		var views int64
		if err := rows.Scan(&pv.Name, &pv.Image, &pv.Price, &pv.SKU, &views); err != nil {
			return out
		}
		pv.Views = int(views)
		out = append(out, pv)
	}
	return out
}

func (s *Store) CustomerExperiments(ctx context.Context, customerID uuid.UUID, since time.Time) ([]analytics.ExperimentArm, error) {
	anonIDs, err := s.AnonIDsForCustomer(ctx, customerID)
	if err != nil || len(anonIDs) == 0 {
		return nil, err
	}
	rows, err := s.conn.Query(ctx, `
		SELECT JSONExtractString(properties, 'experiment') AS experiment,
		       JSONExtractString(properties, 'variation')  AS variation,
		       max(occurred_at)                            AS last_exposed
		FROM analytics_events FINAL
		WHERE source = 'web' AND event_name = 'custom'
		  AND custom_name = '$experiment'
		  AND JSONExtractString(properties, 'experiment') != ''
		  AND occurred_at >= ? AND has(?, anon_id)
		GROUP BY experiment, variation ORDER BY last_exposed DESC`, since, anonIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.ExperimentArm
	for rows.Next() {
		var a analytics.ExperimentArm
		if err := rows.Scan(&a.Experiment, &a.Variation, &a.LastExposedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type person struct {
	cid     uuid.UUID
	channel uuid.UUID
	anon    []string
}

// A zero customer never matches the zero-UUID anonymous events, which is why a visitor passes none
func (p person) filter() (string, []any) {
	pf, args := "has(?, anon_id)", []any{p.anon}
	if p.cid != uuid.Nil {
		pf, args = "(customer_id = ? OR has(?, anon_id))", []any{p.cid, p.anon}
	}
	if p.channel != uuid.Nil {
		return "channel_id = ? AND " + pf, append([]any{p.channel}, args...)
	}
	return pf, args
}

// topValues: the most frequent distinct values of one event-property key across a profile's events (device/browser/attr:<slug>/...)
func (s *Store) topValues(ctx context.Context, key string, who person, since time.Time) ([]analytics.ValueCount, error) {
	pf, pfArgs := who.filter()
	args := append([]any{key}, pfArgs...)
	args = append(args, since)
	rows, err := s.conn.Query(ctx, `
		SELECT v, toInt64(count()) AS c FROM (
			SELECT JSONExtractString(properties, ?) AS v FROM analytics_events FINAL WHERE `+pf+` AND occurred_at >= ?
		) WHERE v != '' GROUP BY v ORDER BY c DESC LIMIT 10`, args...)
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

// affinities: top values of each tracked attribute (attr:<slug>) over a profile
func (s *Store) affinities(ctx context.Context, who person, since time.Time, attrs []analytics.AffinityAttr) ([]analytics.AttributeAffinity, error) {
	out := make([]analytics.AttributeAffinity, 0, len(attrs))
	for _, a := range attrs {
		vals, err := s.topValues(ctx, "attr:"+a.Slug, who, since)
		if err != nil {
			return nil, err
		}
		if len(vals) > 0 {
			out = append(out, analytics.AttributeAffinity{Slug: a.Slug, Name: a.Name, Values: vals})
		}
	}
	return out, nil
}

func (s *Store) CustomerAffinities(ctx context.Context, customerID uuid.UUID, since time.Time, attrs []analytics.AffinityAttr) ([]analytics.AttributeAffinity, error) {
	if len(attrs) == 0 {
		return nil, nil
	}
	anonIDs, err := s.AnonIDsForCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	return s.affinities(ctx, person{cid: customerID, anon: anonIDs}, since, attrs)
}

func (s *Store) VisitorAffinities(ctx context.Context, channelID uuid.UUID, anonID string, since time.Time, attrs []analytics.AffinityAttr) []analytics.AttributeAffinity {
	if anonID == "" || channelID == uuid.Nil || len(attrs) == 0 {
		return nil
	}
	out, _ := s.affinities(ctx, person{channel: channelID, anon: []string{anonID}}, since, attrs)
	return out
}

func (s *Store) AffinityActors(ctx context.Context, channelID uuid.UUID, attrSlug, value string, since time.Time) ([]uuid.UUID, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT `+resolvedCustomer+` AS cust
		FROM analytics_events AS e FINAL
		WHERE e.channel_id = ? AND e.occurred_at >= ? AND `+notBot+`
		  AND JSONExtractString(e.properties, ?) = ? AND `+hasCustomer+`
		GROUP BY `+resolvedCustomer, channelID, since, "attr:"+attrSlug, value)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) AffinityValueCounts(ctx context.Context, channelID uuid.UUID, attrSlug string, since time.Time, limit int) ([]analytics.AffinityValueCount, error) {
	if limit <= 0 {
		limit = 20
	}
	key := "attr:" + attrSlug
	rows, err := s.conn.Query(ctx, `
		SELECT JSONExtractString(e.properties, ?) AS value, toInt64(uniqExact(`+resolvedCustomer+`)) AS actors
		FROM analytics_events AS e FINAL
		WHERE e.channel_id = ? AND e.occurred_at >= ? AND `+notBot+`
		  AND JSONExtractString(e.properties, ?) != '' AND `+hasCustomer+`
		GROUP BY value ORDER BY actors DESC, value LIMIT ?`, key, channelID, since, key, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.AffinityValueCount
	for rows.Next() {
		var v string
		var actors int64
		if err := rows.Scan(&v, &actors); err != nil {
			return nil, err
		}
		out = append(out, analytics.AffinityValueCount{Value: v, Actors: int(actors)})
	}
	return out, rows.Err()
}

func (s *Store) CustomerTopAffinities(ctx context.Context, customerID uuid.UUID, since time.Time) (map[string]string, error) {
	anonIDs, err := s.AnonIDsForCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	pf, pfArgs := person{cid: customerID, anon: anonIDs}.filter()
	args := append(append([]any{}, pfArgs...), since)
	rows, err := s.conn.Query(ctx, `
		SELECT attr_key, attr_val FROM (
			SELECT kv.1 AS attr_key, kv.2 AS attr_val,
			       row_number() OVER (PARTITION BY kv.1 ORDER BY count() DESC, kv.2) AS rn
			FROM analytics_events FINAL
			ARRAY JOIN JSONExtractKeysAndValues(properties, 'String') AS kv
			WHERE `+pf+` AND occurred_at >= ? AND startsWith(kv.1, 'attr:')
			GROUP BY kv.1, kv.2
		) WHERE rn = 1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		if v != "" && strings.HasPrefix(k, "attr:") {
			out[strings.TrimPrefix(k, "attr:")] = v
		}
	}
	return out, rows.Err()
}

// segmentEventActors: customers (resolved) who did `event` >= minCount times since the cutoff - anonymous-only people are excluded, since segments hold customer ids
func (s *Store) segmentEventActors(ctx context.Context, channelID uuid.UUID, event string, since time.Time, minCount int) ([]uuid.UUID, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT `+resolvedCustomer+` AS cust
		FROM analytics_events AS e FINAL
		WHERE e.channel_id = ? AND e.event_name = ? AND e.occurred_at >= ? AND `+notBot+`
		  AND `+hasCustomer+`
		GROUP BY `+resolvedCustomer+` HAVING count() >= ?`, channelID, event, since, int64(minCount))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) EventActors(ctx context.Context, channelID uuid.UUID, event string, minCount, days int, now time.Time) ([]uuid.UUID, error) {
	if minCount < 1 {
		minCount = 1
	}
	if days <= 0 {
		days = 30
	}
	return s.segmentEventActors(ctx, channelID, event, now.AddDate(0, 0, -days), minCount)
}

// SegmentActors evaluates a behavioral segment tree: intersection of positive conditions minus anyone matched by a negated one
func (s *Store) SegmentActors(ctx context.Context, channelID uuid.UUID, conds []analytics.SegmentCondition, now time.Time) ([]uuid.UUID, error) {
	actors := func(c analytics.SegmentCondition) ([]uuid.UUID, error) {
		minCount := max(c.MinCount, 1)
		days := c.Days
		if days <= 0 {
			days = 30
		}
		return s.segmentEventActors(ctx, channelID, c.Event, now.AddDate(0, 0, -days), minCount)
	}
	var result map[uuid.UUID]bool
	positives := 0
	for _, c := range conds {
		if c.Negate {
			continue
		}
		ids, err := actors(c)
		if err != nil {
			return nil, err
		}
		set := make(map[uuid.UUID]bool, len(ids))
		for _, id := range ids {
			set[id] = true
		}
		if positives == 0 {
			result = set
		} else {
			for id := range result {
				if !set[id] {
					delete(result, id)
				}
			}
		}
		positives++
	}
	if positives == 0 {
		return nil, fmt.Errorf("a segment needs at least one positive (did) condition")
	}
	for _, c := range conds {
		if !c.Negate {
			continue
		}
		ids, err := actors(c)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			delete(result, id)
		}
	}
	out := make([]uuid.UUID, 0, len(result))
	for id := range result {
		out = append(out, id)
	}
	return out, nil
}

func (s *Store) SegmentAffinitiesForMembers(ctx context.Context, channelID uuid.UUID, memberIDs []uuid.UUID, since time.Time, limit int) ([]analytics.SegmentAffinityCount, error) {
	// the member set rides an external temp table, not an inline array - a big segment is millions of ids, and external data streams while a bound array parses
	members, err := ext.NewTable("segment_members", ext.Column("id", "UUID"))
	if err != nil {
		return nil, err
	}
	for _, id := range memberIDs {
		if err := members.Append(id); err != nil {
			return nil, err
		}
	}
	rows, err := s.conn.Query(clickhouse.Context(ctx, clickhouse.WithExternalTable(members)), `
		SELECT kv.1 AS attr_key, kv.2 AS attr_val, toInt64(uniqExact(`+resolvedCustomer+`)) AS members
		FROM analytics_events AS e FINAL
		ARRAY JOIN JSONExtractKeysAndValues(e.properties, 'String') AS kv
		WHERE e.channel_id = ? AND e.occurred_at >= ? AND `+notBot+`
		  AND `+resolvedCustomer+` IN (SELECT id FROM segment_members) AND startsWith(kv.1, 'attr:') AND kv.2 != ''
		GROUP BY attr_key, attr_val ORDER BY members DESC, attr_key LIMIT ?`,
		channelID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.SegmentAffinityCount
	for rows.Next() {
		var k, v string
		var members int64
		if err := rows.Scan(&k, &v, &members); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(k, "attr:") {
			continue
		}
		out = append(out, analytics.SegmentAffinityCount{Attr: strings.TrimPrefix(k, "attr:"), Value: v, Members: int(members)})
	}
	return out, rows.Err()
}

// Scoped to the order's channel - another store's email never gets the credit
func (s *Store) RecentEngagementEvents(ctx context.Context, channelID, customerID uuid.UUID, since time.Time) ([]analytics.EngagementCandidate, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT event_name, occurred_at, send_id, source,
		       JSONExtractString(properties, 'campaign_id'), JSONExtractString(properties, 'flow_id')
		FROM analytics_events FINAL
		WHERE channel_id = ? AND customer_id = ? AND event_name IN ('email_opened', 'email_clicked')
		  AND send_id != toUUID('00000000-0000-0000-0000-000000000000') AND occurred_at >= ?
		ORDER BY (event_name = 'email_clicked') DESC, occurred_at DESC`, channelID, customerID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.EngagementCandidate
	for rows.Next() {
		var c analytics.EngagementCandidate
		if err := rows.Scan(&c.EventName, &c.OccurredAt, &c.SendID, &c.Source, &c.CampaignID, &c.FlowID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Order aggregates per customer, netting out orders later cancelled - properties are JSON here, so the order id comes out with JSONExtractString rather than a jsonb operator
// The window applies to the placement only: a cancellation outside it still voids its order, since the question is "what did they buy since X", not "what happened since X"
func (s *Store) ChannelOrderAggregates(ctx context.Context, channelID uuid.UUID, since time.Time) ([]analytics.CustomerOrders, error) {
	window := ""
	args := []any{channelID}
	if !since.IsZero() {
		window = " AND occurred_at >= ?"
		args = append(args, since)
	}
	args = append(args, channelID)
	rows, err := s.conn.Query(ctx, `
		SELECT customer_id,
		       toInt32(count()) AS order_count,
		       toInt64(sum(toInt64OrZero(JSONExtractString(properties, 'total_cents')))) AS monetary_cents,
		       min(occurred_at) AS first_order_at,
		       max(occurred_at) AS last_order_at
		FROM analytics_events FINAL
		WHERE channel_id = ?
		  AND event_name = 'order_placed'
		  AND customer_id != toUUID('00000000-0000-0000-0000-000000000000')
		  AND JSONHas(properties, 'order_id')`+window+`
		  AND JSONExtractString(properties, 'order_id') NOT IN (
		      SELECT JSONExtractString(properties, 'order_id')
		      FROM analytics_events FINAL
		      WHERE channel_id = ? AND event_name = 'order_cancelled' AND JSONHas(properties, 'order_id')
		  )
		GROUP BY customer_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.CustomerOrders
	for rows.Next() {
		var c analytics.CustomerOrders
		if err := rows.Scan(&c.CustomerID, &c.OrderCount, &c.MonetaryCents, &c.FirstOrderAt, &c.LastOrderAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ChannelEngagementCounts(ctx context.Context, channelID uuid.UUID, since time.Time) ([]analytics.CustomerEngagement, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT customer_id, toInt32(least(count(), 100)) AS engagement
		FROM analytics_events FINAL
		WHERE channel_id = ? AND customer_id != toUUID('00000000-0000-0000-0000-000000000000') AND occurred_at >= ?
		GROUP BY customer_id`, channelID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.CustomerEngagement
	for rows.Next() {
		var c analytics.CustomerEngagement
		if err := rows.Scan(&c.CustomerID, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Arrival-time bounds for the health panel - min/max on created_at read part metadata, so this stays instant at any size (no FINAL: a duplicate can't move a min or a max)
func (s *Store) Stats(ctx context.Context) (analytics.SpineStats, error) {
	var out analytics.SpineStats
	var oldest, newest time.Time
	if err := s.conn.QueryRow(ctx,
		"SELECT min(created_at), max(created_at) FROM analytics_events").Scan(&oldest, &newest); err != nil {
		return out, err
	}
	if !oldest.IsZero() {
		o := oldest.UTC()
		out.Oldest = &o
	}
	if !newest.IsZero() {
		n := newest.UTC()
		out.Newest = &n
	}
	return out, nil
}

// no FINAL on either id-slice read - duplicates can't change a DISTINCT id set, and FINAL would force a merge pass over every matching part
func (s *Store) ActiveCustomerIDs(ctx context.Context, channelID uuid.UUID, from, to time.Time) ([]uuid.UUID, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT DISTINCT customer_id FROM analytics_events
		WHERE channel_id = ? AND customer_id != toUUID('00000000-0000-0000-0000-000000000000')
		  AND occurred_at >= ? AND occurred_at < ?`, channelID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// the fresh-slice twin on created_at (arrival time) - the minmax index prunes to the recent granules, which is the whole point of slicing by arrival
func (s *Store) IngestedCustomerIDs(ctx context.Context, channelID uuid.UUID, from, to time.Time) ([]uuid.UUID, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT DISTINCT customer_id FROM analytics_events
		WHERE channel_id = ? AND customer_id != toUUID('00000000-0000-0000-0000-000000000000')
		  AND created_at >= ? AND created_at < ?`, channelID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// the id set is chunk-bounded by the caller, so has() stays cheap
func (s *Store) ChannelEngagementCountsFor(ctx context.Context, channelID uuid.UUID, since time.Time, customerIDs []uuid.UUID) ([]analytics.CustomerEngagement, error) {
	ids := make([]string, len(customerIDs))
	for i, id := range customerIDs {
		ids[i] = id.String()
	}
	rows, err := s.conn.Query(ctx, `
		SELECT customer_id, toInt32(least(count(), 100)) AS engagement
		FROM analytics_events FINAL
		WHERE channel_id = ? AND has(?, toString(customer_id)) AND occurred_at >= ?
		GROUP BY customer_id`, channelID, ids, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.CustomerEngagement
	for rows.Next() {
		var c analytics.CustomerEngagement
		if err := rows.Scan(&c.CustomerID, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// A keyset page of raw events for outbound forwarding - ClickHouse sorts on (channel_id, dedupe_key), so this orders explicitly and pays a sort, which is bounded by the batch limit
// The cursor compares milliseconds and the uuid AS TEXT: a batch insert stamps one created_at across the whole batch, so the tiebreaker decides the page boundary, and ClickHouse orders UUID by its two internal words rather than by its text, so comparing the raw column would disagree with the text cursor and re-serve rows
func (s *Store) EventsForForwarding(ctx context.Context, channelID uuid.UUID, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]analytics.ForwardEvent, error) {
	// the 5s watermark - a batch mid-flush must never end up behind an advanced cursor
	// The bare created_at >= bound is what the minmax skip index can evaluate - without it the function-wrapped tuple reads the channel's entire retained history every poll, and the tuple then draws the exact page boundary within that granule set
	rows, err := s.conn.Query(ctx, `
		SELECT id, event_name, occurred_at, customer_id, anon_id, source, properties, created_at, dedupe_key
		FROM analytics_events FINAL
		WHERE channel_id = ? AND created_at >= ?
		  AND (toUnixTimestamp64Milli(created_at), toString(id)) > (?, ?)
		  AND created_at < now64(3) - INTERVAL 5 SECOND
		ORDER BY toUnixTimestamp64Milli(created_at), toString(id) LIMIT ?`,
		channelID, afterCreatedAt, afterCreatedAt.UnixMilli(), afterID.String(), uint64(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.ForwardEvent
	for rows.Next() {
		var (
			e    analytics.ForwardEvent
			cust uuid.UUID
			raw  string
		)
		if err := rows.Scan(&e.ID, &e.Name, &e.OccurredAt, &cust, &e.AnonID, &e.Source, &raw, &e.CreatedAt, &e.DedupeKey); err != nil {
			return nil, err
		}
		if cust != uuid.Nil {
			id := cust
			e.CustomerID = &id
		}
		e.Properties = json.RawMessage(raw)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Pickable goal-event names, most-frequent first - web custom events by their real name, plus commerce and account event names so an experiment can measure orders
func (s *Store) ExperimentGoalEvents(ctx context.Context, channelID uuid.UUID, since time.Time) ([]string, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT name, toInt64(sum(uses)) AS total FROM (
			SELECT custom_name AS name, count() AS uses
			FROM analytics_events FINAL
			WHERE source = 'web' AND event_name = 'custom' AND channel_id = ? AND occurred_at >= ?
			  AND custom_name != '' AND custom_name != '$experiment'
			GROUP BY name
			UNION ALL
			SELECT event_name AS name, count() AS uses
			FROM analytics_events FINAL
			WHERE source IN ('commerce', 'account') AND channel_id = ? AND occurred_at >= ?
			GROUP BY name
		)
		GROUP BY name ORDER BY total DESC, name ASC LIMIT 100`,
		channelID, since, channelID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		var total int64
		if err := rows.Scan(&name, &total); err != nil {
			return nil, err
		}
		if name != "" {
			out = append(out, name)
		}
	}
	return out, rows.Err()
}

// Per-variation exposed / converted / revenue for one experiment
// A goal matches an exposure by the same person - ClickHouse joins on equality only, so both sides collapse to one actor key first (customer when known, else the stitched customer, else the anon) - equivalent, since the OR only ever bridged those two spellings of one person - the time bound moves into the aggregates rather than a WHERE, which would drop an exposure whose only goal predates it instead of counting it unconverted
func (s *Store) ExperimentResults(ctx context.Context, channelID uuid.UUID, key, goalEvent string, since time.Time) ([]analytics.VariationResult, error) {
	// dictHas, not a zero-sentinel compare on the dictGet - see dictHit
	const goalActor = "if(customer_id != " + zeroUUID + ", toString(customer_id), " +
		"if(dictHas('" + identityDictName + "', tuple(anon_id))" +
		", toString(dictGetUUID('" + identityDictName + "', 'customer_id', tuple(anon_id))), anon_id))"
	rows, err := s.conn.Query(ctx, `
		WITH exposures AS (
			SELECT JSONExtractString(properties, 'variation') AS variation,
			       anon_id,
			       any(`+goalActor+`) AS actor,
			       min(occurred_at) AS first_exp
			FROM analytics_events FINAL
			WHERE source = 'web' AND event_name = 'custom' AND custom_name = '$experiment'
			  AND JSONExtractString(properties, 'experiment') = ?
			  AND channel_id = ? AND occurred_at >= ? AND anon_id != ''
			GROUP BY variation, anon_id
		),
		goals AS (
			SELECT `+goalActor+` AS actor, occurred_at,
			       if(revenue_cents != 0, revenue_cents, JSONExtractInt(properties, 'total_cents')) AS rev
			FROM analytics_events FINAL
			WHERE channel_id = ? AND occurred_at >= ?
			  AND ((event_name = 'custom' AND custom_name = ?) OR event_name = ?)
		)
		SELECT e.variation AS variation,
		       toInt64(uniqExact(e.anon_id)) AS exposed,
		       toInt64(uniqExactIf(e.anon_id, g.actor != '' AND g.occurred_at >= e.first_exp)) AS converted,
		       toInt64(sumIf(g.rev, g.actor != '' AND g.occurred_at >= e.first_exp)) AS revenue_cents
		FROM exposures e
		LEFT JOIN goals g ON g.actor = e.actor
		GROUP BY variation ORDER BY variation`,
		key, channelID, since, channelID, since, goalEvent, goalEvent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.VariationResult
	for rows.Next() {
		var v analytics.VariationResult
		var exposed, converted, revenue int64
		if err := rows.Scan(&v.Variation, &exposed, &converted, &revenue); err != nil {
			return nil, err
		}
		v.Exposed, v.Converted, v.Revenue = int(exposed), int(converted), revenue
		out = append(out, v)
	}
	return out, rows.Err()
}

// --- Lens spine reads (ClickHouse driver) ---
// page_key and custom_name are materialized columns, so the page-scoped filters and the $rage_click / $js_error / $web_vitals discriminators are plain column reads - p75 is the interpolating quantileExactInclusive

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

func (s *Store) CustomerEmailEngagement(ctx context.Context, customerID uuid.UUID) (analytics.EmailEngagement, error) {
	var out analytics.EmailEngagement
	var sent, opened, clicked int64
	var lastSent, lastOpen time.Time
	// maxIf over a DateTime returns the zero date when nothing matches, which is what the nil below means
	if err := s.conn.QueryRow(ctx, `
		SELECT
			toInt64(countIf(event_name = 'email_sent'))                     AS sent,
			toInt64(countIf(event_name = 'email_opened' AND NOT machine))    AS opened,
			toInt64(countIf(event_name = 'email_clicked'))                   AS clicked,
			maxIf(occurred_at, event_name = 'email_sent')                    AS last_sent,
			maxIf(occurred_at, event_name = 'email_opened' AND NOT machine)  AS last_open
		FROM analytics_events FINAL
		WHERE customer_id = ? AND event_name IN ('email_sent', 'email_opened', 'email_clicked')`,
		customerID).Scan(&sent, &opened, &clicked, &lastSent, &lastOpen); err != nil {
		return analytics.EmailEngagement{}, err
	}
	out.Sent, out.Opened, out.Clicked = int(sent), int(opened), int(clicked)
	if !lastSent.IsZero() {
		out.LastSentAt = &lastSent
	}
	if !lastOpen.IsZero() {
		out.LastOpenAt = &lastOpen
	}
	return out, nil
}
