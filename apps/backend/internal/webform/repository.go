package webform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	pgdb "github.com/skeletg/skelet/internal/db"
	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/tx"
)

type Repository interface {
	Create(ctx context.Context, channelID uuid.UUID, in SaveInput) (Form, error)
	Update(ctx context.Context, id uuid.UUID, in SaveInput) (Form, error)
	Get(ctx context.Context, id uuid.UUID) (Form, error)
	GetBySlug(ctx context.Context, channelID uuid.UUID, slug string) (Form, error)
	List(ctx context.Context, channelID uuid.UUID, includeDeleted bool) ([]Form, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
	Restore(ctx context.Context, id uuid.UUID) error

	RecordSubmission(ctx context.Context, s Submission) (Submission, error)
	GetSubmission(ctx context.Context, id uuid.UUID) (Submission, error)
	CountSubmissions(ctx context.Context, formID uuid.UUID) (int64, error)
	ListSubmissions(ctx context.Context, formID uuid.UUID, limit, offset int32) ([]Submission, error)

	WithTx(t *sql.Tx) Repository
}

type postgresRepository struct {
	queries *db.Queries
	tx      *tx.Manager
}

func NewRepository(queries *db.Queries, txMgr *tx.Manager) *postgresRepository {
	return &postgresRepository{queries: queries, tx: txMgr}
}

func (r *postgresRepository) WithTx(t *sql.Tx) Repository {
	return &postgresRepository{queries: r.queries.WithTx(t), tx: r.tx}
}

func (r *postgresRepository) Create(ctx context.Context, channelID uuid.UUID, in SaveInput) (Form, error) {
	fieldsJSON, settingsJSON, err := marshalSchema(in.Fields, in.Settings)
	if err != nil {
		return Form{}, err
	}
	row, err := r.queries.CreateWebForm(ctx, db.CreateWebFormParams{
		ChannelID: channelID,
		Slug:      in.Slug,
		Name:      in.Name,
		Status:    string(in.Status),
		Fields:    fieldsJSON,
		Settings:  settingsJSON,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Form{}, ErrSlugTaken
		}
		return Form{}, fmt.Errorf("create web form: %w", err)
	}
	return toForm(row)
}

func (r *postgresRepository) Update(ctx context.Context, id uuid.UUID, in SaveInput) (Form, error) {
	fieldsJSON, settingsJSON, err := marshalSchema(in.Fields, in.Settings)
	if err != nil {
		return Form{}, err
	}
	row, err := r.queries.UpdateWebForm(ctx, db.UpdateWebFormParams{
		ID:       id,
		Name:     in.Name,
		Status:   string(in.Status),
		Fields:   fieldsJSON,
		Settings: settingsJSON,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Form{}, ErrFormNotFound
		}
		if isUniqueViolation(err) {
			return Form{}, ErrSlugTaken
		}
		return Form{}, fmt.Errorf("update web form: %w", err)
	}
	return toForm(row)
}

func (r *postgresRepository) Get(ctx context.Context, id uuid.UUID) (Form, error) {
	row, err := r.queries.GetWebForm(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Form{}, ErrFormNotFound
		}
		return Form{}, fmt.Errorf("get web form: %w", err)
	}
	return toForm(row)
}

func (r *postgresRepository) GetBySlug(ctx context.Context, channelID uuid.UUID, slug string) (Form, error) {
	row, err := r.queries.GetWebFormBySlug(ctx, db.GetWebFormBySlugParams{ChannelID: channelID, Slug: slug})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Form{}, ErrFormNotFound
		}
		return Form{}, fmt.Errorf("get web form by slug: %w", err)
	}
	return toForm(row)
}

func (r *postgresRepository) List(ctx context.Context, channelID uuid.UUID, includeDeleted bool) ([]Form, error) {
	rows, err := r.queries.ListWebForms(ctx, db.ListWebFormsParams{
		ChannelID:      channelID,
		IncludeDeleted: includeDeleted,
	})
	if err != nil {
		return nil, fmt.Errorf("list web forms: %w", err)
	}
	out := make([]Form, 0, len(rows))
	for _, row := range rows {
		f, err := toForm(row)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// SoftDelete marks the row deleted: submissions already collected keep pointing at a
// real form row - Returns ErrFormNotFound when nothing was live to archive
func (r *postgresRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	n, err := r.queries.SoftDeleteWebForm(ctx, id)
	if err != nil {
		return fmt.Errorf("archive web form: %w", err)
	}
	if n == 0 {
		return ErrFormNotFound
	}
	return nil
}

func (r *postgresRepository) Restore(ctx context.Context, id uuid.UUID) error {
	n, err := r.queries.RestoreWebForm(ctx, id)
	if err != nil {
		return fmt.Errorf("restore web form: %w", err)
	}
	if n == 0 {
		return ErrFormNotFound
	}
	return nil
}

func (r *postgresRepository) RecordSubmission(ctx context.Context, s Submission) (Submission, error) {
	answersJSON, err := json.Marshal(s.Answers)
	if err != nil {
		return Submission{}, fmt.Errorf("marshal answers: %w", err)
	}
	filesJSON, err := json.Marshal(s.Files)
	if err != nil {
		return Submission{}, fmt.Errorf("marshal files: %w", err)
	}
	metaJSON, err := json.Marshal(s.Meta)
	if err != nil {
		return Submission{}, fmt.Errorf("marshal meta: %w", err)
	}
	row, err := r.queries.RecordWebFormSubmission(ctx, db.RecordWebFormSubmissionParams{
		FormID:     s.FormID,
		ChannelID:  s.ChannelID,
		Answers:    answersJSON,
		Files:      filesJSON,
		CustomerID: nullUUID(s.CustomerID),
		Meta:       metaJSON,
	})
	if err != nil {
		return Submission{}, fmt.Errorf("record submission: %w", err)
	}
	return toSubmission(row)
}

func (r *postgresRepository) GetSubmission(ctx context.Context, id uuid.UUID) (Submission, error) {
	row, err := r.queries.GetWebFormSubmission(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Submission{}, ErrSubmissionNotFound
		}
		return Submission{}, fmt.Errorf("get submission: %w", err)
	}
	return toSubmission(row)
}

func (r *postgresRepository) CountSubmissions(ctx context.Context, formID uuid.UUID) (int64, error) {
	n, err := r.queries.CountWebFormSubmissions(ctx, formID)
	if err != nil {
		return 0, fmt.Errorf("count submissions: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) ListSubmissions(ctx context.Context, formID uuid.UUID, limit, offset int32) ([]Submission, error) {
	rows, err := r.queries.ListWebFormSubmissions(ctx, db.ListWebFormSubmissionsParams{
		FormID: formID,
		Lim:    limit,
		Off:    offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list submissions: %w", err)
	}
	out := make([]Submission, 0, len(rows))
	for _, row := range rows {
		s, err := toSubmission(row)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// --- mapping ---

func marshalSchema(fields []Field, settings FormSettings) (json.RawMessage, json.RawMessage, error) {
	if fields == nil {
		fields = []Field{}
	}
	fj, err := json.Marshal(fields)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal fields: %w", err)
	}
	sj, err := json.Marshal(settings)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal settings: %w", err)
	}
	return fj, sj, nil
}

func toForm(row db.WebForm) (Form, error) {
	f := Form{
		ID:        row.ID,
		ChannelID: row.ChannelID,
		Slug:      row.Slug,
		Name:      row.Name,
		Status:    Status(row.Status),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
		DeletedAt: nullTimePtr(row.DeletedAt),
	}
	if len(row.Fields) > 0 {
		if err := json.Unmarshal(row.Fields, &f.Fields); err != nil {
			return Form{}, fmt.Errorf("unmarshal fields for form %s: %w", row.ID, err)
		}
	}
	if len(row.Settings) > 0 {
		if err := json.Unmarshal(row.Settings, &f.Settings); err != nil {
			return Form{}, fmt.Errorf("unmarshal settings for form %s: %w", row.ID, err)
		}
	}
	return f, nil
}

func toSubmission(row db.WebFormSubmission) (Submission, error) {
	s := Submission{
		ID:        row.ID,
		FormID:    row.FormID,
		ChannelID: row.ChannelID,
		CreatedAt: row.CreatedAt,
	}
	if row.CustomerID.Valid {
		id := row.CustomerID.UUID
		s.CustomerID = &id
	}
	if len(row.Answers) > 0 {
		if err := json.Unmarshal(row.Answers, &s.Answers); err != nil {
			return Submission{}, fmt.Errorf("unmarshal answers: %w", err)
		}
	}
	if len(row.Files) > 0 {
		if err := json.Unmarshal(row.Files, &s.Files); err != nil {
			return Submission{}, fmt.Errorf("unmarshal files: %w", err)
		}
	}
	if len(row.Meta) > 0 {
		if err := json.Unmarshal(row.Meta, &s.Meta); err != nil {
			return Submission{}, fmt.Errorf("unmarshal meta: %w", err)
		}
	}
	return s, nil
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func isUniqueViolation(err error) bool { return pgdb.IsUniqueViolation(err) } // shared SQLSTATE-based check - the per-package copies drifted into fragile message matching

// nullTimePtr converts a nullable SQL timestamp into the *time.Time the
// domain structs use
func nullTimePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
