-- name: InsertStoreCreditEntry :one
INSERT INTO store_credit_entries (customer_id, order_id, amount_cents, currency_code, reason, admin_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: StoreCreditBalance :one
-- Signed sum of the customer's ledger in one currency (grants positive,
-- redemptions negative)
SELECT COALESCE(SUM(amount_cents), 0)::BIGINT AS balance
FROM   store_credit_entries
WHERE  customer_id = $1 AND currency_code = $2;

-- name: ListStoreCreditEntries :many
SELECT * FROM store_credit_entries
WHERE  customer_id = $1
ORDER  BY created_at DESC, id DESC
LIMIT  $2;

-- name: StoreCreditEntriesForOrder :many
SELECT * FROM store_credit_entries
WHERE  order_id = $1
ORDER  BY created_at;

-- name: LockCustomerForStoreCredit :exec
-- Serializes redemptions per customer inside the placement tx: an aggregate can't be locked, so we lock the owning customer row before summing the ledger to prevent concurrent overdraw
SELECT 1 FROM customers WHERE id = $1 FOR UPDATE;

-- name: SetOrderStoreCreditTender :one
-- Records the cents covered by store credit + the resulting payment
-- status, inside the placement tx - mirrors SetOrderGiftCardTender
UPDATE orders
SET    store_credit_amount = $2, payment_status = $3, updated_at = NOW()
WHERE  id = $1
RETURNING *;
