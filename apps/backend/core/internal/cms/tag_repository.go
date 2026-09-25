package cms

import (
	"context"
	"fmt"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

func toTag(row db.CmsTag) Tag {
	return Tag{Slug: row.Slug, Name: row.Name, CreatedAt: row.CreatedAt}
}

func (r *postgresRepository) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := r.queries.ListCMSTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	out := make([]Tag, len(rows))
	for i, row := range rows {
		out[i] = toTag(row)
	}
	return out, nil
}

func (r *postgresRepository) UpsertTag(ctx context.Context, slug, name string) (Tag, error) {
	row, err := r.queries.UpsertCMSTag(ctx, db.UpsertCMSTagParams{Slug: slug, Name: name})
	if err != nil {
		return Tag{}, fmt.Errorf("upsert tag: %w", err)
	}
	return toTag(row), nil
}

func (r *postgresRepository) RenameTag(ctx context.Context, slug, name string) (Tag, error) {
	row, err := r.queries.RenameCMSTag(ctx, db.RenameCMSTagParams{Slug: slug, Name: name})
	if err != nil {
		return Tag{}, fmt.Errorf("rename tag: %w", err)
	}
	return toTag(row), nil
}

// DeleteTag removes a tag from the vocabulary AND strips it from every page that
// carries it (so no dangling slug survives in cms_pages.tags)
func (r *postgresRepository) DeleteTag(ctx context.Context, slug string) error {
	if err := r.queries.StripTagFromPages(ctx, slug); err != nil {
		return fmt.Errorf("strip tag from pages: %w", err)
	}
	if err := r.queries.DeleteCMSTag(ctx, slug); err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}
	return nil
}

func (r *postgresRepository) TagUsageCounts(ctx context.Context) (map[string]int, error) {
	rows, err := r.queries.CMSTagUsageCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("tag usage counts: %w", err)
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		if slug, ok := row.Slug.(string); ok {
			out[slug] = int(row.Count)
		}
	}
	return out, nil
}
