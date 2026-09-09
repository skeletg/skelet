-- Spine half of trait compute (the `act` CTE), split out of the commerce RecomputeChannelTraits - feeds the
-- commerce-side customer_engagement_30d projection the recompute reads

-- name: ChannelEngagementCounts :many
-- Per-customer last-window spine event count for a channel, capped at 100 (any
-- source). `since` is as_of - 30 days
SELECT customer_id, LEAST(COUNT(*), 100)::int AS engagement
FROM analytics_events
WHERE channel_id = sqlc.arg(channel_id)
  AND customer_id IS NOT NULL
  AND occurred_at >= sqlc.arg(since)
GROUP BY customer_id;

-- name: ActiveSpineCustomerIDs :many
-- Customers with any spine event in [from, to) - the incremental engagement
-- refresh unions the aged-out and fresh slices to find who needs recomputing.
SELECT DISTINCT customer_id::uuid AS customer_id
FROM analytics_events
WHERE channel_id = sqlc.arg(channel_id)
  AND customer_id IS NOT NULL
  AND occurred_at >= sqlc.arg(from_at)
  AND occurred_at < sqlc.arg(to_at);

-- name: IngestedSpineCustomerIDs :many
-- Customers with events RECEIVED in [from, to) - the incremental refresh's fresh
-- slice keys on arrival time, so a queue-backlog drain or an outbox redelivery
-- whose occurred_at predates the window still gets its customer recomputed.
SELECT DISTINCT customer_id::uuid AS customer_id
FROM analytics_events
WHERE channel_id = sqlc.arg(channel_id)
  AND customer_id IS NOT NULL
  AND received_at >= sqlc.arg(from_at)
  AND received_at < sqlc.arg(to_at);

-- name: ChannelEngagementCountsFor :many
-- ChannelEngagementCounts scoped to a bounded id set - the incremental refresh
-- recomputes only the changed customers.
SELECT customer_id, LEAST(COUNT(*), 100)::int AS engagement
FROM analytics_events
WHERE channel_id = sqlc.arg(channel_id)
  AND customer_id = ANY(sqlc.arg(customer_ids)::uuid[])
  AND occurred_at >= sqlc.arg(since)
GROUP BY customer_id;

-- name: ChannelOrderAggregates :many
-- Lifetime order history per customer, from the spine rather than the orders table - the RFM inputs
-- An order that was later cancelled is excluded entirely, which is what the commerce query's
-- status filter did at read time and an append-only log has to do by netting instead
-- Guest orders carry no customer_id and are skipped, matching the commerce join
SELECT p.customer_id::uuid                                  AS customer_id,
       COUNT(*)::int                                        AS order_count,
       COALESCE(SUM((p.properties->>'total_cents')::bigint), 0)::bigint AS monetary_cents,
       MIN(p.occurred_at)::timestamptz                      AS first_order_at,
       MAX(p.occurred_at)::timestamptz                      AS last_order_at
FROM analytics_events p
WHERE p.channel_id = sqlc.arg(channel_id)
  AND p.event_name = 'order_placed'
  AND p.customer_id IS NOT NULL
  AND p.properties ? 'order_id'
  AND NOT EXISTS (
      SELECT 1 FROM analytics_events c
      WHERE c.channel_id = p.channel_id
        AND c.event_name = 'order_cancelled'
        AND c.properties->>'order_id' = p.properties->>'order_id'
  )
GROUP BY p.customer_id;
