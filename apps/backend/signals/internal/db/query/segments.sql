-- Customer segments and their membership table - segments are an admin bucket of customers; membership is materialised explicitly - the promotion engine reads them via Service.SegmentSlugsForCustomer and merges into the customer's group list, so the existing core.customer_group_in condition handles segment-gated rules with no engine changes

-- name: CreateCustomerSegment :one
INSERT INTO customer_segments (slug, name, description, definition, is_active)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateCustomerSegment :one
UPDATE customer_segments
SET slug         = $2,
    name         = $3,
    description  = $4,
    definition   = $5,
    is_active    = $6,
    updated_at   = NOW()
WHERE id = $1
RETURNING *;

-- name: SetSegmentLifecycleWindows :one
-- Set (or clear, by passing NULLs) a segment's lifecycle-cadence override - kept separate from UpdateCustomerSegment so the Synapse segment-refresh sync (which rewrites name/definition on auto-segments) never disturbs an operator's override
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
-- The admin "assign N customers" path and the refreshes that swap a computed membership in
INSERT INTO customer_segment_memberships (customer_id, segment_id)
SELECT customer_id, sqlc.arg(segment_id)::UUID
FROM unnest(sqlc.arg(customer_ids)::UUID[]) AS t(customer_id)
ON CONFLICT DO NOTHING;

-- name: RemoveCustomerFromSegment :exec
DELETE FROM customer_segment_memberships
WHERE customer_id = $1 AND segment_id = $2;

-- name: ClearSegmentMembership :exec
DELETE FROM customer_segment_memberships WHERE segment_id = $1;

-- name: RemoveSegmentMembersNotIn :exec
-- The leavers half of a refresh - members still in the computed set keep their row, and so their added_at
DELETE FROM customer_segment_memberships
WHERE segment_id = sqlc.arg(segment_id) AND NOT (customer_id = ANY(sqlc.arg(keep_ids)::uuid[]));

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
-- Hot path for the promotion engine: every active segment slug a customer belongs to, so a disabled segment stops firing promotions
-- rules + definition ride along so the caller can tell a computed segment from a hand-picked one
SELECT s.slug, s.rules, s.definition
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

-- name: FillSegmentRules :execrows
UPDATE customer_segments
SET rules = sqlc.arg(rules), updated_at = NOW()
WHERE id = sqlc.arg(id) AND rules IS NOT DISTINCT FROM sqlc.narg(old)::jsonb;

-- name: ListDynamicSegments :many
-- Longest unrefreshed first, so a sweep cut short resumes with what it missed
SELECT s.* FROM customer_segments s
WHERE s.rules IS NOT NULL AND s.is_active = TRUE
ORDER BY (SELECT max(d.day) FROM segment_membership_daily d WHERE d.segment_id = s.id) NULLS FIRST, s.created_at;

-- Newest-first member page for the admin segment detail
-- name: ListSegmentMembers :many
-- Segments are global but their members are not - the channel filter has to live here rather than in the resolver, or paging would return short pages with holes wherever a foreign-channel member was dropped after LIMIT
-- The channel comes off the synced roster now, since customers are core's
SELECT m.customer_id, r.channel_id, m.added_at
FROM customer_segment_memberships m
JOIN customer_roster r ON r.customer_id = m.customer_id
WHERE m.segment_id = $1
  AND (sqlc.arg(all_channels)::boolean OR r.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
ORDER BY m.added_at DESC, m.customer_id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: ListVisibleCustomerSegments :many
-- A behavioral segment belongs to the channel its definition names, so a caller sees it only with that channel; the rest are global
SELECT * FROM customer_segments
WHERE sqlc.arg(all_channels)::boolean
   OR COALESCE(CASE WHEN pg_input_is_valid(definition, 'jsonb') THEN definition::jsonb ->> 'channelId' END, '') = ''
   OR (CASE WHEN pg_input_is_valid(definition, 'jsonb') THEN definition::jsonb ->> 'channelId' END) = ANY(sqlc.arg(channel_ids)::text[])
ORDER BY slug
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountVisibleCustomerSegments :one
SELECT COUNT(*)::INTEGER FROM customer_segments
WHERE sqlc.arg(all_channels)::boolean
   OR COALESCE(CASE WHEN pg_input_is_valid(definition, 'jsonb') THEN definition::jsonb ->> 'channelId' END, '') = ''
   OR (CASE WHEN pg_input_is_valid(definition, 'jsonb') THEN definition::jsonb ->> 'channelId' END) = ANY(sqlc.arg(channel_ids)::text[]);

-- name: CountSegmentMembersInChannels :many
SELECT m.segment_id, COUNT(*)::INTEGER AS count
FROM customer_segment_memberships m
JOIN customer_roster r ON r.customer_id = m.customer_id
WHERE m.segment_id = ANY(sqlc.arg(segment_ids)::uuid[])
  AND (sqlc.arg(all_channels)::boolean OR r.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
GROUP BY m.segment_id;

-- name: RosterChannelsOfCustomers :many
SELECT customer_id, channel_id FROM customer_roster WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[]);
