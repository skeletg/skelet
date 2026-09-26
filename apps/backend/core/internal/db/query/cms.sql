-- ==================== Attributes ====================

-- name: CreateCMSAttribute :one
INSERT INTO cms_attributes (
    slug, input_type, is_translatable, is_required_default, unit, validation, relationship_target, metadata, default_value
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetCMSAttributeByID :one
SELECT * FROM cms_attributes WHERE id = $1;

-- name: GetCMSAttributeBySlug :one
SELECT * FROM cms_attributes WHERE slug = $1;

-- name: ListCMSAttributes :many
SELECT * FROM cms_attributes ORDER BY slug;

-- name: UpdateCMSAttribute :one
-- Slug + input_type are immutable; only mutable fields here
UPDATE cms_attributes
SET is_translatable     = $2,
    is_required_default = $3,
    unit                = $4,
    validation          = $5,
    relationship_target = $6,
    metadata            = $7,
    default_value       = $8,
    updated_at          = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteCMSAttribute :exec
DELETE FROM cms_attributes WHERE id = $1;

-- name: UpsertCMSAttributeContent :one
INSERT INTO cms_attribute_content (attribute_id, locale, name, helper_text, placeholder)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (attribute_id, locale) DO UPDATE
SET name = EXCLUDED.name,
    helper_text = EXCLUDED.helper_text,
    placeholder = EXCLUDED.placeholder,
    updated_at = NOW()
RETURNING *;

-- name: ListCMSAttributeContent :many
SELECT * FROM cms_attribute_content WHERE attribute_id = $1 ORDER BY locale;

-- name: DeleteCMSAttributeContent :execrows
DELETE FROM cms_attribute_content WHERE attribute_id = $1 AND locale = $2;

-- ==================== Attribute Values ====================

-- name: CreateCMSAttributeValue :one
INSERT INTO cms_attribute_values (attribute_id, slug, position)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListCMSAttributeValues :many
SELECT * FROM cms_attribute_values WHERE attribute_id = $1 ORDER BY position, slug;

-- name: ListCMSAttributeValuesByAttributeIDs :many
SELECT * FROM cms_attribute_values
WHERE attribute_id = ANY(sqlc.arg(attribute_ids)::uuid[])
ORDER BY attribute_id, position, slug;

-- name: UpdateCMSAttributeValue :one
UPDATE cms_attribute_values
SET slug = $2, position = $3
WHERE id = $1
RETURNING *;

-- name: DeleteCMSAttributeValue :exec
DELETE FROM cms_attribute_values WHERE id = $1;

-- name: GetCMSAttributeValueByID :one
SELECT * FROM cms_attribute_values WHERE id = $1;

-- name: UpsertCMSAttributeValueContent :exec
INSERT INTO cms_attribute_value_content (value_id, locale, name)
VALUES ($1, $2, $3)
ON CONFLICT (value_id, locale) DO UPDATE SET name = EXCLUDED.name, updated_at = NOW();

-- name: ListCMSAttributeValueContentByValueIDs :many
SELECT value_id, locale, name, updated_at
FROM cms_attribute_value_content
WHERE value_id = ANY(sqlc.arg(value_ids)::uuid[])
ORDER BY value_id, locale;

-- ==================== Attribute Groups ====================

-- name: CreateCMSAttributeGroup :one
INSERT INTO cms_attribute_groups (slug, position) VALUES ($1, $2) RETURNING *;

-- name: GetCMSAttributeGroupByID :one
SELECT * FROM cms_attribute_groups WHERE id = $1;

-- name: ListCMSAttributeGroups :many
SELECT * FROM cms_attribute_groups ORDER BY position, slug;

-- name: UpdateCMSAttributeGroup :one
UPDATE cms_attribute_groups
SET slug = $2, position = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteCMSAttributeGroup :exec
DELETE FROM cms_attribute_groups WHERE id = $1;

-- ==================== Page Types ====================

-- name: CreateCMSPageType :one
INSERT INTO cms_page_types (slug, icon, is_system, position) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetCMSPageTypeByID :one
SELECT * FROM cms_page_types WHERE id = $1;

-- name: GetCMSPageTypeBySlug :one
SELECT * FROM cms_page_types WHERE slug = $1;

-- name: ListCMSPageTypes :many
SELECT * FROM cms_page_types ORDER BY position, slug;

-- name: UpdateCMSPageType :one
UPDATE cms_page_types
SET icon = $2, is_system = $3, position = $4, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteCMSPageType :exec
DELETE FROM cms_page_types WHERE id = $1;

-- name: UpsertCMSPageTypeContent :one
INSERT INTO cms_page_type_content (page_type_id, locale, name, description)
VALUES ($1, $2, $3, $4)
ON CONFLICT (page_type_id, locale) DO UPDATE
SET name = EXCLUDED.name, description = EXCLUDED.description, updated_at = NOW()
RETURNING *;

-- name: ListCMSPageTypeContent :many
SELECT * FROM cms_page_type_content WHERE page_type_id = $1 ORDER BY locale;

-- name: UpdateCMSPageTypeStructure :one
UPDATE cms_page_types
SET root_slug           = $2,
    default_meta_title  = $3,
    default_seo_noindex = $4,
    is_singleton        = $5,
    updated_at          = NOW()
WHERE id = $1
RETURNING *;

-- name: SetCMSPageTypeDefaultBlocks :exec
UPDATE cms_page_types SET default_blocks = $2, updated_at = NOW() WHERE id = $1;

-- name: UpsertCMSPageTypeChannelRoot :exec
INSERT INTO cms_page_type_channel_roots (page_type_id, channel_id, root_slug)
VALUES ($1, $2, $3)
ON CONFLICT (page_type_id, channel_id) DO UPDATE SET root_slug = EXCLUDED.root_slug;

-- name: DeleteCMSPageTypeChannelRoot :exec
DELETE FROM cms_page_type_channel_roots WHERE page_type_id = $1 AND channel_id = $2;

-- name: ListCMSPageTypeChannelRoots :many
SELECT page_type_id, channel_id, root_slug
FROM cms_page_type_channel_roots WHERE page_type_id = $1 ORDER BY channel_id;

-- name: ListCMSPagesByType :many
SELECT * FROM cms_pages WHERE page_type_id = $1 ORDER BY url_path;

-- ==================== Page Type Attributes ====================

-- name: AssignCMSAttributeToPageType :exec
INSERT INTO cms_page_type_attributes (page_type_id, attribute_id, group_id, position, is_required, condition_attribute_id, condition_value)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (page_type_id, attribute_id) DO UPDATE
SET group_id = EXCLUDED.group_id,
    position = EXCLUDED.position,
    is_required = EXCLUDED.is_required,
    condition_attribute_id = EXCLUDED.condition_attribute_id,
    condition_value = EXCLUDED.condition_value;

-- name: UnassignCMSAttributeFromPageType :execrows
DELETE FROM cms_page_type_attributes WHERE page_type_id = $1 AND attribute_id = $2;

-- name: ListCMSPageTypeAttributes :many
SELECT pta.page_type_id, pta.attribute_id, pta.group_id, pta.position, pta.is_required,
       pta.condition_attribute_id, pta.condition_value,
       a.slug AS attribute_slug, a.input_type, a.is_translatable, a.is_required_default,
       a.unit, a.validation, a.default_value, a.relationship_target, a.metadata, a.created_at, a.updated_at
FROM cms_page_type_attributes pta
JOIN cms_attributes a ON a.id = pta.attribute_id
WHERE pta.page_type_id = $1
ORDER BY pta.position;

-- name: ListPageTypesUsingAttribute :many
SELECT page_type_id FROM cms_page_type_attributes WHERE attribute_id = $1;

-- ==================== Pages ====================

-- name: CreateCMSPage :one
INSERT INTO cms_pages (
    page_type_id, parent_id, slug, base_path, url_path, position, is_published, seo_noindex,
    published_at, data, metadata, private_metadata
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetCMSPageByID :one
SELECT * FROM cms_pages WHERE id = $1;

-- name: GetCMSPageBySlug :one
SELECT * FROM cms_pages WHERE slug = $1;

-- name: GetCMSPageByURLPath :one
SELECT * FROM cms_pages WHERE url_path = $1;

-- name: ListCMSPages :many
SELECT * FROM cms_pages
WHERE (sqlc.narg(page_type_id)::uuid IS NULL OR page_type_id = sqlc.narg(page_type_id)::uuid)
  AND (sqlc.narg(parent_id)::uuid IS NULL OR parent_id = sqlc.narg(parent_id)::uuid)
  AND (sqlc.narg(only_published)::boolean IS FALSE OR is_published = TRUE)
  AND (sqlc.narg(workflow_state)::text IS NULL OR workflow_state = sqlc.narg(workflow_state)::text)
ORDER BY position, created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListCMSPageChildren :many
SELECT * FROM cms_pages WHERE parent_id = $1 ORDER BY position, slug;

-- name: ListAllCMSPages :many
SELECT * FROM cms_pages ORDER BY url_path;

-- name: UpdateCMSPage :one
UPDATE cms_pages
SET parent_id        = $2,
    slug             = $3,
    position         = $4,
    is_published     = $5,
    seo_noindex      = $6,
    published_at     = $7,
    data             = $8,
    metadata         = $9,
    private_metadata = $10,
    tags             = $11,
    updated_at       = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateCMSPageURLPath :exec
UPDATE cms_pages SET url_path = $2, updated_at = NOW() WHERE id = $1;

-- name: SetCMSPageTags :exec
UPDATE cms_pages SET tags = $2, updated_at = NOW() WHERE id = $1;

-- name: UpdateCMSPagePaths :exec
UPDATE cms_pages SET base_path = $2, url_path = $3, updated_at = NOW() WHERE id = $1;

-- name: UpdateCMSPageExperimentKey :exec
UPDATE cms_pages SET experiment_key = $2, updated_at = NOW() WHERE id = $1;

-- name: UpsertCMSPageVariantContent :exec
INSERT INTO cms_page_variant_content (page_id, locale, variant_key, body)
VALUES ($1, $2, $3, $4)
ON CONFLICT (page_id, locale, variant_key) DO UPDATE SET body = EXCLUDED.body, updated_at = NOW();

-- name: DeleteCMSPageVariantContent :exec
DELETE FROM cms_page_variant_content WHERE page_id = $1 AND locale = $2 AND variant_key = $3;

-- name: ListCMSPageVariantContent :many
SELECT page_id, locale, variant_key, body, updated_at
FROM cms_page_variant_content WHERE page_id = $1 AND locale = $2 ORDER BY variant_key;

-- name: SetCMSPagePublished :one
UPDATE cms_pages
SET is_published = $2, published_at = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: GoLiveCMSPage :one
-- Every column a publish changes, in one statement, so a page is never live with a stale workflow state or unscheduled without being live
UPDATE cms_pages
SET is_published = TRUE, published_at = sqlc.arg(published_at), workflow_state = 'published',
    scheduled_at = CASE WHEN sqlc.arg(clear_schedule)::bool THEN NULL ELSE scheduled_at END,
    updated_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteCMSPage :exec
DELETE FROM cms_pages WHERE id = $1;

-- name: CountCMSPagesByType :one
SELECT COUNT(*) FROM cms_pages WHERE page_type_id = $1;

-- name: CMSPageSlugExists :one
SELECT EXISTS(SELECT 1 FROM cms_pages WHERE slug = $1);

-- name: ListCMSPagesSearch :many
-- Admin full-text search over a locale's content (title + meta + extracted
-- block body text indexed into cms_page_content.search_vector), ranked
SELECT cp.* FROM cms_pages cp
JOIN cms_page_content c ON c.page_id = cp.id AND c.locale = sqlc.arg(locale)::text
WHERE (sqlc.narg(page_type_id)::uuid IS NULL OR cp.page_type_id = sqlc.narg(page_type_id)::uuid)
  AND (sqlc.narg(parent_id)::uuid IS NULL OR cp.parent_id = sqlc.narg(parent_id)::uuid)
  AND (sqlc.narg(only_published)::boolean IS FALSE OR cp.is_published = TRUE)
  AND (sqlc.narg(workflow_state)::text IS NULL OR cp.workflow_state = sqlc.narg(workflow_state)::text)
  AND c.search_vector @@ plainto_tsquery('simple', sqlc.arg(q)::text)
ORDER BY ts_rank(c.search_vector, plainto_tsquery('simple', sqlc.arg(q)::text)) DESC,
         cp.updated_at DESC
LIMIT $1 OFFSET $2;

-- name: UpsertCMSPageContent :one
INSERT INTO cms_page_content (
    page_id, locale, title, body, meta_title, meta_description, meta_keywords, data
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (page_id, locale) DO UPDATE
SET title            = EXCLUDED.title,
    body             = EXCLUDED.body,
    meta_title       = EXCLUDED.meta_title,
    meta_description = EXCLUDED.meta_description,
    meta_keywords    = EXCLUDED.meta_keywords,
    data             = EXCLUDED.data,
    updated_at       = NOW()
RETURNING *;

-- name: SetCMSPageContentSearchVector :exec
-- Repopulate the full-text index for one (page, locale) - `doc` is the pre-assembled title + meta + block-body plain text (the block tree is walked in Go, so this can't be a generated column or a trigger)
UPDATE cms_page_content
SET search_vector = to_tsvector('simple', sqlc.arg(doc)::text)
WHERE page_id = $1 AND locale = $2;

-- name: GetCMSPageContent :one
SELECT * FROM cms_page_content WHERE page_id = $1 AND locale = $2;

-- name: ListCMSPageContent :many
SELECT * FROM cms_page_content WHERE page_id = $1 ORDER BY locale;

-- name: ListCMSPageContentByScope :many
SELECT * FROM cms_page_content
WHERE page_id = ANY(sqlc.arg(page_ids)::uuid[]) AND locale = $1;

-- name: DeleteCMSPageContent :execrows
DELETE FROM cms_page_content WHERE page_id = $1 AND locale = $2;

-- ==================== Page Attribute Values & Relationships ====================

-- name: SetCMSPageAttributeValue :exec
INSERT INTO cms_page_attribute_values (page_id, attribute_id, value_id) VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: ClearCMSPageAttributeValuesForAttribute :exec
DELETE FROM cms_page_attribute_values WHERE page_id = $1 AND attribute_id = $2;

-- name: ListCMSPageAttributeValues :many
SELECT * FROM cms_page_attribute_values WHERE page_id = $1;

-- name: SetCMSPageRelationship :exec
INSERT INTO cms_page_relationships (page_id, attribute_id, target_type, target_id, position)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (page_id, attribute_id, target_type, target_id) DO UPDATE
SET position = EXCLUDED.position;

-- name: ClearCMSPageRelationshipsForAttribute :exec
DELETE FROM cms_page_relationships WHERE page_id = $1 AND attribute_id = $2;

-- name: ListCMSPageRelationships :many
SELECT * FROM cms_page_relationships WHERE page_id = $1 ORDER BY attribute_id, position;

-- ==================== CMS Page Store Listings ====================

-- name: UpsertCMSPageChannelListing :one
INSERT INTO cms_page_channel_listings (page_id, channel_id, is_published, seo_noindex, is_homepage)
VALUES ($1, $2, $3, $4, sqlc.arg(is_homepage))
ON CONFLICT (page_id, channel_id) DO UPDATE
SET is_published = EXCLUDED.is_published,
    seo_noindex  = EXCLUDED.seo_noindex,
    is_homepage  = EXCLUDED.is_homepage,
    updated_at   = NOW()
RETURNING *;

-- name: ClearCMSChannelHomepage :exec
-- Clears the homepage flag from every page on a channel EXCEPT the given one
-- Run before setting a new homepage so the (channel) partial unique index holds
UPDATE cms_page_channel_listings
SET is_homepage = FALSE, updated_at = NOW()
WHERE channel_id = $1 AND is_homepage = TRUE AND page_id <> $2;

-- name: GetCMSHomePageForChannel :one
-- The channel's homepage: the flagged page, both globally and per-channel published
SELECT cp.* FROM cms_pages cp
JOIN cms_page_channel_listings cpsl ON cpsl.page_id = cp.id
WHERE cpsl.channel_id = $1
  AND cpsl.is_homepage   = TRUE
  AND cpsl.is_published  = TRUE
  AND cp.is_published    = TRUE;

-- name: DeleteCMSPageChannelListing :execrows
DELETE FROM cms_page_channel_listings WHERE page_id = $1 AND channel_id = $2;

-- name: ListCMSPageChannelListings :many
SELECT * FROM cms_page_channel_listings WHERE page_id = $1 ORDER BY channel_id;

-- name: ListCMSPageChannelListingsByPageIDs :many
SELECT * FROM cms_page_channel_listings
WHERE page_id = ANY(sqlc.arg(page_ids)::uuid[]);

-- name: ListPublishedCMSPagesForChannel :many
-- Storefront query: pages that are globally published AND listed+published on the given store
SELECT cp.* FROM cms_pages cp
JOIN cms_page_channel_listings cpsl ON cpsl.page_id = cp.id
WHERE cp.is_published = TRUE
  AND cpsl.channel_id = $1
  AND cpsl.is_published  = TRUE
  AND (sqlc.narg(page_type_id)::uuid IS NULL OR cp.page_type_id = sqlc.narg(page_type_id)::uuid)
  AND (sqlc.narg(parent_id)::uuid IS NULL OR cp.parent_id = sqlc.narg(parent_id)::uuid)
ORDER BY cp.position, cp.created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetPublishedCMSPageForChannel :one
SELECT cp.* FROM cms_pages cp
JOIN cms_page_channel_listings cpsl ON cpsl.page_id = cp.id
WHERE cp.id = $1
  AND cpsl.channel_id = $2
  AND cpsl.is_published  = TRUE
  AND cp.is_published    = TRUE;

-- name: GetPublishedCMSPageBySlugForChannel :one
SELECT cp.* FROM cms_pages cp
JOIN cms_page_channel_listings cpsl ON cpsl.page_id = cp.id
WHERE cp.slug = $1
  AND cpsl.channel_id = $2
  AND cpsl.is_published  = TRUE
  AND cp.is_published    = TRUE;

-- name: GetPublishedCMSPageByURLPathForChannel :one
SELECT cp.* FROM cms_pages cp
JOIN cms_page_channel_listings cpsl ON cpsl.page_id = cp.id
WHERE cp.url_path = $1
  AND cpsl.channel_id = $2
  AND cpsl.is_published  = TRUE
  AND cp.is_published    = TRUE;

-- ==================== CMS Snippets (reusable sections) ====================

-- name: CreateCMSSnippet :one
INSERT INTO cms_snippets (slug, name, blocks) VALUES ($1, $2, $3) RETURNING *;

-- name: GetCMSSnippetByID :one
SELECT * FROM cms_snippets WHERE id = $1;

-- name: GetCMSSnippetBySlug :one
SELECT * FROM cms_snippets WHERE slug = $1;

-- name: ListCMSSnippets :many
SELECT * FROM cms_snippets ORDER BY name;

-- name: UpdateCMSSnippet :one
UPDATE cms_snippets
SET name = COALESCE(sqlc.narg(name), name),
    slug = COALESCE(sqlc.narg(slug), slug),
    blocks = COALESCE(sqlc.narg(blocks), blocks),
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteCMSSnippet :exec
DELETE FROM cms_snippets WHERE id = $1;

-- ==================== CMS Block Types (admin-defined custom blocks) ====================

-- name: CreateCMSBlockType :one
INSERT INTO cms_block_types (key, name, icon, fields, html_template)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetCMSBlockTypeByID :one
SELECT * FROM cms_block_types WHERE id = $1;

-- name: GetCMSBlockTypeByKey :one
SELECT * FROM cms_block_types WHERE key = $1;

-- name: ListCMSBlockTypes :many
SELECT * FROM cms_block_types ORDER BY name;

-- name: UpdateCMSBlockType :one
UPDATE cms_block_types
SET name = COALESCE(sqlc.narg(name), name),
    icon = COALESCE(sqlc.narg(icon), icon),
    fields = COALESCE(sqlc.narg(fields), fields),
    html_template = COALESCE(sqlc.narg(html_template), html_template),
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteCMSBlockType :exec
DELETE FROM cms_block_types WHERE id = $1;

-- ==================== CMS Page Revisions (version history) ====================

-- name: CreateCMSPageRevision :one
INSERT INTO cms_page_revisions (
    page_id, locale, title, body, meta_title, meta_description, meta_keywords, data, author_admin_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListCMSPageRevisions :many
SELECT * FROM cms_page_revisions
WHERE page_id = $1 AND locale = $2
ORDER BY created_at DESC
LIMIT $3;

-- name: GetCMSPageRevisionByID :one
SELECT * FROM cms_page_revisions WHERE id = $1;

-- name: PruneCMSPageRevisions :exec
-- Keep the most-recent N revisions for a (page, locale); delete older
DELETE FROM cms_page_revisions AS rev
WHERE rev.page_id = $1 AND rev.locale = $2
  AND rev.id NOT IN (
    SELECT recent.id FROM cms_page_revisions recent
    WHERE recent.page_id = $1 AND recent.locale = $2
    ORDER BY recent.created_at DESC
    LIMIT sqlc.arg(keep_count)
  );

-- ==================== CMS Scheduled publish ====================

-- name: SetCMSPageScheduledAt :one
UPDATE cms_pages SET scheduled_at = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: ListDueScheduledCMSPages :many
-- Unpublished pages whose scheduled time has arrived - the cms-publish job
-- publishes these and clears scheduled_at
SELECT * FROM cms_pages
WHERE is_published = FALSE AND scheduled_at IS NOT NULL AND scheduled_at <= NOW()
ORDER BY scheduled_at;

-- ==================== Tag library ====================

-- name: ListCMSTags :many
SELECT * FROM cms_tags ORDER BY name;

-- name: UpsertCMSTag :one
INSERT INTO cms_tags (slug, name) VALUES ($1, $2)
ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
RETURNING *;

-- name: RenameCMSTag :one
UPDATE cms_tags SET name = $2 WHERE slug = $1 RETURNING *;

-- name: DeleteCMSTag :exec
DELETE FROM cms_tags WHERE slug = $1;

-- name: StripTagFromPages :exec
UPDATE cms_pages SET tags = array_remove(tags, $1), updated_at = NOW()
WHERE tags @> ARRAY[$1::text];

-- name: CMSTagUsageCounts :many
SELECT t AS slug, COUNT(*)::int AS count
FROM cms_pages, unnest(tags) AS t
GROUP BY t;

-- ==================== Editorial workflow ====================

-- name: SubmitCMSPageForReview :one
UPDATE cms_pages
SET workflow_state = 'in_review', submitted_by = $2, submitted_at = NOW(), updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ApproveCMSPageWorkflow :one
UPDATE cms_pages
SET workflow_state = 'approved', approved_by = $2, approved_at = NOW(), updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetCMSPageWorkflowState :one
UPDATE cms_pages
SET workflow_state = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: AssignCMSPage :one
UPDATE cms_pages
SET assigned_to = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: CreateCMSPageComment :one
INSERT INTO cms_page_comments (page_id, locale, block_id, author_id, body)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListCMSPageComments :many
SELECT * FROM cms_page_comments WHERE page_id = $1 ORDER BY created_at;

-- name: ResolveCMSPageComment :one
UPDATE cms_page_comments
SET resolved = $2, resolved_by = $3,
    resolved_at = CASE WHEN $2 THEN NOW() ELSE NULL END
WHERE id = $1
RETURNING *;

-- name: GetCMSPageComment :one
SELECT * FROM cms_page_comments WHERE id = $1;

-- name: DeleteCMSPageComment :exec
DELETE FROM cms_page_comments WHERE id = $1;
