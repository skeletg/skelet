-- ==================== Categories ====================

-- name: CreateCategory :one
INSERT INTO catalog_categories (parent_id, slug, url_path, position, is_published, seo_noindex, metadata, private_metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetCategoryByID :one
SELECT * FROM catalog_categories WHERE id = $1;

-- name: ListCategoryChain :many
-- The category (depth 0) and its ancestors nearest first - the depth cap stops a parent cycle recursing forever
WITH RECURSIVE chain(id, parent_id, depth) AS (
    SELECT c.id, c.parent_id, 0 FROM catalog_categories c WHERE c.id = sqlc.arg(id)::uuid
    UNION ALL
    SELECT p.id, p.parent_id, chain.depth + 1
    FROM catalog_categories p
    JOIN chain ON p.id = chain.parent_id
    WHERE chain.depth < 64
)
SELECT c.*
FROM chain
JOIN catalog_categories c ON c.id = chain.id
ORDER BY chain.depth;

-- name: GetCategoryBySlug :one
SELECT * FROM catalog_categories WHERE slug = $1;

-- name: ListRootCategories :many
SELECT * FROM catalog_categories WHERE parent_id IS NULL ORDER BY position, slug;

-- name: ListAllCategories :many
SELECT * FROM catalog_categories ORDER BY url_path, position, slug;

-- name: ListCategoryChildren :many
SELECT * FROM catalog_categories WHERE parent_id = $1 ORDER BY position, slug;

-- name: UpdateCategory :one
UPDATE catalog_categories
SET slug             = $2,
    url_path         = $3,
    parent_id        = $4,
    position         = $5,
    is_published     = $6,
    seo_noindex      = $7,
    metadata         = $8,
    private_metadata = $9,
    updated_at       = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateCategoryURLPath :exec
UPDATE catalog_categories SET url_path = $2, updated_at = NOW() WHERE id = $1;

-- name: DeleteCategory :exec
DELETE FROM catalog_categories WHERE id = $1;

-- ==================== Cover image ====================

-- name: SetCategoryCover :one
UPDATE catalog_categories
SET cover_id         = $2,
    cover_s3_key     = $3,
    cover_mime_type  = $4,
    cover_size_bytes = $5,
    cover_alt        = $6,
    cover_status     = 'pending',
    cover_width      = NULL,
    cover_height     = NULL,
    updated_at       = NOW()
WHERE id = $1
RETURNING *;

-- name: ConfirmCategoryCover :one
UPDATE catalog_categories
SET cover_width  = $2,
    cover_height = $3,
    cover_status = 'ready',
    updated_at   = NOW()
WHERE id = $1
RETURNING *;

-- name: ClearCategoryCover :one
UPDATE catalog_categories
SET cover_id         = NULL,
    cover_s3_key     = NULL,
    cover_mime_type  = NULL,
    cover_size_bytes = NULL,
    cover_width      = NULL,
    cover_height     = NULL,
    cover_status     = NULL,
    cover_alt        = '',
    updated_at       = NOW()
WHERE id = $1
RETURNING *;

-- ==================== Category Content ====================

-- name: UpsertCategoryContent :one
INSERT INTO catalog_category_content (category_id, locale, name, short_description, description, meta_title, meta_description, meta_keywords)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (category_id, locale) DO UPDATE
    SET name              = EXCLUDED.name,
        short_description = EXCLUDED.short_description,
        description       = EXCLUDED.description,
        meta_title        = EXCLUDED.meta_title,
        meta_description  = EXCLUDED.meta_description,
        meta_keywords     = EXCLUDED.meta_keywords,
        updated_at        = NOW()
RETURNING *;

-- name: GetCategoryContent :one
SELECT * FROM catalog_category_content WHERE category_id = $1 AND locale = $2;

-- name: ListCategoryContent :many
SELECT * FROM catalog_category_content WHERE category_id = $1 ORDER BY locale;

-- name: ListCategoryContentByScope :many
SELECT * FROM catalog_category_content
WHERE category_id = ANY(sqlc.arg(category_ids)::uuid[])
  AND locale      = sqlc.arg(locale)::text
ORDER BY category_id;

-- name: ListCategoryContentForCategories :many
-- Every locale's row for a set of categories - the admin tree and child lists answer Category.content in one query
SELECT * FROM catalog_category_content
WHERE category_id = ANY(sqlc.arg(category_ids)::uuid[])
ORDER BY category_id, locale;

-- name: DeleteCategoryContent :execrows
DELETE FROM catalog_category_content WHERE category_id = $1 AND locale = $2;

-- ==================== Product-Category junction ====================

-- name: AddProductToCategory :exec
INSERT INTO catalog_product_categories (product_id, category_id, position, is_primary)
VALUES ($1, $2, $3, $4);

-- name: RemoveProductFromCategory :execrows
DELETE FROM catalog_product_categories WHERE product_id = $1 AND category_id = $2;

-- name: SetPrimaryProductCategory :exec
UPDATE catalog_product_categories
SET is_primary = (category_id = sqlc.arg(category_id)::uuid)
WHERE product_id = sqlc.arg(product_id)::uuid;

-- name: ListProductsByCategory :many
-- row_limit 0 means every member
SELECT p.*
FROM catalog_product_categories pc
JOIN catalog_products p ON p.id = pc.product_id
WHERE pc.category_id = sqlc.arg(category_id)
ORDER BY pc.position, p.url
LIMIT NULLIF(sqlc.arg(row_limit)::int, 0);

-- name: CountProductsInCategory :one
-- Direct membership count (products explicitly assigned to this category, not its descendants) - backs admin Category.productCount; served by the idx_product_categories_category(category_id, position) index
SELECT COUNT(*) FROM catalog_product_categories WHERE category_id = $1;

-- name: ListCategoriesByProduct :many
SELECT c.*, pc.is_primary AS pc_is_primary
FROM catalog_product_categories pc
JOIN catalog_categories c ON c.id = pc.category_id
WHERE pc.product_id = $1
ORDER BY pc.is_primary DESC, pc.position;

-- name: ListCategoriesByProductIDs :many
SELECT c.*,
       pc.product_id AS pc_product_id,
       pc.is_primary AS pc_is_primary,
       pc.position   AS pc_position
FROM catalog_product_categories pc
JOIN catalog_categories c ON c.id = pc.category_id
WHERE pc.product_id = ANY(sqlc.arg(product_ids)::uuid[])
ORDER BY pc.product_id, pc.is_primary DESC, pc.position;

-- name: ListPrimaryCategoryByProductIDs :many
SELECT c.*, pc.product_id AS pc_product_id
FROM catalog_product_categories pc
JOIN catalog_categories c ON c.id = pc.category_id
WHERE pc.product_id = ANY(sqlc.arg(product_ids)::uuid[])
  AND pc.is_primary = TRUE;

-- name: ListProductIDsInPrimaryCategoryDescendants :many
-- Recursive CTE: walks every descendant of the root category (inclusive of the root itself), then projects to products that hold ANY of those categories as their primary - used by UpdateCategory to cascade url_path regen to products whose templated URL depends on the category's prefix
WITH RECURSIVE subtree(id) AS (
    SELECT cc.id FROM catalog_categories cc WHERE cc.id = $1
    UNION ALL
    SELECT cc.id
    FROM catalog_categories cc
    JOIN subtree s ON cc.parent_id = s.id
)
SELECT DISTINCT pc.product_id
FROM catalog_product_categories pc
JOIN subtree s ON pc.category_id = s.id
WHERE pc.is_primary = TRUE;

-- ==================== Category Store Listings ==================== Per (category, store) row gating which categories
-- appear on each storefront - mirrors blog_post_channel_listings / product_store_listings

-- name: UpsertCatalogCategoryChannelListing :one
INSERT INTO catalog_category_channel_listings (category_id, channel_id, is_published, sort_order_override)
VALUES ($1, $2, $3, $4)
ON CONFLICT (category_id, channel_id) DO UPDATE
SET is_published        = EXCLUDED.is_published,
    sort_order_override = EXCLUDED.sort_order_override,
    updated_at          = NOW()
RETURNING *;

-- name: DeleteCatalogCategoryChannelListing :execrows
DELETE FROM catalog_category_channel_listings
WHERE category_id = $1 AND channel_id = $2;

-- name: ListCatalogCategoryChannelListings :many
SELECT * FROM catalog_category_channel_listings
WHERE category_id = $1 ORDER BY channel_id;

-- name: ListCatalogCategoryChannelListingsByCategoryIDs :many
SELECT * FROM catalog_category_channel_listings
WHERE category_id = ANY(sqlc.arg(category_ids)::uuid[]);

-- name: GetCatalogCategoryChannelListing :one
SELECT * FROM catalog_category_channel_listings
WHERE category_id = $1 AND channel_id = $2;

-- name: ListPublishedCategoriesForChannel :many
-- Storefront query: categories that are listed+published on the given store, returning each row joined with the global category - caller can filter further (root only, by parent, etc.) in Go since the tree is small enough that fancy SQL recursion isn't worth it yet
SELECT c.*, ccsl.sort_order_override
FROM catalog_categories c
JOIN catalog_category_channel_listings ccsl ON ccsl.category_id = c.id
WHERE ccsl.channel_id = $1 AND ccsl.is_published = TRUE
  AND c.is_published = TRUE;

-- name: ListPublishedRootCategoriesForChannel :many
SELECT c.*, ccsl.sort_order_override
FROM catalog_categories c
JOIN catalog_category_channel_listings ccsl ON ccsl.category_id = c.id
WHERE ccsl.channel_id = $1 AND ccsl.is_published = TRUE
  AND c.is_published = TRUE
  AND c.parent_id IS NULL;

-- name: ListPublishedCategoryChildrenForChannel :many
SELECT c.*, ccsl.sort_order_override
FROM catalog_categories c
JOIN catalog_category_channel_listings ccsl ON ccsl.category_id = c.id
WHERE ccsl.channel_id = $1 AND ccsl.is_published = TRUE
  AND c.is_published = TRUE
  AND c.parent_id = $2;

-- name: ListPublishedCategoryIDsForChannelFiltered :many
-- Direct-publication batch filter for nested category references on the front endpoint (Product.categories, Product.primaryCategory) - returns the subset of candidate IDs whose own (category, channel) listing is published AND whose own row carries is_published=TRUE - does NOT walk the ancestor chain - affiliation listings don't need breadcrumb integrity; callers that need it use ListFullyVisible*
SELECT c.id FROM catalog_categories c
JOIN catalog_category_channel_listings l
  ON l.category_id = c.id
WHERE c.id = ANY(sqlc.arg(category_ids)::uuid[])
  AND l.channel_id = sqlc.arg(channel_id)::uuid
  AND l.is_published = TRUE
  AND c.is_published = TRUE;

-- name: ListFullyVisibleCategoryIDsForChannelFiltered :many
-- Strict-chain filter for breadcrumb/parent enforcement - returns the subset of candidates where the category AND every ancestor up to the root is listed-and-published on the store - implemented as a recursive CTE walking parent_id; outer aggregation keeps only origins whose entire chain evaluated to visible=TRUE
WITH RECURSIVE chain(origin_id, cur_id, parent_id) AS (
    SELECT c.id, c.id, c.parent_id
      FROM catalog_categories c
     WHERE c.id = ANY(sqlc.arg(category_ids)::uuid[])
  UNION ALL
    SELECT ch.origin_id, c.id, c.parent_id
      FROM catalog_categories c
      JOIN chain ch ON c.id = ch.parent_id
),
chain_visibility AS (
    SELECT ch.origin_id,
           (cc.is_published
            AND COALESCE(l.is_published, FALSE)) AS visible
      FROM chain ch
      JOIN catalog_categories cc ON cc.id = ch.cur_id
      LEFT JOIN catalog_category_channel_listings l
        ON l.category_id = ch.cur_id
       AND l.channel_id = sqlc.arg(channel_id)::uuid
)
SELECT origin_id
  FROM chain_visibility
GROUP BY origin_id
HAVING bool_and(visible) = TRUE;

-- name: ListCategoryFacetOverrides :many
-- Joined to the attribute for its slug - browse facets key on slug, the override
-- rows key on id so attribute renames can't orphan them
SELECT o.attribute_id, a.slug AS attribute_slug, o.sort_order, o.is_hidden
  FROM category_facet_overrides o
  JOIN catalog_attributes a ON a.id = o.attribute_id
 WHERE o.category_id = $1
 ORDER BY o.sort_order, a.slug;

-- name: DeleteCategoryFacetOverrides :exec
DELETE FROM category_facet_overrides WHERE category_id = $1;

-- name: InsertCategoryFacetOverride :exec
INSERT INTO category_facet_overrides (category_id, attribute_id, sort_order, is_hidden)
VALUES ($1, $2, $3, $4);
