-- Self-contained browse_seen queries (the anon-keyed browse rollup, fed at ingest) + the signals half of the browse-abandonment scan. The
-- commerce half (exclude active-cart / since-browse-order) runs app-side against carts/orders

-- name: UpsertBrowseSeen :exec
-- Records the latest product browse for an anonymous visitor. On conflict the
-- newer browse wins (timestamp + its product context).
INSERT INTO browse_seen (channel_id, anon_id, last_browse_at, page_key, sku, product_name, product_image)
VALUES (sqlc.arg(channel_id), sqlc.arg(anon_id), sqlc.arg(last_browse_at), sqlc.arg(page_key), sqlc.arg(sku), sqlc.arg(product_name), sqlc.arg(product_image))
ON CONFLICT (channel_id, anon_id) DO UPDATE SET
    page_key       = CASE WHEN EXCLUDED.last_browse_at >= browse_seen.last_browse_at THEN EXCLUDED.page_key      ELSE browse_seen.page_key END,
    sku            = CASE WHEN EXCLUDED.last_browse_at >= browse_seen.last_browse_at THEN EXCLUDED.sku           ELSE browse_seen.sku END,
    product_name   = CASE WHEN EXCLUDED.last_browse_at >= browse_seen.last_browse_at THEN EXCLUDED.product_name  ELSE browse_seen.product_name END,
    product_image  = CASE WHEN EXCLUDED.last_browse_at >= browse_seen.last_browse_at THEN EXCLUDED.product_image ELSE browse_seen.product_image END,
    last_browse_at = GREATEST(browse_seen.last_browse_at, EXCLUDED.last_browse_at);

-- name: PruneBrowseSeen :execrows
DELETE FROM browse_seen WHERE last_browse_at < sqlc.arg(before);

-- name: MarkBrowseNotified :exec
INSERT INTO browse_notified (channel_id, customer_id, last_browse_at)
VALUES (sqlc.arg(channel_id), sqlc.arg(customer_id), sqlc.arg(last_browse_at))
ON CONFLICT (channel_id, customer_id) DO UPDATE SET last_browse_at = GREATEST(browse_notified.last_browse_at, EXCLUDED.last_browse_at);

-- name: PruneBrowseNotified :execrows
DELETE FROM browse_notified WHERE last_browse_at < sqlc.arg(before);

-- name: BrowseSeenSincePage :many
-- One keyset page of anon-keyed browse rows newer than the lookback, newest-first,
-- for the browse-abandonment scan. Identity resolution, the latest-per-customer
-- pick, the idle-threshold check and the notified filter all run app-side (the
-- identities live on the spine engine, which this Postgres table can't join);
-- paging bounds the fetch and each resolve batch on an arbitrarily busy window.
SELECT channel_id, anon_id, last_browse_at, page_key, sku, product_name, product_image
FROM browse_seen
WHERE last_browse_at > sqlc.arg(max_age)::timestamptz
  AND (last_browse_at, anon_id) < (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_anon)::text)
ORDER BY last_browse_at DESC, anon_id DESC
LIMIT sqlc.arg(lim);

-- name: BrowseNotifiedForCustomers :many
-- The notified watermarks for a candidate set, filtered app-side against each
-- candidate's browse time.
SELECT channel_id, customer_id, last_browse_at FROM browse_notified
WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[]);
