-- Order line options - mirrors cart_line_options but keyed on order_line_id
-- Populated at placement by copying the cart line's options

-- name: InsertOrderLineOption :one
INSERT INTO order_line_options (
    order_line_id, product_option_id, option_value_id, input_type,
    option_title, value_label, text_value, price_delta, quantity, sort_order,
    file_url
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;
