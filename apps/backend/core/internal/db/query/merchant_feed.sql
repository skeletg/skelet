-- Google Merchant product feed page - one row per purchasable product published on the channel: channel-priced, requested-locale content falling back to the default locale's, first ready image, keyset on product id
-- Configurable and grouped parents aren't sold themselves and a bundle's price hangs on the shopper's picks, so all three are left out - a variant groups under its parent's SKU, needs that parent published, and links to its parent when it has no path of its own
-- The path is channel-first like the sitemap: the listing's url override, then the channel's canonical rewrite, then the global one

-- name: MerchantFeedProducts :many
SELECT
  p.id,
  p.sku,
  p.kind,
  COALESCE(parent.sku, '')::text AS item_group_id,
  COALESCE(NULLIF(l.url, ''),
    (SELECT ur.request_path FROM url_rewrites ur
      WHERE ur.entity_type = 'product' AND ur.entity_id = p.id AND ur.redirect_type = 0
        AND (ur.channel_id = sqlc.arg(channel_id)::uuid OR ur.channel_id IS NULL)
      ORDER BY ur.channel_id NULLS LAST LIMIT 1),
    NULLIF(parent_l.url, ''),
    (SELECT ur.request_path FROM url_rewrites ur
      WHERE ur.entity_type = 'product' AND ur.entity_id = parent.id AND ur.redirect_type = 0
        AND (ur.channel_id = sqlc.arg(channel_id)::uuid OR ur.channel_id IS NULL)
      ORDER BY ur.channel_id NULLS LAST LIMIT 1),
    parent.url,
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
  COALESCE(NULLIF(c.name, ''), NULLIF(cd.name, ''), p.sku)::text                          AS title,
  COALESCE(NULLIF(c.meta_description, ''), NULLIF(c.short_description ->> 'plain', ''), NULLIF(c.description ->> 'plain', ''),
    NULLIF(cd.meta_description, ''), NULLIF(cd.short_description ->> 'plain', ''), NULLIF(cd.description ->> 'plain', ''),
    '')::text                                                                             AS description,
  COALESCE(NULLIF(p.attributes ->> sqlc.arg(brand_attribute)::text, ''),
    (SELECT COALESCE(NULLIF(vc.name, ''), NULLIF(vd.name, ''), v.name)
     FROM catalog_product_attribute_values pv
     JOIN catalog_attributes a ON a.id = pv.attribute_id
     JOIN catalog_attribute_values v ON v.id = pv.attribute_value_id
     LEFT JOIN catalog_attribute_value_content vc ON vc.value_id = v.id AND vc.locale = sqlc.arg(locale)::text
     LEFT JOIN catalog_attribute_value_content vd ON vd.value_id = v.id AND vd.locale = sqlc.arg(default_locale)::text
     WHERE pv.product_id = p.id AND a.slug = sqlc.arg(brand_attribute)::text
     ORDER BY pv.sort_order, v.id LIMIT 1), '')::text AS brand,
  COALESCE((SELECT jsonb_object_agg(a.slug, COALESCE(NULLIF(vc.name, ''), NULLIF(vd.name, ''), v.name))
    FROM catalog_product_configurable_options o
    JOIN catalog_attributes a ON a.id = o.attribute_id
    JOIN catalog_product_attribute_values pv ON pv.attribute_id = o.attribute_id AND pv.product_id = p.id
    JOIN catalog_attribute_values v ON v.id = pv.attribute_value_id
    LEFT JOIN catalog_attribute_value_content vc ON vc.value_id = v.id AND vc.locale = sqlc.arg(locale)::text
    LEFT JOIN catalog_attribute_value_content vd ON vd.value_id = v.id AND vd.locale = sqlc.arg(default_locale)::text
    WHERE o.product_id = parent.id), '{}'::jsonb)::jsonb AS variant_options,
  COALESCE(
    (SELECT m.s3_key FROM catalog_product_media m
      WHERE m.product_id = p.id AND m.status = 'ready'
      ORDER BY m.sort_order, m.created_at LIMIT 1),
    '')::text                                                                             AS image_key,
  COALESCE(ii.tracked, FALSE)                                                             AS tracked,
  COALESCE(ii.backorderable, FALSE)                                                       AS backorderable
FROM catalog_products p
JOIN catalog_product_channel_listings l
  ON l.product_id = p.id
 AND l.channel_id = sqlc.arg(channel_id)::uuid
 AND l.is_published = TRUE
LEFT JOIN catalog_product_children pc ON pc.child_id = p.id
LEFT JOIN catalog_products parent ON parent.id = pc.parent_id
LEFT JOIN catalog_product_channel_listings parent_l
  ON parent_l.product_id = parent.id AND parent_l.channel_id = sqlc.arg(channel_id)::uuid
LEFT JOIN catalog_product_content c
  ON c.product_id = p.id AND c.locale = sqlc.arg(locale)::text
LEFT JOIN catalog_product_content cd
  ON cd.product_id = p.id AND cd.locale = sqlc.arg(default_locale)::text
LEFT JOIN inventory_items ii ON ii.product_id = p.id
WHERE p.seo_noindex = FALSE
  AND p.kind NOT IN ('configurable', 'grouped', 'bundle')
  AND (parent.id IS NULL OR parent_l.is_published IS TRUE)
  AND p.id > sqlc.arg(after_id)::uuid
ORDER BY p.id
LIMIT sqlc.arg(page_limit);

-- name: MerchantFeedKitComponents :many
-- Every component of a page's kits, so their availability can come from the scarcest one
SELECT product_id AS kit_id, component_product_id, quantity
FROM catalog_product_kit_components
WHERE product_id = ANY(sqlc.arg(kit_ids)::uuid[]);
