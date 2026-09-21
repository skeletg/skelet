-- name: CreateProductMedia :one
INSERT INTO catalog_product_media (
    id, product_id, s3_key, mime_type, size_bytes,
    alt_text, sort_order, status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetProductMediaByID :one
SELECT * FROM catalog_product_media WHERE id = $1;

-- name: ListProductMediaByProduct :many
SELECT * FROM catalog_product_media
WHERE product_id = $1
ORDER BY sort_order, created_at;

-- name: ListProductMediaByProductIDs :many
SELECT * FROM catalog_product_media
WHERE product_id = ANY(sqlc.arg(product_ids)::uuid[])
ORDER BY product_id, sort_order, created_at;

-- name: UpdateProductMediaMetadata :one
UPDATE catalog_product_media
SET alt_text   = $2,
    sort_order = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: MarkProductMediaReady :one
UPDATE catalog_product_media
SET status     = 'ready',
    width      = $2,
    height     = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetProductMediaSortOrder :exec
UPDATE catalog_product_media
SET sort_order = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: DeleteProductMedia :exec
DELETE FROM catalog_product_media WHERE id = $1;

-- name: DeleteProductMediaByIDs :many
DELETE FROM catalog_product_media
WHERE id = ANY(sqlc.arg(ids)::uuid[]) AND product_id = sqlc.arg(product_id)::uuid
RETURNING id, s3_key;

-- name: DeletePendingMediaOlderThan :many
DELETE FROM catalog_product_media
WHERE status = 'pending' AND created_at < $1
RETURNING id, s3_key;
