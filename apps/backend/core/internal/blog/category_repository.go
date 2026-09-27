package blog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

func (r *postgresRepository) CreateCategory(ctx context.Context, in CreateCategoryInput) (Category, error) {
	row, err := r.queries.CreateBlogCategory(ctx, db.CreateBlogCategoryParams{
		ParentID:    sqlnull.UUID(in.ParentID),
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
		MetaTitle:       sqlnull.StringValueNonEmpty(in.MetaTitle),
		MetaDescription: sqlnull.StringValueNonEmpty(in.MetaDescription),
		MetaKeywords:    sqlnull.StringValueNonEmpty(in.MetaKeywords),
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

func toCategory(row db.BlogCategory) Category {
	return Category{
		ID:          row.ID,
		ParentID:    sqlnull.UUIDPtr(row.ParentID),
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
		Description:     sqlnull.RawMessage(row.Description),
		MetaTitle:       sqlnull.StringPtr(row.MetaTitle),
		MetaDescription: sqlnull.StringPtr(row.MetaDescription),
		MetaKeywords:    sqlnull.StringPtr(row.MetaKeywords),
		UpdatedAt:       row.UpdatedAt,
	}
}

// Every locale's row per category, keyed by id - one with no rows is simply absent
func (r *postgresRepository) ListCategoryContentFor(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]CategoryContent, error) {
	out := make(map[uuid.UUID][]CategoryContent, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListBlogCategoryContentForCategories(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("category content by ids: %w", err)
	}
	for _, row := range rows {
		out[row.CategoryID] = append(out[row.CategoryID], toCategoryContent(row))
	}
	return out, nil
}
