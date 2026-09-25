-- Lens read aggregates. Page-view / traffic analytics read off the analytics spine (source='web'); the heatmap
-- grid/snapshot tables are queried in Stage B. Channel-scoped + date-ranged, mirroring the sales/pulse read models

-- name: LensTotalViews :one
-- Rollup-backed total page views (no raw-event scan). `since` is a timestamp; day
-- is derived. Paired with LensVisitorCount for the overview tiles.
SELECT COALESCE(SUM(views), 0)::bigint AS views
FROM lens_traffic_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND dim = 'total';

-- name: LensVisitorCount :one
-- Exact unique visitors over the window from the seen-table - COUNT, not scan.
SELECT COUNT(*)::bigint AS visitors
FROM lens_visitor_seen
WHERE channel_id = sqlc.arg(channel_id) AND last_seen >= sqlc.arg(since);

-- name: LensDailyViews :many
-- Daily total page views over the window (the overview traffic-trend chart).
-- Rollup-backed (dim='total'); one row per day with traffic.
SELECT to_char(day, 'YYYY-MM-DD') AS day, SUM(views)::bigint AS views
FROM lens_traffic_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND dim = 'total'
GROUP BY day
ORDER BY day;

-- name: LensTopPages :many
-- Rollup-backed page views (per-page unique visitors dropped - see service).
SELECT key AS page_key, SUM(views)::bigint AS views
FROM lens_traffic_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND dim = 'page'
GROUP BY key
ORDER BY views DESC
LIMIT sqlc.arg(lim);

-- name: LensDeviceSplit :many
SELECT key AS device, SUM(views)::bigint AS views
FROM lens_traffic_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND dim = 'device'
GROUP BY key
ORDER BY views DESC;

-- name: LensTopReferrers :many
SELECT key AS referrer, SUM(views)::bigint AS views
FROM lens_traffic_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND dim = 'referrer'
GROUP BY key
ORDER BY views DESC
LIMIT sqlc.arg(lim);

-- name: LensTopInteractions :many
-- Named UI interactions (sidebar/modal/accordion toggles + custom-tracked
-- events), counted by name. Rollup-backed (dim='interaction').
SELECT key AS name, SUM(views)::bigint AS count
FROM lens_traffic_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND dim = 'interaction'
GROUP BY key
ORDER BY count DESC
LIMIT sqlc.arg(lim);

-- Bot / crawler analytics (rollup-backed) -------------------------------------
-- Counts are coalesced in-process into analytics_traffic_rollup - dashboards read the rollup and never raw events, so query cost is bounded

-- name: UpsertTrafficRollup :exec
INSERT INTO analytics_traffic_rollup (channel_id, day, kind, bot_name, bot_category, page_key, hits, last_seen)
VALUES (sqlc.arg(channel_id), sqlc.arg(day), sqlc.arg(kind), sqlc.arg(bot_name), sqlc.arg(bot_category), sqlc.arg(page_key), sqlc.arg(hits), sqlc.arg(last_seen))
ON CONFLICT (channel_id, day, kind, bot_name, bot_category, page_key)
DO UPDATE SET hits      = analytics_traffic_rollup.hits + EXCLUDED.hits,
              last_seen = GREATEST(analytics_traffic_rollup.last_seen, EXCLUDED.last_seen);

-- name: PruneTrafficRollup :execrows
DELETE FROM analytics_traffic_rollup WHERE day < sqlc.arg(before)::date;

-- name: BotTrafficSummary :one
SELECT
    COALESCE(SUM(hits) FILTER (WHERE kind = 'bot'), 0)::bigint              AS bot_hits,
    COUNT(DISTINCT bot_name) FILTER (WHERE kind = 'bot' AND bot_name <> '') AS bot_count,
    COUNT(DISTINCT page_key) FILTER (WHERE kind = 'bot' AND page_key <> '') AS page_count,
    COALESCE(SUM(hits) FILTER (WHERE kind = 'human'), 0)::bigint            AS human_views
FROM analytics_traffic_rollup
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date;

-- name: BotByAgent :many
SELECT
    bot_name,
    bot_category,
    SUM(hits)::bigint                                      AS hits,
    COUNT(DISTINCT page_key) FILTER (WHERE page_key <> '') AS pages,
    MAX(last_seen)                                         AS last_seen
FROM analytics_traffic_rollup
WHERE kind = 'bot' AND channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date
GROUP BY bot_name, bot_category
ORDER BY hits DESC;

-- name: BotTopPages :many
-- Most-crawled pages, optionally filtered to one bot (empty bot_name = all bots).
SELECT
    page_key,
    SUM(hits)::bigint                                      AS hits,
    COUNT(DISTINCT bot_name) FILTER (WHERE bot_name <> '') AS bots
FROM analytics_traffic_rollup
WHERE kind = 'bot' AND channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date
  AND (sqlc.arg(bot_name)::text = '' OR bot_name = sqlc.arg(bot_name)::text)
GROUP BY page_key
ORDER BY hits DESC
LIMIT sqlc.arg(lim);

-- name: BotDailySeries :many
SELECT
    to_char(day, 'YYYY-MM-DD')                                  AS day,
    COALESCE(SUM(hits) FILTER (WHERE kind = 'bot'), 0)::bigint   AS bot_hits,
    COALESCE(SUM(hits) FILTER (WHERE kind = 'human'), 0)::bigint AS human_views
FROM analytics_traffic_rollup
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date
GROUP BY day
ORDER BY day;

-- Lens hub rollup writes (coalesced + flushed by the traffic-rollup buffer) ----

-- name: UpsertLensDaily :exec
INSERT INTO lens_traffic_daily (channel_id, day, dim, key, views)
VALUES (sqlc.arg(channel_id), sqlc.arg(day), sqlc.arg(dim), sqlc.arg(key), sqlc.arg(views))
ON CONFLICT (channel_id, day, dim, key)
DO UPDATE SET views = lens_traffic_daily.views + EXCLUDED.views;

-- name: UpsertLensVisitor :exec
INSERT INTO lens_visitor_seen (channel_id, anon_id, last_seen)
VALUES (sqlc.arg(channel_id), sqlc.arg(anon_id), sqlc.arg(last_seen))
ON CONFLICT (channel_id, anon_id)
DO UPDATE SET last_seen = GREATEST(lens_visitor_seen.last_seen, EXCLUDED.last_seen);

-- name: PruneLensDaily :execrows
DELETE FROM lens_traffic_daily WHERE day < sqlc.arg(before)::date;

-- name: PruneLensVisitors :execrows
DELETE FROM lens_visitor_seen WHERE last_seen < sqlc.arg(before);

-- One-time backfill (run off-peak via `skelet lens backfill-rollups`). Aggregates existing raw web events into the rollups so the hub shows
-- history immediately. Idempotent for the view dims (DO UPDATE SET = EXCLUDED overwrites covered days)

-- name: LensUpsertSnapshot :exec
-- Store a layout once per state; a repeat visit to the same layout just bumps last_seen
INSERT INTO lens_page_snapshots (channel_id, page_key, viewport_bucket, state_key, layout_hash, blob_key, width, height)
VALUES (sqlc.arg(channel_id), sqlc.arg(page_key), sqlc.arg(viewport_bucket), sqlc.arg(state_key), sqlc.arg(layout_hash), sqlc.arg(blob_key), sqlc.arg(width), sqlc.arg(height))
ON CONFLICT (channel_id, page_key, viewport_bucket, state_key, layout_hash)
DO UPDATE SET last_seen_at = NOW();

-- Stage B: heatmap reads ------------------------------------------------------

-- name: LensViewports :many
SELECT DISTINCT viewport_bucket FROM lens_page_snapshots
WHERE channel_id = sqlc.arg(channel_id) AND page_key = sqlc.arg(page_key)
ORDER BY viewport_bucket;

-- name: LensStates :many
-- The captured UI states for a page+viewport (default first), for the viewer's state switcher
-- Subquery so the default-first ORDER BY expression needn't be in the DISTINCT select list
SELECT s.state_key FROM (
    SELECT DISTINCT state_key FROM lens_page_snapshots
    WHERE channel_id = sqlc.arg(channel_id) AND page_key = sqlc.arg(page_key) AND viewport_bucket = sqlc.arg(viewport_bucket)
) s
ORDER BY (s.state_key <> 'default'), s.state_key;

-- name: LensLatestSnapshot :one
SELECT blob_key, width, height FROM lens_page_snapshots
WHERE channel_id = sqlc.arg(channel_id) AND page_key = sqlc.arg(page_key)
  AND viewport_bucket = sqlc.arg(viewport_bucket) AND state_key = sqlc.arg(state_key)
ORDER BY last_seen_at DESC
LIMIT 1;

-- name: LensSnapshotBlobKeysBefore :many
-- Object keys for stale snapshots - deleted from object storage before the rows.
SELECT blob_key FROM lens_page_snapshots WHERE last_seen_at < sqlc.arg(before);

-- name: LensPruneStaleSnapshots :execrows
DELETE FROM lens_page_snapshots WHERE last_seen_at < sqlc.arg(before);

-- name: LensUpsertSession :execrows
-- Roll a flush's metadata onto the session row - the first chunk inserts, later ones bump last_event_at, event_count and exit_page
-- started_at stays the earliest seen, and customer_id fills once the visitor is known
-- A flush naming another channel's or another visitor's session id touches nothing, so the caller drops its chunk
INSERT INTO lens_sessions (
    id, channel_id, anon_id, customer_id, started_at, last_event_at,
    event_count, entry_page, exit_page, user_agent, viewport
) VALUES (
    sqlc.arg(id), sqlc.arg(channel_id), sqlc.arg(anon_id), sqlc.narg(customer_id),
    sqlc.arg(started_at), sqlc.arg(last_event_at), sqlc.arg(event_count),
    sqlc.arg(entry_page), sqlc.arg(exit_page), sqlc.arg(user_agent), sqlc.arg(viewport)
)
ON CONFLICT (id) DO UPDATE SET
    customer_id   = COALESCE(EXCLUDED.customer_id, lens_sessions.customer_id),
    started_at    = LEAST(lens_sessions.started_at, EXCLUDED.started_at),
    last_event_at = GREATEST(lens_sessions.last_event_at, EXCLUDED.last_event_at),
    event_count   = lens_sessions.event_count + EXCLUDED.event_count,
    exit_page     = EXCLUDED.exit_page
WHERE lens_sessions.channel_id = EXCLUDED.channel_id AND lens_sessions.anon_id = EXCLUDED.anon_id;

-- name: LensBumpSessionSignal :exec
-- Increment the matching per-session UX-signal counter (kind = rage|dead|error)
-- when a signal custom event carries this recording's sessionId.
UPDATE lens_sessions SET
    rage_count  = rage_count  + (CASE WHEN sqlc.arg(kind)::text = 'rage'  THEN 1 ELSE 0 END),
    dead_count  = dead_count  + (CASE WHEN sqlc.arg(kind)::text = 'dead'  THEN 1 ELSE 0 END),
    error_count = error_count + (CASE WHEN sqlc.arg(kind)::text = 'error' THEN 1 ELSE 0 END)
WHERE id = sqlc.arg(id) AND channel_id = sqlc.arg(channel_id);

-- name: LensInsertChunk :exec
-- blob_key names the gzip bytes in object storage
INSERT INTO lens_session_chunks (session_id, seq, blob_key, event_count, first_ts, last_ts)
VALUES (sqlc.arg(session_id), sqlc.arg(seq), sqlc.arg(blob_key), sqlc.arg(event_count), sqlc.arg(first_ts), sqlc.arg(last_ts))
ON CONFLICT (session_id, seq) DO NOTHING;

-- name: LensSessionsForCustomer :many
-- Every session for a customer: those resolved to the customer id PLUS sessions
-- recorded under any anon id they later identified with (pre-login coverage).
SELECT id, anon_id, customer_id, started_at, last_event_at, event_count, entry_page, exit_page, viewport, user_agent, rage_count, error_count
FROM lens_sessions
WHERE (customer_id = sqlc.arg(customer_id) OR anon_id = ANY(sqlc.arg(anon_ids)::text[]))
  AND (sqlc.arg(all_channels)::boolean OR channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
ORDER BY started_at DESC
LIMIT sqlc.arg(lim);

-- name: LensSessionsForAnon :many
SELECT id, anon_id, customer_id, started_at, last_event_at, event_count, entry_page, exit_page, viewport, user_agent, rage_count, error_count
FROM lens_sessions
WHERE anon_id = sqlc.arg(anon_id)
  AND (sqlc.arg(all_channels)::boolean OR channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
ORDER BY started_at DESC
LIMIT sqlc.arg(lim);

-- name: LensSessionsForChannel :many
SELECT id, anon_id, customer_id, started_at, last_event_at, event_count, entry_page, exit_page, viewport, user_agent, rage_count, error_count
FROM lens_sessions
WHERE channel_id = sqlc.arg(channel_id)
ORDER BY started_at DESC
LIMIT sqlc.arg(lim);

-- name: LensGetSession :one
SELECT id, channel_id, anon_id, customer_id, started_at, last_event_at, event_count, entry_page, exit_page, viewport, user_agent
FROM lens_sessions WHERE id = sqlc.arg(id);

-- name: LensSessionChunks :many
-- The object keys of a session's compressed event chunks, in replay order
SELECT blob_key FROM lens_session_chunks WHERE session_id = sqlc.arg(session_id) ORDER BY seq;

-- name: LensSessionBlobKeysBefore :many
-- Object-storage keys for chunks of sessions past the cutoff - fetched so the
-- prune job can delete the objects before the rows (which cascade) go.
SELECT c.blob_key FROM lens_session_chunks c
JOIN lens_sessions s ON s.id = c.session_id
WHERE s.last_event_at < sqlc.arg(before);

-- name: LensPruneSessions :execrows
DELETE FROM lens_sessions WHERE last_event_at < sqlc.arg(before);

-- name: LensTopSignalPages :many
-- Top pages by a signal dim (rage | dead | error), rollup-backed (lens_traffic_daily).
SELECT key AS page_key, SUM(views)::bigint AS count
FROM lens_traffic_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND dim = sqlc.arg(dim)
GROUP BY key
ORDER BY count DESC
LIMIT sqlc.arg(lim);

-- name: LensTopSearches :many
SELECT key AS term, SUM(views)::bigint AS count
FROM lens_traffic_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND dim = 'search'
GROUP BY key
ORDER BY count DESC
LIMIT sqlc.arg(lim);

-- name: LensZeroResultSearches :many
SELECT key AS term, SUM(views)::bigint AS count
FROM lens_traffic_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND dim = 'search_zero'
GROUP BY key
ORDER BY count DESC
LIMIT sqlc.arg(lim);

-- name: LensSearchTotals :one
SELECT
  COALESCE(SUM(views) FILTER (WHERE dim = 'search'), 0)::bigint      AS total_searches,
  COUNT(DISTINCT key) FILTER (WHERE dim = 'search')::bigint          AS distinct_terms,
  COALESCE(SUM(views) FILTER (WHERE dim = 'search_zero'), 0)::bigint AS zero_result_searches
FROM lens_traffic_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND dim IN ('search', 'search_zero');

-- name: LensSearchDaily :many
SELECT to_char(day, 'YYYY-MM-DD') AS day,
  COALESCE(SUM(views) FILTER (WHERE dim = 'search'), 0)::bigint      AS searches,
  COALESCE(SUM(views) FILTER (WHERE dim = 'search_zero'), 0)::bigint AS zero_results
FROM lens_traffic_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND dim IN ('search', 'search_zero')
GROUP BY day
ORDER BY day;
