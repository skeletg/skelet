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
		MetaTitle:       sqlnull.StringValueNonEmpty(in.MetaTitle),
		MetaDescription: sqlnull.StringValueNonEmpty(in.MetaDescription),
		MetaKeywords:    sqlnull.StringValueNonEmpty(in.MetaKeywords),
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
		Description:     sqlnull.RawMessage(row.Description),
		MetaTitle:       sqlnull.StringPtr(row.MetaTitle),
		MetaDescription: sqlnull.StringPtr(row.MetaDescription),
		MetaKeywords:    sqlnull.StringPtr(row.MetaKeywords),
		UpdatedAt:       row.UpdatedAt,
	}
}
