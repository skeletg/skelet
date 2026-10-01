-- name: UpsertProductChannelListing :one
INSERT INTO catalog_product_channel_listings (
  product_id, channel_id, price, is_published, url,
  compare_at_price, special_price, special_from, special_to
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (product_id, channel_id) DO UPDATE
SET price            = EXCLUDED.price,
    is_published     = EXCLUDED.is_published,
    url              = EXCLUDED.url,
    compare_at_price = EXCLUDED.compare_at_price,
    special_price    = EXCLUDED.special_price,
    special_from     = EXCLUDED.special_from,
    special_to       = EXCLUDED.special_to,
    updated_at       = NOW()
RETURNING *;

-- name: UpsertProductChannelListingPublished :one
-- Touches only is_published, so a bulk publish toggle (CSV import) keeps the listing's prices and url override
INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published)
VALUES ($1, $2, $3)
ON CONFLICT (product_id, channel_id) DO UPDATE
SET is_published = EXCLUDED.is_published,
    updated_at   = NOW()
RETURNING *;

-- name: GetProductChannelListing :one
SELECT * FROM catalog_product_channel_listings
WHERE product_id = $1 AND channel_id = $2;

-- name: ListProductChannelListingsByProduct :many
SELECT * FROM catalog_product_channel_listings
WHERE product_id = $1
ORDER BY channel_id;

-- name: ListProductChannelListingsByProductIDs :many
-- Batched loader: storefront PLP renders N products and each one's effective-price card needs its listing rows - one query keyed by product replaces N round-trips
SELECT * FROM catalog_product_channel_listings
WHERE product_id = ANY(sqlc.arg(product_ids)::uuid[])
ORDER BY product_id, channel_id;

-- name: ListProductChannelListingsByProductIDsForChannel :many
SELECT * FROM catalog_product_channel_listings
WHERE product_id = ANY(sqlc.arg(product_ids)::uuid[]) AND channel_id = sqlc.arg(channel_id)
ORDER BY product_id;

-- name: ListProductChannelListingsByChannel :many
SELECT * FROM catalog_product_channel_listings
WHERE channel_id = $1
ORDER BY product_id;

-- name: GetListingProductIDByChannelURL :one
-- Channel-override url → product - the storefront tries this before the global catalog_products.url lookup so a channel's own URL namespace wins on that channel
SELECT product_id FROM catalog_product_channel_listings
WHERE channel_id = $1 AND url = $2 AND is_published;

-- name: DeleteProductChannelListing :exec
DELETE FROM catalog_product_channel_listings
WHERE product_id = $1 AND channel_id = $2;

-- name: CountInheritingListingsForChannel :one
-- How many listings on this store currently inherit price (price IS NULL) Used by the currency-change guard to block changing a store's currency while any listing depends on the global default price
SELECT COUNT(*)::bigint
FROM catalog_product_channel_listings
WHERE channel_id = $1 AND price IS NULL;

-- name: CountInheritingListingsWithCurrencyMismatchForProduct :one
SELECT COUNT(*)::bigint
FROM catalog_product_channel_listings l
JOIN channels c ON c.id = l.channel_id
WHERE l.product_id = sqlc.arg(product_id)
  AND l.price IS NULL
  AND c.currency_code <> sqlc.arg(currency_code)::text;
