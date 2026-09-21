-- Back-in-stock requests - storefront submits interest; the restock notifier
-- drains waiting requests per (product, channel) and marks them notified

-- name: UpsertBackInStockRequest :exec
-- Record interest - A repeat request re-arms an already-notified row (so a shopper can be told again after a future out-of-stock episode) and refreshes the customer/anon linkage + display snapshot in case they've since changed
INSERT INTO back_in_stock_requests (channel_id, product_id, sku, email, customer_id, anon_id, product_name, product_image, product_url)
VALUES (sqlc.arg(channel_id), sqlc.arg(product_id), sqlc.arg(sku), sqlc.arg(email), sqlc.narg(customer_id), sqlc.arg(anon_id), sqlc.arg(product_name), sqlc.arg(product_image), sqlc.arg(product_url))
ON CONFLICT (channel_id, product_id, email) DO UPDATE
    SET notified_at   = NULL,
        customer_id   = COALESCE(excluded.customer_id, back_in_stock_requests.customer_id),
        anon_id       = CASE WHEN excluded.anon_id <> '' THEN excluded.anon_id ELSE back_in_stock_requests.anon_id END,
        product_name  = excluded.product_name,
        product_image = excluded.product_image,
        product_url   = excluded.product_url;

-- name: PendingBackInStockChannels :many
-- Distinct channels with waiting requests for a product (the restock handler
-- checks per-channel salability before notifying)
SELECT DISTINCT channel_id FROM back_in_stock_requests
WHERE product_id = sqlc.arg(product_id) AND notified_at IS NULL;

-- name: ListPendingBackInStock :many
-- Waiting requests for a (product, channel), to notify
SELECT id, email, customer_id, sku, product_name, product_image, product_url
FROM back_in_stock_requests
WHERE product_id = sqlc.arg(product_id) AND channel_id = sqlc.arg(channel_id) AND notified_at IS NULL
ORDER BY created_at;

-- name: MarkBackInStockNotified :exec
-- Mark every waiting request for a (product, channel) notified, once we've
-- enrolled/emailed them
UPDATE back_in_stock_requests
SET notified_at = NOW()
WHERE product_id = sqlc.arg(product_id) AND channel_id = sqlc.arg(channel_id) AND notified_at IS NULL;

-- name: CountPendingBackInStock :one
-- Dashboard: how many shoppers are waiting on a product (channel-scoped)
SELECT COUNT(*)::bigint FROM back_in_stock_requests
WHERE channel_id = sqlc.arg(channel_id) AND notified_at IS NULL;
