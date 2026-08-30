-- Identity stitching + per-customer profile reads (moved from the commerce db
-- package at cutover 4b). analytics_identities lives in the signals db alongside
-- the spine. The profile reads take the customer's pre-resolved anon ids as a
-- param (AnonIDsForCustomer runs first), so they touch only analytics_events -
-- no inline identity join.

-- name: UpsertIdentity :exec
INSERT INTO analytics_identities (anon_id, customer_id, channel_id)
VALUES (sqlc.arg(anon_id), sqlc.arg(customer_id), sqlc.narg(channel_id))
ON CONFLICT (anon_id) DO UPDATE
    SET customer_id = excluded.customer_id,
        channel_id  = COALESCE(excluded.channel_id, analytics_identities.channel_id),
        last_seen   = NOW();

-- name: AnonIDsForCustomer :many
SELECT anon_id FROM analytics_identities WHERE customer_id = sqlc.arg(customer_id);

-- name: CustomersForAnonIDs :many
-- Bulk anon -> customer resolution for batch scans (browse abandonment) - kept
-- engine-side so the ClickHouse driver answers from its own identities.
SELECT anon_id, customer_id FROM analytics_identities WHERE anon_id = ANY(sqlc.arg(anon_ids)::text[]);

-- name: AnonIDsForCustomerSet :many
-- Batch resolve for the segment-affinity pre-filter (rides the customer_id index).
SELECT anon_id FROM analytics_identities WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[]);

-- name: PruneAnalyticsIdentities :execrows
-- Retention: a stitch shouldn't outlive its last confirmation (identify bumps
-- last_seen on every tracker mount while logged in) by more than the spine
-- retention window - past it, the mapping resolves nothing the spine still
-- holds. Deliberately unindexed: last_seen mutates on every identify, an index
-- would cost the upsert hot path daily, and this sweep is what keeps the table
-- small enough to scan.
DELETE FROM analytics_identities WHERE last_seen < sqlc.arg(before);

-- name: EnqueueIdentityErasure :exec
-- ClickHouse-engine erasures batch through this queue (one mutation per sweep,
-- not per request) - the upsert makes re-requesting free.
INSERT INTO gdpr_identity_erasures (customer_id) VALUES (sqlc.arg(customer_id))
ON CONFLICT (customer_id) DO UPDATE SET requested_at = now();

-- name: QueuedIdentityErasures :many
SELECT customer_id FROM gdpr_identity_erasures ORDER BY requested_at LIMIT sqlc.arg(lim);

-- name: DeleteIdentityErasures :exec
-- Only after the batched spine delete landed - a failed sweep retries these.
DELETE FROM gdpr_identity_erasures WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[]);

-- name: DeleteCustomerAnalyticsIdentities :execrows
-- GDPR erasure: drop the customers' anon-id mappings. A separate signals-handle
-- call, since the mapping table lives here rather than in commerce, so it is not
-- atomic with the commerce scrub.
DELETE FROM analytics_identities WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[]);

-- name: CustomerOverview :one
SELECT
    COUNT(*)                   AS total,
    MIN(occurred_at)           AS first_at,
    MAX(occurred_at)           AS last_at
FROM analytics_events
WHERE (customer_id = sqlc.arg(customer_id) OR anon_id = ANY(sqlc.arg(anon_ids)::text[]))
  AND occurred_at >= sqlc.arg(since);

-- name: CustomerEventCounts :many
SELECT event_name, COUNT(*) AS count
FROM analytics_events
WHERE (customer_id = sqlc.arg(customer_id) OR anon_id = ANY(sqlc.arg(anon_ids)::text[]))
  AND occurred_at >= sqlc.arg(since)
GROUP BY event_name
ORDER BY count DESC;

-- name: CustomerTimeline :many
SELECT event_name, occurred_at, source, properties
FROM analytics_events
WHERE (customer_id = sqlc.arg(customer_id) OR anon_id = ANY(sqlc.arg(anon_ids)::text[]))
  AND occurred_at >= sqlc.arg(since)
ORDER BY occurred_at DESC
LIMIT sqlc.arg(lim);

-- name: ProfileTopValues :many
-- Top distinct values of a single event-property key for a profile (a customer
-- and/or their anon ids) since the cutoff - powers the visitor "context" panel
-- (device / browser / referrer / ip). The property key is a parameter so one
-- query serves every dimension.
SELECT v AS value, COUNT(*) AS count
FROM (
    SELECT properties->>sqlc.arg(prop_key)::text AS v
    FROM analytics_events
    WHERE (customer_id = sqlc.arg(customer_id) OR anon_id = ANY(sqlc.arg(anon_ids)::text[]))
      AND occurred_at >= sqlc.arg(since)
) t
WHERE v IS NOT NULL AND v <> ''
GROUP BY v
ORDER BY count DESC
LIMIT 10;

-- name: ProfileSessionCount :one
-- Number of distinct visits/sessions for a profile, derived by a >30-minute gap
-- between consecutive events (the spine has no stored session id).
SELECT COUNT(*) AS sessions
FROM (
    SELECT CASE
        WHEN LAG(occurred_at) OVER (ORDER BY occurred_at) IS NULL
          OR occurred_at - LAG(occurred_at) OVER (ORDER BY occurred_at) > interval '30 minutes'
        THEN 1 ELSE 0 END AS new_session
    FROM analytics_events
    WHERE (customer_id = sqlc.arg(customer_id) OR anon_id = ANY(sqlc.arg(anon_ids)::text[]))
      AND occurred_at >= sqlc.arg(since)
) t
WHERE t.new_session = 1;

-- name: CustomerConversions :many
SELECT
    COALESCE(properties->>'currency', '')                            AS currency,
    COUNT(*)                                                         AS orders,
    COALESCE(SUM((properties->>'revenue_cents')::bigint), 0)::bigint AS cents
FROM analytics_events
WHERE event_name = 'conversion'
  AND (customer_id = sqlc.arg(customer_id) OR anon_id = ANY(sqlc.arg(anon_ids)::text[]))
  AND occurred_at >= sqlc.arg(since)
GROUP BY properties->>'currency';

-- name: ProfileTopProducts :many
-- Products a profile viewed, richest-first, with the display name + image +
-- price + sku carried on the product page_view (powers visual product cards).
SELECT
    e.properties->>'entity_name'                      AS name,
    MAX(e.properties->>'entity_image')                AS image,
    MAX(e.properties->>'entity_price')                AS price,
    MAX(e.properties->>'sku')                         AS sku,
    COUNT(*)::int                                     AS views,
    MAX(e.occurred_at)                                AS last_viewed
FROM analytics_events e
WHERE (e.customer_id = sqlc.arg(customer_id) OR e.anon_id = ANY(sqlc.arg(anon_ids)::text[]))
  AND e.event_name = 'page_view'
  AND e.properties->>'entity' = 'product'
  AND e.properties->>'entity_name' IS NOT NULL AND e.properties->>'entity_name' <> ''
  AND e.occurred_at >= sqlc.arg(since)
GROUP BY e.properties->>'entity_name'
ORDER BY views DESC, name
LIMIT 12;

-- name: ProfileTopPages :many
-- Pages a profile viewed, with view count + average dwell (seconds): the gap
-- from each page_view to the visitor's next event, capped at 30 min to drop
-- idle/session-boundary outliers. Page key carried for a Lens heatmap deep-link.
-- total_pages is the distinct-page count BEFORE the LIMIT (window functions run
-- over the full grouped set), so the UI can say "top 12 of N pages".
SELECT t.title AS title, t.page_key AS page_key, COUNT(*)::int AS views,
       COALESCE(AVG(CASE WHEN t.dwell BETWEEN 1 AND 1800 THEN t.dwell END), 0)::int AS avg_dwell,
       COUNT(*) OVER ()::int AS total_pages
FROM (
    SELECT e.properties->>'title' AS title, e.properties->>'page_key' AS page_key, e.event_name AS event_name,
           EXTRACT(EPOCH FROM (LEAD(e.occurred_at) OVER (PARTITION BY COALESCE(e.customer_id::text, e.anon_id) ORDER BY e.occurred_at) - e.occurred_at)) AS dwell
    FROM analytics_events e
    WHERE (e.customer_id = sqlc.arg(customer_id) OR e.anon_id = ANY(sqlc.arg(anon_ids)::text[]))
      AND e.occurred_at >= sqlc.arg(since)
) t
WHERE t.event_name = 'page_view' AND t.title IS NOT NULL AND t.title <> ''
GROUP BY t.title, t.page_key
ORDER BY views DESC, t.title
LIMIT 12;

-- name: CustomerTopAffinities :many
-- The single most-frequent value of each attr:<slug> affinity prop for a profile
-- (customer + their anon ids) since the cutoff - one row per attribute. Powers
-- the {{customer.affinity.<slug>}} email/SMS merge tag.
SELECT t.attr_key AS attr_key, t.attr_val AS attr_val
FROM (
    SELECT kv.k AS attr_key, kv.v AS attr_val,
           ROW_NUMBER() OVER (PARTITION BY kv.k ORDER BY COUNT(*) DESC, kv.v) AS rn
    FROM analytics_events e, LATERAL jsonb_each_text(e.properties) AS kv(k, v)
    WHERE (e.customer_id = sqlc.arg(customer_id) OR e.anon_id = ANY(sqlc.arg(anon_ids)::text[]))
      AND e.occurred_at >= sqlc.arg(since)
      AND kv.k LIKE 'attr:%'
    GROUP BY kv.k, kv.v
) t
WHERE t.rn = 1;
