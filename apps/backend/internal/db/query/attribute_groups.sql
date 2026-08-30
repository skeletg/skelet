-- name: CreateAttributeGroup :one
INSERT INTO catalog_attribute_groups (product_type_id, name, sort_order)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetAttributeGroupByID :one
SELECT * FROM catalog_attribute_groups WHERE id = $1;

-- name: ListAttributeGroupsByProductType :many
SELECT * FROM catalog_attribute_groups
WHERE product_type_id = $1
ORDER BY sort_order, name;

-- name: UpdateAttributeGroup :one
UPDATE catalog_attribute_groups
SET name = $2, sort_order = $3
WHERE id = $1
RETURNING *;

-- name: DeleteAttributeGroup :exec
DELETE FROM catalog_attribute_groups WHERE id = $1;
