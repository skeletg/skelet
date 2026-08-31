package blog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	pgdb "github.com/skeletg/skelet/apps/backend/internal/db"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
)

type postgresRepository struct {
	queries *db.Queries
	tx      *tx.Manager
}

func NewRepository(queries *db.Queries, txMgr *tx.Manager) *postgresRepository {
	return &postgresRepository{queries: queries, tx: txMgr}
}

func (r *postgresRepository) WithTx(t *sql.Tx) *postgresRepository {
	return &postgresRepository{queries: r.queries.WithTx(t), tx: r.tx}
}

// ==================== Authors ====================

func (r *postgresRepository) CreateAuthor(ctx context.Context, in CreateAuthorInput) (Author, error) {
	row, err := r.queries.CreateBlogAuthor(ctx, db.CreateBlogAuthorParams{
		Slug:          in.Slug,
		DisplayName:   in.DisplayName,
		Email:         nullableString(ptr.Deref(in.Email)),
		AvatarAssetID: nullUUID(in.AvatarAssetID),
		LinkedAdminID: nullUUID(in.LinkedAdminID),
		Metadata:      metadataToJSON(in.Metadata),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Author{}, ErrSlugTaken
		}
		return Author{}, fmt.Errorf("create blog author: %w", err)
	}
	return toAuthor(row), nil
}

func (r *postgresRepository) GetAuthorByID(ctx context.Context, id uuid.UUID) (Author, error) {
	row, err := r.queries.GetBlogAuthorByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Author{}, ErrAuthorNotFound
		}
		return Author{}, fmt.Errorf("get blog author: %w", err)
	}
	a := toAuthor(row)
	all, err := r.queries.ListBlogAuthorContent(ctx, id)
	if err != nil {
		return Author{}, fmt.Errorf("list blog author content: %w", err)
	}
	a.AllContent = make([]AuthorContent, len(all))
	for i, c := range all {
		a.AllContent[i] = toAuthorContent(c)
	}
	return a, nil
}

func (r *postgresRepository) GetAuthorBySlug(ctx context.Context, slug string) (Author, error) {
	row, err := r.queries.GetBlogAuthorBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Author{}, ErrAuthorNotFound
		}
		return Author{}, fmt.Errorf("get blog author by slug: %w", err)
	}
	return r.GetAuthorByID(ctx, row.ID)
}

func (r *postgresRepository) ListAuthors(ctx context.Context) ([]Author, error) {
	rows, err := r.queries.ListBlogAuthors(ctx)
	if err != nil {
		return nil, fmt.Errorf("list blog authors: %w", err)
	}
	out := make([]Author, len(rows))
	for i, row := range rows {
		out[i] = toAuthor(row)
	}
	return out, nil
}

// ListAuthorsByIDs fetches a set of authors in one round-trip, keyed by id Post lists use it to hydrate bylines without
// an N+1 per row - Content is left to ListAuthorContentByScope so callers pick the locale
func (r *postgresRepository) ListAuthorsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Author, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]Author{}, nil
	}
	rows, err := r.queries.ListBlogAuthorsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list blog authors by ids: %w", err)
	}
	out := make(map[uuid.UUID]Author, len(rows))
	for _, row := range rows {
		a := toAuthor(row)
		out[a.ID] = a
	}
	return out, nil
}

func (r *postgresRepository) UpdateAuthor(ctx context.Context, in UpdateAuthorInput) (Author, error) {
	current, err := r.queries.GetBlogAuthorByID(ctx, in.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Author{}, ErrAuthorNotFound
		}
		return Author{}, fmt.Errorf("load blog author: %w", err)
	}

	slug := current.Slug
	if in.Slug != nil {
		slug = *in.Slug
	}
	displayName := current.DisplayName
	if in.DisplayName != nil {
		displayName = *in.DisplayName
	}
	email := current.Email
	if in.ClearEmail {
		email = sql.NullString{}
	} else if in.Email != nil {
		email = nullableString(*in.Email)
	}
	avatar := current.AvatarAssetID
	if in.ClearAvatar {
		avatar = uuid.NullUUID{}
	} else if in.AvatarAssetID != nil {
		avatar = uuid.NullUUID{UUID: *in.AvatarAssetID, Valid: true}
	}
	admin := current.LinkedAdminID
	if in.ClearAdmin {
		admin = uuid.NullUUID{}
	} else if in.LinkedAdminID != nil {
		admin = uuid.NullUUID{UUID: *in.LinkedAdminID, Valid: true}
	}
	metadata := current.Metadata
	if in.Metadata != nil {
		metadata = metadataToJSON(*in.Metadata)
	}

	row, err := r.queries.UpdateBlogAuthor(ctx, db.UpdateBlogAuthorParams{
		ID:            in.ID,
		Slug:          slug,
		DisplayName:   displayName,
		Email:         email,
		AvatarAssetID: avatar,
		LinkedAdminID: admin,
		Metadata:      metadata,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Author{}, ErrSlugTaken
		}
		return Author{}, fmt.Errorf("update blog author: %w", err)
	}
	return toAuthor(row), nil
}

func (r *postgresRepository) DeleteAuthor(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteBlogAuthor(ctx, id); err != nil {
		return fmt.Errorf("delete blog author: %w", err)
	}
	return nil
}

func (r *postgresRepository) UpsertAuthorContent(ctx context.Context, authorID uuid.UUID, in AuthorContentInput) (AuthorContent, error) {
	row, err := r.queries.UpsertBlogAuthorContent(ctx, db.UpsertBlogAuthorContentParams{
		AuthorID: authorID,
		Locale:   in.Locale,
		Name:     in.Name,
		Bio:      pqtype.NullRawMessage{RawMessage: in.Bio, Valid: len(in.Bio) > 0},
	})
	if err != nil {
		return AuthorContent{}, fmt.Errorf("upsert blog author content: %w", err)
	}
	return toAuthorContent(row), nil
}

func (r *postgresRepository) GetAuthorContent(ctx context.Context, authorID uuid.UUID, locale string) (AuthorContent, error) {
	row, err := r.queries.GetBlogAuthorContent(ctx, db.GetBlogAuthorContentParams{
		AuthorID: authorID, Locale: locale,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AuthorContent{}, ErrAuthorContentNotFound
		}
		return AuthorContent{}, fmt.Errorf("get blog author content: %w", err)
	}
	return toAuthorContent(row), nil
}

func (r *postgresRepository) ListAuthorContent(ctx context.Context, authorID uuid.UUID) ([]AuthorContent, error) {
	rows, err := r.queries.ListBlogAuthorContent(ctx, authorID)
	if err != nil {
		return nil, fmt.Errorf("list blog author content: %w", err)
	}
	out := make([]AuthorContent, len(rows))
	for i, c := range rows {
		out[i] = toAuthorContent(c)
	}
	return out, nil
}

func (r *postgresRepository) ListAuthorContentByScope(ctx context.Context, authorIDs []uuid.UUID, locale string) (map[uuid.UUID]AuthorContent, error) {
	if len(authorIDs) == 0 {
		return map[uuid.UUID]AuthorContent{}, nil
	}
	rows, err := r.queries.ListBlogAuthorContentByScope(ctx, db.ListBlogAuthorContentByScopeParams{
		AuthorIds: authorIDs,
		Locale:    locale,
	})
	if err != nil {
		return nil, fmt.Errorf("list blog author content by scope: %w", err)
	}
	out := make(map[uuid.UUID]AuthorContent, len(rows))
	for _, c := range rows {
		out[c.AuthorID] = toAuthorContent(c)
	}
	return out, nil
}

func (r *postgresRepository) DeleteAuthorContent(ctx context.Context, authorID uuid.UUID, locale string) error {
	_, err := r.queries.DeleteBlogAuthorContent(ctx, db.DeleteBlogAuthorContentParams{
		AuthorID: authorID, Locale: locale,
	})
	if err != nil {
		return fmt.Errorf("delete blog author content: %w", err)
	}
	return nil
}

// ==================== Categories ====================

func (r *postgresRepository) CreateCategory(ctx context.Context, in CreateCategoryInput) (Category, error) {
	row, err := r.queries.CreateBlogCategory(ctx, db.CreateBlogCategoryParams{
		ParentID:    nullUUID(in.ParentID),
		Slug:        in.Slug,
		UrlPath:     in.Slug, // overwritten by the service after parent path is computed
		Position:    in.Position,
		IsPublished: in.IsPublished,
		SeoNoindex:  in.SEONoindex,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Category{}, ErrSlugTaken
		}
		return Category{}, fmt.Errorf("create blog category: %w", err)
	}
	return toCategory(row), nil
}

func (r *postgresRepository) GetCategoryByID(ctx context.Context, id uuid.UUID) (Category, error) {
	row, err := r.queries.GetBlogCategoryByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, fmt.Errorf("get blog category: %w", err)
	}
	c := toCategory(row)
	all, err := r.queries.ListBlogCategoryContent(ctx, id)
	if err != nil {
		return Category{}, fmt.Errorf("list blog category content: %w", err)
	}
	c.AllContent = make([]CategoryContent, len(all))
	for i, cc := range all {
		c.AllContent[i] = toCategoryContent(cc)
	}
	return c, nil
}

func (r *postgresRepository) GetCategoryBySlug(ctx context.Context, slug string) (Category, error) {
	row, err := r.queries.GetBlogCategoryBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, fmt.Errorf("get blog category by slug: %w", err)
	}
	return r.GetCategoryByID(ctx, row.ID)
}

func (r *postgresRepository) ListAllCategories(ctx context.Context) ([]Category, error) {
	rows, err := r.queries.ListAllBlogCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list blog categories: %w", err)
	}
	out := make([]Category, len(rows))
	for i, row := range rows {
		out[i] = toCategory(row)
	}
	return out, nil
}

func (r *postgresRepository) ListRootCategories(ctx context.Context) ([]Category, error) {
	rows, err := r.queries.ListRootBlogCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list root blog categories: %w", err)
	}
	out := make([]Category, len(rows))
	for i, row := range rows {
		out[i] = toCategory(row)
	}
	return out, nil
}

func (r *postgresRepository) ListCategoryChildren(ctx context.Context, parentID uuid.UUID) ([]Category, error) {
	rows, err := r.queries.ListBlogCategoryChildren(ctx, uuid.NullUUID{UUID: parentID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list blog category children: %w", err)
	}
	out := make([]Category, len(rows))
	for i, row := range rows {
		out[i] = toCategory(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateCategory(ctx context.Context, in UpdateCategoryInput) (Category, error) {
	current, err := r.queries.GetBlogCategoryByID(ctx, in.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, fmt.Errorf("load blog category: %w", err)
	}

	parentID := current.ParentID
	if in.ClearParent {
		parentID = uuid.NullUUID{}
	} else if in.ParentID != nil {
		parentID = uuid.NullUUID{UUID: *in.ParentID, Valid: true}
	}
	slug := current.Slug
	if in.Slug != nil {
		slug = *in.Slug
	}
	position := current.Position
	if in.Position != nil {
		position = *in.Position
	}
	isPublished := current.IsPublished
	if in.IsPublished != nil {
		isPublished = *in.IsPublished
	}
	seoNoindex := current.SeoNoindex
	if in.SEONoindex != nil {
		seoNoindex = *in.SEONoindex
	}

	row, err := r.queries.UpdateBlogCategory(ctx, db.UpdateBlogCategoryParams{
		ID:          in.ID,
		ParentID:    parentID,
		Slug:        slug,
		Position:    position,
		IsPublished: isPublished,
		SeoNoindex:  seoNoindex,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Category{}, ErrSlugTaken
		}
		return Category{}, fmt.Errorf("update blog category: %w", err)
	}
	return toCategory(row), nil
}

func (r *postgresRepository) UpdateCategoryURLPath(ctx context.Context, id uuid.UUID, urlPath string) error {
	if err := r.queries.UpdateBlogCategoryURLPath(ctx, db.UpdateBlogCategoryURLPathParams{
		ID:      id,
		UrlPath: urlPath,
	}); err != nil {
		return fmt.Errorf("update blog category url_path: %w", err)
	}
	return nil
}

func (r *postgresRepository) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteBlogCategory(ctx, id); err != nil {
		return fmt.Errorf("delete blog category: %w", err)
	}
	return nil
}

func (r *postgresRepository) UpsertCategoryContent(ctx context.Context, categoryID uuid.UUID, in CategoryContentInput) (CategoryContent, error) {
	row, err := r.queries.UpsertBlogCategoryContent(ctx, db.UpsertBlogCategoryContentParams{
		CategoryID:      categoryID,
		Locale:          in.Locale,
		Name:            in.Name,
		Description:     pqtype.NullRawMessage{RawMessage: in.Description, Valid: len(in.Description) > 0},
		MetaTitle:       nullableString(in.MetaTitle),
		MetaDescription: nullableString(in.MetaDescription),
		MetaKeywords:    nullableString(in.MetaKeywords),
	})
	if err != nil {
		return CategoryContent{}, fmt.Errorf("upsert blog category content: %w", err)
	}
	return toCategoryContent(row), nil
}

func (r *postgresRepository) GetCategoryContent(ctx context.Context, categoryID uuid.UUID, locale string) (CategoryContent, error) {
	row, err := r.queries.GetBlogCategoryContent(ctx, db.GetBlogCategoryContentParams{
		CategoryID: categoryID, Locale: locale,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CategoryContent{}, ErrCategoryContentNotFound
		}
		return CategoryContent{}, fmt.Errorf("get blog category content: %w", err)
	}
	return toCategoryContent(row), nil
}

func (r *postgresRepository) ListCategoryContent(ctx context.Context, categoryID uuid.UUID) ([]CategoryContent, error) {
	rows, err := r.queries.ListBlogCategoryContent(ctx, categoryID)
	if err != nil {
		return nil, fmt.Errorf("list blog category content: %w", err)
	}
	out := make([]CategoryContent, len(rows))
	for i, row := range rows {
		out[i] = toCategoryContent(row)
	}
	return out, nil
}

func (r *postgresRepository) ListCategoryContentByScope(ctx context.Context, categoryIDs []uuid.UUID, locale string) (map[uuid.UUID]CategoryContent, error) {
	if len(categoryIDs) == 0 {
		return map[uuid.UUID]CategoryContent{}, nil
	}
	rows, err := r.queries.ListBlogCategoryContentByScope(ctx, db.ListBlogCategoryContentByScopeParams{
		CategoryIds: categoryIDs,
		Locale:      locale,
	})
	if err != nil {
		return nil, fmt.Errorf("list blog category content by scope: %w", err)
	}
	out := make(map[uuid.UUID]CategoryContent, len(rows))
	for _, row := range rows {
		out[row.CategoryID] = toCategoryContent(row)
	}
	return out, nil
}

func (r *postgresRepository) DeleteCategoryContent(ctx context.Context, categoryID uuid.UUID, locale string) error {
	_, err := r.queries.DeleteBlogCategoryContent(ctx, db.DeleteBlogCategoryContentParams{
		CategoryID: categoryID, Locale: locale,
	})
	if err != nil {
		return fmt.Errorf("delete blog category content: %w", err)
	}
	return nil
}

// ==================== Tags ====================

func (r *postgresRepository) CreateTag(ctx context.Context, in CreateTagInput) (Tag, error) {
	row, err := r.queries.CreateBlogTag(ctx, db.CreateBlogTagParams{
		Slug:        in.Slug,
		IsPublished: in.IsPublished,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Tag{}, ErrSlugTaken
		}
		return Tag{}, fmt.Errorf("create blog tag: %w", err)
	}
	return toTag(row), nil
}

func (r *postgresRepository) GetTagByID(ctx context.Context, id uuid.UUID) (Tag, error) {
	row, err := r.queries.GetBlogTagByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Tag{}, ErrTagNotFound
		}
		return Tag{}, fmt.Errorf("get blog tag: %w", err)
	}
	t := toTag(row)
	all, err := r.queries.ListBlogTagContent(ctx, id)
	if err != nil {
		return Tag{}, fmt.Errorf("list blog tag content: %w", err)
	}
	t.AllContent = make([]TagContent, len(all))
	for i, c := range all {
		t.AllContent[i] = toTagContent(c)
	}
	return t, nil
}

func (r *postgresRepository) GetTagBySlug(ctx context.Context, slug string) (Tag, error) {
	row, err := r.queries.GetBlogTagBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Tag{}, ErrTagNotFound
		}
		return Tag{}, fmt.Errorf("get blog tag by slug: %w", err)
	}
	return r.GetTagByID(ctx, row.ID)
}

func (r *postgresRepository) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := r.queries.ListBlogTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list blog tags: %w", err)
	}
	out := make([]Tag, len(rows))
	for i, row := range rows {
		out[i] = toTag(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateTag(ctx context.Context, in UpdateTagInput) (Tag, error) {
	current, err := r.queries.GetBlogTagByID(ctx, in.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Tag{}, ErrTagNotFound
		}
		return Tag{}, fmt.Errorf("load blog tag: %w", err)
	}
	slug := current.Slug
	if in.Slug != nil {
		slug = *in.Slug
	}
	isPublished := current.IsPublished
	if in.IsPublished != nil {
		isPublished = *in.IsPublished
	}
	row, err := r.queries.UpdateBlogTag(ctx, db.UpdateBlogTagParams{
		ID: in.ID, Slug: slug, IsPublished: isPublished,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Tag{}, ErrSlugTaken
		}
		return Tag{}, fmt.Errorf("update blog tag: %w", err)
	}
	return toTag(row), nil
}

func (r *postgresRepository) DeleteTag(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteBlogTag(ctx, id); err != nil {
		return fmt.Errorf("delete blog tag: %w", err)
	}
	return nil
}

func (r *postgresRepository) UpsertTagContent(ctx context.Context, tagID uuid.UUID, in TagContentInput) (TagContent, error) {
	row, err := r.queries.UpsertBlogTagContent(ctx, db.UpsertBlogTagContentParams{
		TagID:           tagID,
		Locale:          in.Locale,
		Name:            in.Name,
		Description:     pqtype.NullRawMessage{RawMessage: in.Description, Valid: len(in.Description) > 0},
		MetaTitle:       nullableString(in.MetaTitle),
		MetaDescription: nullableString(in.MetaDescription),
		MetaKeywords:    nullableString(in.MetaKeywords),
	})
	if err != nil {
		return TagContent{}, fmt.Errorf("upsert blog tag content: %w", err)
	}
	return toTagContent(row), nil
}

func (r *postgresRepository) GetTagContent(ctx context.Context, tagID uuid.UUID, locale string) (TagContent, error) {
	row, err := r.queries.GetBlogTagContent(ctx, db.GetBlogTagContentParams{
		TagID: tagID, Locale: locale,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TagContent{}, ErrTagContentNotFound
		}
		return TagContent{}, fmt.Errorf("get blog tag content: %w", err)
	}
	return toTagContent(row), nil
}

func (r *postgresRepository) ListTagContent(ctx context.Context, tagID uuid.UUID) ([]TagContent, error) {
	rows, err := r.queries.ListBlogTagContent(ctx, tagID)
	if err != nil {
		return nil, fmt.Errorf("list blog tag content: %w", err)
	}
	out := make([]TagContent, len(rows))
	for i, row := range rows {
		out[i] = toTagContent(row)
	}
	return out, nil
}

func (r *postgresRepository) ListTagContentByScope(ctx context.Context, tagIDs []uuid.UUID, locale string) (map[uuid.UUID]TagContent, error) {
	if len(tagIDs) == 0 {
		return map[uuid.UUID]TagContent{}, nil
	}
	rows, err := r.queries.ListBlogTagContentByScope(ctx, db.ListBlogTagContentByScopeParams{
		TagIds: tagIDs,
		Locale: locale,
	})
	if err != nil {
		return nil, fmt.Errorf("list blog tag content by scope: %w", err)
	}
	out := make(map[uuid.UUID]TagContent, len(rows))
	for _, row := range rows {
		out[row.TagID] = toTagContent(row)
	}
	return out, nil
}

func (r *postgresRepository) DeleteTagContent(ctx context.Context, tagID uuid.UUID, locale string) error {
	_, err := r.queries.DeleteBlogTagContent(ctx, db.DeleteBlogTagContentParams{
		TagID: tagID, Locale: locale,
	})
	if err != nil {
		return fmt.Errorf("delete blog tag content: %w", err)
	}
	return nil
}

// ==================== Posts ====================

func (r *postgresRepository) CreatePost(ctx context.Context, in CreatePostInput) (Post, error) {
	row, err := r.queries.CreateBlogPost(ctx, db.CreateBlogPostParams{
		Slug:               in.Slug,
		AuthorID:           nullUUID(in.AuthorID),
		CoverAssetID:       nullUUID(in.CoverAssetID),
		Status:             string(in.Status),
		PublishedAt:        nullTime(in.PublishedAt),
		SeoNoindex:         in.SEONoindex,
		ReadingTimeMinutes: nullInt32(in.ReadingTimeMinutes),
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
		PublishedAt: nullTime(publishedAt),
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
		MetaTitle:       nullableString(in.MetaTitle),
		MetaDescription: nullableString(in.MetaDescription),
		MetaKeywords:    nullableString(in.MetaKeywords),
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

// ==================== Store listings ====================

func (r *postgresRepository) UpsertPostChannelListing(ctx context.Context, in SetPostChannelListingInput) (PostChannelListing, error) {
	row, err := r.queries.UpsertBlogPostChannelListing(ctx, db.UpsertBlogPostChannelListingParams{
		PostID:      in.PostID,
		ChannelID:   in.ChannelID,
		IsPublished: in.IsPublished,
		SeoNoindex:  in.SEONoindex,
	})
	if err != nil {
		return PostChannelListing{}, fmt.Errorf("upsert post store listing: %w", err)
	}
	return toPostChannelListing(row), nil
}

func (r *postgresRepository) DeletePostChannelListing(ctx context.Context, postID, channelID uuid.UUID) error {
	_, err := r.queries.DeleteBlogPostChannelListing(ctx, db.DeleteBlogPostChannelListingParams{
		PostID: postID, ChannelID: channelID,
	})
	if err != nil {
		return fmt.Errorf("delete post store listing: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListPostChannelListings(ctx context.Context, postID uuid.UUID) ([]PostChannelListing, error) {
	rows, err := r.queries.ListBlogPostChannelListings(ctx, postID)
	if err != nil {
		return nil, fmt.Errorf("list post store listings: %w", err)
	}
	out := make([]PostChannelListing, len(rows))
	for i, row := range rows {
		out[i] = toPostChannelListing(row)
	}
	return out, nil
}

func (r *postgresRepository) ListPostChannelListingsByPostIDs(ctx context.Context, postIDs []uuid.UUID) (map[uuid.UUID][]PostChannelListing, error) {
	if len(postIDs) == 0 {
		return map[uuid.UUID][]PostChannelListing{}, nil
	}
	rows, err := r.queries.ListBlogPostChannelListingsByPostIDs(ctx, postIDs)
	if err != nil {
		return nil, fmt.Errorf("list post store listings by post ids: %w", err)
	}
	out := make(map[uuid.UUID][]PostChannelListing)
	for _, row := range rows {
		out[row.PostID] = append(out[row.PostID], toPostChannelListing(row))
	}
	return out, nil
}

func (r *postgresRepository) GetPublishedPostForChannel(ctx context.Context, postID, channelID uuid.UUID) (Post, error) {
	row, err := r.queries.GetPublishedBlogPostForChannel(ctx, db.GetPublishedBlogPostForChannelParams{
		ID: postID, ChannelID: channelID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Post{}, ErrPostNotFound
		}
		return Post{}, fmt.Errorf("get published post for store: %w", err)
	}
	return r.GetPostByID(ctx, row.ID)
}

func (r *postgresRepository) GetPublishedPostBySlugForChannel(ctx context.Context, slug string, channelID uuid.UUID) (Post, error) {
	row, err := r.queries.GetPublishedBlogPostBySlugForChannel(ctx, db.GetPublishedBlogPostBySlugForChannelParams{
		Slug: slug, ChannelID: channelID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Post{}, ErrPostNotFound
		}
		return Post{}, fmt.Errorf("get published post by slug for store: %w", err)
	}
	return r.GetPostByID(ctx, row.ID)
}

// ==================== Junctions ====================

func (r *postgresRepository) AddPostCategory(ctx context.Context, postID, categoryID uuid.UUID, position int32) error {
	if err := r.queries.AddBlogPostCategory(ctx, db.AddBlogPostCategoryParams{
		PostID: postID, CategoryID: categoryID, Position: position,
	}); err != nil {
		return fmt.Errorf("add blog post category: %w", err)
	}
	return nil
}

func (r *postgresRepository) RemovePostCategory(ctx context.Context, postID, categoryID uuid.UUID) error {
	_, err := r.queries.RemoveBlogPostCategory(ctx, db.RemoveBlogPostCategoryParams{
		PostID: postID, CategoryID: categoryID,
	})
	if err != nil {
		return fmt.Errorf("remove blog post category: %w", err)
	}
	return nil
}

func (r *postgresRepository) ClearPostCategories(ctx context.Context, postID uuid.UUID) error {
	if err := r.queries.ClearBlogPostCategories(ctx, postID); err != nil {
		return fmt.Errorf("clear blog post categories: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListPostCategories(ctx context.Context, postID uuid.UUID) ([]Category, error) {
	rows, err := r.queries.ListBlogPostCategories(ctx, postID)
	if err != nil {
		return nil, fmt.Errorf("list blog post categories: %w", err)
	}
	out := make([]Category, len(rows))
	for i, row := range rows {
		out[i] = toCategory(row)
	}
	return out, nil
}

func (r *postgresRepository) ListCategoriesByPostIDs(ctx context.Context, postIDs []uuid.UUID) (map[uuid.UUID][]Category, error) {
	if len(postIDs) == 0 {
		return map[uuid.UUID][]Category{}, nil
	}
	rows, err := r.queries.ListCategoriesByBlogPostIDs(ctx, postIDs)
	if err != nil {
		return nil, fmt.Errorf("list categories by post ids: %w", err)
	}
	out := make(map[uuid.UUID][]Category)
	for _, row := range rows {
		c := Category{
			ID:          row.ID,
			ParentID:    nullUUIDPtr(row.ParentID),
			Slug:        row.Slug,
			URLPath:     row.UrlPath,
			Position:    row.Position,
			IsPublished: row.IsPublished,
			SEONoindex:  row.SeoNoindex,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		}
		out[row.PostID] = append(out[row.PostID], c)
	}
	return out, nil
}

func (r *postgresRepository) AddPostTag(ctx context.Context, postID, tagID uuid.UUID) error {
	if err := r.queries.AddBlogPostTag(ctx, db.AddBlogPostTagParams{
		PostID: postID, TagID: tagID,
	}); err != nil {
		return fmt.Errorf("add blog post tag: %w", err)
	}
	return nil
}

func (r *postgresRepository) RemovePostTag(ctx context.Context, postID, tagID uuid.UUID) error {
	_, err := r.queries.RemoveBlogPostTag(ctx, db.RemoveBlogPostTagParams{
		PostID: postID, TagID: tagID,
	})
	if err != nil {
		return fmt.Errorf("remove blog post tag: %w", err)
	}
	return nil
}

func (r *postgresRepository) ClearPostTags(ctx context.Context, postID uuid.UUID) error {
	if err := r.queries.ClearBlogPostTags(ctx, postID); err != nil {
		return fmt.Errorf("clear blog post tags: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListPostTags(ctx context.Context, postID uuid.UUID) ([]Tag, error) {
	rows, err := r.queries.ListBlogPostTags(ctx, postID)
	if err != nil {
		return nil, fmt.Errorf("list blog post tags: %w", err)
	}
	out := make([]Tag, len(rows))
	for i, row := range rows {
		out[i] = toTag(row)
	}
	return out, nil
}

func (r *postgresRepository) ListTagsByPostIDs(ctx context.Context, postIDs []uuid.UUID) (map[uuid.UUID][]Tag, error) {
	if len(postIDs) == 0 {
		return map[uuid.UUID][]Tag{}, nil
	}
	rows, err := r.queries.ListTagsByBlogPostIDs(ctx, postIDs)
	if err != nil {
		return nil, fmt.Errorf("list tags by post ids: %w", err)
	}
	out := make(map[uuid.UUID][]Tag)
	for _, row := range rows {
		t := Tag{
			ID:          row.ID,
			Slug:        row.Slug,
			IsPublished: row.IsPublished,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		}
		out[row.PostID] = append(out[row.PostID], t)
	}
	return out, nil
}

// ==================== mappers ====================

func toAuthor(row db.BlogAuthor) Author {
	return Author{
		ID:            row.ID,
		Slug:          row.Slug,
		DisplayName:   row.DisplayName,
		Email:         nullStringPtr(row.Email),
		AvatarAssetID: nullUUIDPtr(row.AvatarAssetID),
		LinkedAdminID: nullUUIDPtr(row.LinkedAdminID),
		Metadata:      metadataFromJSON(row.Metadata),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func toAuthorContent(row db.BlogAuthorContent) AuthorContent {
	return AuthorContent{
		Locale:    row.Locale,
		Name:      row.Name,
		Bio:       nullRawMessage(row.Bio),
		UpdatedAt: row.UpdatedAt,
	}
}

func toCategory(row db.BlogCategory) Category {
	return Category{
		ID:          row.ID,
		ParentID:    nullUUIDPtr(row.ParentID),
		Slug:        row.Slug,
		URLPath:     row.UrlPath,
		Position:    row.Position,
		IsPublished: row.IsPublished,
		SEONoindex:  row.SeoNoindex,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func toCategoryContent(row db.BlogCategoryContent) CategoryContent {
	return CategoryContent{
		Locale:          row.Locale,
		Name:            row.Name,
		Description:     nullRawMessage(row.Description),
		MetaTitle:       nullStringPtr(row.MetaTitle),
		MetaDescription: nullStringPtr(row.MetaDescription),
		MetaKeywords:    nullStringPtr(row.MetaKeywords),
		UpdatedAt:       row.UpdatedAt,
	}
}

func toTag(row db.BlogTag) Tag {
	return Tag{
		ID:          row.ID,
		Slug:        row.Slug,
		IsPublished: row.IsPublished,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func toTagContent(row db.BlogTagContent) TagContent {
	return TagContent{
		Locale:          row.Locale,
		Name:            row.Name,
		Description:     nullRawMessage(row.Description),
		MetaTitle:       nullStringPtr(row.MetaTitle),
		MetaDescription: nullStringPtr(row.MetaDescription),
		MetaKeywords:    nullStringPtr(row.MetaKeywords),
		UpdatedAt:       row.UpdatedAt,
	}
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
		AuthorID:           nullUUIDPtr(row.AuthorID),
		CoverAssetID:       nullUUIDPtr(row.CoverAssetID),
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

func toPostChannelListing(row db.BlogPostChannelListing) PostChannelListing {
	return PostChannelListing{
		ChannelID:   row.ChannelID,
		IsPublished: row.IsPublished,
		SEONoindex:  row.SeoNoindex,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func toPostContent(row db.BlogPostContent) PostContent {
	return PostContent{
		Locale:          row.Locale,
		Title:           row.Title,
		Excerpt:         nullRawMessage(row.Excerpt),
		Body:            nullRawMessage(row.Body),
		MetaTitle:       nullStringPtr(row.MetaTitle),
		MetaDescription: nullStringPtr(row.MetaDescription),
		MetaKeywords:    nullStringPtr(row.MetaKeywords),
		UpdatedAt:       row.UpdatedAt,
	}
}

// ==================== helpers ====================

func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullStringPtr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func nullUUIDPtr(n uuid.NullUUID) *uuid.UUID {
	if !n.Valid {
		return nil
	}
	v := n.UUID
	return &v
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func nullInt32(n *int32) sql.NullInt32 {
	if n == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: *n, Valid: true}
}

func nullRawMessage(n pqtype.NullRawMessage) json.RawMessage {
	if !n.Valid {
		return nil
	}
	return n.RawMessage
}

func metadataToJSON(m Metadata) json.RawMessage {
	if len(m) == 0 {
		return json.RawMessage(`{}`)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func metadataFromJSON(raw json.RawMessage) Metadata {
	if len(raw) == 0 {
		return Metadata{}
	}
	var m Metadata
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return Metadata{}
	}
	return m
}

func isUniqueViolation(err error) bool { return pgdb.IsUniqueViolation(err) } // shared SQLSTATE-based check - the per-package copies drifted into fragile message matching
