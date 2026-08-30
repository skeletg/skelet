-- Cart line options capture the customer's product custom-option picks on
-- a (simple) cart line - one line maps to N option rows

-- name: InsertCartLineOption :one
INSERT INTO cart_line_options (
    cart_line_id, product_option_id, option_value_id, input_type,
    option_title, value_label, text_value, price_delta, quantity, sort_order,
    file_url
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: ListCartLineOptionsByLineIDs :many
SELECT * FROM cart_line_options
WHERE cart_line_id = ANY(sqlc.arg(cart_line_ids)::uuid[])
ORDER BY cart_line_id, sort_order, created_at;

-- name: DeleteCartLineOptionsByLine :exec
DELETE FROM cart_line_options WHERE cart_line_id = $1;

-- Configured (options-bearing) cart-line upsert - like the bundle path the
-- ON CONFLICT REPLACES rather than sums: re-adding the same SKU submits a
-- fresh configuration, not more units of the previous one
-- name: UpsertConfiguredCartLine :one
INSERT INTO cart_lines (cart_id, product_name, product_sku, unit_price, quantity)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (cart_id, product_sku) DO UPDATE
  SET quantity     = EXCLUDED.quantity,
      unit_price   = EXCLUDED.unit_price,
      product_name = EXCLUDED.product_name,
      updated_at   = NOW()
RETURNING *;
