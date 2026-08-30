-- Spine queries that touch ONLY analytics_events (the moving table). Queries that join commerce tables (email_sends attribution,
-- MostRecentEmailEngagement) stay in the commerce db package - they become app-side/cross-db reads at cutover

-- name: RecordAnalyticsEvent :execrows
-- Append-only; ON CONFLICT makes the dedupe_key the idempotency authority
-- (returns 1 when a genuinely new event was logged, 0 on a duplicate).
INSERT INTO analytics_events (
    event_name, occurred_at, channel_id, customer_id, anon_id, source, send_id, properties, dedupe_key
) VALUES (
    sqlc.arg(event_name), sqlc.arg(occurred_at), sqlc.narg(channel_id), sqlc.narg(customer_id),
    sqlc.narg(anon_id), sqlc.arg(source), sqlc.narg(send_id), sqlc.arg(properties), sqlc.arg(dedupe_key)
)
ON CONFLICT (dedupe_key) DO NOTHING;

-- name: RecordAnalyticsEventsBatch :execrows
-- Batch twin of RecordAnalyticsEvent - the queue consumer lands one fetch as one
-- statement instead of one insert per event. Zero uuids / empty strings mark the
-- nullable columns (mirroring the single insert's narg mapping); duplicate keys
-- inside one batch collapse the same way concurrent inserts do.
INSERT INTO analytics_events (
    event_name, occurred_at, channel_id, customer_id, anon_id, source, send_id, properties, dedupe_key
)
SELECT unnest(sqlc.arg(event_names)::text[]),
       unnest(sqlc.arg(occurred_ats)::timestamptz[]),
       NULLIF(unnest(sqlc.arg(channel_ids)::uuid[]), '00000000-0000-0000-0000-000000000000'),
       NULLIF(unnest(sqlc.arg(customer_ids)::uuid[]), '00000000-0000-0000-0000-000000000000'),
       NULLIF(unnest(sqlc.arg(anon_ids)::text[]), ''),
       unnest(sqlc.arg(sources)::text[]),
       NULLIF(unnest(sqlc.arg(send_ids)::uuid[]), '00000000-0000-0000-0000-000000000000'),
       unnest(sqlc.arg(properties)::text[])::jsonb,
       unnest(sqlc.arg(dedupe_keys)::text[])
ON CONFLICT (dedupe_key) DO NOTHING;

-- name: EngagementSummary :one
-- Channel-scoped open/click/conversion counts since a cutoff. Opens/clicks are
-- split raw vs human (machine-flagged excluded) for honest rates.
SELECT
    COUNT(*) FILTER (WHERE event_name = 'email_opened')                                          AS opens_raw,
    COUNT(*) FILTER (WHERE event_name = 'email_opened'  AND COALESCE(properties->>'machine','false') = 'false') AS opens_human,
    COUNT(*) FILTER (WHERE event_name = 'email_clicked' AND COALESCE(properties->>'machine','false') = 'false') AS clicks,
    COUNT(*) FILTER (WHERE event_name = 'conversion')                                            AS conversions
FROM analytics_events
WHERE channel_id = sqlc.arg(channel_id) AND occurred_at >= sqlc.arg(since);

-- name: AttributedRevenue :many
-- Revenue attributed to marketing email, grouped by currency.
SELECT
    COALESCE(properties->>'currency', '')                            AS currency,
    COALESCE(SUM((properties->>'revenue_cents')::bigint), 0)::bigint AS cents,
    COUNT(*)                                                         AS orders
FROM analytics_events
WHERE channel_id = sqlc.arg(channel_id) AND event_name = 'conversion' AND occurred_at >= sqlc.arg(since)
GROUP BY properties->>'currency'
ORDER BY cents DESC;

-- name: TopClickedLinks :many
SELECT COALESCE(properties->>'url', '') AS url, COUNT(*) AS clicks
FROM analytics_events
WHERE channel_id = sqlc.arg(channel_id) AND event_name = 'email_clicked'
  AND COALESCE(properties->>'machine','false') = 'false' AND occurred_at >= sqlc.arg(since)
GROUP BY properties->>'url'
ORDER BY clicks DESC
LIMIT 10;

-- name: SpineStats :one
-- Health panel: is ingest flowing, and is retention actually running? newest
-- answers the first, oldest the second (it should sit near the retention
-- window - drifting past it means the prune isn't happening).
-- Keyed on received_at because that's the indexed column AND the axis the
-- Postgres prune deletes on, so both reads are index-only scans. No row count
-- here on purpose: COUNT(*) would full-scan the largest table in the system to
-- render a status widget, and size isn't what the panel is asking.
-- plain aggregate, uncast: sqlc types a bare MIN/MAX as nullable (an empty spine
-- has neither), and Postgres still answers both from the index
SELECT MIN(received_at) AS oldest, MAX(received_at) AS newest FROM analytics_events;

-- name: PruneAnalyticsEvents :execrows
-- Retention sweep: drop spine events received before the cutoff.
DELETE FROM analytics_events WHERE received_at < sqlc.arg(before);

-- name: PruneAnalyticsEventsBySource :execrows
-- Source-scoped retention (Lens prunes web events on a shorter window than the
-- global spine sweep).
DELETE FROM analytics_events WHERE source = sqlc.arg(source) AND received_at < sqlc.arg(before);

-- name: OldestAnalyticsEventBefore :one
-- The archive sweep's day finder: the oldest aged event (optionally source-
-- scoped, '' = any) tells it which received_at day to export next, so it never
-- iterates empty days. Index-ordered scan on analytics_events_received.
SELECT received_at FROM analytics_events
WHERE received_at < sqlc.arg(before)
  AND (sqlc.arg(source)::text = '' OR source = sqlc.arg(source))
ORDER BY received_at
LIMIT 1;

-- name: ListAnalyticsEventsForArchive :many
-- One keyset page of a day slice for the archive export. Ordered on
-- (received_at, id) so the received_at index feeds an incremental sort;
-- customer_id / anon_id / dedupe_key are deliberately not selected - archives
-- are identity-stripped (dedupe keys can embed anon/customer ids, e.g.
-- identify's).
SELECT id, event_name, occurred_at, received_at, channel_id, source, send_id, properties
FROM analytics_events
WHERE received_at >= sqlc.arg(from_at) AND received_at < sqlc.arg(to_at)
  AND (sqlc.arg(source)::text = '' OR source = sqlc.arg(source))
  AND (received_at > sqlc.arg(cursor_at) OR (received_at = sqlc.arg(cursor_at) AND id > sqlc.arg(cursor_id)))
ORDER BY received_at, id
LIMIT sqlc.arg(lim);

-- name: PruneAnalyticsEventsRange :execrows
-- Deletes exactly one exported day slice - the archive sweep's delete half,
-- so a failure between days never drops anything that wasn't uploaded first.
DELETE FROM analytics_events
WHERE received_at >= sqlc.arg(from_at) AND received_at < sqlc.arg(to_at)
  AND (sqlc.arg(source)::text = '' OR source = sqlc.arg(source));
