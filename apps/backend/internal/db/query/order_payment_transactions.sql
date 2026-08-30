-- name: InsertOrderPaymentTransaction :one
-- occurred_at backdates the row - an offline payment is recorded when the operator
-- gets to it, not when the money landed, and the books have to match the bank
INSERT INTO order_payment_transactions (
    order_id, kind, amount_cents, status, provider_slug, provider_reference, idempotency_key, reason, return_id, created_at
) VALUES (
    sqlc.arg(order_id), sqlc.arg(kind), sqlc.arg(amount_cents), sqlc.arg(status),
    sqlc.arg(provider_slug), sqlc.arg(provider_reference), sqlc.arg(idempotency_key),
    sqlc.arg(reason), sqlc.arg(return_id), COALESCE(sqlc.narg(occurred_at), NOW())
)
RETURNING *;

-- name: MarkOrderPaymentTransaction :one
-- Flip a pending transaction to succeeded/failed and stamp the provider's
-- reference for the action (e.g. a Stripe refund id)
UPDATE order_payment_transactions
SET status = $2, provider_reference = $3
WHERE id = $1
RETURNING *;

-- name: ListOrderPaymentTransactions :many
SELECT * FROM order_payment_transactions
WHERE order_id = $1
ORDER BY created_at ASC;

-- name: SumOrderPaymentByKind :many
-- Summed succeeded amounts per kind - the basis for deriving payment status
-- and enforcing over-capture / over-refund guards
SELECT kind, COALESCE(SUM(amount_cents), 0)::BIGINT AS total
FROM order_payment_transactions
WHERE order_id = $1 AND status = 'succeeded'
GROUP BY kind;

-- name: GetOrderPaymentTransactionByIdempotencyKey :one
SELECT * FROM order_payment_transactions
WHERE idempotency_key = $1 AND status <> 'failed'
LIMIT 1;

-- name: SumOrderPaymentByKindActive :many
-- Same shape as SumOrderPaymentByKind but counts PENDING rows too - the
-- over-capture / over-refund guards must use this: a pending intent is real
-- money already in flight at the gateway, and counting only 'succeeded' let
-- two concurrent intents both pass the guard and double-charge
SELECT kind, COALESCE(SUM(amount_cents), 0)::BIGINT AS total
FROM order_payment_transactions
WHERE order_id = $1 AND status <> 'failed'
GROUP BY kind;
