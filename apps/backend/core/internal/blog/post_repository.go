package blog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

func (r *postgresRepository) CreatePost(ctx context.Context, in CreatePostInput) (Post, error) {
	row, err := r.queries.CreateBlogPost(ctx, db.CreateBlogPostParams{
		Slug:               in.Slug,
		AuthorID:           sqlnull.UUID(in.AuthorID),
		CoverAssetID:       sqlnull.UUID(in.CoverAssetID),
		Status:             string(in.Status),
		PublishedAt:        sqlnull.Time(in.PublishedAt),
		SeoNoindex:         in.SEONoindex,
		ReadingTimeMinutes: sqlnull.Int32(in.ReadingTimeMinutes),
		Metadata:           metadataToJSON(in.Metadata),
		PrivateMetadata:    metadataToJSON(in.PrivateMetadata),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Post{}, ErrSlugTaken
		}
		return Post{}, fmt.Errorf("create blog post: %w", err)
	}
	return toPost(row), nil
}

func (r *postgresRepository) GetPostByID(ctx context.Context, id uuid.UUID) (Post, error) {
	row, err := r.queries.GetBlogPostByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Post{}, ErrPostNotFound
		}
		return Post{}, fmt.Errorf("get blog post: %w", err)
	}
	p := toPost(row)
	all, err := r.queries.ListBlogPostContent(ctx, id)
	if err != nil {
		return Post{}, fmt.Errorf("list blog post content: %w", err)
	}
	p.AllContent = make([]PostContent, len(all))
	for i, c := range all {
		p.AllContent[i] = toPostContent(c)
	}
	cats, err := r.queries.ListBlogPostCategories(ctx, id)
	if err != nil {
		return Post{}, fmt.Errorf("list blog post categories: %w", err)
	}
	p.Categories = make([]Category, len(cats))
	for i, c := range cats {
		p.Categories[i] = toCategory(c)
	}
	tags, err := r.queries.ListBlogPostTags(ctx, id)
	if err != nil {
		return Post{}, fmt.Errorf("list blog post tags: %w", err)
	}
	p.Tags = make([]Tag, len(tags))
	for i, t := range tags {
		p.Tags[i] = toTag(t)
	}
	listings, err := r.queries.ListBlogPostChannelListings(ctx, id)
	if err != nil {
		return Post{}, fmt.Errorf("list blog post store listings: %w", err)
	}
	p.ChannelListings = make([]PostChannelListing, len(listings))
	for i, l := range listings {
		p.ChannelListings[i] = toPostChannelListing(l)
	}
	return p, nil
}

func (r *postgresRepository) GetPostBySlug(ctx context.Context, slug string) (Post, error) {
	row, err := r.queries.GetBlogPostBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Post{}, ErrPostNotFound
		}
		return Post{}, fmt.Errorf("get blog post by slug: %w", err)
	}
	return r.GetPostByID(ctx, row.ID)
}

func (r *postgresRepository) ListPosts(ctx context.Context, filter PostListFilter) ([]Post, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	onlyPub := sql.NullBool{Bool: filter.OnlyPublished, Valid: true}

	var rows []db.BlogPost
	var err error
	// Storefront paths: when ChannelID is supplied, posts must have an active listing on that store - Branch into the
	// *ForChannel queries so the join happens server-side rather than as a post-filter pass
	if filter.ChannelID != nil {
		switch {
		case filter.CategoryID != nil:
			rows, err = r.queries.ListBlogPostsForChannelByCategory(ctx, db.ListBlogPostsForChannelByCategoryParams{
				CategoryID: *filter.CategoryID,
				ChannelID:  *filter.ChannelID,
				Limit:      limit,
				Offset:     offset,
			})
		case filter.TagID != nil:
			rows, err = r.queries.ListBlogPostsForChannelByTag(ctx, db.ListBlogPostsForChannelByTagParams{
				TagID:     *filter.TagID,
				ChannelID: *filter.ChannelID,
				Limit:     limit,
				Offset:    offset,
			})
		case filter.AuthorID != nil:
			rows, err = r.queries.ListBlogPostsForChannelByAuthor(ctx, db.ListBlogPostsForChannelByAuthorParams{
				AuthorID:  uuid.NullUUID{UUID: *filter.AuthorID, Valid: true},
				ChannelID: *filter.ChannelID,
				Limit:     limit,
				Offset:    offset,
			})
		default:
			rows, err = r.queries.ListPublishedBlogPostsForChannel(ctx, db.ListPublishedBlogPostsForChannelParams{
				ChannelID: *filter.ChannelID,
				Limit:     limit,
				Offset:    offset,
			})
		}
	} else {
		switch {
		case filter.CategoryID != nil:
			rows, err = r.queries.ListBlogPostsByCategory(ctx, db.ListBlogPostsByCategoryParams{
				CategoryID:    *filter.CategoryID,
				Limit:         limit,
				Offset:        offset,
				OnlyPublished: onlyPub,
			})
		case filter.TagID != nil:
			rows, err = r.queries.ListBlogPostsByTag(ctx, db.ListBlogPostsByTagParams{
				TagID:         *filter.TagID,
				Limit:         limit,
				Offset:        offset,
				OnlyPublished: onlyPub,
			})
		case filter.AuthorID != nil:
			rows, err = r.queries.ListBlogPostsByAuthor(ctx, db.ListBlogPostsByAuthorParams{
				AuthorID:      uuid.NullUUID{UUID: *filter.AuthorID, Valid: true},
				Limit:         limit,
				Offset:        offset,
				OnlyPublished: onlyPub,
			})
		case filter.OnlyPublished:
			rows, err = r.queries.ListPublishedBlogPosts(ctx, db.ListPublishedBlogPostsParams{
				Limit: limit, Offset: offset,
			})
		default:
			var status sql.NullString
			if filter.Status != nil {
				status = sql.NullString{String: string(*filter.Status), Valid: true}
			}
			rows, err = r.queries.ListBlogPosts(ctx, db.ListBlogPostsParams{
				Limit: limit, Offset: offset, Status: status,
			})
		}
	}
	if err != nil {
		return nil, fmt.Errorf("list blog posts: %w", err)
	}
	out := make([]Post, len(rows))
	for i, row := range rows {
		out[i] = toPost(row)
	}
	return out, nil
}

func (r *postgresRepository) ListDueScheduledPosts(ctx context.Context) ([]Post, error) {
	rows, err := r.queries.ListDueScheduledBlogPosts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list due scheduled blog posts: %w", err)
	}
	out := make([]Post, len(rows))
	for i, row := range rows {
		out[i] = toPost(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdatePost(ctx context.Context, in UpdatePostInput) (Post, error) {
	current, err := r.queries.GetBlogPostByID(ctx, in.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Post{}, ErrPostNotFound
		}
		return Post{}, fmt.Errorf("load blog post: %w", err)
	}

	slug := current.Slug
	if in.Slug != nil {
		slug = *in.Slug
	}
	authorID := current.AuthorID
	if in.ClearAuthor {
		authorID = uuid.NullUUID{}
	} else if in.AuthorID != nil {
		authorID = uuid.NullUUID{UUID: *in.AuthorID, Valid: true}
	}
	cover := current.CoverAssetID
	if in.ClearCover {
		cover = uuid.NullUUID{}
	} else if in.CoverAssetID != nil {
		cover = uuid.NullUUID{UUID: *in.CoverAssetID, Valid: true}
	}
	seoNoindex := current.SeoNoindex
	if in.SEONoindex != nil {
		seoNoindex = *in.SEONoindex
	}
	rt := current.ReadingTimeMinutes
	if in.ReadingTimeMinutes != nil {
		rt = sql.NullInt32{Int32: *in.ReadingTimeMinutes, Valid: true}
	}
	metadata := current.Metadata
	if in.Metadata != nil {
		metadata = metadataToJSON(*in.Metadata)
	}
	privateMetadata := current.PrivateMetadata
	if in.PrivateMetadata != nil {
		privateMetadata = metadataToJSON(*in.PrivateMetadata)
	}

	row, err := r.queries.UpdateBlogPost(ctx, db.UpdateBlogPostParams{
		ID:                 in.ID,
		Slug:               slug,
		AuthorID:           authorID,
		CoverAssetID:       cover,
		SeoNoindex:         seoNoindex,
		ReadingTimeMinutes: rt,
		Metadata:           metadata,
		PrivateMetadata:    privateMetadata,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Post{}, ErrSlugTaken
		}
		return Post{}, fmt.Errorf("update blog post: %w", err)
	}
	return toPost(row), nil
}

func (r *postgresRepository) SetPostStatus(ctx context.Context, id uuid.UUID, status PostStatus, publishedAt *time.Time) (Post, error) {
	row, err := r.queries.SetBlogPostStatus(ctx, db.SetBlogPostStatusParams{
		ID:          id,
		Status:      string(status),
		PublishedAt: sqlnull.Time(publishedAt),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Post{}, ErrPostNotFound
		}
		return Post{}, fmt.Errorf("set blog post status: %w", err)
	}
	return toPost(row), nil
}

func (r *postgresRepository) DeletePost(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteBlogPost(ctx, id); err != nil {
		return fmt.Errorf("delete blog post: %w", err)
	}
	return nil
}

func (r *postgresRepository) UpsertPostContent(ctx context.Context, postID uuid.UUID, in PostContentInput) (PostContent, error) {
	row, err := r.queries.UpsertBlogPostContent(ctx, db.UpsertBlogPostContentParams{
		PostID:          postID,
		Locale:          in.Locale,
		Title:           in.Title,
		Excerpt:         pqtype.NullRawMessage{RawMessage: in.Excerpt, Valid: len(in.Excerpt) > 0},
		Body:            pqtype.NullRawMessage{RawMessage: in.Body, Valid: len(in.Body) > 0},
		MetaTitle:       sqlnull.StringValueNonEmpty(in.MetaTitle),
		MetaDescription: sqlnull.StringValueNonEmpty(in.MetaDescription),
		MetaKeywords:    sqlnull.StringValueNonEmpty(in.MetaKeywords),
	})
	if err != nil {
		return PostContent{}, fmt.Errorf("upsert blog post content: %w", err)
	}
	return toPostContent(row), nil
}

func (r *postgresRepository) GetPostContent(ctx context.Context, postID uuid.UUID, locale string) (PostContent, error) {
	row, err := r.queries.GetBlogPostContent(ctx, db.GetBlogPostContentParams{
		PostID: postID, Locale: locale,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PostContent{}, ErrPostContentNotFound
		}
		return PostContent{}, fmt.Errorf("get blog post content: %w", err)
	}
	return toPostContent(row), nil
}

func (r *postgresRepository) ListPostContent(ctx context.Context, postID uuid.UUID) ([]PostContent, error) {
	rows, err := r.queries.ListBlogPostContent(ctx, postID)
	if err != nil {
		return nil, fmt.Errorf("list blog post content: %w", err)
	}
	out := make([]PostContent, len(rows))
	for i, row := range rows {
		out[i] = toPostContent(row)
	}
	return out, nil
}

func (r *postgresRepository) ListPostContentByScope(ctx context.Context, postIDs []uuid.UUID, locale string) (map[uuid.UUID]PostContent, error) {
	if len(postIDs) == 0 {
		return map[uuid.UUID]PostContent{}, nil
	}
	rows, err := r.queries.ListBlogPostContentByScope(ctx, db.ListBlogPostContentByScopeParams{
		PostIds: postIDs,
		Locale:  locale,
	})
	if err != nil {
		return nil, fmt.Errorf("list blog post content by scope: %w", err)
	}
	out := make(map[uuid.UUID]PostContent, len(rows))
	for _, row := range rows {
		out[row.PostID] = toPostContent(row)
	}
	return out, nil
}

func (r *postgresRepository) DeletePostContent(ctx context.Context, postID uuid.UUID, locale string) error {
	_, err := r.queries.DeleteBlogPostContent(ctx, db.DeleteBlogPostContentParams{
		PostID: postID, Locale: locale,
	})
	if err != nil {
		return fmt.Errorf("delete blog post content: %w", err)
	}
	return nil
}

func toPost(row db.BlogPost) Post {
	var publishedAt *time.Time
	if row.PublishedAt.Valid {
		t := row.PublishedAt.Time
		publishedAt = &t
	}
	var rt *int32
	if row.ReadingTimeMinutes.Valid {
		v := row.ReadingTimeMinutes.Int32
		rt = &v
	}
	return Post{
		ID:                 row.ID,
		Slug:               row.Slug,
		AuthorID:           sqlnull.UUIDPtr(row.AuthorID),
		CoverAssetID:       sqlnull.UUIDPtr(row.CoverAssetID),
		Status:             PostStatus(row.Status),
		PublishedAt:        publishedAt,
		SEONoindex:         row.SeoNoindex,
		ReadingTimeMinutes: rt,
		Metadata:           metadataFromJSON(row.Metadata),
		PrivateMetadata:    metadataFromJSON(row.PrivateMetadata),
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

func toPostContent(row db.BlogPostContent) PostContent {
	return PostContent{
		Locale:          row.Locale,
		Title:           row.Title,
		Excerpt:         sqlnull.RawMessage(row.Excerpt),
		Body:            sqlnull.RawMessage(row.Body),
		MetaTitle:       sqlnull.StringPtr(row.MetaTitle),
		MetaDescription: sqlnull.StringPtr(row.MetaDescription),
		MetaKeywords:    sqlnull.StringPtr(row.MetaKeywords),
		UpdatedAt:       row.UpdatedAt,
	}
}
