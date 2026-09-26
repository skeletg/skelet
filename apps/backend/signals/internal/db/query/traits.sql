-- Spine half of trait compute (the `act` CTE), split out of the commerce RecomputeChannelTraits - feeds the
-- commerce-side customer_engagement_30d projection the recompute reads

-- name: UpsertCustomerRoster :exec
-- Lands a page of the synced roster - paired arrays, one statement per page
-- A deleted customer's tombstone keeps a page read before the delete from bringing them back, and a row core read later than this page is left alone
INSERT INTO customer_roster (customer_id, channel_id, email, first_name, phone, is_active, customer_group_id, customer_group_slug, marketing_opted_in, sms_opted_in, consents, source_read_at, synced_at)
SELECT u.customer_id, u.channel_id, u.email, u.first_name, u.phone, u.is_active, u.group_id, u.group_slug,
       u.opted_in, u.sms_opted_in, u.consents::jsonb,
       CASE WHEN u.read_at > 'epoch'::timestamptz THEN u.read_at END, sqlc.arg(synced_at)
FROM (SELECT unnest(sqlc.arg(customer_ids)::uuid[]) AS customer_id, unnest(sqlc.arg(channel_ids)::uuid[]) AS channel_id,
             unnest(sqlc.arg(emails)::text[]) AS email, unnest(sqlc.arg(first_names)::text[]) AS first_name,
             unnest(sqlc.arg(phones)::text[]) AS phone, unnest(sqlc.arg(actives)::boolean[]) AS is_active,
             unnest(sqlc.arg(group_ids)::uuid[]) AS group_id, unnest(sqlc.arg(group_slugs)::text[]) AS group_slug,
             unnest(sqlc.arg(opted_in)::boolean[]) AS opted_in, unnest(sqlc.arg(sms_opted_in)::boolean[]) AS sms_opted_in,
             unnest(sqlc.arg(consents)::text[]) AS consents, unnest(sqlc.arg(read_ats)::timestamptz[]) AS read_at) u
WHERE NOT EXISTS (SELECT 1 FROM customer_roster_tombstones t WHERE t.customer_id = u.customer_id)
ON CONFLICT (customer_id) DO UPDATE
    SET channel_id = EXCLUDED.channel_id, email = EXCLUDED.email, first_name = EXCLUDED.first_name,
        phone = EXCLUDED.phone, is_active = EXCLUDED.is_active, customer_group_id = EXCLUDED.customer_group_id,
        customer_group_slug = EXCLUDED.customer_group_slug,
        marketing_opted_in = EXCLUDED.marketing_opted_in, sms_opted_in = EXCLUDED.sms_opted_in,
        consents = EXCLUDED.consents, source_read_at = EXCLUDED.source_read_at, synced_at = EXCLUDED.synced_at
    WHERE customer_roster.source_read_at IS NULL OR EXCLUDED.source_read_at IS NULL
       OR EXCLUDED.source_read_at >= customer_roster.source_read_at;

-- name: TouchCustomerRoster :exec
UPDATE customer_roster SET synced_at = sqlc.arg(synced_at)
WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[]) AND synced_at < sqlc.arg(synced_at);

-- name: UpsertCustomerRosterEntry :exec
-- One customer, arriving on a change rather than a sweep - synced_at moves too, so the next sweep does not prune a row that just landed
INSERT INTO customer_roster (customer_id, channel_id, email, first_name, phone, is_active, customer_group_id, customer_group_slug, marketing_opted_in, sms_opted_in, consents, source_read_at, synced_at)
SELECT sqlc.arg(customer_id)::uuid, sqlc.arg(channel_id)::uuid, sqlc.arg(email)::text, sqlc.arg(first_name)::text,
       sqlc.arg(phone)::text, sqlc.arg(is_active)::boolean, sqlc.narg(customer_group_id)::uuid, sqlc.arg(customer_group_slug)::text,
       sqlc.arg(marketing_opted_in)::boolean, sqlc.arg(sms_opted_in)::boolean, sqlc.arg(consents)::jsonb,
       CASE WHEN sqlc.arg(read_at)::timestamptz > 'epoch'::timestamptz THEN sqlc.arg(read_at)::timestamptz END, now()
WHERE NOT EXISTS (SELECT 1 FROM customer_roster_tombstones t WHERE t.customer_id = sqlc.arg(customer_id)::uuid)
ON CONFLICT (customer_id) DO UPDATE
    SET channel_id = EXCLUDED.channel_id, email = EXCLUDED.email, first_name = EXCLUDED.first_name,
        phone = EXCLUDED.phone, is_active = EXCLUDED.is_active, customer_group_id = EXCLUDED.customer_group_id,
        customer_group_slug = EXCLUDED.customer_group_slug,
        marketing_opted_in = EXCLUDED.marketing_opted_in, sms_opted_in = EXCLUDED.sms_opted_in,
        consents = EXCLUDED.consents, source_read_at = EXCLUDED.source_read_at, synced_at = EXCLUDED.synced_at
    WHERE customer_roster.source_read_at IS NULL OR EXCLUDED.source_read_at IS NULL
       OR EXCLUDED.source_read_at >= customer_roster.source_read_at;

-- name: DeleteCustomerRosterEntry :execrows
-- A customer core dropped, ahead of the sweep that would have caught it
DELETE FROM customer_roster WHERE customer_id = sqlc.arg(customer_id);

-- name: TombstoneCustomerRosterEntry :exec
INSERT INTO customer_roster_tombstones (customer_id) VALUES (sqlc.arg(customer_id))
ON CONFLICT (customer_id) DO NOTHING;

-- name: PruneRosterTombstones :execrows
DELETE FROM customer_roster_tombstones WHERE deleted_at < sqlc.arg(before);

-- name: DeleteCustomerTraits :exec
DELETE FROM customer_traits WHERE customer_id = sqlc.arg(customer_id);

-- name: DeleteCustomerMemberships :exec
DELETE FROM customer_segment_memberships WHERE customer_id = sqlc.arg(customer_id);

-- name: DeleteCustomerFlowRuns :exec
DELETE FROM flow_runs WHERE customer_id = sqlc.arg(customer_id)::uuid;

-- name: DetachCustomerCampaignRecipients :exec
-- A deleted customer's campaign rows stay as the campaign's history with the address gone, and a row still waiting to send is skipped
UPDATE email_campaign_recipients
SET customer_id = NULL, email = 'redacted-' || id::text || '@anonymized.invalid', first_name = '',
    error = CASE WHEN status = 'pending' THEN 'customer deleted' ELSE error END,
    status = CASE WHEN status = 'pending' THEN 'skipped' ELSE status END
WHERE customer_id = sqlc.arg(customer_id)::uuid;

-- name: PruneCustomerRoster :many
-- Customers gone from commerce since this sync started - stands in for an FK cascade that can't cross databases, and the ids scope the cleanup after
DELETE FROM customer_roster WHERE synced_at < sqlc.arg(before) RETURNING customer_id;

-- name: PruneOrphanedTraits :execrows
-- Traits for customers no longer on the roster - the recompute only upserts, so without this a
-- deleted customer keeps their scores and stays in every trait-based segment
DELETE FROM customer_traits t WHERE NOT EXISTS (
    SELECT 1 FROM customer_roster r WHERE r.customer_id = t.customer_id
);

-- name: RecomputeChannelTraits :execrows
-- Recompute every customer's RFM, lifecycle and engagement traits for a channel in one set-based statement, so trait-compute scales without per-customer round-trips
-- RFM are 1..5 quintile ranks among the channel's purchasers with 0 for never-ordered, recency reversed and ties broken by customer_id; lifecycle uses the channel's configured windows
WITH agg AS (
    -- Every input is local now: the roster is synced from commerce, and the order aggregate is fed by the spine Driven by the roster rather than customers, so a never-ordered customer still keeps a row and lands as a prospect, and the lifecycle override rides along on it rather than being derived from segments here
    SELECT c.customer_id                            AS customer_id,
           COALESCE(a.order_count, 0)::int          AS order_count,
           COALESCE(a.monetary_cents, 0)::bigint    AS monetary_cents,
           a.last_order_at                          AS last_order_at,
           a.first_order_at                         AS first_order_at,
           o.new_days, o.active_days, o.at_risk_days
    FROM customer_roster c
    LEFT JOIN LATERAL (
        -- The shortest-window override segment the customer belongs to, most aggressive cadence winning
        SELECT s.lifecycle_new_days AS new_days, s.lifecycle_active_days AS active_days, s.lifecycle_at_risk_days AS at_risk_days
        FROM customer_segment_memberships m
        JOIN customer_segments s ON s.id = m.segment_id
        WHERE m.customer_id = c.customer_id AND s.is_active AND s.lifecycle_at_risk_days IS NOT NULL
        ORDER BY s.lifecycle_at_risk_days ASC, s.id
        LIMIT 1
    ) o ON TRUE
    LEFT JOIN customer_order_agg a ON a.customer_id = c.customer_id
    WHERE c.channel_id = sqlc.arg(channel_id)::uuid
),
act AS (
    -- Reverse-read projection, refreshed from the spine by the trait-compute job right before this recompute
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
           COALESCE(e.engagement, 0) AS engagement,
           a.new_days, a.active_days, a.at_risk_days
    FROM agg a
    LEFT JOIN act e ON e.customer_id = a.customer_id
),
predict AS (
    -- churn_risk: repeaters get 35 x (recency / cadence) clamped 5..95, a single purchase gets coarse window-based risk, and a prospect gets 0
    -- next_order_days is cadence minus recency floored at 0 (-1 unknown), and predicted_clv_cents is AOV x expected 12-month orders x survival
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
           WHEN b.recency_days BETWEEN 0 AND COALESCE(b.new_days,     sqlc.arg(new_days)::int)     AND b.order_count = 1 THEN 'new'
           WHEN b.recency_days BETWEEN 0 AND COALESCE(b.active_days,  sqlc.arg(active_days)::int)  THEN 'active'
           WHEN b.recency_days BETWEEN 0 AND COALESCE(b.at_risk_days, sqlc.arg(at_risk_days)::int) THEN 'at_risk'
           ELSE 'lapsed'
       END AS lifecycle_stage,
       b.engagement,
       p.churn_risk, p.predicted_clv_cents, p.next_order_days, NOW()
FROM base b
LEFT JOIN ranked r ON r.customer_id = b.customer_id
LEFT JOIN predict p ON p.customer_id = b.customer_id
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
-- Rewrites a row only when a column rules read changed, so idle customers churn no WAL - computed_at means "last changed", not "last recomputed"
WHERE customer_traits.frequency        IS DISTINCT FROM EXCLUDED.frequency
   OR customer_traits.recency_days      IS DISTINCT FROM EXCLUDED.recency_days
   OR customer_traits.next_order_days   IS DISTINCT FROM EXCLUDED.next_order_days
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
-- Atomically swap a channel's projection to the given per-customer counts (paired arrays: customer_ids[i] has engagements[i]) - one statement, so a crash can't leave the projection half-cleared before RecomputeChannelTraits reads it - the delete and the upsert touch disjoint rows (NOT IN set vs in set), which single-statement row rules require
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

-- name: ReplaceChannelOrderAgg :exec
-- Atomically swap a channel's order projection to the given per-customer aggregates (paired arrays), one statement for the same reason the engagement swap is one: a crash must not leave it half-cleared before RecomputeChannelTraits reads it and zeroes everyone's RFM
WITH pruned AS (
    DELETE FROM customer_order_agg
    WHERE channel_id = sqlc.arg(channel_id)::uuid
      AND NOT (customer_id = ANY(sqlc.arg(customer_ids)::uuid[]))
)
INSERT INTO customer_order_agg (customer_id, channel_id, order_count, monetary_cents, first_order_at, last_order_at)
SELECT unnest(sqlc.arg(customer_ids)::uuid[]), sqlc.arg(channel_id)::uuid,
       unnest(sqlc.arg(order_counts)::int[]), unnest(sqlc.arg(monetary_cents)::bigint[]),
       unnest(sqlc.arg(first_order_ats)::timestamptz[]), unnest(sqlc.arg(last_order_ats)::timestamptz[])
ON CONFLICT (customer_id) DO UPDATE
    SET channel_id = EXCLUDED.channel_id, order_count = EXCLUDED.order_count,
        monetary_cents = EXCLUDED.monetary_cents, first_order_at = EXCLUDED.first_order_at,
        last_order_at = EXCLUDED.last_order_at, computed_at = now();

-- name: ClearChannelOrderAgg :exec
-- The empty-refresh case: a channel whose spine holds no surviving orders
DELETE FROM customer_order_agg WHERE channel_id = sqlc.arg(channel_id);

-- name: LifecycleMixForCustomers :many
SELECT lifecycle_stage::text AS stage, COUNT(*)::int AS count
FROM customer_traits
WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[])
GROUP BY lifecycle_stage
ORDER BY count DESC;

-- name: RfmDistributionForCustomers :many
SELECT x.dimension::text AS dimension, x.score::int AS score, COUNT(*)::int AS count
FROM (
    SELECT 'recency'::text AS dimension, rfm_r AS score FROM customer_traits WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[])
    UNION ALL
    SELECT 'frequency'::text, rfm_f FROM customer_traits WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[])
    UNION ALL
    SELECT 'monetary'::text, rfm_m FROM customer_traits WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[])
) x
GROUP BY x.dimension, x.score
ORDER BY x.dimension, x.score;

-- name: TraitsForCustomers :many
SELECT customer_id, lifecycle_stage, recency_days, engagement_score
FROM customer_traits
WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[]);

-- name: ChannelForCustomer :one
-- The channel a customer belongs to, for a channel-scoped permission check in a service with no customers table
SELECT channel_id FROM customer_roster WHERE customer_id = sqlc.arg(customer_id);

-- name: PruneOrphanedMemberships :execrows
-- Memberships for customers no longer on the roster - the foreign key to customers could not cross databases, so this replaces its cascade the way PruneOrphanedTraits does for scores
DELETE FROM customer_segment_memberships m WHERE NOT EXISTS (
    SELECT 1 FROM customer_roster r WHERE r.customer_id = m.customer_id
);

-- name: SegmentMemberOrderStats :many
-- The order half of a segment's member panel, off the same projection the RFM scores beside it derive
-- from, so a member's orders and spend agree with their recency/frequency/monetary
SELECT customer_id,
       order_count::int        AS orders,
       monetary_cents::bigint  AS spend_cents,
       last_order_at
FROM customer_order_agg
WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[]);

-- name: SegmentOrderRollup :one
-- The same projection summed for a segment's rollup - order_count is already distinct orders per
-- customer, so the sum needs no DISTINCT of its own
SELECT COALESCE(SUM(order_count), 0)::bigint    AS orders,
       COALESCE(SUM(monetary_cents), 0)::bigint AS revenue_cents
FROM customer_order_agg
WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[]);

-- name: RosterChannels :many
SELECT channel_id FROM channel_roster ORDER BY channel_id;

-- name: TraitComputeChannels :many
SELECT c.channel_id FROM channel_roster c
LEFT JOIN trait_compute_runs t ON t.channel_id = c.channel_id
ORDER BY GREATEST(t.attempted_at, t.finished_at) NULLS FIRST, c.channel_id;

-- name: MarkTraitComputeRun :exec
INSERT INTO trait_compute_runs (channel_id, finished_at) VALUES (sqlc.arg(channel_id), sqlc.arg(finished_at)::timestamptz)
ON CONFLICT (channel_id) DO UPDATE SET finished_at = EXCLUDED.finished_at;

-- name: MarkTraitComputeAttempt :exec
INSERT INTO trait_compute_runs (channel_id, attempted_at) VALUES (sqlc.arg(channel_id), sqlc.arg(attempted_at)::timestamptz)
ON CONFLICT (channel_id) DO UPDATE SET attempted_at = EXCLUDED.attempted_at;

-- name: ChannelRosterCustomerIDs :many
-- Every customer id in a channel - the universe a behavioral segment draws from, for evaluating a
-- negated node (NOT X is the universe minus X)
SELECT customer_id FROM customer_roster WHERE channel_id = sqlc.arg(channel_id);

-- name: UpsertChannelRoster :exec
INSERT INTO channel_roster (channel_id, is_default, is_active, name, public_url, from_address, from_name, logo_url, synced_at)
SELECT unnest(sqlc.arg(channel_ids)::uuid[]), unnest(sqlc.arg(is_defaults)::boolean[]),
       unnest(sqlc.arg(is_actives)::boolean[]),
       unnest(sqlc.arg(names)::text[]), unnest(sqlc.arg(public_urls)::text[]),
       unnest(sqlc.arg(from_addresses)::text[]), unnest(sqlc.arg(from_names)::text[]),
       unnest(sqlc.arg(logo_urls)::text[]), sqlc.arg(synced_at)
ON CONFLICT (channel_id) DO UPDATE
    SET is_default = EXCLUDED.is_default, is_active = EXCLUDED.is_active, name = EXCLUDED.name,
        public_url = EXCLUDED.public_url, from_address = EXCLUDED.from_address, from_name = EXCLUDED.from_name,
        logo_url = EXCLUDED.logo_url, synced_at = EXCLUDED.synced_at;

-- name: LockChannelRoster :exec
SELECT pg_advisory_xact_lock(hashtext('signals.channel_roster'));

-- name: PruneChannelRoster :execrows
-- Channels core no longer has at all - a deactivated one is still pushed and keeps its row
WITH gone AS (
    DELETE FROM channel_roster WHERE NOT (channel_id = ANY(sqlc.arg(keep)::uuid[])) RETURNING channel_id
)
INSERT INTO channel_roster_tombstones (channel_id) SELECT channel_id FROM gone
ON CONFLICT (channel_id) DO UPDATE SET pruned_at = EXCLUDED.pruned_at;

-- name: ChannelRosterTombstones :many
SELECT channel_id FROM channel_roster_tombstones;

-- name: ClearChannelRosterTombstones :exec
DELETE FROM channel_roster_tombstones WHERE channel_id = ANY(sqlc.arg(channel_ids)::uuid[]);

-- name: ChannelRoster :many
SELECT channel_id, is_default, is_active, name, public_url, from_address, from_name, logo_url FROM channel_roster;


-- name: DetachCampaignRecipientsForCustomers :execrows
-- DetachCustomerCampaignRecipients for everyone the roster just dropped - a guest row keeps its address, having no account to delete
UPDATE email_campaign_recipients cr
SET customer_id = NULL, email = 'redacted-' || id::text || '@anonymized.invalid', first_name = '',
    error = CASE WHEN cr.status = 'pending' THEN 'customer deleted' ELSE cr.error END,
    status = CASE WHEN cr.status = 'pending' THEN 'skipped' ELSE cr.status END
WHERE cr.customer_id = ANY(sqlc.arg(customer_ids)::uuid[]);

-- name: PruneOrphanedFlows :execrows
-- Flows for channels core no longer runs, the channel roster being what says which ones still exist
DELETE FROM flows f WHERE NOT EXISTS (
    SELECT 1 FROM channel_roster c WHERE c.channel_id = f.channel_id
);

-- name: PruneOrphanedCampaigns :execrows
-- The same, for campaigns - a draft for a retired channel can never send
DELETE FROM email_campaigns ec WHERE NOT EXISTS (
    SELECT 1 FROM channel_roster c WHERE c.channel_id = ec.channel_id
);

-- name: PruneOrphanedEmailLibrary :execrows
-- Saved emails for channels core no longer runs - their revisions follow through an in-database cascade
DELETE FROM email_library el WHERE NOT EXISTS (
    SELECT 1 FROM channel_roster c WHERE c.channel_id = el.channel_id
);

-- name: PruneOrphanedEmailBlocks :execrows
-- The same, for admin-defined block types
DELETE FROM email_blocks eb WHERE NOT EXISTS (
    SELECT 1 FROM channel_roster c WHERE c.channel_id = eb.channel_id
);

-- name: PruneOrphanedEmailTemplates :execrows
-- The same, for per-channel template overrides - a NULL channel is the global row every channel falls
-- back to, so it belongs to no channel and must survive
DELETE FROM email_templates et WHERE et.channel_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM channel_roster c WHERE c.channel_id = et.channel_id
);

-- name: RosterContact :one
-- One person a flow run can reach, as core last pushed them - consent is read here rather than
-- carried on the run, so someone who unsubscribes mid-journey is not sent to
SELECT customer_id, channel_id, email, first_name, phone, is_active, marketing_opted_in, sms_opted_in
FROM customer_roster WHERE customer_id = sqlc.arg(customer_id);

-- name: ChannelRosterContacts :many
-- Everyone a channel could mail, as core last pushed them - the audience a campaign draws from,
-- consent included so the send loop filters without a second read
SELECT customer_id, channel_id, email, first_name, is_active, customer_group_id, marketing_opted_in
FROM customer_roster WHERE channel_id = sqlc.arg(channel_id)
ORDER BY synced_at, customer_id;

-- name: RosterMarketingConsent :many
-- Marketing consent as it stands right now for a page of recipients - re-read at send time, since
-- someone who unsubscribed or was deactivated after the campaign was built must not be mailed
SELECT customer_id, (marketing_opted_in AND is_active)::boolean AS marketing_opted_in FROM customer_roster
WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[]);

-- name: GetRosterState :one
SELECT filled_version, filled_at FROM customer_roster_state WHERE one_row;

-- name: SetRosterFilled :exec
INSERT INTO customer_roster_state (one_row, filled_version, filled_at) VALUES (true, sqlc.arg(filled_version), now())
ON CONFLICT (one_row) DO UPDATE SET filled_version = EXCLUDED.filled_version, filled_at = EXCLUDED.filled_at;

-- name: PruneOrphanedDestinations :execrows
-- Destinations hold another shop's credentials, and a retired channel's must not keep forwarding
DELETE FROM event_destinations d WHERE NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = d.channel_id);

-- name: PruneOrphanedExperiments :execrows
DELETE FROM experiments e WHERE NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = e.channel_id);

-- name: PruneOrphanedChannelSettings :execrows
DELETE FROM config_values v WHERE v.channel_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = v.channel_id);

-- name: OrphanedLensSessionBlobKeys :many
SELECT k.blob_key FROM lens_session_chunks k JOIN lens_sessions s ON s.id = k.session_id
WHERE NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = s.channel_id);

-- name: OrphanedLensSnapshotBlobKeys :many
SELECT s.blob_key FROM lens_page_snapshots s WHERE NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = s.channel_id);

-- name: PruneOrphanedLensSessions :execrows
DELETE FROM lens_sessions s WHERE NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = s.channel_id);

-- name: PruneOrphanedLensSnapshots :execrows
DELETE FROM lens_page_snapshots s WHERE NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = s.channel_id);

-- name: PruneOrphanedLensTraffic :execrows
DELETE FROM lens_traffic_daily t WHERE NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = t.channel_id);

-- name: PruneOrphanedLensVisitors :execrows
DELETE FROM lens_visitor_seen v WHERE NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = v.channel_id);

-- name: PruneOrphanedTrafficRollup :execrows
DELETE FROM analytics_traffic_rollup t WHERE NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = t.channel_id);

-- name: PruneOrphanedBrowseSeen :execrows
DELETE FROM browse_seen b WHERE NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = b.channel_id);

-- name: PruneOrphanedBrowseNotified :execrows
DELETE FROM browse_notified b WHERE NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = b.channel_id);

-- name: PruneOrphanedTraitRuns :execrows
DELETE FROM trait_compute_runs t WHERE NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = t.channel_id);

-- name: DeleteOrphanedLiveFlowRuns :execrows
-- Backstop for a cleanup that died before reaching its ids - live runs only, which stay small, and none enrolled during this sync
DELETE FROM flow_runs r
WHERE r.status IN ('active', 'waiting') AND r.customer_id IS NOT NULL AND r.created_at < sqlc.arg(before)
  AND NOT EXISTS (SELECT 1 FROM customer_roster c WHERE c.customer_id = r.customer_id);

-- name: DetachOrphanedPendingRecipients :execrows
UPDATE email_campaign_recipients cr
SET customer_id = NULL, email = 'redacted-' || id::text || '@anonymized.invalid', first_name = '', error = 'customer deleted', status = 'skipped'
WHERE cr.status = 'pending' AND cr.customer_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM customer_roster c WHERE c.customer_id = cr.customer_id);
