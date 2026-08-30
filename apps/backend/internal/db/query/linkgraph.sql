-- name: LinkGraphNodes :many
WITH canon AS (
    SELECT DISTINCT ON (entity_type, entity_id)
        entity_type,
        entity_id,
        request_path
    FROM url_rewrites
    WHERE redirect_type = 0
      AND (channel_id = sqlc.arg(channel_id)::uuid OR channel_id IS NULL)
    -- A per-channel row beats the global one for the same entity
    ORDER BY entity_type, entity_id, (channel_id IS NOT NULL) DESC
)
SELECT 'product'::text AS node_type, p.id AS node_id, COALESCE(nm.name, p.sku)::text AS label,
       c.request_path::text AS path, p.seo_noindex AS noindex, false AS is_homepage
FROM catalog_products p
JOIN canon c ON c.entity_type = 'product' AND c.entity_id = p.id
JOIN catalog_product_channel_listings l ON l.product_id = p.id
    AND l.channel_id = sqlc.arg(channel_id)::uuid AND l.is_published
LEFT JOIN LATERAL (
    SELECT pc.name FROM catalog_product_content pc WHERE pc.product_id = p.id
    ORDER BY (pc.locale = sqlc.arg(locale)::text) DESC, pc.locale LIMIT 1
) nm ON true

UNION ALL
SELECT 'category'::text, cat.id, COALESCE(nm.name, cat.slug)::text,
       c.request_path::text, cat.seo_noindex, false
FROM catalog_categories cat
JOIN canon c ON c.entity_type = 'category' AND c.entity_id = cat.id
JOIN catalog_category_channel_listings l ON l.category_id = cat.id
    AND l.channel_id = sqlc.arg(channel_id)::uuid AND l.is_published
LEFT JOIN LATERAL (
    SELECT cc.name FROM catalog_category_content cc WHERE cc.category_id = cat.id
    ORDER BY (cc.locale = sqlc.arg(locale)::text) DESC, cc.locale LIMIT 1
) nm ON true
WHERE cat.is_published

UNION ALL
SELECT 'collection'::text, col.id, COALESCE(nm.name, col.slug)::text,
       c.request_path::text, col.seo_noindex, false
FROM catalog_collections col
JOIN canon c ON c.entity_type = 'collection' AND c.entity_id = col.id
LEFT JOIN LATERAL (
    SELECT cc.name FROM catalog_collection_content cc WHERE cc.collection_id = col.id
    ORDER BY (cc.locale = sqlc.arg(locale)::text) DESC, cc.locale LIMIT 1
) nm ON true
WHERE col.is_published

UNION ALL
SELECT 'cms_page'::text, pg.id, COALESCE(nm.title, pg.slug)::text,
       c.request_path::text, (pg.seo_noindex OR l.seo_noindex), l.is_homepage
FROM cms_pages pg
JOIN canon c ON c.entity_type = 'cms_page' AND c.entity_id = pg.id
JOIN cms_page_channel_listings l ON l.page_id = pg.id
    AND l.channel_id = sqlc.arg(channel_id)::uuid AND l.is_published
LEFT JOIN LATERAL (
    SELECT pc.title FROM cms_page_content pc WHERE pc.page_id = pg.id
    ORDER BY (pc.locale = sqlc.arg(locale)::text) DESC, pc.locale LIMIT 1
) nm ON true
WHERE pg.is_published

UNION ALL
SELECT 'blog_post'::text, bp.id, COALESCE(nm.title, bp.slug)::text,
       c.request_path::text, (bp.seo_noindex OR l.seo_noindex), false
FROM blog_posts bp
JOIN canon c ON c.entity_type = 'blog_post' AND c.entity_id = bp.id
JOIN blog_post_channel_listings l ON l.post_id = bp.id
    AND l.channel_id = sqlc.arg(channel_id)::uuid AND l.is_published
LEFT JOIN LATERAL (
    SELECT bc.title FROM blog_post_content bc WHERE bc.post_id = bp.id
    ORDER BY (bc.locale = sqlc.arg(locale)::text) DESC, bc.locale LIMIT 1
) nm ON true

UNION ALL
SELECT 'blog_category'::text, bc.id, COALESCE(nm.name, bc.slug)::text,
       c.request_path::text, bc.seo_noindex, false
FROM blog_categories bc
JOIN canon c ON c.entity_type = 'blog_category' AND c.entity_id = bc.id
LEFT JOIN LATERAL (
    SELECT cc.name FROM blog_category_content cc WHERE cc.category_id = bc.id
    ORDER BY (cc.locale = sqlc.arg(locale)::text) DESC, cc.locale LIMIT 1
) nm ON true
WHERE bc.is_published

UNION ALL
SELECT 'blog_tag'::text, bt.id, COALESCE(nm.name, bt.slug)::text,
       c.request_path::text, false, false
FROM blog_tags bt
JOIN canon c ON c.entity_type = 'blog_tag' AND c.entity_id = bt.id
LEFT JOIN LATERAL (
    SELECT tc.name FROM blog_tag_content tc WHERE tc.tag_id = bt.id
    ORDER BY (tc.locale = sqlc.arg(locale)::text) DESC, tc.locale LIMIT 1
) nm ON true
WHERE bt.is_published

UNION ALL
SELECT 'blog_author'::text, ba.id, COALESCE(nm.name, ba.display_name)::text,
       c.request_path::text, false, false
FROM blog_authors ba
JOIN canon c ON c.entity_type = 'blog_author' AND c.entity_id = ba.id
LEFT JOIN LATERAL (
    SELECT ac.name FROM blog_author_content ac WHERE ac.author_id = ba.id
    ORDER BY (ac.locale = sqlc.arg(locale)::text) DESC, ac.locale LIMIT 1
) nm ON true

UNION ALL
SELECT 'menu'::text, m.id, m.name::text, ''::text, false, false -- a menu is not a page, but it links from every page - as a root it makes click depth come out right
FROM menus m
JOIN menu_channel_assignments a ON a.menu_id = m.id AND a.channel_id = sqlc.arg(channel_id)::uuid;
