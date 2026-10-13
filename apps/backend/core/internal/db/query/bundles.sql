-- name: CreateBundleOption :one
INSERT INTO catalog_product_bundle_options (
    product_id, input_type, is_required, sort_order, min_selections, max_selections
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetBundleOptionByID :one
SELECT * FROM catalog_product_bundle_options WHERE id = $1;

-- name: GetBundleOptionByIDForUpdate :one
SELECT * FROM catalog_product_bundle_options WHERE id = $1 FOR UPDATE;

-- name: ListBundleOptionsByProductID :many
SELECT * FROM catalog_product_bundle_options
WHERE product_id = $1
ORDER BY sort_order, created_at;

-- name: UpdateBundleOption :one
UPDATE catalog_product_bundle_options
SET input_type     = $2,
    is_required    = $3,
    sort_order     = $4,
    min_selections = $5,
    max_selections = $6,
    updated_at     = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteBundleOption :exec
DELETE FROM catalog_product_bundle_options WHERE id = $1;

-- name: UpsertBundleOptionContent :one
INSERT INTO catalog_product_bundle_option_content (option_id, locale, title)
VALUES ($1, $2, $3)
ON CONFLICT (option_id, locale)
DO UPDATE SET title = EXCLUDED.title
RETURNING *;

-- name: ListBundleOptionContent :many
SELECT * FROM catalog_product_bundle_option_content
WHERE option_id = $1
ORDER BY locale;

-- name: ListBundleOptionContentByOptionIDs :many
SELECT * FROM catalog_product_bundle_option_content
WHERE option_id = ANY(sqlc.arg(option_ids)::uuid[])
ORDER BY option_id, locale;

-- name: DeleteBundleOptionContent :exec
DELETE FROM catalog_product_bundle_option_content
WHERE option_id = $1 AND locale = $2;

-- name: CreateBundleSelection :one
INSERT INTO catalog_product_bundle_selections (
    option_id, product_id, quantity, is_default, sort_order
)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetBundleSelectionByID :one
SELECT * FROM catalog_product_bundle_selections WHERE id = $1;

-- name: ListBundleSelectionsByOptionIDs :many
SELECT * FROM catalog_product_bundle_selections
WHERE option_id = ANY(sqlc.arg(option_ids)::uuid[])
ORDER BY option_id, sort_order, id;

-- name: CountBundleSelectionDefaultsForOption :one
SELECT COUNT(*) FROM catalog_product_bundle_selections
WHERE option_id = $1 AND is_default = TRUE;

-- name: UpdateBundleSelection :one
UPDATE catalog_product_bundle_selections
SET quantity   = $2,
    is_default = $3,
    sort_order = $4
WHERE id = $1
RETURNING *;

-- name: DeleteBundleSelection :exec
DELETE FROM catalog_product_bundle_selections WHERE id = $1;

-- name: ListBundleSelectionLinksByProductIDs :many
-- Inverse: every bundle selection a given product appears in, joined to its option so we can reach the owning bundle (o.product_id) - batched; powers Product.partOf.bundles - A product may appear in many options/bundles
SELECT s.product_id, s.option_id, s.quantity, s.is_default, s.sort_order,
       o.product_id AS bundle_id
FROM catalog_product_bundle_selections s
JOIN catalog_product_bundle_options o ON o.id = s.option_id
WHERE s.product_id = ANY(sqlc.arg(product_ids)::uuid[])
ORDER BY s.product_id, o.sort_order, s.sort_order;
