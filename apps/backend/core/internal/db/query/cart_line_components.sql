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

-- Component rows hold the line's TOTAL per component (per-bundle quantity x line quantity), and reservation reads them as-is, so a bundle line's quantity change rescales them by new/old in the same statement - the locked read keeps two concurrent edits from scaling off a stale old quantity
-- name: SetBundleLineQuantity :execrows
WITH old AS (
    SELECT cl.id, cl.quantity FROM cart_lines cl WHERE cl.id = sqlc.arg(line_id) FOR UPDATE
), scaled AS (
    UPDATE cart_line_components c
    SET quantity = ROUND(c.quantity * sqlc.arg(quantity)::numeric / old.quantity, 4)
    FROM old
    WHERE c.cart_line_id = old.id
)
UPDATE cart_lines l
SET quantity = sqlc.arg(quantity)::numeric, updated_at = NOW()
FROM old
WHERE l.id = old.id;

-- name: DeleteCartLineComponentsByLine :exec
DELETE FROM cart_line_components WHERE cart_line_id = $1;

-- Bundle cart-line upsert: unlike the simple-product path the ON CONFLICT clause REPLACES quantity + price + name rather than summing - when a customer reconfigures the bundle they're submitting a fresh selection set, not adding more units of the previous configuration
-- name: UpsertBundleCartLine :one
INSERT INTO cart_lines (cart_id, product_name, product_sku, unit_price, quantity, bundle_hook_priced, config_key)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (cart_id, product_sku, config_key) DO UPDATE
  SET quantity     = EXCLUDED.quantity,
      unit_price   = EXCLUDED.unit_price,
      product_name = EXCLUDED.product_name,
      bundle_hook_priced = EXCLUDED.bundle_hook_priced,
      updated_at   = NOW()
RETURNING *;
