-- Kit components: the fixed component list of a kind=kit product

-- name: CreateKitComponent :one
INSERT INTO catalog_product_kit_components (
    product_id, component_product_id, quantity, sort_order
)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetKitComponentByID :one
SELECT * FROM catalog_product_kit_components WHERE id = $1;

-- name: ListKitComponentsByProductID :many
SELECT * FROM catalog_product_kit_components
WHERE product_id = $1
ORDER BY sort_order, created_at;

-- name: UpdateKitComponent :one
UPDATE catalog_product_kit_components
SET quantity   = $2,
    sort_order = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteKitComponent :exec
DELETE FROM catalog_product_kit_components WHERE id = $1;
