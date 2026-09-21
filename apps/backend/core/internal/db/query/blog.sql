-- ==================== Blog Authors ====================

-- name: CreateBlogAuthor :one
INSERT INTO blog_authors (
    slug, display_name, email, avatar_asset_id, linked_admin_id, metadata
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetBlogAuthorByID :one
SELECT * FROM blog_authors WHERE id = $1;

-- name: GetBlogAuthorBySlug :one
SELECT * FROM blog_authors WHERE slug = $1;

-- name: ListBlogAuthors :many
SELECT * FROM blog_authors ORDER BY display_name;

-- name: ListBlogAuthorsByIDs :many
SELECT * FROM blog_authors WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: UpdateBlogAuthor :one
UPDATE blog_authors
SET slug            = $2,
    display_name    = $3,
    email           = $4,
    avatar_asset_id = $5,
    linked_admin_id = $6,
    metadata        = $7,
    updated_at      = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteBlogAuthor :exec
DELETE FROM blog_authors WHERE id = $1;

-- name: UpsertBlogAuthorContent :one
INSERT INTO blog_author_content (author_id, locale, name, bio)
VALUES ($1, $2, $3, $4)
ON CONFLICT (author_id, locale) DO UPDATE
SET name = EXCLUDED.name, bio = EXCLUDED.bio, updated_at = NOW()
RETURNING *;

-- name: GetBlogAuthorContent :one
SELECT * FROM blog_author_content WHERE author_id = $1 AND locale = $2;

-- name: ListBlogAuthorContent :many
SELECT * FROM blog_author_content WHERE author_id = $1 ORDER BY locale;

-- name: ListBlogAuthorContentByScope :many
SELECT * FROM blog_author_content
WHERE author_id = ANY(sqlc.arg(author_ids)::uuid[]) AND locale = $1;

-- name: DeleteBlogAuthorContent :execrows
DELETE FROM blog_author_content WHERE author_id = $1 AND locale = $2;

-- ==================== Blog Categories ====================

-- name: CreateBlogCategory :one
INSERT INTO blog_categories (parent_id, slug, url_path, position, is_published, seo_noindex)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetBlogCategoryByID :one
SELECT * FROM blog_categories WHERE id = $1;

-- name: GetBlogCategoryBySlug :one
SELECT * FROM blog_categories WHERE slug = $1;

-- name: ListAllBlogCategories :many
SELECT * FROM blog_categories ORDER BY position, slug;

-- name: ListBlogCategoryChildren :many
SELECT * FROM blog_categories WHERE parent_id = $1 ORDER BY position, slug;

-- name: ListRootBlogCategories :many
SELECT * FROM blog_categories WHERE parent_id IS NULL ORDER BY position, slug;

-- name: UpdateBlogCategory :one
UPDATE blog_categories
SET parent_id    = $2,
    slug         = $3,
    position     = $4,
    is_published = $5,
    seo_noindex  = $6,
    updated_at   = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateBlogCategoryURLPath :exec
UPDATE blog_categories SET url_path = $2, updated_at = NOW() WHERE id = $1;

-- name: DeleteBlogCategory :exec
DELETE FROM blog_categories WHERE id = $1;

-- name: UpsertBlogCategoryContent :one
INSERT INTO blog_category_content (category_id, locale, name, description, meta_title, meta_description, meta_keywords)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (category_id, locale) DO UPDATE
SET name = EXCLUDED.name,
    description = EXCLUDED.description,
    meta_title = EXCLUDED.meta_title,
    meta_description = EXCLUDED.meta_description,
    meta_keywords = EXCLUDED.meta_keywords,
    updated_at = NOW()
RETURNING *;

-- name: GetBlogCategoryContent :one
SELECT * FROM blog_category_content WHERE category_id = $1 AND locale = $2;

-- name: ListBlogCategoryContent :many
SELECT * FROM blog_category_content WHERE category_id = $1 ORDER BY locale;

-- name: ListBlogCategoryContentByScope :many
SELECT * FROM blog_category_content
WHERE category_id = ANY(sqlc.arg(category_ids)::uuid[]) AND locale = $1;

-- name: DeleteBlogCategoryContent :execrows
DELETE FROM blog_category_content WHERE category_id = $1 AND locale = $2;

-- ==================== Blog Tags ====================

-- name: CreateBlogTag :one
INSERT INTO blog_tags (slug, is_published) VALUES ($1, $2) RETURNING *;

-- name: GetBlogTagByID :one
SELECT * FROM blog_tags WHERE id = $1;

-- name: GetBlogTagBySlug :one
SELECT * FROM blog_tags WHERE slug = $1;

-- name: ListBlogTags :many
SELECT * FROM blog_tags ORDER BY slug;

-- name: UpdateBlogTag :one
UPDATE blog_tags
SET slug = $2, is_published = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteBlogTag :exec
DELETE FROM blog_tags WHERE id = $1;

-- name: UpsertBlogTagContent :one
INSERT INTO blog_tag_content (tag_id, locale, name, description, meta_title, meta_description, meta_keywords)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tag_id, locale) DO UPDATE
SET name = EXCLUDED.name,
    description = EXCLUDED.description,
    meta_title = EXCLUDED.meta_title,
    meta_description = EXCLUDED.meta_description,
    meta_keywords = EXCLUDED.meta_keywords,
    updated_at = NOW()
RETURNING *;

-- name: GetBlogTagContent :one
SELECT * FROM blog_tag_content WHERE tag_id = $1 AND locale = $2;

-- name: ListBlogTagContent :many
SELECT * FROM blog_tag_content WHERE tag_id = $1 ORDER BY locale;

-- name: ListBlogTagContentByScope :many
SELECT * FROM blog_tag_content
WHERE tag_id = ANY(sqlc.arg(tag_ids)::uuid[]) AND locale = $1;

-- name: DeleteBlogTagContent :execrows
DELETE FROM blog_tag_content WHERE tag_id = $1 AND locale = $2;

-- ==================== Blog Posts ====================

-- name: CreateBlogPost :one
INSERT INTO blog_posts (
    slug, author_id, cover_asset_id, status, published_at,
    seo_noindex, reading_time_minutes, metadata, private_metadata
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetBlogPostByID :one
SELECT * FROM blog_posts WHERE id = $1;

-- name: GetBlogPostBySlug :one
SELECT * FROM blog_posts WHERE slug = $1;

-- name: ListBlogPosts :many
SELECT * FROM blog_posts
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY COALESCE(published_at, created_at) DESC, created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListPublishedBlogPosts :many
SELECT * FROM blog_posts
WHERE status = 'published'
ORDER BY published_at DESC NULLS LAST, created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListBlogPostsByCategory :many
SELECT bp.* FROM blog_posts bp
JOIN blog_post_categories bpc ON bpc.post_id = bp.id
WHERE bpc.category_id = $1
  AND (sqlc.narg(only_published)::boolean IS FALSE OR bp.status = 'published')
ORDER BY COALESCE(bp.published_at, bp.created_at) DESC, bp.created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListBlogPostsByTag :many
SELECT bp.* FROM blog_posts bp
JOIN blog_post_tags bpt ON bpt.post_id = bp.id
WHERE bpt.tag_id = $1
  AND (sqlc.narg(only_published)::boolean IS FALSE OR bp.status = 'published')
ORDER BY COALESCE(bp.published_at, bp.created_at) DESC, bp.created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListBlogPostsByAuthor :many
SELECT * FROM blog_posts
WHERE author_id = $1
  AND (sqlc.narg(only_published)::boolean IS FALSE OR status = 'published')
ORDER BY COALESCE(published_at, created_at) DESC, created_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateBlogPost :one
UPDATE blog_posts
SET slug                 = $2,
    author_id            = $3,
    cover_asset_id       = $4,
    seo_noindex          = $5,
    reading_time_minutes = $6,
    metadata             = $7,
    private_metadata     = $8,
    updated_at           = NOW()
WHERE id = $1
RETURNING *;

-- name: SetBlogPostStatus :one
UPDATE blog_posts
SET status       = $2,
    published_at = $3,
    updated_at   = NOW()
WHERE id = $1
RETURNING *;

-- name: ListDueScheduledBlogPosts :many
SELECT * FROM blog_posts
WHERE status = 'scheduled' AND published_at IS NOT NULL AND published_at <= NOW();

-- name: DeleteBlogPost :exec
DELETE FROM blog_posts WHERE id = $1;

-- name: UpsertBlogPostContent :one
INSERT INTO blog_post_content (
    post_id, locale, title, excerpt, body, meta_title, meta_description, meta_keywords
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (post_id, locale) DO UPDATE
SET title = EXCLUDED.title,
    excerpt = EXCLUDED.excerpt,
    body = EXCLUDED.body,
    meta_title = EXCLUDED.meta_title,
    meta_description = EXCLUDED.meta_description,
    meta_keywords = EXCLUDED.meta_keywords,
    updated_at = NOW()
RETURNING *;

-- name: GetBlogPostContent :one
SELECT * FROM blog_post_content WHERE post_id = $1 AND locale = $2;

-- name: ListBlogPostContent :many
SELECT * FROM blog_post_content WHERE post_id = $1 ORDER BY locale;

-- name: ListBlogPostContentByScope :many
SELECT * FROM blog_post_content
WHERE post_id = ANY(sqlc.arg(post_ids)::uuid[]) AND locale = $1;

-- name: DeleteBlogPostContent :execrows
DELETE FROM blog_post_content WHERE post_id = $1 AND locale = $2;

-- ==================== Junctions ====================

-- name: AddBlogPostCategory :exec
INSERT INTO blog_post_categories (post_id, category_id, position)
VALUES ($1, $2, $3)
ON CONFLICT (post_id, category_id) DO UPDATE SET position = EXCLUDED.position;

-- name: RemoveBlogPostCategory :execrows
DELETE FROM blog_post_categories WHERE post_id = $1 AND category_id = $2;

-- name: ClearBlogPostCategories :exec
DELETE FROM blog_post_categories WHERE post_id = $1;

-- name: ListBlogPostCategories :many
SELECT bc.* FROM blog_categories bc
JOIN blog_post_categories bpc ON bpc.category_id = bc.id
WHERE bpc.post_id = $1
ORDER BY bpc.position, bc.slug;

-- name: ListCategoriesByBlogPostIDs :many
SELECT bpc.post_id, bc.* FROM blog_categories bc
JOIN blog_post_categories bpc ON bpc.category_id = bc.id
WHERE bpc.post_id = ANY(sqlc.arg(post_ids)::uuid[])
ORDER BY bpc.position, bc.slug;

-- name: AddBlogPostTag :exec
INSERT INTO blog_post_tags (post_id, tag_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveBlogPostTag :execrows
DELETE FROM blog_post_tags WHERE post_id = $1 AND tag_id = $2;

-- name: ClearBlogPostTags :exec
DELETE FROM blog_post_tags WHERE post_id = $1;

-- name: ListBlogPostTags :many
SELECT bt.* FROM blog_tags bt
JOIN blog_post_tags bpt ON bpt.tag_id = bt.id
WHERE bpt.post_id = $1
ORDER BY bt.slug;

-- name: ListTagsByBlogPostIDs :many
SELECT bpt.post_id, bt.* FROM blog_tags bt
JOIN blog_post_tags bpt ON bpt.tag_id = bt.id
WHERE bpt.post_id = ANY(sqlc.arg(post_ids)::uuid[])
ORDER BY bt.slug;

-- ==================== Blog Post Store Listings ====================

-- name: UpsertBlogPostChannelListing :one
INSERT INTO blog_post_channel_listings (post_id, channel_id, is_published, seo_noindex)
VALUES ($1, $2, $3, $4)
ON CONFLICT (post_id, channel_id) DO UPDATE
SET is_published = EXCLUDED.is_published,
    seo_noindex  = EXCLUDED.seo_noindex,
    updated_at   = NOW()
RETURNING *;

-- name: DeleteBlogPostChannelListing :execrows
DELETE FROM blog_post_channel_listings WHERE post_id = $1 AND channel_id = $2;

-- name: ListBlogPostChannelListings :many
SELECT * FROM blog_post_channel_listings WHERE post_id = $1 ORDER BY channel_id;

-- name: ListBlogPostChannelListingsByPostIDs :many
SELECT * FROM blog_post_channel_listings
WHERE post_id = ANY(sqlc.arg(post_ids)::uuid[]);

-- name: ListPublishedBlogPostsForChannel :many
-- Storefront query: posts that are globally published AND listed+published on the given store
SELECT bp.* FROM blog_posts bp
JOIN blog_post_channel_listings bpsl ON bpsl.post_id = bp.id
WHERE bp.status = 'published'
  AND bpsl.channel_id = $1
  AND bpsl.is_published  = TRUE
ORDER BY bp.published_at DESC NULLS LAST, bp.created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListBlogPostsForChannelByCategory :many
SELECT bp.* FROM blog_posts bp
JOIN blog_post_categories bpc  ON bpc.post_id = bp.id
JOIN blog_post_channel_listings bpsl ON bpsl.post_id = bp.id
WHERE bpc.category_id = $1
  AND bpsl.channel_id = $2
  AND bpsl.is_published  = TRUE
  AND bp.status = 'published'
ORDER BY bp.published_at DESC NULLS LAST, bp.created_at DESC
LIMIT $3 OFFSET $4;

-- name: ListBlogPostsForChannelByTag :many
SELECT bp.* FROM blog_posts bp
JOIN blog_post_tags bpt        ON bpt.post_id = bp.id
JOIN blog_post_channel_listings bpsl ON bpsl.post_id = bp.id
WHERE bpt.tag_id = $1
  AND bpsl.channel_id = $2
  AND bpsl.is_published  = TRUE
  AND bp.status = 'published'
ORDER BY bp.published_at DESC NULLS LAST, bp.created_at DESC
LIMIT $3 OFFSET $4;

-- name: ListBlogPostsForChannelByAuthor :many
SELECT bp.* FROM blog_posts bp
JOIN blog_post_channel_listings bpsl ON bpsl.post_id = bp.id
WHERE bp.author_id = $1
  AND bpsl.channel_id = $2
  AND bpsl.is_published  = TRUE
  AND bp.status = 'published'
ORDER BY bp.published_at DESC NULLS LAST, bp.created_at DESC
LIMIT $3 OFFSET $4;

-- name: GetPublishedBlogPostForChannel :one
SELECT bp.* FROM blog_posts bp
JOIN blog_post_channel_listings bpsl ON bpsl.post_id = bp.id
WHERE bp.id = $1
  AND bpsl.channel_id = $2
  AND bpsl.is_published  = TRUE
  AND bp.status = 'published';

-- name: GetPublishedBlogPostBySlugForChannel :one
SELECT bp.* FROM blog_posts bp
JOIN blog_post_channel_listings bpsl ON bpsl.post_id = bp.id
WHERE bp.slug = $1
  AND bpsl.channel_id = $2
  AND bpsl.is_published  = TRUE
  AND bp.status = 'published';
