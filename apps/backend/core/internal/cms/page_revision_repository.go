package cms

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
)

func toPageRevision(r db.CmsPageRevision) PageRevision {
	rev := PageRevision{
		ID: r.ID, PageID: r.PageID, Locale: r.Locale, Title: r.Title,
		Data: r.Data, CreatedAt: r.CreatedAt,
	}
	if r.Body.Valid {
		rev.Body = r.Body.RawMessage
	}
	if r.MetaTitle.Valid {
		rev.MetaTitle = &r.MetaTitle.String
	}
	if r.MetaDescription.Valid {
		rev.MetaDescription = &r.MetaDescription.String
	}
	if r.MetaKeywords.Valid {
		rev.MetaKeywords = &r.MetaKeywords.String
	}
	if r.AuthorAdminID.Valid {
		id := r.AuthorAdminID.UUID
		rev.AuthorAdminID = &id
	}
	return rev
}

func (r *postgresRepository) CreateRevision(ctx context.Context, pageID uuid.UUID, in PageContentInput, author *uuid.UUID) (PageRevision, error) {
	data := in.Data
	if data == nil {
		data = json.RawMessage("{}")
	}
	row, err := r.queries.CreateCMSPageRevision(ctx, db.CreateCMSPageRevisionParams{
		PageID:          pageID,
		Locale:          in.Locale,
		Title:           in.Title,
		Body:            pqtype.NullRawMessage{RawMessage: in.Body, Valid: in.Body != nil},
		MetaTitle:       sql.NullString{String: in.MetaTitle, Valid: in.MetaTitle != ""},
		MetaDescription: sql.NullString{String: in.MetaDescription, Valid: in.MetaDescription != ""},
		MetaKeywords:    sql.NullString{String: in.MetaKeywords, Valid: in.MetaKeywords != ""},
		Data:            data,
		AuthorAdminID:   uuid.NullUUID{UUID: ptr.Deref(author), Valid: author != nil},
	})
	if err != nil {
		return PageRevision{}, fmt.Errorf("create cms page revision: %w", err)
	}
	return toPageRevision(row), nil
}

func (r *postgresRepository) ListRevisions(ctx context.Context, pageID uuid.UUID, locale string, limit int32) ([]PageRevision, error) {
	rows, err := r.queries.ListCMSPageRevisions(ctx, db.ListCMSPageRevisionsParams{PageID: pageID, Locale: locale, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("list cms page revisions: %w", err)
	}
	out := make([]PageRevision, len(rows))
	for i, row := range rows {
		out[i] = toPageRevision(row)
	}
	return out, nil
}

func (r *postgresRepository) GetRevision(ctx context.Context, id uuid.UUID) (PageRevision, error) {
	row, err := r.queries.GetCMSPageRevisionByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PageRevision{}, ErrPageNotFound
		}
		return PageRevision{}, fmt.Errorf("get cms page revision: %w", err)
	}
	return toPageRevision(row), nil
}

func (r *postgresRepository) PruneRevisions(ctx context.Context, pageID uuid.UUID, locale string, keep int32) error {
	return r.queries.PruneCMSPageRevisions(ctx, db.PruneCMSPageRevisionsParams{PageID: pageID, Locale: locale, KeepCount: keep})
}

func (r *postgresRepository) SetScheduledAt(ctx context.Context, pageID uuid.UUID, at *time.Time) (Page, error) {
	var st sql.NullTime
	if at != nil {
		st = sql.NullTime{Time: *at, Valid: true}
	}
	row, err := r.queries.SetCMSPageScheduledAt(ctx, db.SetCMSPageScheduledAtParams{ID: pageID, ScheduledAt: st})
	if err != nil {
		return Page{}, fmt.Errorf("set cms page scheduled_at: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) ListDueScheduledPages(ctx context.Context) ([]Page, error) {
	rows, err := r.queries.ListDueScheduledCMSPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("list due scheduled cms pages: %w", err)
	}
	out := make([]Page, len(rows))
	for i, row := range rows {
		out[i] = toPage(row)
	}
	return out, nil
}
