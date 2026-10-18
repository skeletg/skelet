-- name: CreateOrderReturn :one
INSERT INTO order_returns (
    order_id, status, reason, restock, refund_target, admin_id, resolution
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: AddOrderReturnLine :one
INSERT INTO order_return_lines (
    return_id, order_line_id, quantity, reason, refund_unit_cents
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetOrderReturn :one
SELECT * FROM order_returns WHERE id = $1;

-- name: ListOrderReturns :many
SELECT * FROM order_returns
WHERE order_id = $1
ORDER BY created_at ASC;

-- name: ListOrderReturnLinesByReturnIDs :many
SELECT * FROM order_return_lines
WHERE return_id = ANY($1::uuid[])
ORDER BY created_at ASC;

-- name: UpdateOrderReturnStatus :one
-- Only from the status the caller validated - Receive racing Reject left a restocked return rejected, which made its units returnable again
UPDATE order_returns
SET status = sqlc.arg(status), updated_at = NOW()
WHERE id = sqlc.arg(id) AND status = sqlc.arg(from_status)
RETURNING *;

-- name: MarkOrderReturnRefunded :one
-- Stamp the refund outcome and move the return to 'refunded' in one write
UPDATE order_returns
SET status = 'refunded',
    refund_amount_cents = $2,
    issued_gift_card_id = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SumReturnedQtyByOrderLine :many
-- Quantity already returned per order line across all non-rejected returns -
-- the basis for the "return qty ≤ shipped − already-returned" guard
SELECT rl.order_line_id, COALESCE(SUM(rl.quantity), 0)::NUMERIC(14,4) AS returned
FROM order_return_lines rl
JOIN order_returns r ON r.id = rl.return_id
WHERE r.order_id = $1 AND r.status <> 'rejected'
GROUP BY rl.order_line_id;

-- name: SetOrderReturnExchangeOrder :one
-- Link the replacement draft opened for an exchange RMA
UPDATE order_returns
SET exchange_order_id = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;
