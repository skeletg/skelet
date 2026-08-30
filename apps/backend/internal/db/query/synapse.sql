-- Synapse commerce-side reads: the segment composition + membership queries that join commerce tables (customer_segment_memberships /
-- customer_traits / orders / customers) - the spine aggregates moved to the signals db package (signals/query/synapse.sql)

-- name: SegmentMemberStats :one
-- Commerce rollup over a segment's members: distinct non-cancelled orders + total
-- spend (cents) - AOV is derived in the resolver - currency is treated as the
-- channel's single currency (mixed-currency segments sum naively)
SELECT
    COUNT(DISTINCT o.id)::bigint                AS orders,
    COALESCE(SUM(o.total_amount), 0)::bigint    AS revenue_cents
FROM customer_segment_memberships m
JOIN orders o ON o.customer_id = m.customer_id AND o.status NOT IN ('cancelled', 'draft')
WHERE m.segment_id = sqlc.arg(segment_id);

-- name: SegmentMembers :many
-- A segment's members with their traits + commerce (spend / orders / last order),
-- richest-spender first - capped by `lim`; names/emails resolved in the resolver
SELECT
    m.customer_id                                  AS customer_id,
    COALESCE(t.lifecycle_stage, '')                AS lifecycle,
    COALESCE(t.recency_days, -1)::int              AS recency_days,
    COALESCE(t.engagement_score, 0)::int           AS engagement_score,
    COUNT(o.id)::int                               AS orders,
    COALESCE(SUM(o.total_amount), 0)::bigint       AS spend_cents,
    MAX(o.created_at)                              AS last_order_at
FROM customer_segment_memberships m
LEFT JOIN customer_traits t ON t.customer_id = m.customer_id
LEFT JOIN orders o ON o.customer_id = m.customer_id AND o.status NOT IN ('cancelled', 'draft')
WHERE m.segment_id = sqlc.arg(segment_id)
GROUP BY m.customer_id, t.lifecycle_stage, t.recency_days, t.engagement_score
ORDER BY spend_cents DESC, m.customer_id
LIMIT sqlc.arg(lim);

-- name: SegmentLifecycleMix :many
-- Members of a segment grouped by lifecycle stage ('' = traits not computed yet)
SELECT COALESCE(t.lifecycle_stage, '')::text AS stage, COUNT(*)::int AS count
FROM customer_segment_memberships m
LEFT JOIN customer_traits t ON t.customer_id = m.customer_id
WHERE m.segment_id = sqlc.arg(segment_id)
GROUP BY COALESCE(t.lifecycle_stage, '')
ORDER BY count DESC;

-- name: SegmentRfmDistribution :many
-- Distribution of members across each RFM dimension's 1–5 score (0 = never
-- ordered), unpivoted to (dimension, score, count) - members without traits drop out
SELECT x.dimension::text AS dimension, x.score::int AS score, COUNT(*)::int AS count
FROM (
    SELECT 'recency'::text AS dimension, t.rfm_r AS score
    FROM customer_segment_memberships m JOIN customer_traits t ON t.customer_id = m.customer_id
    WHERE m.segment_id = sqlc.arg(segment_id)
    UNION ALL
    SELECT 'frequency'::text, t.rfm_f
    FROM customer_segment_memberships m JOIN customer_traits t ON t.customer_id = m.customer_id
    WHERE m.segment_id = sqlc.arg(segment_id)
    UNION ALL
    SELECT 'monetary'::text, t.rfm_m
    FROM customer_segment_memberships m JOIN customer_traits t ON t.customer_id = m.customer_id
    WHERE m.segment_id = sqlc.arg(segment_id)
) x
GROUP BY x.dimension, x.score
ORDER BY x.dimension, x.score;

-- name: SegmentGrowth :many
-- Daily member-count series for a segment over the last `days` days (from the
-- snapshot table) - accrues forward from when the segment first materialised
SELECT day::timestamptz AS day, member_count
FROM segment_membership_daily
WHERE segment_id = sqlc.arg(segment_id) AND day >= CURRENT_DATE - sqlc.arg(days)::int
ORDER BY day;

-- name: SegmentMemberIDs :many
-- The customer ids in a segment - the commerce half of the app-side
-- SegmentTopAffinities read (the affinity spine scan runs in the signals db,
-- passed these ids). analytics_events moved out of this db, so the join can't
SELECT customer_id FROM customer_segment_memberships WHERE segment_id = sqlc.arg(segment_id);

-- name: ChannelCustomerIDs :many
-- Every customer id in a channel - the universe a behavioral segment draws from
-- Used to evaluate negated / OR'd nodes in a segment definition tree (NOT X =
-- universe minus X)
SELECT id FROM customers WHERE channel_id = sqlc.arg(channel_id);
