-- Synapse's segment-side reads - the behavioural aggregates live on the spine driver

-- name: SegmentGrowth :many
-- Daily member-count series for a segment over the last `days` days (from the
-- snapshot table) - accrues forward from when the segment first materialised
SELECT day::timestamptz AS day, member_count
FROM segment_membership_daily
WHERE segment_id = sqlc.arg(segment_id) AND day >= CURRENT_DATE - sqlc.arg(days)::int
ORDER BY day;

-- name: SegmentMemberIDs :many
-- The customer ids in a segment - the affinity scan on the spine takes these ids, since it cannot join this table
SELECT customer_id FROM customer_segment_memberships WHERE segment_id = sqlc.arg(segment_id);
