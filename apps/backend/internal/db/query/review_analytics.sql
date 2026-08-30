-- ====================
-- Review insights (category analytics) - date-ranged + channel-scoped
-- Mirrors sales_reports.sql (all_channels boolean guard + channel_ids array)
-- category_id is nullable: NULL rows are "uncategorized" - the caller runs the
-- count query twice (current window + prior equal-length window) for deltas
-- ====================

-- ReviewCategoryCounts: per-category-slug tally for the window - unnests the
-- jsonb category_slugs array so a review in N categories counts toward each
-- (a review with no category contributes nothing - uncategorized is counted
-- separately by ReviewUncategorizedCount)
-- name: ReviewCategoryCounts :many
SELECT
    slug             AS slug,
    COUNT(*)::BIGINT AS review_count
FROM reviews r
CROSS JOIN LATERAL jsonb_array_elements_text(r.category_slugs) AS slug
WHERE r.submitted_at >= sqlc.arg(start_at)
  AND r.submitted_at <= sqlc.arg(end_at)
  AND (sqlc.arg(all_channels)::boolean OR r.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
GROUP BY slug;

-- ReviewUncategorizedCount: reviews with no category in the window
-- name: ReviewUncategorizedCount :one
SELECT COUNT(*)::BIGINT
FROM reviews r
WHERE jsonb_array_length(r.category_slugs) = 0
  AND r.submitted_at >= sqlc.arg(start_at)
  AND r.submitted_at <= sqlc.arg(end_at)
  AND (sqlc.arg(all_channels)::boolean OR r.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]));

-- ReviewCategoryTimeseries: per (period, category slug) - multi-category reviews
-- appear in each of their category bands
-- name: ReviewCategoryTimeseries :many
SELECT
    date_trunc(sqlc.arg(bucket)::text, r.submitted_at)::timestamptz AS period,
    slug             AS slug,
    COUNT(*)::BIGINT AS review_count
FROM reviews r
CROSS JOIN LATERAL jsonb_array_elements_text(r.category_slugs) AS slug
WHERE r.submitted_at >= sqlc.arg(start_at)
  AND r.submitted_at <= sqlc.arg(end_at)
  AND (sqlc.arg(all_channels)::boolean OR r.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
GROUP BY period, slug
ORDER BY period;

-- ReviewVolumeTimeseries: true review count per bucket (one row per review, no
-- category unnest) - drives the volume trend correctly under multi-category
-- name: ReviewVolumeTimeseries :many
SELECT
    date_trunc(sqlc.arg(bucket)::text, r.submitted_at)::timestamptz AS period,
    COUNT(*)::BIGINT AS review_count
FROM reviews r
WHERE r.submitted_at >= sqlc.arg(start_at)
  AND r.submitted_at <= sqlc.arg(end_at)
  AND (sqlc.arg(all_channels)::boolean OR r.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
GROUP BY period
ORDER BY period;

-- ReviewStatusCounts: per-status tally for the window (pending/approved/rejected)
-- name: ReviewStatusCounts :many
SELECT
    r.status         AS status,
    COUNT(*)::BIGINT AS review_count
FROM reviews r
WHERE r.submitted_at >= sqlc.arg(start_at)
  AND r.submitted_at <= sqlc.arg(end_at)
  AND (sqlc.arg(all_channels)::boolean OR r.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
GROUP BY r.status;

-- ReviewTagCounts: per-moderation-tag tally for the window - unnests the jsonb
-- moderation_tags array so a review flagged with N tags counts toward each
-- Reviews with no tags simply contribute nothing
-- name: ReviewTagCounts :many
SELECT
    tag              AS tag,
    COUNT(*)::BIGINT AS review_count
FROM reviews r
CROSS JOIN LATERAL jsonb_array_elements_text(r.moderation_tags) AS tag
WHERE r.submitted_at >= sqlc.arg(start_at)
  AND r.submitted_at <= sqlc.arg(end_at)
  AND (sqlc.arg(all_channels)::boolean OR r.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
GROUP BY tag;
