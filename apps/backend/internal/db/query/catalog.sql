-- ==================== Product Types ====================

-- name: CreateProductType :one
INSERT INTO catalog_product_types (
    slug, name, is_digital, is_shipping_required,
    url_template, tracks_inventory,
    metadata, private_metadata
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetProductTypeByID :one
SELECT * FROM catalog_product_types WHERE id = $1;

-- GetProductTypesByIDs is the batched form used by the storefront
-- DataLoader: every per-product `productType` field resolver across a
-- page collapses into one query for the distinct ids
-- name: GetProductTypesByIDs :many
SELECT * FROM catalog_product_types WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListProductTypes :many
SELECT * FROM catalog_product_types ORDER BY name;

-- name: UpdateProductType :one
UPDATE catalog_product_types
SET name                 = $2,
    is_digital            = $3,
    is_shipping_required  = $4,
    url_template          = $5,
    tracks_inventory      = $6,
    metadata              = $7,
    private_metadata      = $8,
    updated_at            = NOW()
WHERE id = $1
RETURNING *;

-- name: SetProductTypeURLPathTemplate :one
UPDATE catalog_product_types
SET url_template = $2,
    updated_at   = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteProductType :exec
DELETE FROM catalog_product_types WHERE id = $1;

-- ==================== Attributes ====================

-- name: CreateAttribute :one
INSERT INTO catalog_attributes (
    slug, name, input_type, unit,
    value_required, filterable, searchable, is_public,
    facet_collapsed, is_indexable, h1_position,
    sort_order, metadata, private_metadata, default_value, synapse_track, auto_segment,
    relationship_target
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
RETURNING *;

-- name: GetAttributeByID :one
SELECT * FROM catalog_attributes WHERE id = $1;

-- name: ListAttributes :many
SELECT * FROM catalog_attributes ORDER BY sort_order, name;

-- name: UpdateAttribute :one
UPDATE catalog_attributes
SET name               = $2,
    unit               = $3,
    value_required     = $4,
    filterable         = $5,
    searchable         = $6,
    is_public = $7,
    facet_collapsed    = $8,
    is_indexable       = $9,
    h1_position        = $10,
    sort_order         = $11,
    metadata           = $12,
    private_metadata   = $13,
    default_value      = $14,
    synapse_track      = $15,
    auto_segment       = $16,
    relationship_target = $17,
    updated_at         = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteAttribute :exec
DELETE FROM catalog_attributes WHERE id = $1;

-- ==================== Attribute Values ====================

-- name: CreateAttributeValue :one
INSERT INTO catalog_attribute_values (attribute_id, slug, name, value, sort_order, is_indexable)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetAttributeValueByID :one
SELECT * FROM catalog_attribute_values WHERE id = $1;

-- name: ListAttributeValues :many
SELECT * FROM catalog_attribute_values
WHERE attribute_id = $1
ORDER BY sort_order, name;

-- name: ListAttributeValuesByAttributeIDs :many
SELECT * FROM catalog_attribute_values
WHERE attribute_id = ANY(sqlc.arg(attribute_ids)::uuid[])
ORDER BY attribute_id, sort_order, name;

-- name: UpdateAttributeValue :one
UPDATE catalog_attribute_values
SET slug         = $2,
    name         = $3,
    value        = $4,
    sort_order   = $5,
    is_indexable = $6
WHERE id = $1
RETURNING *;

-- name: SetAttributeValuesIndexable :execrows
-- Bulk facet opt-in from the SEO page - a targeted UPDATE rather than N UpdateAttributeValue
-- calls, which rewrite the whole row and re-run the slug-rename URL regeneration
UPDATE catalog_attribute_values
SET is_indexable = sqlc.arg(is_indexable)::boolean
WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: DeleteAttributeValue :exec
DELETE FROM catalog_attribute_values WHERE id = $1;

-- ==================== Attribute / AttributeValue Localised Descriptions ====================

-- name: ListAttributeContent :many
SELECT * FROM catalog_attribute_content
WHERE attribute_id = $1
ORDER BY locale;

-- name: ListAttributeContentByAttributeIDs :many
SELECT * FROM catalog_attribute_content
WHERE attribute_id = ANY(sqlc.arg(attribute_ids)::uuid[]);

-- name: UpsertAttributeContent :one
INSERT INTO catalog_attribute_content (attribute_id, locale, name, description)
VALUES ($1, $2, $3, $4)
ON CONFLICT (attribute_id, locale) DO UPDATE
    SET name        = EXCLUDED.name,
        description = EXCLUDED.description,
        updated_at  = NOW()
RETURNING *;

-- name: DeleteAttributeContent :exec
DELETE FROM catalog_attribute_content
WHERE attribute_id = $1 AND locale = $2;

-- name: ListAttributeValueContent :many
SELECT * FROM catalog_attribute_value_content
WHERE value_id = $1
ORDER BY locale;

-- name: ListAttributeValueContentByValueIDs :many
SELECT * FROM catalog_attribute_value_content
WHERE value_id = ANY(sqlc.arg(value_ids)::uuid[]);

-- name: UpsertAttributeValueContent :one
INSERT INTO catalog_attribute_value_content (value_id, locale, name, description)
VALUES ($1, $2, $3, $4)
ON CONFLICT (value_id, locale) DO UPDATE
    SET name        = EXCLUDED.name,
        description = EXCLUDED.description,
        updated_at  = NOW()
RETURNING *;

-- name: DeleteAttributeValueContent :exec
DELETE FROM catalog_attribute_value_content
WHERE value_id = $1 AND locale = $2;

-- name: SetAttributeIndexableCascade :one
-- Toggles the master indexable flag on the attribute - for selectable
-- attributes the cascade into AttributeValue is done in the separate
-- statement below - kept here together so the service can run them in
-- one transaction - scalar attributes have no per-row indexability: the
-- attribute flag is read directly by the indexer
UPDATE catalog_attributes
SET is_indexable = $2,
    updated_at   = NOW()
WHERE id = $1
RETURNING *;

-- name: CascadeAttributeValuesIndexable :exec
UPDATE catalog_attribute_values
SET is_indexable = $2
WHERE attribute_id = $1;

-- name: CountAttributeValues :one
SELECT COUNT(*)::BIGINT FROM catalog_attribute_values WHERE attribute_id = $1;

-- name: CountProductAttributeSlugs :one
SELECT COUNT(*)::BIGINT FROM catalog_product_attribute_slugs WHERE attribute_id = $1;

-- ====================
-- Product Attribute Slugs
-- ====================
-- Per-product URL slug override for one scalar attribute on one
-- product - selectable attrs live on catalog_attribute_values; this
-- table covers free-text scalars - indexability is not stored
-- here - scalar facets follow the attribute-level is_indexable flag

-- name: ListProductAttributeSlugs :many
SELECT * FROM catalog_product_attribute_slugs
WHERE product_id = $1
ORDER BY attribute_id;

-- Bulk variant used by the indexer when rebuilding all products on
-- one store - returns every override across the requested products in
-- a single query so the indexer can build its map without N+1
-- name: ListProductAttributeSlugsByProductIDs :many
SELECT * FROM catalog_product_attribute_slugs
WHERE product_id = ANY(sqlc.arg(product_ids)::uuid[]);

-- name: GetProductAttributeSlug :one
SELECT * FROM catalog_product_attribute_slugs
WHERE product_id = $1 AND attribute_id = $2;

-- Global override (applies on every channel without its own row)
-- name: UpsertProductAttributeSlug :one
INSERT INTO catalog_product_attribute_slugs (product_id, attribute_id, slug)
VALUES ($1, $2, $3)
ON CONFLICT (product_id, attribute_id) WHERE channel_id IS NULL DO UPDATE
    SET slug       = EXCLUDED.slug,
        updated_at = NOW()
RETURNING *;

-- Channel-narrowed override; wins over the global row on that channel
-- name: UpsertProductAttributeSlugForChannel :one
INSERT INTO catalog_product_attribute_slugs (product_id, attribute_id, channel_id, slug)
VALUES ($1, $2, $3, $4)
ON CONFLICT (product_id, attribute_id, channel_id) WHERE channel_id IS NOT NULL DO UPDATE
    SET slug       = EXCLUDED.slug,
        updated_at = NOW()
RETURNING *;

-- name: DeleteProductAttributeSlug :exec
DELETE FROM catalog_product_attribute_slugs WHERE id = $1;

-- Convenience delete by composite key - admin "clear override" action
-- doesn't have the row id handy in the URL. channel_id narrows to one
-- channel row; sqlc.narg NULL clears the global row
-- name: DeleteProductAttributeSlugByProductAttribute :exec
DELETE FROM catalog_product_attribute_slugs
WHERE product_id = $1 AND attribute_id = $2
  AND channel_id IS NOT DISTINCT FROM sqlc.narg(channel_id)::uuid;

-- ====================
-- Product Attribute Locale Values (plain_text translations)
-- ====================
-- Per-(product, attribute, locale) override of the canonical scalar
-- value stored in catalog_products.attributes JSONB. plain_text only;
-- the service layer enforces the input type

-- name: ListProductAttributeLocaleValues :many
SELECT * FROM catalog_product_attribute_locale_values
WHERE product_id = $1
ORDER BY attribute_id, locale;

-- name: ListProductAttributeLocaleValuesByProductIDs :many
SELECT * FROM catalog_product_attribute_locale_values
WHERE product_id = ANY(sqlc.arg(product_ids)::uuid[]);

-- name: UpsertProductAttributeLocaleValue :one
INSERT INTO catalog_product_attribute_locale_values (product_id, attribute_id, locale, value)
VALUES ($1, $2, $3, $4)
ON CONFLICT (product_id, attribute_id, locale) DO UPDATE
    SET value      = EXCLUDED.value,
        updated_at = NOW()
RETURNING *;

-- name: DeleteProductAttributeLocaleValue :exec
DELETE FROM catalog_product_attribute_locale_values
WHERE product_id = $1 AND attribute_id = $2 AND locale = $3;

-- Clearing a scalar attribute on a product also clears its
-- translations - they describe a value that no longer exists
-- name: DeleteProductAttributeLocaleValuesForAttribute :exec
DELETE FROM catalog_product_attribute_locale_values
WHERE product_id = $1 AND attribute_id = $2;

-- ==================== Attribute Value Channel Slugs (selectable values, per channel) ==================== Channel
-- override of catalog_attribute_values.slug for filter URLs

-- name: ListAttributeValueChannelSlugsByValueIDs :many
SELECT * FROM catalog_attribute_value_channel_slugs
WHERE value_id = ANY(sqlc.arg(value_ids)::uuid[]);

-- name: ListAttributeValueChannelSlugsForChannel :many
SELECT * FROM catalog_attribute_value_channel_slugs
WHERE channel_id = $1;

-- name: UpsertAttributeValueChannelSlug :one
INSERT INTO catalog_attribute_value_channel_slugs (value_id, attribute_id, channel_id, slug)
VALUES ($1, $2, $3, $4)
ON CONFLICT (value_id, channel_id) DO UPDATE
    SET slug       = EXCLUDED.slug,
        updated_at = NOW()
RETURNING *;

-- name: DeleteAttributeValueChannelSlug :exec
DELETE FROM catalog_attribute_value_channel_slugs
WHERE value_id = $1 AND channel_id = $2;

-- ==================== Product Type ↔ Attribute Assignment ====================

-- name: AssignAttributeToProductType :exec
INSERT INTO catalog_product_type_attributes (product_type_id, attribute_id, sort_order, group_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (product_type_id, attribute_id) DO UPDATE
    SET sort_order = EXCLUDED.sort_order,
        group_id   = EXCLUDED.group_id;

-- name: MoveAttributeToGroup :exec
UPDATE catalog_product_type_attributes
SET group_id = $3
WHERE product_type_id = $1 AND attribute_id = $2;

-- name: UnassignAttributeFromProductType :execrows
DELETE FROM catalog_product_type_attributes
WHERE product_type_id = $1 AND attribute_id = $2;

-- name: ListProductTypeAttributes :many
SELECT a.id, a.slug, a.name, a.input_type, a.relationship_target, a.unit,
       a.value_required, a.filterable, a.searchable, a.is_public,
       a.facet_collapsed, a.is_indexable, a.h1_position,
       a.sort_order AS attribute_sort_order,
       a.metadata, a.private_metadata, a.default_value,
       a.created_at, a.updated_at,
       pta.sort_order AS assignment_sort_order,
       pta.group_id   AS group_id
FROM catalog_product_type_attributes pta
JOIN catalog_attributes a ON a.id = pta.attribute_id
WHERE pta.product_type_id = $1
ORDER BY pta.sort_order, a.name;

-- name: ListProductTypeAttributesByTypeIDs :many
-- Batched twin of the above so a list of types hydrates its attributes in one
-- query instead of one per row - carries product_type_id so Go can group them
SELECT pta.product_type_id,
       a.id, a.slug, a.name, a.input_type, a.relationship_target, a.unit,
       a.value_required, a.filterable, a.searchable, a.is_public,
       a.facet_collapsed, a.is_indexable, a.h1_position,
       a.sort_order AS attribute_sort_order,
       a.metadata, a.private_metadata, a.default_value,
       a.created_at, a.updated_at,
       pta.sort_order AS assignment_sort_order,
       pta.group_id   AS group_id
FROM catalog_product_type_attributes pta
JOIN catalog_attributes a ON a.id = pta.attribute_id
WHERE pta.product_type_id = ANY(sqlc.arg(product_type_ids)::uuid[])
ORDER BY pta.product_type_id, pta.sort_order, a.name;

-- ==================== Product Children ====================

-- name: AddProductChild :execrows
INSERT INTO catalog_product_children (parent_id, child_id, sort_order)
VALUES ($1, $2, $3)
ON CONFLICT (parent_id, child_id) DO UPDATE SET sort_order = EXCLUDED.sort_order;

-- name: RemoveProductChild :execrows
DELETE FROM catalog_product_children WHERE parent_id = $1 AND child_id = $2;

-- name: ListProductChildren :many
SELECT p.*
FROM catalog_product_children pc
JOIN catalog_products p ON p.id = pc.child_id
WHERE pc.parent_id = $1
ORDER BY pc.sort_order, p.url;

-- name: ListProductChildrenByParentIDs :many
SELECT pc.parent_id, p.*
FROM catalog_product_children pc
JOIN catalog_products p ON p.id = pc.child_id
WHERE pc.parent_id = ANY(sqlc.arg(parent_ids)::uuid[])
ORDER BY pc.parent_id, pc.sort_order, p.url;

-- name: ListSynapseTrackedAttributes :many
-- Attributes flagged for affinity tracking - Signals wants the slug and label alone, so this stays a narrow read rather than the whole catalog service
SELECT slug, name, auto_segment FROM catalog_attributes WHERE synapse_track = true ORDER BY slug;
