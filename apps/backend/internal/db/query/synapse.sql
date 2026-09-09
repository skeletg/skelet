-- Synapse's commerce-side reads - the order half of the segment panels, whose membership and traits halves live in the signals database and are joined in the caller

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

-- name: SegmentOrderRollup :one
-- Distinct non-cancelled orders and total spend for a known set of customers - the commerce half of a segment's rollup, since membership moved and the join cannot span both
SELECT
    COUNT(DISTINCT o.id)::bigint             AS orders,
    COALESCE(SUM(o.total_amount), 0)::bigint AS revenue_cents
FROM orders o
WHERE o.customer_id = ANY(sqlc.arg(customer_ids)::uuid[])
  AND o.status NOT IN ('cancelled', 'draft');
