-- name: CreateCreditMemo :one
-- The number is sequence-assigned by the column default
INSERT INTO credit_memos (
    order_id, return_id, lines_total_cents, shipping_refund_cents,
    adjustment_refund_cents, adjustment_fee_cents, total_cents,
    refund_target, offline, reason, admin_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: AddCreditMemoLine :one
INSERT INTO credit_memo_lines (credit_memo_id, order_line_id, quantity, unit_cents, total_cents)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetCreditMemo :one
SELECT * FROM credit_memos WHERE id = $1;

-- name: GetCreditMemoByReturn :one
-- Returns issue exactly one memo - used to make a replayed return refund a no-op
SELECT * FROM credit_memos WHERE return_id = $1;

-- name: ListCreditMemosByOrder :many
SELECT * FROM credit_memos WHERE order_id = $1 ORDER BY created_at, number;

-- name: ListCreditMemoLines :many
SELECT * FROM credit_memo_lines WHERE credit_memo_id = $1 ORDER BY created_at;

-- name: ListCreditMemoLinesByMemoIDs :many
-- Batch hydration for the order detail page - one query for every memo's lines
-- instead of one per memo
SELECT * FROM credit_memo_lines
WHERE credit_memo_id = ANY(sqlc.arg(memo_ids)::uuid[])
ORDER BY credit_memo_id, created_at;

-- name: SumCreditMemoQtyByOrderLine :many
-- Quantity already credited per order line across every memo on the order - the basis for the "credit qty <= ordered - already credited" guard on a memo issued without a return (a return has its own shipped-quantity guard)
SELECT ml.order_line_id, COALESCE(SUM(ml.quantity), 0)::NUMERIC(14,4) AS credited
FROM credit_memo_lines ml
JOIN credit_memos m ON m.id = ml.credit_memo_id
WHERE m.order_id = $1
GROUP BY ml.order_line_id;

-- name: SetPaymentTransactionCreditMemo :exec
-- Point the money row at the memo that explains it
UPDATE order_payment_transactions SET credit_memo_id = $2 WHERE id = $1;
