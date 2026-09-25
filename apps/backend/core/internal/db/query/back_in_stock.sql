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

-- name: BackInStockProduct :one
-- Built from the channel's own listing, never the storefront's copy - an unlisted product has no row
SELECT
  p.sku,
  COALESCE(NULLIF(c.name, ''), p.sku)::text AS name,
  COALESCE(NULLIF(l.url, ''),
    (SELECT ur.request_path FROM url_rewrites ur
      WHERE ur.entity_type = 'product' AND ur.entity_id = p.id AND ur.redirect_type = 0
        AND (ur.channel_id = sqlc.arg(channel_id)::uuid OR ur.channel_id IS NULL)
      ORDER BY ur.channel_id NULLS LAST LIMIT 1),
    '')::text AS path,
  COALESCE(
    (SELECT m.s3_key FROM catalog_product_media m
      WHERE m.product_id = p.id AND m.status = 'ready'
      ORDER BY m.sort_order, m.created_at LIMIT 1),
    '')::text AS image_key
FROM catalog_products p
JOIN catalog_product_channel_listings l
  ON l.product_id = p.id AND l.channel_id = sqlc.arg(channel_id)::uuid AND l.is_published = TRUE
LEFT JOIN catalog_product_content c ON c.product_id = p.id AND c.locale = sqlc.arg(locale)::text
WHERE p.id = sqlc.arg(product_id)::uuid;

-- name: CountPendingBackInStockForEmail :one
SELECT COUNT(*)::bigint FROM back_in_stock_requests
WHERE channel_id = sqlc.arg(channel_id) AND email = sqlc.arg(email) AND notified_at IS NULL;
