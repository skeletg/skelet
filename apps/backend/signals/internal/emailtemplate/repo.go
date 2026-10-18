package emailtemplate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// Repository is the persistence seam for templates + reusable blocks
type Repository interface {
	// read-only: the email library is where templates are authored, but stored per-key rows still resolve at send time
	Get(ctx context.Context, key string, channelID *uuid.UUID) (Template, bool, error)

	GetBlock(ctx context.Context, channelID uuid.UUID, slug string) (Snippet, bool, error)
	ListBlocks(ctx context.Context, channelID uuid.UUID) ([]Snippet, error)
	SaveBlock(ctx context.Context, s Snippet) error
	DeleteBlock(ctx context.Context, channelID uuid.UUID, slug string) error

	// Unified emails (standalone named emails + group + transactional slot)
	CreateLibraryEmail(ctx context.Context, e LibraryEmail) (LibraryEmail, error)
	GetLibraryEmail(ctx context.Context, id uuid.UUID) (LibraryEmail, bool, error)
	ListLibraryEmails(ctx context.Context, channelID uuid.UUID, groupSlug string) ([]LibraryEmail, error)
	GetSlotEmail(ctx context.Context, channelID uuid.UUID, slot string) (LibraryEmail, bool, error)
	UpdateLibraryEmail(ctx context.Context, e LibraryEmail) (bool, error)
	ClearSlot(ctx context.Context, channelID uuid.UUID, slot string) error
	AssignSlot(ctx context.Context, id uuid.UUID, slot string) error
	DeleteLibraryEmail(ctx context.Context, id uuid.UUID) error

	// Email group taxonomy
	ListEmailGroups(ctx context.Context) ([]EmailGroup, error)
	GetEmailGroup(ctx context.Context, id uuid.UUID) (EmailGroup, error)
	CreateEmailGroup(ctx context.Context, name string) (EmailGroup, error)
	RenameEmailGroup(ctx context.Context, id uuid.UUID, name string) (EmailGroup, error)
	DeleteEmailGroup(ctx context.Context, id uuid.UUID) error
	CountEmailsInGroup(ctx context.Context, groupID uuid.UUID) (int64, error)

	// Version history
	InsertRevision(ctx context.Context, e LibraryEmail, adminID *uuid.UUID, keep int) error
	ListRevisions(ctx context.Context, emailID uuid.UUID, limit int32) ([]EmailRevision, error)
	GetRevision(ctx context.Context, id uuid.UUID) (EmailRevision, bool, error)
}

type repo struct{ q *db.Queries }

// NewRepository wires the sqlc-backed repository
func NewRepository(q *db.Queries) Repository { return &repo{q: q} }

func (r *repo) Get(ctx context.Context, key string, channelID *uuid.UUID) (Template, bool, error) {
	row, err := r.q.GetEmailTemplate(ctx, db.GetEmailTemplateParams{Key: key, ChannelID: nullUUID(channelID)})
	if errors.Is(err, sql.ErrNoRows) {
		return Template{}, false, nil
	}
	if err != nil {
		return Template{}, false, err
	}
	t, err := toTemplate(row)
	return t, err == nil, err
}

func (r *repo) GetBlock(ctx context.Context, channelID uuid.UUID, slug string) (Snippet, bool, error) {
	row, err := r.q.GetEmailBlock(ctx, db.GetEmailBlockParams{ChannelID: channelID, Slug: slug})
	if errors.Is(err, sql.ErrNoRows) {
		return Snippet{}, false, nil
	}
	if err != nil {
		return Snippet{}, false, err
	}
	s, err := toSnippet(row)
	return s, err == nil, err
}

func decodeFields(raw json.RawMessage) ([]FieldDef, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var fs []FieldDef
	if err := json.Unmarshal(raw, &fs); err != nil {
		return nil, err
	}
	return fs, nil
}

func (r *repo) ListBlocks(ctx context.Context, channelID uuid.UUID) ([]Snippet, error) {
	rows, err := r.q.ListEmailBlocks(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := make([]Snippet, 0, len(rows))
	for _, row := range rows {
		s, err := toSnippet(row)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *repo) SaveBlock(ctx context.Context, s Snippet) error {
	raw, err := json.Marshal(s.Blocks)
	if err != nil {
		return err
	}
	fields, err := json.Marshal(s.Fields)
	if err != nil {
		return err
	}
	_, err = r.q.UpsertEmailBlock(ctx, db.UpsertEmailBlockParams{
		ChannelID: s.ChannelID, Slug: s.Slug, Name: s.Name, Icon: s.Icon,
		Fields: fields, HtmlTemplate: s.HTMLTemplate, TextTemplate: s.TextTemplate,
		Css: s.CSS, Scripted: s.Scripted, Blocks: raw,
	})
	return err
}

func (r *repo) DeleteBlock(ctx context.Context, channelID uuid.UUID, slug string) error {
	return r.q.DeleteEmailBlock(ctx, db.DeleteEmailBlockParams{ChannelID: channelID, Slug: slug})
}

func toTemplate(row db.EmailTemplate) (Template, error) {
	blocks, err := decodeBlocks(row.Blocks)
	if err != nil {
		return Template{}, err
	}
	t := Template{Key: row.Key, Subject: row.Subject, Blocks: blocks}
	if len(row.Styles) > 0 {
		if err := json.Unmarshal(row.Styles, &t.Styles); err != nil {
			return Template{}, err
		}
	}
	if row.ChannelID.Valid {
		id := row.ChannelID.UUID
		t.ChannelID = &id
	}
	return t, nil
}

func toSnippet(row db.EmailBlock) (Snippet, error) {
	blocks, err := decodeBlocks(row.Blocks)
	if err != nil {
		return Snippet{}, err
	}
	fields, err := decodeFields(row.Fields)
	if err != nil {
		return Snippet{}, err
	}
	return Snippet{
		ChannelID: row.ChannelID, Slug: row.Slug, Name: row.Name, Icon: row.Icon,
		Fields: fields, Blocks: blocks, HTMLTemplate: row.HtmlTemplate, TextTemplate: row.TextTemplate,
		CSS: row.Css, Scripted: row.Scripted,
	}, nil
}

func decodeBlocks(raw json.RawMessage) ([]Block, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var blocks []Block
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
	}
	return blocks, nil
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}
