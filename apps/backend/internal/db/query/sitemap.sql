-- Catalog sitemap feeds: canonical paths for entities published on a channel
-- Channel-first: a channel-scoped canonical rewrite (redirect_type = 0)
-- overrides the global one, and a product listing url_path override (the
-- channel's own URL vocabulary) wins over both. seo_noindex entities are
-- excluded - they should not be advertised to crawlers

-- name: SitemapProductPaths :many
SELECT DISTINCT ON (ur.entity_id)
  COALESCE(NULLIF(l.url, ''), ur.request_path)::text AS path,
  GREATEST(ur.updated_at, p.updated_at)::timestamptz      AS updated_at
FROM url_rewrites ur
JOIN catalog_products p ON p.id = ur.entity_id
JOIN catalog_product_channel_listings l
  ON l.product_id = p.id AND l.channel_id = $1 AND l.is_published = TRUE
WHERE ur.entity_type = 'product'
  AND ur.redirect_type = 0
  AND (ur.channel_id = $1 OR ur.channel_id IS NULL)
  AND p.seo_noindex = FALSE
ORDER BY ur.entity_id, ur.channel_id NULLS LAST
LIMIT $2 OFFSET $3;

-- name: SitemapProductCount :one
SELECT COUNT(DISTINCT ur.entity_id)
FROM url_rewrites ur
JOIN catalog_products p ON p.id = ur.entity_id
JOIN catalog_product_channel_listings l
  ON l.product_id = p.id AND l.channel_id = $1 AND l.is_published = TRUE
WHERE ur.entity_type = 'product'
  AND ur.redirect_type = 0
  AND (ur.channel_id = $1 OR ur.channel_id IS NULL)
  AND p.seo_noindex = FALSE;

-- name: SitemapCategoryPaths :many
SELECT DISTINCT ON (ur.entity_id)
  ur.request_path::text                              AS path,
  GREATEST(ur.updated_at, c.updated_at)::timestamptz AS updated_at
FROM url_rewrites ur
JOIN catalog_categories c ON c.id = ur.entity_id
JOIN catalog_category_channel_listings cl
  ON cl.category_id = c.id AND cl.channel_id = $1 AND cl.is_published = TRUE
WHERE ur.entity_type = 'category'
  AND ur.redirect_type = 0
  AND (ur.channel_id = $1 OR ur.channel_id IS NULL)
  AND c.is_published = TRUE
  AND c.seo_noindex = FALSE
ORDER BY ur.entity_id, ur.channel_id NULLS LAST
LIMIT $2 OFFSET $3;

-- name: SitemapCategoryCount :one
SELECT COUNT(DISTINCT ur.entity_id)
FROM url_rewrites ur
JOIN catalog_categories c ON c.id = ur.entity_id
JOIN catalog_category_channel_listings cl
  ON cl.category_id = c.id AND cl.channel_id = $1 AND cl.is_published = TRUE
WHERE ur.entity_type = 'category'
  AND ur.redirect_type = 0
  AND (ur.channel_id = $1 OR ur.channel_id IS NULL)
  AND c.is_published = TRUE
  AND c.seo_noindex = FALSE;

-- name: SitemapCollectionPaths :many
SELECT DISTINCT ON (ur.entity_id)
  ur.request_path::text                              AS path,
  GREATEST(ur.updated_at, c.updated_at)::timestamptz AS updated_at
FROM url_rewrites ur
JOIN catalog_collections c ON c.id = ur.entity_id
WHERE ur.entity_type = 'collection'
  AND ur.redirect_type = 0
  AND (ur.channel_id = $1 OR ur.channel_id IS NULL)
  AND c.is_published = TRUE
  AND c.seo_noindex = FALSE
ORDER BY ur.entity_id, ur.channel_id NULLS LAST
LIMIT $2 OFFSET $3;

-- name: SitemapCollectionCount :one
SELECT COUNT(DISTINCT ur.entity_id)
FROM url_rewrites ur
JOIN catalog_collections c ON c.id = ur.entity_id
WHERE ur.entity_type = 'collection'
  AND ur.redirect_type = 0
  AND (ur.channel_id = $1 OR ur.channel_id IS NULL)
  AND c.is_published = TRUE
  AND c.seo_noindex = FALSE;
