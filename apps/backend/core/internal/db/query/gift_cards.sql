-- name: CreateGiftCard :one
-- A card issued against an order belongs to the order's channel unless the caller names one
INSERT INTO gift_cards (
    code, currency_code, initial_balance, balance, status,
    recipient_email, message, issued_order_id, customer_id, expires_at, issue_ref, channel_id
)
VALUES (
    sqlc.arg(code), sqlc.arg(currency_code), sqlc.arg(initial_balance), sqlc.arg(initial_balance), 'active',
    sqlc.arg(recipient_email), sqlc.arg(message), sqlc.narg(issued_order_id), sqlc.narg(customer_id), sqlc.narg(expires_at), sqlc.arg(issue_ref),
    COALESCE(sqlc.narg(channel_id)::uuid, (SELECT o.channel_id FROM orders o WHERE o.id = sqlc.narg(issued_order_id)::uuid))
)
RETURNING *;

-- name: GetGiftCardByIssueRef :one
-- The replay lookup for a keyed issue - a refund that already minted a card under this
-- ref must hand back that card rather than mint a second one
SELECT * FROM gift_cards WHERE issue_ref = $1 AND issue_ref <> '';

-- name: GetGiftCardByID :one
SELECT * FROM gift_cards WHERE id = $1;

-- name: GetGiftCardByCode :one
SELECT * FROM gift_cards WHERE code = $1;

-- name: GetGiftCardByCodeForUpdate :one
-- Row-locked read used inside redeem/adjust transactions so concurrent
-- redemptions of the same card serialize and can't overspend
SELECT * FROM gift_cards WHERE code = $1 FOR UPDATE;

-- name: GetGiftCardByIDForUpdate :one
-- The cancel-restore path only knows card ids - it locks before checking whether the restore already ran, since the outbox delivers order.cancelled at-least-once and dispatches a claimed batch concurrently
SELECT * FROM gift_cards WHERE id = $1 FOR UPDATE;

-- name: UpdateGiftCardBalance :one
UPDATE gift_cards
SET balance    = $2,
    status     = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListGiftCards :many
-- search matches the code or the recipient's email; CountGiftCards keeps the same WHERE so a page's total agrees with its rows
SELECT * FROM gift_cards
WHERE (sqlc.narg(channel_ids)::uuid[] IS NULL OR channel_id = ANY(sqlc.narg(channel_ids)::uuid[]))
  AND (sqlc.arg(search)::text = '' OR code ILIKE '%' || sqlc.arg(search)::text || '%' OR recipient_email ILIKE '%' || sqlc.arg(search)::text || '%')
ORDER BY created_at DESC LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountGiftCards :one
SELECT COUNT(*)::INTEGER FROM gift_cards
WHERE (sqlc.narg(channel_ids)::uuid[] IS NULL OR channel_id = ANY(sqlc.narg(channel_ids)::uuid[]))
  AND (sqlc.arg(search)::text = '' OR code ILIKE '%' || sqlc.arg(search)::text || '%' OR recipient_email ILIKE '%' || sqlc.arg(search)::text || '%');

-- name: ListGiftCardsByIssuedOrder :many
-- Used for issuance idempotency: if any card already references this order,
-- the OrderPlaced handler is replaying and should not re-issue
SELECT * FROM gift_cards WHERE issued_order_id = $1;

-- name: InsertGiftCardTransaction :one
INSERT INTO gift_card_transactions (gift_card_id, amount, reason, order_id, note)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListGiftCardTransactions :many
SELECT * FROM gift_card_transactions
WHERE gift_card_id = $1
ORDER BY created_at;

-- name: ListGiftCardTransactionsByOrder :many
-- Every gift-card ledger row tied to an order, used to reverse redemptions when the order is cancelled - without this a cancelled order kept the customer's balance spent - store credit, B2B credit and loyalty all restore, gift cards were the one tender that didn't
SELECT * FROM gift_card_transactions
WHERE order_id = $1
ORDER BY created_at;
