-- Customer segments and their membership table - segments are an admin
-- bucket of customers; membership is materialised explicitly - the
-- promotion engine reads them via Service.SegmentSlugsForCustomer and
-- merges into the customer's group list, so the existing
-- core.customer_group_in condition handles segment-gated rules with no
-- engine changes

-- name: CreateCustomerSegment :one
INSERT INTO customer_segments (slug, name, description, refresh_sql, is_active)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateCustomerSegment :one
UPDATE customer_segments
SET slug         = $2,
    name         = $3,
    description  = $4,
    refresh_sql  = $5,
    is_active    = $6,
    updated_at   = NOW()
WHERE id = $1
RETURNING *;

-- name: SetSegmentLifecycleWindows :one
-- Set (or clear, by passing NULLs) a segment's lifecycle-cadence override - kept
-- separate from UpdateCustomerSegment so the Synapse segment-refresh sync (which
-- rewrites name/refresh_sql on auto-segments) never disturbs an operator's override
UPDATE customer_segments
SET lifecycle_new_days     = sqlc.narg(new_days),
    lifecycle_active_days  = sqlc.narg(active_days),
    lifecycle_at_risk_days = sqlc.narg(at_risk_days),
    updated_at             = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: RecordSegmentSnapshot :exec
-- Upsert today's member-count snapshot for a segment (call right after
-- re-materialising membership) so the per-segment growth chart accrues history
INSERT INTO segment_membership_daily (segment_id, day, member_count)
SELECT sqlc.arg(segment_id), CURRENT_DATE, COUNT(*)
FROM customer_segment_memberships WHERE segment_id = sqlc.arg(segment_id)
ON CONFLICT (segment_id, day) DO UPDATE SET member_count = EXCLUDED.member_count;

-- name: DeleteCustomerSegment :exec
DELETE FROM customer_segments WHERE id = $1;

-- name: GetCustomerSegment :one
SELECT * FROM customer_segments WHERE id = $1;

-- name: GetCustomerSegmentBySlug :one
SELECT * FROM customer_segments WHERE slug = $1;

-- name: ListCustomerSegments :many
SELECT * FROM customer_segments
ORDER BY slug
LIMIT $1 OFFSET $2;

-- name: CountCustomerSegments :one
SELECT COUNT(*)::INTEGER FROM customer_segments;

-- name: AddCustomerToSegment :exec
INSERT INTO customer_segment_memberships (customer_id, segment_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: BulkAddCustomersToSegment :exec
-- Used by the admin "assign N customers" path and by the future
-- refresh-job that re-runs a segment's refresh_sql
INSERT INTO customer_segment_memberships (customer_id, segment_id)
SELECT customer_id, sqlc.arg(segment_id)::UUID
FROM unnest(sqlc.arg(customer_ids)::UUID[]) AS t(customer_id)
ON CONFLICT DO NOTHING;

-- name: RemoveCustomerFromSegment :exec
DELETE FROM customer_segment_memberships
WHERE customer_id = $1 AND segment_id = $2;

-- name: ClearSegmentMembership :exec
DELETE FROM customer_segment_memberships WHERE segment_id = $1;

-- name: CountSegmentMembers :one
SELECT COUNT(*)::INTEGER FROM customer_segment_memberships WHERE segment_id = $1;

-- name: CountSegmentMembersByIDs :many
-- Batched member counts for a set of segments (one row per segment that has
-- ≥1 member) - backs the segment-list resolver so it isn't an N+1
SELECT segment_id, COUNT(*)::INTEGER AS count
FROM customer_segment_memberships
WHERE segment_id = ANY(sqlc.arg(segment_ids)::uuid[])
GROUP BY segment_id;

-- name: SegmentMemberCustomerIDs :many
-- Every customer id in a segment (used to resolve a campaign audience)
SELECT customer_id FROM customer_segment_memberships WHERE segment_id = sqlc.arg(segment_id);

-- name: SegmentSlugsForCustomer :many
-- Hot path for the promotion engine: returns every active segment
-- slug a customer belongs to - filters out inactive segments so a
-- temporarily-disabled segment doesn't keep firing promotions
SELECT s.slug
FROM customer_segments s
JOIN customer_segment_memberships m ON m.segment_id = s.id
WHERE m.customer_id = $1 AND s.is_active = TRUE
ORDER BY s.slug;

-- name: SegmentsForCustomer :many
-- Active segments a customer belongs to, with id + name (for the Synapse
-- profile "audiences" panel - chips that deep-link to each segment page)
SELECT s.id, s.name, s.slug
FROM customer_segments s
JOIN customer_segment_memberships m ON m.segment_id = s.id
WHERE m.customer_id = $1 AND s.is_active = TRUE
ORDER BY s.name;
-- name: SetSegmentRules :one
UPDATE customer_segments
SET rules = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListDynamicSegments :many
SELECT * FROM customer_segments
WHERE rules IS NOT NULL AND is_active = TRUE
ORDER BY created_at;

-- SegmentMemberEmails feeds audience sync/export: the member emails of
-- one segment (join skips deleted customers)
-- name: SegmentMemberEmails :many
SELECT c.email
FROM customer_segment_memberships m
JOIN customers c ON c.id = m.customer_id
WHERE m.segment_id = $1
ORDER BY c.email;

-- Newest-first member page for the admin segment detail
-- name: ListSegmentMembers :many
-- Segments are global but their members are not - the channel filter has to
-- live here rather than in the resolver, or paging would return short pages
-- with holes wherever a foreign-channel member was dropped after LIMIT
SELECT c.id, c.email, c.first_name, c.last_name, c.channel_id, m.added_at
FROM customer_segment_memberships m
JOIN customers c ON c.id = m.customer_id
WHERE m.segment_id = $1
  AND (sqlc.arg(all_channels)::boolean OR c.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
ORDER BY m.added_at DESC, c.email
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);
