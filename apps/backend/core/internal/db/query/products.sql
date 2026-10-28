-- ==================== Products ====================

-- name: CreateProduct :one
INSERT INTO catalog_products (
    product_type_id,
    default_price, default_currency_code, sku, seo_noindex, canonical_url, url,
    url_auto, url_template_override,
    tracks_inventory_override,
    quantity_decimals, quantity_step,
    attributes, metadata, private_metadata,
    kind
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING *;

-- name: GetProductByID :one
SELECT * FROM catalog_products WHERE id = $1;

-- name: GetProductByIDForUpdate :one
-- UpdateProduct merges the input over this row in Go, so the lock keeps a concurrent edit from being overwritten
SELECT * FROM catalog_products WHERE id = $1 FOR UPDATE;

-- name: CountActiveCartReferencesToSKU :one
SELECT ((SELECT COUNT(*) FROM cart_lines cl JOIN carts c ON c.id = cl.cart_id
         WHERE c.status = 'active' AND cl.product_sku = sqlc.arg(sku)::text)
      + (SELECT COUNT(*) FROM cart_saved_items si JOIN carts c ON c.id = si.cart_id
         WHERE c.status = 'active' AND si.product_sku = sqlc.arg(sku)::text)
      + (SELECT COUNT(*) FROM cart_line_components lc JOIN cart_lines cl ON cl.id = lc.cart_line_id JOIN carts c ON c.id = cl.cart_id
         WHERE c.status = 'active' AND lc.product_sku = sqlc.arg(sku)::text))::bigint;

-- Batched product loader for the storefront browse hydration - one query replaces N GetProductByID round-trips - order is restored Go-side from the caller's id slice
-- name: GetProductsByIDs :many
SELECT * FROM catalog_products WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListProductTypeIDsByProductIDs :many
SELECT id, product_type_id FROM catalog_products WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: GetPublishedProductForChannel :one
-- Front endpoint single-product fetch - returns the product only when a listing exists on the given channel with is_published=TRUE - mirrors the (product, channel) gating that ListPublishedRootCategoriesForChannel applies to categories
SELECT p.* FROM catalog_products p
JOIN catalog_product_channel_listings l
  ON l.product_id = p.id
WHERE p.id = $1
  AND l.channel_id = $2
  AND l.is_published = TRUE;

-- name: ListPublishedProductIDsForStoreFiltered :many
-- Batch-filter helper used by nested-field resolvers (bundle selections, grouped members, configurable children, category products) - given a set of candidate product IDs, returns the subset that is published on this channel - empty input returns no rows
SELECT p.id FROM catalog_products p
JOIN catalog_product_channel_listings l
  ON l.product_id = p.id
WHERE p.id = ANY(sqlc.arg(product_ids)::uuid[])
  AND l.channel_id = sqlc.arg(channel_id)::uuid
  AND l.is_published = TRUE;

-- name: GetProductBySKU :one
SELECT * FROM catalog_products WHERE sku = $1 LIMIT 1;

-- name: GetProductsBySKUs :many
-- Batch form of GetProductBySKU: resolve many SKUs in one query - powers the cart-render hydration (names + images) with one query rather than a lookup per line + component
SELECT * FROM catalog_products WHERE sku = ANY(sqlc.arg(skus)::text[]);

-- name: CountProductsUsingConfigurableOption :one
-- How many products have this attribute as a configurable option (any type)
SELECT COUNT(*)::bigint FROM catalog_product_configurable_options WHERE attribute_id = $1;

-- name: CountProductsUsingConfigurableOptionForType :one
-- How many products of a specific product type have this attribute as a
-- configurable option - used to block unassigning an attribute from a type
SELECT COUNT(*)::bigint
FROM catalog_product_configurable_options pco
JOIN catalog_products p ON p.id = pco.product_id
WHERE pco.attribute_id = $1 AND p.product_type_id = $2;

-- name: ListProductParents :many
SELECT p.*
FROM catalog_product_children pc
JOIN catalog_products p ON p.id = pc.parent_id
WHERE pc.child_id = $1
ORDER BY p.url;

-- name: ListProducts :many
-- row_limit 0 means every row - only the CSV export needs that, and the GraphQL surface never does: an uncapped list is 101k rows and ~17MB on a real catalog, which pins a connection for ~20s and is reachable by any admin who can view the catalog
SELECT * FROM catalog_products ORDER BY created_at DESC LIMIT NULLIF(sqlc.arg(row_limit)::int, 0);

-- name: ListProductsForChannel :many
SELECT p.* FROM catalog_products p
WHERE EXISTS (SELECT 1 FROM catalog_product_channel_listings l WHERE l.product_id = p.id AND l.channel_id = sqlc.arg(channel_id))
ORDER BY p.created_at DESC LIMIT NULLIF(sqlc.arg(row_limit)::int, 0);

-- name: UpdateProduct :one
UPDATE catalog_products
SET default_price              = $2,
    default_currency_code      = $3,
    sku                        = $4,
    seo_noindex                = $5,
    canonical_url              = $6,
    url                        = $7,
    url_auto                   = $8,
    url_template_override      = $9,
    tracks_inventory_override  = $10,
    quantity_decimals          = $11,
    quantity_step              = $12,
    metadata                   = $13,
    private_metadata           = $14,
    compare_at_price           = $15,
    special_price              = $16,
    special_from               = $17,
    special_to                 = $18,
    weight_grams               = $19,
    length_mm                  = $20,
    width_mm                   = $21,
    height_mm                  = $22,
    min_order_qty              = $23,
    max_order_qty              = $24,
    cost_cents                 = $25,
    updated_at                 = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateProductURL :one
UPDATE catalog_products
SET url        = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListProductsByType :many
SELECT * FROM catalog_products WHERE product_type_id = $1 ORDER BY created_at DESC;

-- name: UpdateProductAttributes :one
UPDATE catalog_products
SET attributes = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteProduct :exec
DELETE FROM catalog_products WHERE id = $1;

-- ==================== Product Attribute Values (selectable) ====================

-- name: ListProductAttributeValues :many
SELECT pav.product_id, pav.attribute_id, pav.attribute_value_id, pav.sort_order,
       av.slug  AS value_slug,
       av.name  AS value_name,
       av.value AS value_hex
FROM catalog_product_attribute_values pav
JOIN catalog_attribute_values av ON av.id = pav.attribute_value_id
WHERE pav.product_id = $1
ORDER BY pav.attribute_id, pav.sort_order;

-- Batched selections loader (browse hydration) - same row shape as
-- ListProductAttributeValues but across many products in one query
-- name: ListProductAttributeSelectionsByProductIDs :many
SELECT pav.product_id, pav.attribute_id, pav.attribute_value_id, pav.sort_order,
       av.slug  AS value_slug,
       av.name  AS value_name,
       av.value AS value_hex
FROM catalog_product_attribute_values pav
JOIN catalog_attribute_values av ON av.id = pav.attribute_value_id
WHERE pav.product_id = ANY(sqlc.arg(product_ids)::uuid[])
ORDER BY pav.product_id, pav.attribute_id, pav.sort_order;

-- name: InsertProductAttributeValue :exec
INSERT INTO catalog_product_attribute_values (product_id, attribute_id, attribute_value_id, sort_order)
VALUES ($1, $2, $3, $4)
ON CONFLICT (product_id, attribute_id, attribute_value_id) DO NOTHING;

-- name: ListProductAttributeValuesByProductIDs :many
SELECT pav.product_id, pav.attribute_id, pav.attribute_value_id
FROM catalog_product_attribute_values pav
WHERE pav.product_id = ANY(sqlc.arg(product_ids)::uuid[])
  AND pav.attribute_id = ANY(sqlc.arg(attribute_ids)::uuid[])
ORDER BY pav.product_id, pav.attribute_id;

-- name: DeleteAllProductAttributeSelections :exec
DELETE FROM catalog_product_attribute_values WHERE product_id = $1;

-- name: ListProductIDsByAttributeValue :many
-- Products that (a) hold the given attribute value AND (b) belong to a product type whose URL template references that attribute AND (c) have url_path_auto = true
-- used by UpdateAttributeValue to cascade slug-change regenerations only where they actually matter, skipping products whose URLs don't depend on the value's slug
SELECT DISTINCT p.id
FROM catalog_product_attribute_values pav
JOIN catalog_products p ON p.id = pav.product_id
JOIN catalog_product_types pt ON pt.id = p.product_type_id
WHERE pav.attribute_value_id = $1
  AND p.url_auto = TRUE
  AND (
    (p.url_template_override <> '' AND p.url_template_override LIKE sqlc.arg(token_pattern)::text)
    OR
    (p.url_template_override = '' AND pt.url_template LIKE sqlc.arg(token_pattern)::text)
  );

-- ==================== Product Attribute References (reference-type attributes) ====================

-- name: InsertProductAttributeReference :exec
INSERT INTO catalog_product_attribute_references (product_id, attribute_id, target_type, target_id, position)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (product_id, attribute_id, target_type, target_id) DO UPDATE SET position = EXCLUDED.position;

-- name: DeleteProductAttributeReferences :exec
-- Clears all references for one (product, attribute) - the service re-sets the
-- whole list, mirroring DeleteAllProductAttributeSelections for the junction
DELETE FROM catalog_product_attribute_references
WHERE product_id = $1 AND attribute_id = $2;

-- name: ListProductAttributeReferencesByProductIDs :many
-- Batched loader (product + children hydration), ordered for stable display
SELECT product_id, attribute_id, target_type, target_id, position
FROM catalog_product_attribute_references
WHERE product_id = ANY(sqlc.arg(product_ids)::uuid[])
ORDER BY product_id, attribute_id, position;

-- ==================== Configurable Options ====================

-- name: SetConfigurableOptions :exec
-- Atomically replaces the configurable options for a product
-- Caller deletes all then re-inserts in a transaction
DELETE FROM catalog_product_configurable_options WHERE product_id = $1;

-- name: InsertConfigurableOption :exec
INSERT INTO catalog_product_configurable_options (product_id, attribute_id, sort_order)
VALUES ($1, $2, $3)
ON CONFLICT (product_id, attribute_id) DO UPDATE SET sort_order = EXCLUDED.sort_order;

-- name: ListConfigurableOptions :many
SELECT a.id, a.slug, a.name, a.input_type, a.unit,
       a.value_required, a.filterable, a.searchable, a.is_public,
       a.facet_collapsed, a.is_indexable, a.h1_position,
       a.sort_order AS attribute_sort_order,
       a.metadata, a.private_metadata,
       a.created_at, a.updated_at,
       pco.sort_order AS option_sort_order
FROM catalog_product_configurable_options pco
JOIN catalog_attributes a ON a.id = pco.attribute_id
WHERE pco.product_id = $1
ORDER BY pco.sort_order, a.name;

-- name: ListConfigurableOptionsByProductIDs :many
SELECT pco.product_id,
       a.id, a.slug, a.name, a.input_type, a.unit,
       a.value_required, a.filterable, a.searchable, a.is_public,
       a.facet_collapsed, a.is_indexable, a.h1_position,
       a.sort_order AS attribute_sort_order,
       a.metadata, a.private_metadata,
       a.created_at, a.updated_at,
       pco.sort_order AS option_sort_order
FROM catalog_product_configurable_options pco
JOIN catalog_attributes a ON a.id = pco.attribute_id
WHERE pco.product_id = ANY(sqlc.arg(product_ids)::uuid[])
ORDER BY pco.product_id, pco.sort_order, a.name;

-- ==================== Product Content (translatable fields) ==================== After Phase 3, translations are keyed by (product_id, locale) only - no per-store dimension - per-store text variation, if ever needed, would come back via a sibling override table, not this primary translation store

-- name: UpsertProductContent :one
INSERT INTO catalog_product_content (product_id, locale, name, short_description, description, meta_title, meta_description, meta_keywords)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (product_id, locale) DO UPDATE
    SET name              = EXCLUDED.name,
        short_description = EXCLUDED.short_description,
        description       = EXCLUDED.description,
        meta_title        = EXCLUDED.meta_title,
        meta_description  = EXCLUDED.meta_description,
        meta_keywords     = EXCLUDED.meta_keywords,
        updated_at        = NOW()
RETURNING *;

-- name: GetProductContent :one
SELECT * FROM catalog_product_content
WHERE product_id = $1 AND locale = $2;

-- name: ListProductContent :many
SELECT * FROM catalog_product_content
WHERE product_id = $1
ORDER BY locale;

-- name: ListProductContentByProductIDs :many
-- Every locale's content for a set of products
SELECT * FROM catalog_product_content
WHERE product_id = ANY(sqlc.arg(product_ids)::uuid[])
ORDER BY product_id, locale;

-- name: ListProductContentByScope :many
-- Batch-loads content for a set of products at one specific locale
SELECT * FROM catalog_product_content
WHERE product_id = ANY(sqlc.arg(product_ids)::uuid[])
  AND locale     = sqlc.arg(locale)::text
ORDER BY product_id;

-- name: ListProductContentForProducts :many
-- Every locale's row for a set of products - admin product lists answer Product.content in one query
SELECT * FROM catalog_product_content
WHERE product_id = ANY(sqlc.arg(product_ids)::uuid[])
ORDER BY product_id, locale;

-- name: DeleteProductContent :execrows
DELETE FROM catalog_product_content
WHERE product_id = $1 AND locale = $2;

-- name: UpdateProductTaxClass :one
UPDATE catalog_products
SET tax_class = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: GetProductTaxClassesBySKUs :many
-- Order-placement lookup: resolve each cart line's product tax class
-- from its SKU so the tax engine can evaluate per line
SELECT sku, tax_class FROM catalog_products
WHERE sku = ANY(sqlc.arg(skus)::text[]);

-- name: GetProductDimensionsBySKUs :many
-- What shipping rates a line by - its physical size, and its product type for the methods' per-type rates
SELECT sku, weight_grams, length_mm, width_mm, height_mm, product_type_id FROM catalog_products
WHERE sku = ANY(sqlc.arg(skus)::text[]);

-- name: GetProductCostsBySKUs :many
-- Order-placement COGS snapshot: resolve each cart line's unit cost from its SKU in one query (mirrors GetProductTaxClassesBySKUs) instead of a per-line pricing round-trip - rows with NULL cost_cents are omitted, so a present key always carries a real cost
SELECT sku, cost_cents FROM catalog_products
WHERE sku = ANY(sqlc.arg(skus)::text[]) AND cost_cents IS NOT NULL;

-- name: SetProductVariantMode :one
-- Sets the attribute/list variant flavor on a configurable product
UPDATE catalog_products
SET variant_mode = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListProductChildLinksByChildIDs :many
-- Inverse of the configurable parent→child link, batched - the UNIQUE index on catalog_product_children(child_id) guarantees each child maps to at most one parent; sort_order is the variant's position under that parent - powers Product.partOf.configurable
SELECT child_id, parent_id, sort_order
FROM catalog_product_children
WHERE child_id = ANY(sqlc.arg(child_ids)::uuid[]);

-- name: BrowseProductsFallback :many
-- Postgres listing path for when OpenSearch is unavailable. browseProducts is the ONLY product-listing surface, so without this a missing or downed cluster renders every category, collection and PLP as an empty catalogue with HTTP 200 Deliberately narrower than the index: no attribute facets and no relevance scoring - it exists to keep the store shoppable, not to match search quality The effective-price CASE is repeated rather than aliased because sqlc's analyzer resolves neither a CTE alias nor a LATERAL alias in WHERE/ORDER BY - it folds the active special exactly as the storefront card does, so price sort and the range filter agree with the price on screen
SELECT p.id, p.sku, p.created_at,
       CASE
               WHEN l.special_price IS NOT NULL
                    AND (l.special_from IS NULL OR l.special_from <= NOW())
                    AND (l.special_to   IS NULL OR l.special_to   >= NOW())
               THEN l.special_price
               ELSE COALESCE(l.price, p.default_price)
           END AS effective_price,
       COUNT(*) OVER () AS total_count
FROM catalog_products p
JOIN catalog_product_channel_listings l
  ON l.product_id = p.id AND l.channel_id = sqlc.arg(channel_id)
WHERE l.is_published = TRUE
  AND (cardinality(sqlc.arg(kinds)::text[]) = 0
       OR p.kind::text = ANY(sqlc.arg(kinds)::text[]))
  -- Same arms SearchProductsFallback uses, so a term typed into a filtered PLP finds what the search box finds - matching SKU alone meant a shopper who typed a product NAME on a category page got an empty grid on this tier Shaped as a UNION of per-table arms for the same reason: each is an index path, where one OR across p and c would force a scan of the whole join The <> '' guard inside each arm matters - without it an unfiltered PLP (the common case) still builds the subquery, and ILIKE '%%' matches every row
  AND (sqlc.arg(search)::text = '' OR p.id IN (
        SELECT mc.product_id
          FROM catalog_product_content mc
         WHERE sqlc.arg(search)::text <> ''
           AND mc.locale = sqlc.arg(locale)::text
           AND (mc.search_tsv @@ websearch_to_tsquery('simple', sqlc.arg(search)::text)
                OR mc.name ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%')
        UNION
        SELECT mp.id
          FROM catalog_products mp
         WHERE sqlc.arg(search)::text <> ''
           AND mp.sku ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%'))
  AND (NOT sqlc.arg(filter_category)::boolean OR EXISTS (
        SELECT 1 FROM catalog_product_categories pc
        WHERE pc.product_id = p.id AND pc.category_id = sqlc.arg(category_id)))
  AND (NOT sqlc.arg(filter_collection)::boolean OR EXISTS (
        SELECT 1 FROM catalog_collection_products cp
        WHERE cp.product_id = p.id AND cp.collection_id = sqlc.arg(collection_id)))
  -- Attribute filters: AND across attributes, OR within one - A value id belongs to exactly one attribute, so counting distinct matched attributes and requiring all of them expresses that without building SQL per request - offering facets that didn't actually filter would be worse than offering none
  AND (sqlc.arg(attr_count)::int = 0 OR EXISTS (
        SELECT 1 FROM catalog_product_attribute_values av
        WHERE av.product_id = p.id
          AND av.attribute_value_id = ANY(sqlc.arg(attr_value_ids)::uuid[])
        HAVING count(DISTINCT av.attribute_id) = sqlc.arg(attr_count)::int))
  AND (NOT sqlc.arg(filter_min)::boolean OR CASE
               WHEN l.special_price IS NOT NULL
                    AND (l.special_from IS NULL OR l.special_from <= NOW())
                    AND (l.special_to   IS NULL OR l.special_to   >= NOW())
               THEN l.special_price
               ELSE COALESCE(l.price, p.default_price)
           END >= sqlc.arg(price_min))
  AND (NOT sqlc.arg(filter_max)::boolean OR CASE
               WHEN l.special_price IS NOT NULL
                    AND (l.special_from IS NULL OR l.special_from <= NOW())
                    AND (l.special_to   IS NULL OR l.special_to   >= NOW())
               THEN l.special_price
               ELSE COALESCE(l.price, p.default_price)
           END <= sqlc.arg(price_max))
ORDER BY
    CASE WHEN sqlc.arg(sort)::text = 'price_asc'  THEN CASE
               WHEN l.special_price IS NOT NULL
                    AND (l.special_from IS NULL OR l.special_from <= NOW())
                    AND (l.special_to   IS NULL OR l.special_to   >= NOW())
               THEN l.special_price
               ELSE COALESCE(l.price, p.default_price)
           END END ASC,
    CASE WHEN sqlc.arg(sort)::text = 'price_desc' THEN CASE
               WHEN l.special_price IS NOT NULL
                    AND (l.special_from IS NULL OR l.special_from <= NOW())
                    AND (l.special_to   IS NULL OR l.special_to   >= NOW())
               THEN l.special_price
               ELSE COALESCE(l.price, p.default_price)
           END END DESC,
    CASE WHEN sqlc.arg(sort)::text = 'sku'        THEN p.sku END ASC,
    p.created_at DESC,
    p.id ASC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: SearchProductsFallback :many
-- Full-text product search for the Postgres tier, used when OpenSearch is unavailable - ranked by ts_rank over the weighted name/meta vector, with an ILIKE arm so partial words and SKU fragments still hit ('simple' does no stemming, so a prefix match is the only way to hit a partial word) ts_rank is strict, so a hit from the SKU/name arms with no content row for this locale ranked NULL - which scans into the generated float32 as an error and failed the entire query. COALESCE lands those last, where an unranked match belongs
SELECT p.id, p.sku,
       COALESCE(c.name, '') AS name,
       COALESCE(l.url, p.url, '') AS url,
       COALESCE(ts_rank(c.search_tsv, websearch_to_tsquery('simple', sqlc.arg(q)::text)), 0)::real AS rank,
       COUNT(*) OVER () AS total_count
FROM catalog_products p
JOIN catalog_product_channel_listings l
  ON l.product_id = p.id AND l.channel_id = sqlc.arg(channel_id)
LEFT JOIN catalog_product_content c
  ON c.product_id = p.id AND c.locale = sqlc.arg(locale)
WHERE l.is_published = TRUE
  -- Matching is a UNION of single-table arms rather than one OR across p and c. An OR spanning two relations can't be an index path for either - the planner has to join everything and filter, so the trigram and tsv indexes went unused. Each arm here scans ONE table and is indexable; the content arm's own OR is a bitmap of the tsv and name GINs. ILIKE on the bare column (not lower(col) LIKE, which only an expression index answers) is what lets one index serve all three queries
  AND p.id IN (
        SELECT mc.product_id
          FROM catalog_product_content mc
         WHERE mc.locale = sqlc.arg(locale)
           AND (mc.search_tsv @@ websearch_to_tsquery('simple', sqlc.arg(q)::text)
                OR mc.name ILIKE '%' || escape_like(sqlc.arg(q)::text) || '%')
        UNION
        SELECT mp.id
          FROM catalog_products mp
         WHERE mp.sku ILIKE '%' || escape_like(sqlc.arg(q)::text) || '%'
  )
ORDER BY rank DESC, p.created_at DESC, p.id ASC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: BrowseFacetCountsFallback :many
-- Facet counts for ONE attribute on the Postgres tier, with the Algolia-style multi-select exclusion the index does: the target attribute's own selections are left out of the constraint set, so ticking one colour doesn't collapse the colour panel to that single value
-- the caller runs this once per filterable attribute
WITH base AS (
    SELECT p.id
    FROM catalog_products p
    JOIN catalog_product_channel_listings l
      ON l.product_id = p.id AND l.channel_id = sqlc.arg(channel_id)
    WHERE l.is_published = TRUE
      AND (cardinality(sqlc.arg(kinds)::text[]) = 0
           OR p.kind::text = ANY(sqlc.arg(kinds)::text[]))
      -- Character-for-character the predicate BrowseProductsFallback applies - a
      -- facet counted over a wider set than the grid shows is a count that lies
      AND (sqlc.arg(search)::text = '' OR p.id IN (
            SELECT mc.product_id
              FROM catalog_product_content mc
             WHERE sqlc.arg(search)::text <> ''
               AND mc.locale = sqlc.arg(locale)::text
               AND (mc.search_tsv @@ websearch_to_tsquery('simple', sqlc.arg(search)::text)
                    OR mc.name ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%')
            UNION
            SELECT mp.id
              FROM catalog_products mp
             WHERE sqlc.arg(search)::text <> ''
               AND mp.sku ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%'))
      AND (NOT sqlc.arg(filter_category)::boolean OR EXISTS (
            SELECT 1 FROM catalog_product_categories pc
            WHERE pc.product_id = p.id AND pc.category_id = sqlc.arg(category_id)))
      AND (NOT sqlc.arg(filter_collection)::boolean OR EXISTS (
            SELECT 1 FROM catalog_collection_products cp
            WHERE cp.product_id = p.id AND cp.collection_id = sqlc.arg(collection_id)))
), constrained AS (
    SELECT av.product_id
    FROM catalog_product_attribute_values av
    WHERE av.attribute_value_id = ANY(sqlc.arg(other_value_ids)::uuid[])
    GROUP BY av.product_id
    HAVING count(DISTINCT av.attribute_id) = sqlc.arg(other_attr_count)::int
)
SELECT v.id AS value_id, v.slug, v.name, v.is_indexable, count(*)::bigint AS product_count
FROM catalog_product_attribute_values av
JOIN base b ON b.id = av.product_id
JOIN catalog_attribute_values v ON v.id = av.attribute_value_id
WHERE av.attribute_id = sqlc.arg(target_attribute_id)
  AND (sqlc.arg(other_attr_count)::int = 0
       OR av.product_id IN (SELECT product_id FROM constrained))
GROUP BY v.id, v.slug, v.name, v.is_indexable, v.sort_order
-- Count first, THEN sort_order as the tie-break - ordering by sort_order alone made max_values truncate by position instead of popularity: on a 141-value attribute that kept a 193-product bucket and dropped a 901-product one, while the index - a terms agg, so count-ordered - kept the big one - same ordering as the index now, which is also what a shopper wants at the top of a filter panel
ORDER BY product_count DESC, v.sort_order, v.name
LIMIT sqlc.arg(max_values);

-- name: ListProductsPagedAdmin :many
-- Server-side page for the admin product list - the unpaginated `products` query hydrates the full graph for every row: measured at 101k products that was a 152MB response taking 20s, and the tab then froze normalising it into the Apollo cache
-- this returns one page of ids plus the filtered total; the caller hydrates only those ids through the same path it always used
WITH RECURSIVE category_tree AS (
    SELECT c.id FROM catalog_categories c WHERE c.id = ANY(sqlc.arg(category_ids)::uuid[])
    UNION
    SELECT c.id FROM catalog_categories c JOIN category_tree t ON c.parent_id = t.id
),
filtered AS (
    SELECT p.id,
           p.sku,
           p.url,
           p.kind,
           p.default_price,
           p.created_at,
           p.updated_at,
           pt.name AS type_name,
           co.name AS content_name
    FROM catalog_products p
    JOIN catalog_product_types pt ON pt.id = p.product_type_id
    LEFT JOIN catalog_product_content co
      ON co.product_id = p.id AND co.locale = sqlc.arg(locale)::text
    LEFT JOIN currencies cur ON cur.code = p.default_currency_code
    WHERE (sqlc.arg(all_channels)::boolean OR EXISTS (
              SELECT 1 FROM catalog_product_channel_listings l
              WHERE l.product_id = p.id AND l.channel_id = sqlc.arg(channel_id)))
      AND (sqlc.arg(search)::text = ''
           OR p.sku ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%'
           OR p.url ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%'
           OR co.name ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%')
      AND (cardinality(sqlc.arg(kinds)::text[]) = 0 OR p.kind::text = ANY(sqlc.arg(kinds)::text[]))
      AND (cardinality(sqlc.arg(type_slugs)::text[]) = 0 OR pt.slug = ANY(sqlc.arg(type_slugs)::text[]))
      -- Published means "has a published listing in scope" - in global mode any
      -- channel counts, which matches what the unpaginated list showed
      AND (NOT sqlc.arg(filter_published)::boolean
           OR sqlc.arg(is_published)::boolean = EXISTS (
                SELECT 1 FROM catalog_product_channel_listings l2
                WHERE l2.product_id = p.id
                  AND l2.is_published
                  AND (sqlc.arg(all_channels)::boolean OR l2.channel_id = sqlc.arg(channel_id))))
      -- Same AND-across-attributes / OR-within-one shape the facet count uses: a value id belongs to exactly one
      -- attribute, so counting distinct matched attributes and requiring all of them needs no SQL built per request
      AND (cardinality(sqlc.arg(attr_value_ids)::uuid[]) = 0 OR p.id IN (
              SELECT av.product_id
              FROM catalog_product_attribute_values av
              WHERE av.attribute_value_id = ANY(sqlc.arg(attr_value_ids)::uuid[])
              GROUP BY av.product_id
              -- How many distinct attributes the selection spans, derived here so no caller has to know: two values of the SAME attribute
              -- must count once, or picking two colours becomes an unsatisfiable AND instead of an OR
              HAVING count(DISTINCT av.attribute_id) = (
                  SELECT count(DISTINCT v2.attribute_id)
                  FROM catalog_attribute_values v2
                  WHERE v2.id = ANY(sqlc.arg(attr_value_ids)::uuid[]))))
      -- A category reaches its subcategories, the way browsing it in the storefront does
      AND (cardinality(sqlc.arg(category_ids)::uuid[]) = 0 OR p.id IN (
              SELECT pc.product_id
              FROM catalog_product_categories pc
              WHERE pc.category_id IN (SELECT id FROM category_tree)))
      AND (cardinality(sqlc.arg(collection_ids)::uuid[]) = 0 OR EXISTS (
              SELECT 1 FROM catalog_collection_products cp
              WHERE cp.product_id = p.id AND cp.collection_id = ANY(sqlc.arg(collection_ids)::uuid[])))
      -- Bounds arrive as typed ('12.50') and scale by each product's own currency, so they mean 12.50 dollars and 12 yen alike
      AND (NULLIF(sqlc.arg(price_min)::text, '') IS NULL
           OR p.default_price >= NULLIF(sqlc.arg(price_min)::text, '')::numeric * power(10, coalesce(cur.decimals, 2)))
      AND (NULLIF(sqlc.arg(price_max)::text, '') IS NULL
           OR p.default_price <= NULLIF(sqlc.arg(price_max)::text, '')::numeric * power(10, coalesce(cur.decimals, 2)))
      -- inventory's Salable in SQL: no item is always in stock, flat mode reads the item's own quantity, otherwise the levels at
      -- the scope's locations (every location globally) less reservations for tracked items - a remote stock provider isn't seen here
      AND (sqlc.arg(stock)::text = '' OR (sqlc.arg(stock)::text = 'in') = (
              NOT EXISTS (SELECT 1 FROM inventory_items i0 WHERE i0.product_id = p.id)
              OR EXISTS (
                  SELECT 1 FROM inventory_items ii
                  WHERE ii.product_id = p.id
                    AND CASE WHEN sqlc.arg(flat_inventory)::boolean THEN NOT ii.tracked OR ii.quantity > 0
                             ELSE (SELECT coalesce(sum(CASE WHEN ii.tracked THEN lv.quantity - greatest(lv.reserved_quantity, 0) ELSE lv.quantity END), 0)
                                   FROM inventory_levels lv
                                   WHERE lv.inventory_item_id = ii.id
                                     AND (sqlc.arg(all_channels)::boolean OR lv.location_id IN (
                                          SELECT cl.location_id FROM inventory_channel_locations cl
                                          WHERE cl.channel_id = sqlc.arg(channel_id)))) > 0
                        END)))
)
SELECT id, COUNT(*) OVER () AS total_count
FROM filtered
-- Grouped by result type rather than one CASE pair per column: every branch of a CASE has to agree on type, so text
-- keys share a term, money another, times a third - id last keeps paging stable when the sort key ties
ORDER BY
    CASE WHEN sqlc.arg(sort_desc)::boolean THEN NULL ELSE
        CASE sqlc.arg(sort_key)::text
            WHEN 'name' THEN coalesce(content_name, sku)
            WHEN 'sku' THEN sku
            WHEN 'url' THEN url
            WHEN 'kind' THEN kind::text
            WHEN 'productType' THEN type_name
        END
    END ASC NULLS LAST,
    CASE WHEN sqlc.arg(sort_desc)::boolean THEN
        CASE sqlc.arg(sort_key)::text
            WHEN 'name' THEN coalesce(content_name, sku)
            WHEN 'sku' THEN sku
            WHEN 'url' THEN url
            WHEN 'kind' THEN kind::text
            WHEN 'productType' THEN type_name
        END
    END DESC NULLS LAST,
    CASE WHEN sqlc.arg(sort_desc)::boolean THEN NULL ELSE
        CASE sqlc.arg(sort_key)::text WHEN 'defaultPrice' THEN default_price END
    END ASC NULLS LAST,
    CASE WHEN sqlc.arg(sort_desc)::boolean THEN
        CASE sqlc.arg(sort_key)::text WHEN 'defaultPrice' THEN default_price END
    END DESC NULLS LAST,
    CASE WHEN sqlc.arg(sort_desc)::boolean THEN NULL ELSE
        CASE sqlc.arg(sort_key)::text
            WHEN 'createdAt' THEN created_at
            WHEN 'updatedAt' THEN updated_at
        END
    END ASC NULLS LAST,
    CASE WHEN sqlc.arg(sort_desc)::boolean THEN
        CASE sqlc.arg(sort_key)::text
            WHEN 'createdAt' THEN created_at
            WHEN 'updatedAt' THEN updated_at
        END
    END DESC NULLS LAST,
    id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: ExportProductParentSKUs :many
-- Every variant's parent SKU in one read. The export pages through products, so a child can surface before its
-- parent does - this is the whole map, which is small because only configurables have children
SELECT pc.child_id, parent.sku AS parent_sku
FROM catalog_product_children pc
JOIN catalog_products parent ON parent.id = pc.parent_id;
