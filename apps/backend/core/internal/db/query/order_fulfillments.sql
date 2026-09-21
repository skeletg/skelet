-- name: CreateOrderFulfillment :one
INSERT INTO order_fulfillments (order_id, status, carrier, tracking_number, sequence, shipped_at, insured_value_cents)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: CreateOrderFulfillmentLine :one
INSERT INTO order_fulfillment_lines (fulfillment_id, order_line_id, quantity)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListOrderFulfillments :many
SELECT * FROM order_fulfillments WHERE order_id = $1 ORDER BY sequence ASC;

-- name: GetOrderFulfillment :one
SELECT * FROM order_fulfillments WHERE id = $1;

-- name: ListOrderFulfillmentLinesByFulfillmentIDs :many
SELECT * FROM order_fulfillment_lines
WHERE fulfillment_id = ANY(sqlc.arg(fulfillment_ids)::uuid[])
ORDER BY fulfillment_id, created_at;

-- name: SumShippedByOrderLine :many
-- Per-line already-shipped quantity (excludes cancelled fulfillments) - the
-- basis for the over-ship guard and the all-shipped status derivation
SELECT ofl.order_line_id, COALESCE(SUM(ofl.quantity), 0)::numeric AS shipped
FROM order_fulfillment_lines ofl
JOIN order_fulfillments f ON f.id = ofl.fulfillment_id
WHERE f.order_id = $1 AND f.status <> 'cancelled'
GROUP BY ofl.order_line_id;

-- name: CountOrderFulfillments :one
SELECT COUNT(*) FROM order_fulfillments WHERE order_id = $1;
