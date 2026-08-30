-- Cart line components capture the customer's per-option picks on a
-- bundle cart line - one bundle line maps to N component rows

-- name: InsertCartLineComponent :one
INSERT INTO cart_line_components (
    cart_line_id, bundle_option_id, bundle_selection_id,
    product_id, product_sku, product_name, unit_price, quantity, sort_order
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListCartLineComponentsByLineIDs :many
SELECT * FROM cart_line_components
WHERE cart_line_id = ANY(sqlc.arg(cart_line_ids)::uuid[])
ORDER BY cart_line_id, sort_order, created_at;

-- name: DeleteCartLineComponentsByLine :exec
DELETE FROM cart_line_components WHERE cart_line_id = $1;

-- Bundle cart-line upsert: unlike the simple-product path the ON
-- CONFLICT clause REPLACES quantity + price + name rather than summing -
-- when a customer reconfigures the bundle they're submitting a fresh
-- selection set, not adding more units of the previous configuration
-- name: UpsertBundleCartLine :one
INSERT INTO cart_lines (cart_id, product_name, product_sku, unit_price, quantity)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (cart_id, product_sku) DO UPDATE
  SET quantity     = EXCLUDED.quantity,
      unit_price   = EXCLUDED.unit_price,
      product_name = EXCLUDED.product_name,
      updated_at   = NOW()
RETURNING *;
