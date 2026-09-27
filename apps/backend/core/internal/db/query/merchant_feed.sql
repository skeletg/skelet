-- Google Merchant product feed page - one row per product published on the channel: channel-priced, default-locale content, first ready
-- image, and what the channel's own locations can sell (physical minus held for tracked items) - pages by product id for stable iteration
-- The path is channel-first like the sitemap: the listing's url override, then the channel's canonical rewrite, then the global one

-- name: MerchantFeedProducts :many
SELECT
  p.id,
  p.sku,
  p.kind,
  COALESCE(NULLIF(l.url, ''),
    (SELECT ur.request_path FROM url_rewrites ur
      WHERE ur.entity_type = 'product' AND ur.entity_id = p.id AND ur.redirect_type = 0
        AND (ur.channel_id = sqlc.arg(channel_id)::uuid OR ur.channel_id IS NULL)
      ORDER BY ur.channel_id NULLS LAST LIMIT 1),
    p.url)::text AS path,
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
  COALESCE((SELECT sum(CASE WHEN ii.tracked THEN lv.quantity - GREATEST(lv.reserved_quantity, 0) ELSE lv.quantity END)
    FROM inventory_levels lv
    JOIN inventory_channel_locations cl ON cl.location_id = lv.location_id AND cl.channel_id = sqlc.arg(channel_id)::uuid
    WHERE lv.inventory_item_id = ii.id), 0)::float8                                       AS channel_salable,
  COALESCE(ii.backorderable, FALSE)                                                       AS backorderable
FROM catalog_products p
JOIN catalog_product_channel_listings l
  ON l.product_id = p.id
 AND l.channel_id = sqlc.arg(channel_id)::uuid
 AND l.is_published = TRUE
LEFT JOIN catalog_product_content c
  ON c.product_id = p.id AND c.locale = sqlc.arg(locale)::text
LEFT JOIN inventory_items ii ON ii.product_id = p.id
WHERE p.seo_noindex = FALSE
ORDER BY p.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);
