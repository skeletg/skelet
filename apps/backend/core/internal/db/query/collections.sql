-- ==================== Collections ====================

-- name: CreateCollection :one
INSERT INTO catalog_collections (slug, is_published, seo_noindex, metadata, private_metadata)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetCollectionByID :one
SELECT * FROM catalog_collections WHERE id = $1;

-- name: ListCollections :many
SELECT * FROM catalog_collections ORDER BY created_at DESC;

-- name: UpdateCollection :one
UPDATE catalog_collections
SET slug             = $2,
    is_published     = $3,
    seo_noindex      = $4,
    metadata         = $5,
    private_metadata = $6,
    updated_at       = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteCollection :exec
DELETE FROM catalog_collections WHERE id = $1;

-- ==================== Cover image ====================

-- name: SetCollectionCover :one
UPDATE catalog_collections
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

-- name: ConfirmCollectionCover :one
UPDATE catalog_collections
SET cover_width  = $2,
    cover_height = $3,
    cover_status = 'ready',
    updated_at   = NOW()
WHERE id = $1
RETURNING *;

-- name: ClearCollectionCover :one
UPDATE catalog_collections
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

-- ==================== Collection Content ====================

-- name: UpsertCollectionContent :one
INSERT INTO catalog_collection_content (collection_id, locale, name, description, meta_title, meta_description, meta_keywords)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (collection_id, locale) DO UPDATE
    SET name             = EXCLUDED.name,
        description      = EXCLUDED.description,
        meta_title       = EXCLUDED.meta_title,
        meta_description = EXCLUDED.meta_description,
        meta_keywords    = EXCLUDED.meta_keywords,
        updated_at       = NOW()
RETURNING *;

-- name: GetCollectionContent :one
SELECT * FROM catalog_collection_content
WHERE collection_id = $1 AND locale = $2;

-- name: ListCollectionContentForCollections :many
-- Every locale's row for a set of collections - admin collection lists answer Collection.content in one query
SELECT * FROM catalog_collection_content
WHERE collection_id = ANY(sqlc.arg(collection_ids)::uuid[])
ORDER BY collection_id, locale;

-- name: ListCollectionContent :many
SELECT * FROM catalog_collection_content
WHERE collection_id = $1
ORDER BY locale;

-- name: ListCollectionContentByScope :many
-- Batch-loads content for a set of collections at one specific locale
SELECT * FROM catalog_collection_content
WHERE collection_id = ANY(sqlc.arg(collection_ids)::uuid[])
  AND locale        = sqlc.arg(locale)::text
ORDER BY collection_id;

-- name: DeleteCollectionContent :execrows
DELETE FROM catalog_collection_content
WHERE collection_id = $1 AND locale = $2;

-- ==================== Collection Products ====================

-- name: AddProductToCollection :exec
INSERT INTO catalog_collection_products (collection_id, product_id, sort_order)
VALUES ($1, $2, $3);

-- name: RemoveProductFromCollection :execrows
DELETE FROM catalog_collection_products
WHERE collection_id = $1 AND product_id = $2;

-- name: ReorderCollectionProduct :exec
UPDATE catalog_collection_products
SET sort_order = $3
WHERE collection_id = $1 AND product_id = $2;

-- name: ListProductsByCollection :many
SELECT p.*
FROM catalog_collection_products cp
JOIN catalog_products p ON p.id = cp.product_id
WHERE cp.collection_id = $1
ORDER BY cp.sort_order, p.url;

-- name: ListCollectionsByProduct :many
SELECT c.*
FROM catalog_collection_products cp
JOIN catalog_collections c ON c.id = cp.collection_id
WHERE cp.product_id = $1
ORDER BY c.slug;

-- name: ListCollectionsByProductIDs :many
SELECT cp.product_id, c.*
FROM catalog_collection_products cp
JOIN catalog_collections c ON c.id = cp.collection_id
WHERE cp.product_id = ANY(sqlc.arg(product_ids)::uuid[])
ORDER BY cp.product_id, c.slug;

-- name: ListCollectionFacetOverrides :many
-- Joined for the slug - browse facets key on slug, override rows key on id
SELECT o.attribute_id, a.slug AS attribute_slug, o.sort_order, o.is_hidden
  FROM collection_facet_overrides o
  JOIN catalog_attributes a ON a.id = o.attribute_id
 WHERE o.collection_id = $1
 ORDER BY o.sort_order, a.slug;

-- name: DeleteCollectionFacetOverrides :exec
DELETE FROM collection_facet_overrides WHERE collection_id = $1;

-- name: InsertCollectionFacetOverride :exec
INSERT INTO collection_facet_overrides (collection_id, attribute_id, sort_order, is_hidden)
VALUES ($1, $2, $3, $4);
