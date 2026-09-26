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
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
)

func (r *postgresRepository) CreateAuthor(ctx context.Context, in CreateAuthorInput) (Author, error) {
	row, err := r.queries.CreateBlogAuthor(ctx, db.CreateBlogAuthorParams{
		Slug:          in.Slug,
		DisplayName:   in.DisplayName,
		Email:         sqlnull.StringValueNonEmpty(ptr.Deref(in.Email)),
		AvatarAssetID: sqlnull.UUID(in.AvatarAssetID),
		LinkedAdminID: sqlnull.UUID(in.LinkedAdminID),
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
		email = sqlnull.StringValueNonEmpty(*in.Email)
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

func toAuthor(row db.BlogAuthor) Author {
	return Author{
		ID:            row.ID,
		Slug:          row.Slug,
		DisplayName:   row.DisplayName,
		Email:         sqlnull.StringPtr(row.Email),
		AvatarAssetID: sqlnull.UUIDPtr(row.AvatarAssetID),
		LinkedAdminID: sqlnull.UUIDPtr(row.LinkedAdminID),
		Metadata:      metadataFromJSON(row.Metadata),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func toAuthorContent(row db.BlogAuthorContent) AuthorContent {
	return AuthorContent{
		Locale:    row.Locale,
		Name:      row.Name,
		Bio:       sqlnull.RawMessage(row.Bio),
		UpdatedAt: row.UpdatedAt,
	}
}
