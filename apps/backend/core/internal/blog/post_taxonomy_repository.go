package blog

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

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
			ParentID:    sqlnull.UUIDPtr(row.ParentID),
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
