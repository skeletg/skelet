-- Google Merchant product feed page - one row per product published on the channel: channel-priced, default-locale content, first ready
-- image, and the denormalized inventory totals for availability - pages by product id for stable iteration

-- name: MerchantFeedProducts :many
SELECT
  p.id,
  p.sku,
  p.kind,
  COALESCE(NULLIF(l.url, ''), ur.request_path, p.url)::text AS path,
  COALESCE(l.price, p.default_price)                                                      AS price_cents,
  l.compare_at_price AS listing_compare_at,
  p.compare_at_price AS product_compare_at,
  l.special_price    AS listing_special,
  l.special_from     AS listing_special_from,
  l.special_to       AS listing_special_to,
  p.special_price    AS product_special,
  p.special_from     AS product_special_from,
  p.special_to       AS product_special_to,
  COALESCE(NULLIF(c.name, ''), p.sku)::text                                               AS title,
  COALESCE(NULLIF(c.meta_description, ''), c.short_description ->> 'plain', c.description ->> 'plain', '')::text AS description,
  COALESCE(p.attributes ->> 'brand', '')::text                                            AS brand,
  COALESCE(
    (SELECT m.s3_key FROM catalog_product_media m
      WHERE m.product_id = p.id AND m.status = 'ready'
      ORDER BY m.sort_order, m.created_at LIMIT 1),
    '')::text                                                                             AS image_key,
  COALESCE(ii.tracked, FALSE)                                                             AS tracked,
  COALESCE(ii.quantity, 0)::bigint                                                        AS quantity,
  COALESCE(ii.backorderable, FALSE)                                                       AS backorderable
FROM catalog_products p
JOIN catalog_product_channel_listings l
  ON l.product_id = p.id
 AND l.channel_id = sqlc.arg(channel_id)::uuid
 AND l.is_published = TRUE
LEFT JOIN catalog_product_content c
  ON c.product_id = p.id AND c.locale = sqlc.arg(locale)::text
LEFT JOIN url_rewrites ur
  ON ur.entity_type = 'product' AND ur.entity_id = p.id
 AND ur.redirect_type = 0 AND ur.channel_id IS NULL
LEFT JOIN inventory_items ii ON ii.product_id = p.id
WHERE p.seo_noindex = FALSE
ORDER BY p.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);
