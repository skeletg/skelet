-- ==================== Review Moderation Tags (operator-curated abstract moderation-concern taxonomy)
-- ====================

-- name: CreateReviewModerationTag :one
INSERT INTO review_moderation_tags (
    slug, name, description, is_active, sort_order
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetReviewModerationTagByID :one
SELECT * FROM review_moderation_tags WHERE id = $1;

-- name: ListReviewModerationTags :many
SELECT * FROM review_moderation_tags ORDER BY sort_order, name;

-- name: ListActiveReviewModerationTags :many
SELECT * FROM review_moderation_tags WHERE is_active ORDER BY sort_order, name;

-- name: UpdateReviewModerationTag :one
UPDATE review_moderation_tags
SET name        = $2,
    description = $3,
    is_active   = $4,
    sort_order  = $5,
    updated_at  = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteReviewModerationTag :exec
DELETE FROM review_moderation_tags WHERE id = $1;
