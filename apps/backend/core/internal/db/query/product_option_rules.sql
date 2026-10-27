-- name: ListProductOptionRulesByProductIDs :many
SELECT * FROM catalog_product_option_rules
WHERE product_id = ANY(sqlc.arg(product_ids)::uuid[])
ORDER BY product_id, sort_order, created_at;

-- name: CreateProductOptionRule :one
INSERT INTO catalog_product_option_rules (product_id, template_id, sort_order, labels, condition, actions)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateProductOptionRule :one
UPDATE catalog_product_option_rules
SET sort_order = sqlc.arg(sort_order),
    labels     = sqlc.arg(labels),
    condition  = sqlc.arg(condition),
    actions    = sqlc.arg(actions),
    updated_at = NOW()
WHERE id = sqlc.arg(id)
  AND product_id IS NOT DISTINCT FROM sqlc.narg(product_id)::uuid
  AND template_id IS NOT DISTINCT FROM sqlc.narg(template_id)::uuid
RETURNING *;

-- name: DeleteProductOptionRule :exec
DELETE FROM catalog_product_option_rules
WHERE id = sqlc.arg(id)
  AND product_id IS NOT DISTINCT FROM sqlc.narg(product_id)::uuid
  AND template_id IS NOT DISTINCT FROM sqlc.narg(template_id)::uuid;
