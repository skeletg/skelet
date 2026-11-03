-- Order line components - mirrors cart_line_components but keyed on order_line_id - PlaceOrder copies the cart side
-- over so the order snapshot preserves the customer's bundle configuration, and snapshots a kit's recipe the same way
-- with no bundle option or selection

-- name: InsertOrderLineComponent :one
INSERT INTO order_line_components (
    order_line_id, bundle_option_id, bundle_selection_id,
    product_id, product_sku, product_name, unit_price, quantity, sort_order, returnable
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: ListOrderLineComponentsByOrderLineIDs :many
SELECT * FROM order_line_components
WHERE order_line_id = ANY($1::uuid[])
ORDER BY order_line_id, sort_order, created_at;

-- name: InsertOrderLineStockPart :exec
INSERT INTO order_line_stock_parts (order_line_id, product_id, quantity, source, sort_order)
VALUES ($1, $2, $3, $4, $5);

-- name: ListOrderLineStockPartsByOrderLineIDs :many
SELECT * FROM order_line_stock_parts
WHERE order_line_id = ANY($1::uuid[])
ORDER BY order_line_id, sort_order, created_at;
