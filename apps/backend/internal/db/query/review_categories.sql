-- ==================== Review Categories (operator-curated topic/issue taxonomy) ====================

-- name: CreateReviewCategory :one
INSERT INTO review_categories (
    slug, name, description, is_active, sort_order
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetReviewCategoryByID :one
SELECT * FROM review_categories WHERE id = $1;

-- name: GetReviewCategoryBySlug :one
SELECT * FROM review_categories WHERE slug = $1;

-- name: ListReviewCategories :many
SELECT * FROM review_categories ORDER BY sort_order, name;

-- name: ListActiveReviewCategories :many
SELECT * FROM review_categories WHERE is_active ORDER BY sort_order, name;

-- name: UpdateReviewCategory :one
UPDATE review_categories
SET name        = $2,
    description = $3,
    is_active   = $4,
    sort_order  = $5,
    updated_at  = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteReviewCategory :exec
DELETE FROM review_categories WHERE id = $1;
