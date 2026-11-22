-- Cart line options capture the customer's product custom-option picks on
-- a (simple) cart line - one line maps to N option rows

-- name: InsertCartLineOption :one
INSERT INTO cart_line_options (
    cart_line_id, product_option_id, option_value_id, input_type,
    option_title, value_label, text_value, price_delta, quantity, sort_order,
    file_url, source_product_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: ListCartLineOptionsByLineIDs :many
SELECT * FROM cart_line_options
WHERE cart_line_id = ANY(sqlc.arg(cart_line_ids)::uuid[])
ORDER BY cart_line_id, sort_order, created_at;

-- name: DeleteCartLineOptionsByLine :exec
DELETE FROM cart_line_options WHERE cart_line_id = $1;

-- Configured (options-bearing) cart-line upsert - like the bundle path the ON CONFLICT REPLACES rather than sums: re-adding the same SKU submits a fresh configuration, not more units of the previous one
-- name: UpsertConfiguredCartLine :one
INSERT INTO cart_lines (cart_id, product_name, product_sku, unit_price, quantity)
VALUES (sqlc.arg(cart_id), sqlc.arg(product_name), sqlc.arg(product_sku), sqlc.arg(unit_price), sqlc.arg(quantity))
ON CONFLICT (cart_id, product_sku, config_key) DO UPDATE
  SET quantity     = EXCLUDED.quantity,
      -- picking again prices the new picks. An admin doing it keeps a price set on the line; a shopper doing it is
      -- choosing something else than the price was set for, so the line goes back to the catalog's
      unit_price   = CASE WHEN cart_lines.price_source <> 'catalog' AND sqlc.arg(keep_set_price)::boolean THEN cart_lines.unit_price ELSE EXCLUDED.unit_price END,
      list_unit_price = CASE WHEN cart_lines.price_source <> 'catalog' AND sqlc.arg(keep_set_price)::boolean THEN EXCLUDED.unit_price END,
      price_source = CASE WHEN sqlc.arg(keep_set_price)::boolean THEN cart_lines.price_source ELSE 'catalog' END,
      price_set_by = CASE WHEN sqlc.arg(keep_set_price)::boolean THEN cart_lines.price_set_by END,
      price_reason = CASE WHEN sqlc.arg(keep_set_price)::boolean THEN cart_lines.price_reason ELSE '' END,
      price_max_quantity = CASE WHEN sqlc.arg(keep_set_price)::boolean THEN cart_lines.price_max_quantity END,
      price_expires_at = CASE WHEN sqlc.arg(keep_set_price)::boolean THEN cart_lines.price_expires_at END,
      no_promotions = sqlc.arg(keep_set_price)::boolean AND cart_lines.no_promotions,
      product_name = EXCLUDED.product_name,
      updated_at   = NOW()
RETURNING *;
