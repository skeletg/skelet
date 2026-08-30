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

-- name: LensPageEngagement :many
-- Per-page click + scroll rollup from the heatmap grid (cumulative - the grid
-- has no per-hit timestamp). avg scroll depth = scroll_weight / scroll_total,
-- computed in Go to dodge div-by-zero.
SELECT
    page_key,
    COALESCE(SUM(count) FILTER (WHERE kind = 'click'), 0)::bigint          AS clicks,
    COALESCE(SUM(cell_y * count) FILTER (WHERE kind = 'scroll'), 0)::bigint AS scroll_weight,
    COALESCE(SUM(count) FILTER (WHERE kind = 'scroll'), 0)::bigint          AS scroll_total
FROM lens_heatmap_cells
WHERE channel_id = sqlc.arg(channel_id)
GROUP BY page_key;

-- name: LensPageDeviceSplit :many
-- Device distribution for ONE page (all-time, like the page's heatmap) - shown
-- on the page's detail view, where a full split is meaningful (unlike a single
-- "top device" per row in the overview table).
SELECT
    COALESCE(NULLIF(properties->>'device', ''), 'unknown') AS device,
    COUNT(*)                                               AS views
FROM analytics_events
WHERE source = 'web' AND event_name = 'page_view'
  AND channel_id = sqlc.arg(channel_id)
  AND COALESCE(properties->>'page_key', '') = sqlc.arg(page_key)::text
GROUP BY 1
ORDER BY views DESC;

-- name: LensPageTotals :one
SELECT COUNT(*) AS views, COUNT(DISTINCT anon_id) AS visitors
FROM analytics_events
WHERE source = 'web' AND event_name = 'page_view' AND channel_id = sqlc.arg(channel_id)
  AND COALESCE(properties->>'page_key', '') = sqlc.arg(page_key)::text;

-- name: LensPageReferrers :many
SELECT COALESCE(NULLIF(properties->>'referrer', ''), '(direct)') AS referrer, COUNT(*) AS views
FROM analytics_events
WHERE source = 'web' AND event_name = 'page_view' AND channel_id = sqlc.arg(channel_id)
  AND COALESCE(properties->>'page_key', '') = sqlc.arg(page_key)::text
GROUP BY 1 ORDER BY views DESC LIMIT 10;

-- name: LensPageClients :many
SELECT COALESCE(NULLIF(properties->>'client', ''), 'unknown') AS client, COUNT(*) AS views
FROM analytics_events
WHERE source = 'web' AND event_name = 'page_view' AND channel_id = sqlc.arg(channel_id)
  AND COALESCE(properties->>'page_key', '') = sqlc.arg(page_key)::text
GROUP BY 1 ORDER BY views DESC LIMIT 10;

-- name: LensPageTimeline :many
-- Daily page-view counts for one page over the window (sparkline).
SELECT to_char(date_trunc('day', occurred_at), 'YYYY-MM-DD') AS date, COUNT(*) AS views
FROM analytics_events
WHERE source = 'web' AND event_name = 'page_view' AND channel_id = sqlc.arg(channel_id)
  AND COALESCE(properties->>'page_key', '') = sqlc.arg(page_key)::text
  AND occurred_at >= sqlc.arg(since)
GROUP BY 1 ORDER BY 1;

-- name: LensPageGrid :one
-- One page's cumulative click total + scroll weighted-avg inputs (all viewports/states).
SELECT
    COALESCE(SUM(count) FILTER (WHERE kind = 'click'), 0)::bigint          AS clicks,
    COALESCE(SUM(cell_y * count) FILTER (WHERE kind = 'scroll'), 0)::bigint AS scroll_weight,
    COALESCE(SUM(count) FILTER (WHERE kind = 'scroll'), 0)::bigint          AS scroll_total
FROM lens_heatmap_cells
WHERE channel_id = sqlc.arg(channel_id) AND page_key = sqlc.arg(page_key);

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
-- Crawl + human page-view counts are coalesced in-process and flushed into
-- analytics_traffic_rollup (kind='bot' rows carry bot_name/category/page_key;
-- kind='human' rows are a per-(channel,day) view counter). Dashboards read the
-- rollup, NEVER raw events - so query cost is bounded regardless of volume.

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

-- name: BackfillLensTotals :exec
INSERT INTO lens_traffic_daily (channel_id, day, dim, key, views)
SELECT channel_id, occurred_at::date, 'total', '', COUNT(*)
FROM analytics_events
WHERE source = 'web' AND event_name = 'page_view' AND channel_id IS NOT NULL
  AND COALESCE((properties->>'is_bot')::boolean, false) = false
GROUP BY channel_id, occurred_at::date
ON CONFLICT (channel_id, day, dim, key) DO UPDATE SET views = EXCLUDED.views;

-- name: BackfillLensPages :exec
INSERT INTO lens_traffic_daily (channel_id, day, dim, key, views)
SELECT channel_id, occurred_at::date, 'page', COALESCE(properties->>'page_key', ''), COUNT(*)
FROM analytics_events
WHERE source = 'web' AND event_name = 'page_view' AND channel_id IS NOT NULL
  AND COALESCE((properties->>'is_bot')::boolean, false) = false
GROUP BY channel_id, occurred_at::date, COALESCE(properties->>'page_key', '')
ON CONFLICT (channel_id, day, dim, key) DO UPDATE SET views = EXCLUDED.views;

-- name: BackfillLensDevices :exec
INSERT INTO lens_traffic_daily (channel_id, day, dim, key, views)
SELECT channel_id, occurred_at::date, 'device', COALESCE(NULLIF(properties->>'device', ''), 'unknown'), COUNT(*)
FROM analytics_events
WHERE source = 'web' AND event_name = 'page_view' AND channel_id IS NOT NULL
  AND COALESCE((properties->>'is_bot')::boolean, false) = false
GROUP BY channel_id, occurred_at::date, COALESCE(NULLIF(properties->>'device', ''), 'unknown')
ON CONFLICT (channel_id, day, dim, key) DO UPDATE SET views = EXCLUDED.views;

-- name: BackfillLensReferrers :exec
INSERT INTO lens_traffic_daily (channel_id, day, dim, key, views)
SELECT channel_id, occurred_at::date, 'referrer', COALESCE(NULLIF(properties->>'referrer', ''), '(direct)'), COUNT(*)
FROM analytics_events
WHERE source = 'web' AND event_name = 'page_view' AND channel_id IS NOT NULL
  AND COALESCE((properties->>'is_bot')::boolean, false) = false
GROUP BY channel_id, occurred_at::date, COALESCE(NULLIF(properties->>'referrer', ''), '(direct)')
ON CONFLICT (channel_id, day, dim, key) DO UPDATE SET views = EXCLUDED.views;

-- name: BackfillLensInteractions :exec
INSERT INTO lens_traffic_daily (channel_id, day, dim, key, views)
SELECT channel_id, occurred_at::date, 'interaction', COALESCE(NULLIF(properties->>'name', ''), '(unnamed)'), COUNT(*)
FROM analytics_events
WHERE source = 'web' AND event_name = 'custom' AND channel_id IS NOT NULL
GROUP BY channel_id, occurred_at::date, COALESCE(NULLIF(properties->>'name', ''), '(unnamed)')
ON CONFLICT (channel_id, day, dim, key) DO UPDATE SET views = EXCLUDED.views;

-- name: BackfillLensVisitors :exec
INSERT INTO lens_visitor_seen (channel_id, anon_id, last_seen)
SELECT channel_id, anon_id, MAX(occurred_at)
FROM analytics_events
WHERE source = 'web' AND event_name = 'page_view' AND channel_id IS NOT NULL
  AND anon_id IS NOT NULL AND anon_id <> ''
  AND COALESCE((properties->>'is_bot')::boolean, false) = false
GROUP BY channel_id, anon_id
ON CONFLICT (channel_id, anon_id) DO UPDATE SET last_seen = GREATEST(lens_visitor_seen.last_seen, EXCLUDED.last_seen);

-- Stage B: heatmap aggregate upserts (binned on ingest) -----------------------

-- name: LensUpsertCell :exec
-- Increment one grid cell. The service pre-aggregates a batch's points per cell
-- (so the per-flush loop is bounded by distinct cells, not raw points) and
-- loops this - sqlc's analyzer rejects the parallel-array unnest form.
INSERT INTO lens_heatmap_cells (channel_id, page_key, viewport_bucket, state_key, kind, cell_x, cell_y, count)
VALUES (sqlc.arg(channel_id), sqlc.arg(page_key), sqlc.arg(viewport_bucket), sqlc.arg(state_key), sqlc.arg(kind), sqlc.arg(cell_x), sqlc.arg(cell_y), sqlc.arg(count))
ON CONFLICT (channel_id, page_key, viewport_bucket, state_key, kind, cell_x, cell_y)
DO UPDATE SET count = lens_heatmap_cells.count + excluded.count, updated_at = NOW();

-- name: LensUpsertElement :exec
INSERT INTO lens_element_clicks (channel_id, page_key, viewport_bucket, selector, text, count)
VALUES (sqlc.arg(channel_id), sqlc.arg(page_key), sqlc.arg(viewport_bucket), sqlc.arg(selector), sqlc.arg(text), sqlc.arg(count))
ON CONFLICT (channel_id, page_key, viewport_bucket, selector)
DO UPDATE SET count = lens_element_clicks.count + excluded.count, text = excluded.text, updated_at = NOW();

-- name: LensUpsertSnapshot :exec
-- Store a layout once per state; a repeat visit to the same layout just bumps
-- last_seen. html (in-db) or blob_key (object storage) is set, not both.
INSERT INTO lens_page_snapshots (channel_id, page_key, viewport_bucket, state_key, layout_hash, html, blob_key, width, height)
VALUES (sqlc.arg(channel_id), sqlc.arg(page_key), sqlc.arg(viewport_bucket), sqlc.arg(state_key), sqlc.arg(layout_hash), sqlc.narg(html), sqlc.narg(blob_key), sqlc.arg(width), sqlc.arg(height))
ON CONFLICT (channel_id, page_key, viewport_bucket, state_key, layout_hash)
DO UPDATE SET last_seen_at = NOW();

-- Stage B: heatmap reads ------------------------------------------------------

-- name: LensListPages :many
-- Pages with web traffic (page views) - the heatmap viewer's page picker, kept
-- consistent with the overview's Top Pages list (sourced from page views, not
-- snapshots). A page may lack a snapshot yet; the viewer shows a "browse it to
-- capture" notice for those.
SELECT COALESCE(properties->>'page_key', '') AS page_key, COUNT(*) AS views
FROM analytics_events
WHERE source = 'web' AND event_name = 'page_view' AND channel_id = sqlc.arg(channel_id)
GROUP BY 1
ORDER BY views DESC
LIMIT 200;

-- name: LensViewports :many
SELECT DISTINCT viewport_bucket FROM lens_page_snapshots
WHERE channel_id = sqlc.arg(channel_id) AND page_key = sqlc.arg(page_key)
ORDER BY viewport_bucket;

-- name: LensStates :many
-- The captured UI states for a page+viewport (default first), for the viewer's
-- state switcher. Subquery so the 'default-first' ORDER BY expression needn't be
-- in the DISTINCT select list.
SELECT s.state_key FROM (
    SELECT DISTINCT state_key FROM lens_page_snapshots
    WHERE channel_id = sqlc.arg(channel_id) AND page_key = sqlc.arg(page_key) AND viewport_bucket = sqlc.arg(viewport_bucket)
) s
ORDER BY (s.state_key <> 'default'), s.state_key;

-- name: LensLatestSnapshot :one
SELECT html, blob_key, width, height FROM lens_page_snapshots
WHERE channel_id = sqlc.arg(channel_id) AND page_key = sqlc.arg(page_key)
  AND viewport_bucket = sqlc.arg(viewport_bucket) AND state_key = sqlc.arg(state_key)
ORDER BY last_seen_at DESC
LIMIT 1;

-- name: LensCells :many
SELECT cell_x, cell_y, count FROM lens_heatmap_cells
WHERE channel_id = sqlc.arg(channel_id) AND page_key = sqlc.arg(page_key)
  AND viewport_bucket = sqlc.arg(viewport_bucket) AND state_key = sqlc.arg(state_key) AND kind = sqlc.arg(kind)
ORDER BY count DESC;

-- name: LensElements :many
SELECT selector, text, count FROM lens_element_clicks
WHERE channel_id = sqlc.arg(channel_id) AND page_key = sqlc.arg(page_key) AND viewport_bucket = sqlc.arg(viewport_bucket)
ORDER BY count DESC
LIMIT sqlc.arg(lim);

-- name: LensCellsFromEvents :many
-- The event-derived heatmap grid (mirrors LensCells) - increments live as raw
-- lens_cell events under the ClickHouse engine; this is the Postgres twin so
-- the seam behaves identically on both drivers.
SELECT (properties->>'x')::int AS cell_x, (properties->>'y')::int AS cell_y,
       SUM((properties->>'n')::bigint)::bigint AS count
FROM analytics_events
WHERE event_name = 'lens_cell' AND channel_id = sqlc.arg(channel_id)
  AND properties->>'page_key' = sqlc.arg(page_key)::text AND properties->>'viewport' = sqlc.arg(viewport)::text
  AND properties->>'state' = sqlc.arg(state)::text AND properties->>'kind' = sqlc.arg(kind)::text
GROUP BY 1, 2
ORDER BY count DESC;

-- name: LensElementsFromEvents :many
-- Event-derived twin of LensElements. MAX(text) is a deterministic label pick
-- where the table kept the latest - labels for one selector rarely differ.
SELECT (properties->>'selector')::text AS selector, MAX(properties->>'text')::text AS text,
       SUM((properties->>'n')::bigint)::bigint AS count
FROM analytics_events
WHERE event_name = 'lens_element' AND channel_id = sqlc.arg(channel_id)
  AND properties->>'page_key' = sqlc.arg(page_key)::text AND properties->>'viewport' = sqlc.arg(viewport)::text
GROUP BY 1
ORDER BY count DESC
LIMIT sqlc.arg(lim);

-- name: LensPageGridFromEvents :one
-- Event-derived twin of LensPageGrid.
SELECT
    COALESCE(SUM((properties->>'n')::bigint) FILTER (WHERE properties->>'kind' = 'click'), 0)::bigint                              AS clicks,
    COALESCE(SUM((properties->>'y')::bigint * (properties->>'n')::bigint) FILTER (WHERE properties->>'kind' = 'scroll'), 0)::bigint AS scroll_weight,
    COALESCE(SUM((properties->>'n')::bigint) FILTER (WHERE properties->>'kind' = 'scroll'), 0)::bigint                              AS scroll_total
FROM analytics_events
WHERE event_name = 'lens_cell' AND channel_id = sqlc.arg(channel_id) AND properties->>'page_key' = sqlc.arg(page_key)::text;

-- Stage B: retention prune ----------------------------------------------------
-- (web spine events prune through SpineStore.PruneEventsBySource, on the engine)

-- name: LensSnapshotBlobKeysBefore :many
-- Object keys for stale snapshots - deleted from object storage before the rows.
SELECT blob_key FROM lens_page_snapshots WHERE last_seen_at < sqlc.arg(before) AND blob_key IS NOT NULL;

-- name: LensPruneStaleSnapshots :execrows
DELETE FROM lens_page_snapshots WHERE last_seen_at < sqlc.arg(before);

-- name: LensSnapshotsInDB :many
-- Snapshots whose bytes are still inline (candidates to externalize).
SELECT channel_id, page_key, viewport_bucket, state_key, layout_hash, html FROM lens_page_snapshots
WHERE blob_key IS NULL AND html IS NOT NULL
ORDER BY channel_id LIMIT sqlc.arg(lim);

-- name: LensSnapshotsInObject :many
SELECT channel_id, page_key, viewport_bucket, state_key, layout_hash, blob_key FROM lens_page_snapshots
WHERE blob_key IS NOT NULL
ORDER BY channel_id LIMIT sqlc.arg(lim);

-- name: LensSetSnapshotObject :exec
UPDATE lens_page_snapshots SET blob_key = sqlc.arg(blob_key), html = NULL
WHERE channel_id = sqlc.arg(channel_id) AND page_key = sqlc.arg(page_key)
  AND viewport_bucket = sqlc.arg(viewport_bucket) AND state_key = sqlc.arg(state_key) AND layout_hash = sqlc.arg(layout_hash);

-- name: LensSetSnapshotBytes :exec
UPDATE lens_page_snapshots SET html = sqlc.arg(html), blob_key = NULL
WHERE channel_id = sqlc.arg(channel_id) AND page_key = sqlc.arg(page_key)
  AND viewport_bucket = sqlc.arg(viewport_bucket) AND state_key = sqlc.arg(state_key) AND layout_hash = sqlc.arg(layout_hash);

-- name: LensSnapshotStorageCounts :one
SELECT
    COUNT(*) FILTER (WHERE blob_key IS NULL)::bigint     AS in_db,
    COUNT(*) FILTER (WHERE blob_key IS NOT NULL)::bigint AS in_object
FROM lens_page_snapshots;

-- name: LensPruneStaleCells :execrows
DELETE FROM lens_heatmap_cells WHERE updated_at < sqlc.arg(before);

-- name: LensPruneStaleElements :execrows
DELETE FROM lens_element_clicks WHERE updated_at < sqlc.arg(before);

-- --- Session replay (rrweb) ----------------------------------------------

-- name: LensUpsertSession :exec
-- Roll a flush's metadata onto the session row. First chunk inserts; later
-- chunks bump last_event_at + event_count + exit_page and fill customer_id once
-- the visitor is known. started_at stays the earliest seen.
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
    exit_page     = EXCLUDED.exit_page;

-- name: LensBumpSessionSignal :exec
-- Increment the matching per-session UX-signal counter (kind = rage|dead|error)
-- when a signal custom event carries this recording's sessionId.
UPDATE lens_sessions SET
    rage_count  = rage_count  + (CASE WHEN sqlc.arg(kind)::text = 'rage'  THEN 1 ELSE 0 END),
    dead_count  = dead_count  + (CASE WHEN sqlc.arg(kind)::text = 'dead'  THEN 1 ELSE 0 END),
    error_count = error_count + (CASE WHEN sqlc.arg(kind)::text = 'error' THEN 1 ELSE 0 END)
WHERE id = sqlc.arg(id);

-- name: LensInsertChunk :exec
-- Exactly one of events / blob_key is set: events for in-db storage, blob_key
-- when the gzip bytes live in object storage.
INSERT INTO lens_session_chunks (session_id, seq, events, blob_key, event_count, first_ts, last_ts)
VALUES (sqlc.arg(session_id), sqlc.arg(seq), sqlc.narg(events), sqlc.narg(blob_key), sqlc.arg(event_count), sqlc.arg(first_ts), sqlc.arg(last_ts))
ON CONFLICT (session_id, seq) DO NOTHING;

-- name: LensSessionsForCustomer :many
-- Every session for a customer: those resolved to the customer id PLUS sessions
-- recorded under any anon id they later identified with (pre-login coverage).
SELECT id, anon_id, customer_id, started_at, last_event_at, event_count, entry_page, exit_page, viewport, user_agent, rage_count, error_count
FROM lens_sessions
WHERE customer_id = sqlc.arg(customer_id) OR anon_id = ANY(sqlc.arg(anon_ids)::text[])
ORDER BY started_at DESC
LIMIT sqlc.arg(lim);

-- name: LensSessionsForAnon :many
SELECT id, anon_id, customer_id, started_at, last_event_at, event_count, entry_page, exit_page, viewport, user_agent, rage_count, error_count
FROM lens_sessions
WHERE anon_id = sqlc.arg(anon_id)
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
-- Compressed event chunks for a session, in replay order. Either events (in-db)
-- or blob_key (object storage) is set per row.
SELECT events, blob_key FROM lens_session_chunks WHERE session_id = sqlc.arg(session_id) ORDER BY seq;

-- name: LensSessionBlobKeysBefore :many
-- Object-storage keys for chunks of sessions past the cutoff - fetched so the
-- prune job can delete the objects before the rows (which cascade) go.
SELECT c.blob_key FROM lens_session_chunks c
JOIN lens_sessions s ON s.id = c.session_id
WHERE s.last_event_at < sqlc.arg(before) AND c.blob_key IS NOT NULL;

-- name: LensPruneSessions :execrows
DELETE FROM lens_sessions WHERE last_event_at < sqlc.arg(before);

-- --- Blob-storage migration (skelet lens migrate-blobs) ---

-- name: LensChunksInDB :many
-- Chunks whose bytes are still inline (candidates to push to object storage).
SELECT session_id, seq, events FROM lens_session_chunks
WHERE blob_key IS NULL AND events IS NOT NULL
ORDER BY session_id, seq LIMIT sqlc.arg(lim);

-- name: LensChunksInObject :many
-- Chunks externalized to object storage (candidates to pull back inline).
SELECT session_id, seq, blob_key FROM lens_session_chunks
WHERE blob_key IS NOT NULL
ORDER BY session_id, seq LIMIT sqlc.arg(lim);

-- name: LensSetChunkObject :exec
UPDATE lens_session_chunks SET blob_key = sqlc.arg(blob_key), events = NULL
WHERE session_id = sqlc.arg(session_id) AND seq = sqlc.arg(seq);

-- name: LensSetChunkBytes :exec
UPDATE lens_session_chunks SET events = sqlc.arg(events), blob_key = NULL
WHERE session_id = sqlc.arg(session_id) AND seq = sqlc.arg(seq);

-- name: LensChunkStorageCounts :one
SELECT
    COUNT(*) FILTER (WHERE blob_key IS NULL)::bigint     AS in_db,
    COUNT(*) FILTER (WHERE blob_key IS NOT NULL)::bigint AS in_object
FROM lens_session_chunks;

-- --- UX signals: reads (frustration / errors / web vitals) ---

-- name: LensTopSignalPages :many
-- Top pages by a signal dim (rage | dead | error), rollup-backed (lens_traffic_daily).
SELECT key AS page_key, SUM(views)::bigint AS count
FROM lens_traffic_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND dim = sqlc.arg(dim)
GROUP BY key
ORDER BY count DESC
LIMIT sqlc.arg(lim);

-- name: LensTopRageElements :many
-- Most rage-clicked elements (selector + latest label), from the raw spine -
-- rage is low-volume so a bounded scan is fine.
SELECT COALESCE(e.properties->>'selector', '') AS selector,
       COALESCE(MAX(e.properties->>'text'), '') AS text,
       COUNT(*)::bigint AS count
FROM analytics_events e
WHERE e.channel_id = sqlc.arg(channel_id) AND e.event_name = 'custom'
  AND e.properties->>'name' = '$rage_click' AND e.occurred_at >= sqlc.arg(since)
  AND COALESCE(e.properties->>'selector', '') <> ''
GROUP BY e.properties->>'selector'
ORDER BY count DESC
LIMIT sqlc.arg(lim);

-- name: LensTopErrors :many
-- JS errors grouped by message: count, distinct pages, last seen. Raw spine
-- (errors are low-volume; rollup only tracks per-page counts).
SELECT COALESCE(e.properties->>'message', '') AS message,
       COUNT(*)::bigint AS count,
       COUNT(DISTINCT e.properties->>'page_key')::bigint AS pages,
       MAX(e.occurred_at) AS last_seen
FROM analytics_events e
WHERE e.channel_id = sqlc.arg(channel_id) AND e.event_name = 'custom'
  AND e.properties->>'name' = '$js_error' AND e.occurred_at >= sqlc.arg(since)
GROUP BY e.properties->>'message'
ORDER BY count DESC
LIMIT sqlc.arg(lim);

-- name: LensVitalsByPage :many
-- p75 of each web-vital metric per page, from the raw spine.
SELECT COALESCE(e.properties->>'page_key', '') AS page_key,
       COALESCE(e.properties->>'metric', '')   AS metric,
       percentile_cont(0.75) WITHIN GROUP (ORDER BY (e.properties->>'value')::float8) AS p75,
       COUNT(*)::bigint AS samples
FROM analytics_events e
WHERE e.channel_id = sqlc.arg(channel_id) AND e.event_name = 'custom'
  AND e.properties->>'name' = '$web_vitals' AND e.occurred_at >= sqlc.arg(since)
  AND (e.properties->>'value') ~ '^-?[0-9.]+$'
GROUP BY e.properties->>'page_key', e.properties->>'metric'
ORDER BY page_key, metric;

-- name: LensSessionSignals :many
-- This recording's JS errors + web vitals, matched by sessionId in props and
-- bounded to the session's channel + time window (so it doesn't scan the spine).
SELECT
    COALESCE(e.properties->>'name', '')    AS name,
    COALESCE(e.properties->>'message', '') AS message,
    COALESCE(e.properties->>'metric', '')  AS metric,
    COALESCE(e.properties->>'value', '')   AS value,
    e.occurred_at                          AS occurred_at
FROM analytics_events e
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.event_name = 'custom'
  AND e.occurred_at >= sqlc.arg(from_ts) AND e.occurred_at <= sqlc.arg(to_ts)
  AND e.properties->>'sessionId' = sqlc.arg(session_id)::text
  AND e.properties->>'name' IN ('$js_error', '$web_vitals')
ORDER BY e.occurred_at;

-- name: LensVitalsOverall :many
-- Site-wide p75 per web-vital metric (not grouped by page) for the Lens-home
-- Core Web Vitals summary.
SELECT COALESCE(e.properties->>'metric', '') AS metric,
       percentile_cont(0.75) WITHIN GROUP (ORDER BY (e.properties->>'value')::float8) AS p75,
       COUNT(*)::bigint AS samples
FROM analytics_events e
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.event_name = 'custom'
  AND e.properties->>'name' = '$web_vitals'
  AND e.occurred_at >= sqlc.arg(since)
  AND (e.properties->>'value') ~ '^-?[0-9.]+$'
GROUP BY e.properties->>'metric';

-- On-site search analytics (rollup-backed; dim='search' total volume per term, dim='search_zero' zero-result volume per
-- term). Same rollup mechanism as the frustration/error signals, so reads stay bounded regardless of search volume

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
