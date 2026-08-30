-- Tier (customer-group / quantity-break) prices per product

-- name: ListProductGroupPrices :many
SELECT * FROM catalog_product_group_prices
WHERE product_id = $1
ORDER BY min_qty, customer_group_id NULLS FIRST;

-- name: DeleteProductGroupPrices :exec
DELETE FROM catalog_product_group_prices WHERE product_id = $1;

-- name: InsertProductGroupPrice :one
INSERT INTO catalog_product_group_prices (product_id, channel_id, customer_group_id, min_qty, price)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;
