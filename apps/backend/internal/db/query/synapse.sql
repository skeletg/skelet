-- Synapse's commerce-side reads - the membership and order halves of the segment panels, whose traits half lives in signals/query/traits.sql and is joined in the caller

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

-- name: SegmentMemberOrderStats :many
-- The commerce half of a segment's member panel - traits are read separately from the signals
-- database and joined in the caller, since the two no longer share one
SELECT o.customer_id::uuid AS customer_id,
       COUNT(o.id)::int AS orders,
       COALESCE(SUM(o.total_amount), 0)::bigint AS spend_cents,
       MAX(o.created_at)::timestamptz AS last_order_at
FROM orders o
WHERE o.customer_id = ANY(sqlc.arg(customer_ids)::uuid[])
  AND o.status NOT IN ('cancelled', 'draft')
GROUP BY o.customer_id;
