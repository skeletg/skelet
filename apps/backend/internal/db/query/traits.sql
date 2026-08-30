-- name: RecomputeChannelTraits :execrows
-- Recompute every customer's RFM / lifecycle / engagement traits for a channel
-- in ONE set-based statement - no per-customer round-trips - so the trait-compute
-- job scales to a large customer base - reproduces the former in-Go logic exactly:
--   * RFM are 1..5 quintile RANKS among the channel's *purchasers* (5 = best),
--     0 for never-ordered - bucket = floor((rank-1) * 5 / purchasers) + 1 over the
--     value sorted ascending; recency is reversed (fewer days = better) - the
--     customer_id tiebreaker makes ties deterministic (the Go sort was not)
--   * recency_days = whole days since the last non-cancelled/draft order (-1 = never)
--   * lifecycle from recency + order count, using the channel's configured windows
--     (new_days / active_days / at_risk_days) so it matches the catalog's repurchase
--     cadence (a tire shop's "active" is far longer than a consumables store's),
--     NARROWED per customer by the shortest-window override segment they belong to
--     (seg_override) - e.g. an "All-terrain" affinity segment with a faster cadence
--   * engagement = last-30d spine events, ≤100
-- as_of is the run clock (recency + the 30-day engagement window derive from it)
WITH agg AS (
    SELECT c.id                                     AS customer_id,
           COUNT(o.id)::int                         AS order_count,
           COALESCE(SUM(o.total_amount), 0)::bigint AS monetary_cents,
           MAX(o.created_at)                        AS last_order_at,
           MIN(o.created_at)                        AS first_order_at
    FROM customers c
    LEFT JOIN orders o
        ON o.customer_id = c.id
       AND o.status NOT IN ('cancelled', 'draft')
    WHERE c.channel_id = sqlc.arg(channel_id)::uuid
    GROUP BY c.id
),
act AS (
    -- Reverse-read projection, refreshed from the signals spine by the trait-compute job right before this recompute
    -- (the spine moved out of commerce, so the count can't be an inline analytics_events CTE anymore)
    SELECT customer_id, engagement
    FROM customer_engagement_30d
    WHERE channel_id = sqlc.arg(channel_id)::uuid
),
base AS (
    SELECT a.customer_id, a.order_count, a.monetary_cents,
           CASE WHEN a.last_order_at IS NULL THEN -1
                ELSE GREATEST(0, floor(EXTRACT(EPOCH FROM (sqlc.arg(as_of)::timestamptz - a.last_order_at)) / 86400))::int
           END                       AS recency_days,
           -- Mean days between orders (own cadence) - NULL until the
           -- second order - a single purchase has no cadence yet
           CASE WHEN a.order_count >= 2
                THEN GREATEST(1.0, EXTRACT(EPOCH FROM (a.last_order_at - a.first_order_at)) / 86400.0 / (a.order_count - 1))
           END                       AS avg_gap_days,
           COALESCE(e.engagement, 0) AS engagement
    FROM agg a
    LEFT JOIN act e ON e.customer_id = a.customer_id
),
predict AS (
    -- churn_risk: how far past their own cadence the customer is
    --   repeaters: 35 x (recency / cadence), clamped 5..95
    --   single-purchase: window-based coarse risk (unproven loyalty)
    --   prospects: 0 (nothing to churn from)
    -- next_order_days: cadence minus recency, floored at 0 (-1 unknown)
    -- predicted_clv_cents: AOV x expected 12-month orders x survival
    SELECT b.customer_id,
           CASE
               WHEN b.order_count = 0 THEN 0
               WHEN b.avg_gap_days IS NOT NULL THEN LEAST(95, GREATEST(5, round(35 * b.recency_days / b.avg_gap_days)))::int
               WHEN b.recency_days > sqlc.arg(at_risk_days)::int THEN 85
               WHEN b.recency_days > sqlc.arg(active_days)::int THEN 60
               ELSE 40
           END AS churn_risk,
           CASE
               WHEN b.avg_gap_days IS NULL THEN -1
               ELSE GREATEST(0, round(b.avg_gap_days - b.recency_days))::int
           END AS next_order_days,
           CASE
               WHEN b.order_count = 0 THEN 0
               WHEN b.avg_gap_days IS NOT NULL THEN GREATEST(0, round(
                   (b.monetary_cents::numeric / b.order_count)
                   * (365.0 / b.avg_gap_days)
                   * (1 - LEAST(95, GREATEST(5, round(35 * b.recency_days / b.avg_gap_days))) / 100.0)
               ))::bigint
               ELSE GREATEST(0, round(
                   b.monetary_cents::numeric
                   * (1 - CASE WHEN b.recency_days > sqlc.arg(at_risk_days)::int THEN 0.85
                               WHEN b.recency_days > sqlc.arg(active_days)::int THEN 0.60
                               ELSE 0.40 END)
               ))::bigint
           END AS predicted_clv_cents
    FROM base b
),
ranked AS (
    SELECT customer_id,
           count(*) OVER ()                                             AS np,
           row_number() OVER (ORDER BY recency_days ASC,   customer_id) AS rn_r,
           row_number() OVER (ORDER BY order_count ASC,    customer_id) AS rn_f,
           row_number() OVER (ORDER BY monetary_cents ASC, customer_id) AS rn_m
    FROM base
    WHERE order_count > 0
),
seg_override AS (
    -- Per-customer lifecycle override = the SHORTEST-window override segment the
    -- customer belongs to (most aggressive cadence wins → re-engage sooner; e.g. an
    -- "All-terrain" affinity segment carrying a faster cadence) - only active segments
    -- with an override set participate; the whole triple comes from that one segment
    SELECT DISTINCT ON (m.customer_id)
           m.customer_id,
           s.lifecycle_new_days     AS new_days,
           s.lifecycle_active_days  AS active_days,
           s.lifecycle_at_risk_days AS at_risk_days
    FROM customer_segment_memberships m
    JOIN customer_segments s ON s.id = m.segment_id
    WHERE s.is_active AND s.lifecycle_at_risk_days IS NOT NULL
    ORDER BY m.customer_id, s.lifecycle_at_risk_days ASC, s.id
)
INSERT INTO customer_traits (
    customer_id, channel_id, recency_days, frequency, monetary_cents,
    rfm_r, rfm_f, rfm_m, lifecycle_stage, engagement_score,
    churn_risk, predicted_clv_cents, next_order_days, computed_at
)
SELECT b.customer_id, sqlc.arg(channel_id)::uuid, b.recency_days, b.order_count, b.monetary_cents,
       CASE WHEN r.customer_id IS NULL THEN 0 ELSE 6 - LEAST(5, ((r.rn_r - 1) * 5 / r.np)::int + 1) END AS rfm_r,
       CASE WHEN r.customer_id IS NULL THEN 0 ELSE     LEAST(5, ((r.rn_f - 1) * 5 / r.np)::int + 1) END AS rfm_f,
       CASE WHEN r.customer_id IS NULL THEN 0 ELSE     LEAST(5, ((r.rn_m - 1) * 5 / r.np)::int + 1) END AS rfm_m,
       CASE
           WHEN b.order_count = 0 THEN 'prospect'
           WHEN b.recency_days BETWEEN 0 AND COALESCE(o.new_days,     sqlc.arg(new_days)::int)     AND b.order_count = 1 THEN 'new'
           WHEN b.recency_days BETWEEN 0 AND COALESCE(o.active_days,  sqlc.arg(active_days)::int)  THEN 'active'
           WHEN b.recency_days BETWEEN 0 AND COALESCE(o.at_risk_days, sqlc.arg(at_risk_days)::int) THEN 'at_risk'
           ELSE 'lapsed'
       END AS lifecycle_stage,
       b.engagement,
       p.churn_risk, p.predicted_clv_cents, p.next_order_days, NOW()
FROM base b
LEFT JOIN ranked r ON r.customer_id = b.customer_id
LEFT JOIN predict p ON p.customer_id = b.customer_id
LEFT JOIN seg_override o ON o.customer_id = b.customer_id
ON CONFLICT (customer_id) DO UPDATE SET
    channel_id = EXCLUDED.channel_id,
    recency_days = EXCLUDED.recency_days,
    frequency = EXCLUDED.frequency,
    monetary_cents = EXCLUDED.monetary_cents,
    rfm_r = EXCLUDED.rfm_r,
    rfm_f = EXCLUDED.rfm_f,
    rfm_m = EXCLUDED.rfm_m,
    lifecycle_stage = EXCLUDED.lifecycle_stage,
    engagement_score = EXCLUDED.engagement_score,
    churn_risk = EXCLUDED.churn_risk,
    predicted_clv_cents = EXCLUDED.predicted_clv_cents,
    next_order_days = EXCLUDED.next_order_days,
    computed_at = EXCLUDED.computed_at
-- Skip no-op writes: only rewrite a row when a DECISION-relevant column actually
-- changed - this is what stops the long-tail of dormant customers (no orders /
-- activity) from churning a new tuple + WAL every run - their scores/lifecycle
-- are stable, so they're left untouched - deliberately excludes recency_days
-- (drifts ~½ day each run with no real signal) and computed_at from the gate, so
-- a row's computed_at means "last time its traits materially changed" (use the
-- trait-compute job's last_run_at for "when we last recomputed") - lifecycle
-- threshold crossings (30/90/180d) and quintile shifts DO flip a gated column, so
-- genuine transitions are still captured
WHERE customer_traits.frequency        IS DISTINCT FROM EXCLUDED.frequency
   OR customer_traits.monetary_cents    IS DISTINCT FROM EXCLUDED.monetary_cents
   OR customer_traits.rfm_r             IS DISTINCT FROM EXCLUDED.rfm_r
   OR customer_traits.rfm_f             IS DISTINCT FROM EXCLUDED.rfm_f
   OR customer_traits.rfm_m             IS DISTINCT FROM EXCLUDED.rfm_m
   OR customer_traits.lifecycle_stage   IS DISTINCT FROM EXCLUDED.lifecycle_stage
   OR customer_traits.engagement_score  IS DISTINCT FROM EXCLUDED.engagement_score
   -- Predictive fields drift daily with recency - gate on DECILE moves (churn) and half-order-of-magnitude moves (CLV)
   -- so stable dormant rows still skip the write, but meaningful shifts persist
   OR customer_traits.churn_risk / 10   IS DISTINCT FROM EXCLUDED.churn_risk / 10
   OR floor(ln(GREATEST(1, customer_traits.predicted_clv_cents)) * 2) IS DISTINCT FROM floor(ln(GREATEST(1, EXCLUDED.predicted_clv_cents)) * 2);

-- name: ClearChannelEngagement :exec
-- Wipe a channel's engagement projection - the empty-refresh case (no active
-- customers in the window)
DELETE FROM customer_engagement_30d WHERE channel_id = sqlc.arg(channel_id);

-- name: ReplaceChannelEngagement :exec
-- Atomically swap a channel's projection to the given per-customer counts
-- (paired arrays: customer_ids[i] has engagements[i]) - one statement, so a
-- crash can't leave the projection half-cleared before RecomputeChannelTraits
-- reads it - the delete and the upsert touch disjoint rows (NOT IN set vs in
-- set), which single-statement row rules require
WITH pruned AS (
    DELETE FROM customer_engagement_30d
    WHERE channel_id = sqlc.arg(channel_id)::uuid
      AND NOT (customer_id = ANY(sqlc.arg(customer_ids)::uuid[]))
)
INSERT INTO customer_engagement_30d (customer_id, channel_id, engagement)
SELECT unnest(sqlc.arg(customer_ids)::uuid[]), sqlc.arg(channel_id)::uuid, unnest(sqlc.arg(engagements)::int[])
ON CONFLICT (customer_id) DO UPDATE
    SET channel_id = EXCLUDED.channel_id, engagement = EXCLUDED.engagement, computed_at = now();

-- name: UpsertChannelEngagementFor :exec
-- Incremental-refresh upsert: only the changed customers (paired arrays), no
-- channel-wide pruning - deletions come from DeleteChannelEngagementFor
INSERT INTO customer_engagement_30d (customer_id, channel_id, engagement)
SELECT unnest(sqlc.arg(customer_ids)::uuid[]), sqlc.arg(channel_id)::uuid, unnest(sqlc.arg(engagements)::int[])
ON CONFLICT (customer_id) DO UPDATE
    SET channel_id = EXCLUDED.channel_id, engagement = EXCLUDED.engagement, computed_at = now();

-- name: DeleteChannelEngagementFor :exec
-- Customers whose 30-day window emptied since the last run
DELETE FROM customer_engagement_30d WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[]);

-- name: GetEngagementWatermark :one
SELECT ran_at FROM customer_engagement_refresh WHERE channel_id = sqlc.arg(channel_id);

-- name: SetEngagementWatermark :exec
INSERT INTO customer_engagement_refresh (channel_id, ran_at)
VALUES (sqlc.arg(channel_id), sqlc.arg(ran_at))
ON CONFLICT (channel_id) DO UPDATE SET ran_at = EXCLUDED.ran_at;

-- name: GetCustomerTrait :one
SELECT * FROM customer_traits WHERE customer_id = sqlc.arg(customer_id);

-- name: ListCustomerTraitsByChannel :many
SELECT * FROM customer_traits WHERE channel_id = sqlc.arg(channel_id);
