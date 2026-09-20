-- name: InsertLoyaltyEntry :one
INSERT INTO loyalty_point_entries (customer_id, order_id, points, reason)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: LoyaltyBalance :one
SELECT COALESCE(SUM(points), 0)::BIGINT AS balance
FROM   loyalty_point_entries
WHERE  customer_id = $1;

-- name: LockCustomerForLoyalty :exec
-- Same trick as LockCustomerForStoreCredit - the points ledger is an aggregate and can't be locked, so the owning customer row stands in for it - revokeForOrder has to hold this before it sums what's already revoked, since order.cancelled arrives at-least-once and the outbox dispatches a claimed batch concurrently
SELECT 1 FROM customers WHERE id = $1 FOR UPDATE;

-- name: ListLoyaltyEntries :many
SELECT * FROM loyalty_point_entries
WHERE  customer_id = $1
ORDER  BY created_at DESC, id DESC
LIMIT  $2;

-- name: LoyaltyEntriesForOrder :many
SELECT * FROM loyalty_point_entries
WHERE  order_id = $1
ORDER  BY created_at;
