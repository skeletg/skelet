-- Order line components - mirrors cart_line_components but keyed on order_line_id - PlaceOrder copies the cart side
-- over so the order snapshot preserves the customer's bundle configuration

-- name: InsertOrderLineComponent :one
INSERT INTO order_line_components (
    order_line_id, bundle_option_id, bundle_selection_id,
    product_id, product_sku, product_name, unit_price, quantity, sort_order
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListOrderLineComponentsByOrderLineIDs :many
SELECT * FROM order_line_components
WHERE order_line_id = ANY($1::uuid[])
ORDER BY order_line_id, sort_order, created_at;
