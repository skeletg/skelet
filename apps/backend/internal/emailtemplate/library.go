package emailtemplate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/internal/db/sqlc"
)

// LibraryEmail is a standalone, named email (subject + typed blocks + styles)
// built once in the visual canvas - the unified "emails" record - GroupID places
// it in the admin group taxonomy; Slot (when non-empty) binds it to a
// transactional send slot (order_confirmation, …), which the render path
// resolves before the built-in fallback
type LibraryEmail struct {
	ID        uuid.UUID
	ChannelID uuid.UUID
	Name      string
	Subject   string
	Preheader string
	Blocks    []Block
	Styles    EmailStyles
	GroupID   *uuid.UUID
	Slot      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// EmailGroup is one row of the admin-managed group taxonomy
type EmailGroup struct {
	ID        uuid.UUID
	Slug      string
	Name      string
	IsSystem  bool
	SortOrder int
}

// EmailRevision is a saved content snapshot of an email (version history)
type EmailRevision struct {
	ID            uuid.UUID
	EmailID       uuid.UUID
	Name          string
	Subject       string
	Preheader     string
	Blocks        []Block
	Styles        EmailStyles
	AuthorAdminID *uuid.UUID
	CreatedAt     time.Time
}

// maxRevisionsKept bounds version history per email
const maxRevisionsKept = 40

// Snapshot records the email's current content as a revision (called after each
// save) - Best-effort + pruned to the newest maxRevisionsKept
func (s *Service) Snapshot(ctx context.Context, e LibraryEmail, adminID *uuid.UUID) error {
	return s.repo.InsertRevision(ctx, e, adminID, maxRevisionsKept)
}

func (s *Service) ListRevisions(ctx context.Context, emailID uuid.UUID, limit int32) ([]EmailRevision, error) {
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	return s.repo.ListRevisions(ctx, emailID, limit)
}

// RestoreRevision copies a revision's content back onto its email (which itself snapshots the restored state), and
// returns the updated email - Group + slot are organizational, not content, so they're preserved
func (s *Service) RestoreRevision(ctx context.Context, revisionID uuid.UUID, adminID *uuid.UUID) (LibraryEmail, error) {
	rev, ok, err := s.repo.GetRevision(ctx, revisionID)
	if err != nil || !ok {
		return LibraryEmail{}, fmt.Errorf("revision not found")
	}
	e, ok, err := s.repo.GetLibraryEmail(ctx, rev.EmailID)
	if err != nil || !ok {
		return LibraryEmail{}, fmt.Errorf("email not found")
	}
	e.Name, e.Subject, e.Preheader, e.Blocks, e.Styles = rev.Name, rev.Subject, rev.Preheader, rev.Blocks, rev.Styles
	if _, err := s.repo.UpdateLibraryEmail(ctx, e); err != nil {
		return LibraryEmail{}, err
	}
	_ = s.Snapshot(ctx, e, adminID)
	got, _, err := s.repo.GetLibraryEmail(ctx, e.ID)
	return got, err
}

// TransactionalSlots are the send slots a Transactional email can be assigned
// to - Mirrors the transactional template keys (order/auth), excluding layout
var TransactionalSlots = []string{
	KeyOrderConfirmation, KeyOrderShipment, KeyOrderCancellation, KeyOrderRefund,
	KeyVerification, KeyPasswordReset,
}

func isTransactionalSlot(slot string) bool {
	return slices.Contains(TransactionalSlots, slot)
}

// --- service ---

func (s *Service) CreateLibraryEmail(ctx context.Context, e LibraryEmail) (LibraryEmail, error) {
	if err := validateLibraryEmail(s, e); err != nil {
		return LibraryEmail{}, err
	}
	return s.repo.CreateLibraryEmail(ctx, e)
}

// ListEmailGroups returns the group taxonomy (system groups first by sort order)
func (s *Service) ListEmailGroups(ctx context.Context) ([]EmailGroup, error) {
	return s.repo.ListEmailGroups(ctx)
}

// CreateEmailGroup adds an operator group; slug derived from the name
func (s *Service) CreateEmailGroup(ctx context.Context, name string) (EmailGroup, error) {
	if strings.TrimSpace(name) == "" {
		return EmailGroup{}, fmt.Errorf("group name is required")
	}
	return s.repo.CreateEmailGroup(ctx, name)
}

// RenameEmailGroup renames a group (system groups can be renamed, not deleted)
func (s *Service) RenameEmailGroup(ctx context.Context, id uuid.UUID, name string) (EmailGroup, error) {
	if strings.TrimSpace(name) == "" {
		return EmailGroup{}, fmt.Errorf("group name is required")
	}
	return s.repo.RenameEmailGroup(ctx, id, name)
}

// DeleteEmailGroup removes a group (including the seeded Transactional / Marketing ones - nothing is hardcoded) -
// Emails in it fall back to no group (ON DELETE SET NULL); they remain in the library, just untagged
func (s *Service) DeleteEmailGroup(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteEmailGroup(ctx, id)
}

// AssignSlot binds an email to a transactional slot (or clears it with slot="") Enforces one email per (channel, slot)
// by freeing the slot first - Only emails in the Transactional group should be assigned; the resolver only reads slots
func (s *Service) AssignSlot(ctx context.Context, emailID uuid.UUID, slot string) error {
	if slot != "" && !isTransactionalSlot(slot) {
		return fmt.Errorf("unknown transactional slot %q", slot)
	}
	e, ok, err := s.repo.GetLibraryEmail(ctx, emailID)
	if err != nil || !ok {
		return fmt.Errorf("email not found")
	}
	if slot != "" {
		if err := s.repo.ClearSlot(ctx, e.ChannelID, slot); err != nil {
			return err
		}
	}
	return s.repo.AssignSlot(ctx, emailID, slot)
}

func (s *Service) UpdateLibraryEmail(ctx context.Context, e LibraryEmail) (LibraryEmail, error) {
	if err := validateLibraryEmail(s, e); err != nil {
		return LibraryEmail{}, err
	}
	ok, err := s.repo.UpdateLibraryEmail(ctx, e)
	if err != nil {
		return LibraryEmail{}, err
	}
	if !ok {
		return LibraryEmail{}, fmt.Errorf("email not found")
	}
	got, _, err := s.repo.GetLibraryEmail(ctx, e.ID)
	return got, err
}

func (s *Service) GetLibraryEmail(ctx context.Context, id uuid.UUID) (LibraryEmail, bool, error) {
	return s.repo.GetLibraryEmail(ctx, id)
}

// ListLibraryEmails lists a channel's emails, optionally filtered to one group
// slug (” = all) - Powers the unified emails list + group filter
func (s *Service) ListLibraryEmails(ctx context.Context, channelID uuid.UUID, groupSlug string) ([]LibraryEmail, error) {
	return s.repo.ListLibraryEmails(ctx, channelID, groupSlug)
}

// SlotEmail returns the email assigned to a transactional slot for a channel
func (s *Service) SlotEmail(ctx context.Context, channelID uuid.UUID, slot string) (LibraryEmail, bool, error) {
	return s.repo.GetSlotEmail(ctx, channelID, slot)
}

// CountEmailsInGroup is the per-group email tally for the group chips
func (s *Service) CountEmailsInGroup(ctx context.Context, groupID uuid.UUID) (int64, error) {
	return s.repo.CountEmailsInGroup(ctx, groupID)
}

func (s *Service) DeleteLibraryEmail(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteLibraryEmail(ctx, id)
}

// RenderLibraryEmail loads a library email + renders it with the caller's merge vars - the shared render path for flow
// send-email nodes and campaigns that reference a library email
func (s *Service) RenderLibraryEmail(ctx context.Context, emailID uuid.UUID, vars map[string]string) (subject, html, text string, err error) {
	e, ok, err := s.repo.GetLibraryEmail(ctx, emailID)
	if err != nil {
		return "", "", "", err
	}
	if !ok {
		return "", "", "", fmt.Errorf("library email %s not found", emailID)
	}
	return s.RenderContent(ctx, e.ChannelID, e.Subject, e.Preheader, e.Blocks, e.Styles, vars)
}

func validateLibraryEmail(s *Service, e LibraryEmail) error {
	if strings.TrimSpace(e.Name) == "" {
		return fmt.Errorf("email name is required")
	}
	return s.ValidateContent(e.Blocks, e.Styles)
}

// --- repo impl ---

func (r *repo) CreateLibraryEmail(ctx context.Context, e LibraryEmail) (LibraryEmail, error) {
	blocks, styles, err := marshalLibrary(e)
	if err != nil {
		return LibraryEmail{}, err
	}
	row, err := r.q.CreateLibraryEmail(ctx, db.CreateLibraryEmailParams{
		ChannelID: e.ChannelID, Name: e.Name, Subject: e.Subject, Preheader: e.Preheader, Blocks: blocks, Styles: styles,
		GroupID: nullUUIDp(e.GroupID), Slot: e.Slot,
	})
	if err != nil {
		return LibraryEmail{}, err
	}
	return toLibraryEmail(row)
}

func (r *repo) GetLibraryEmail(ctx context.Context, id uuid.UUID) (LibraryEmail, bool, error) {
	row, err := r.q.GetLibraryEmail(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return LibraryEmail{}, false, nil
	}
	if err != nil {
		return LibraryEmail{}, false, err
	}
	e, err := toLibraryEmail(row)
	return e, err == nil, err
}

func (r *repo) ListLibraryEmails(ctx context.Context, channelID uuid.UUID, groupSlug string) ([]LibraryEmail, error) {
	rows, err := r.q.ListLibraryEmails(ctx, db.ListLibraryEmailsParams{ChannelID: channelID, GroupSlug: groupSlug})
	if err != nil {
		return nil, err
	}
	out := make([]LibraryEmail, 0, len(rows))
	for _, row := range rows {
		e, err := toLibraryEmail(row)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *repo) GetSlotEmail(ctx context.Context, channelID uuid.UUID, slot string) (LibraryEmail, bool, error) {
	row, err := r.q.GetSlotEmail(ctx, db.GetSlotEmailParams{ChannelID: channelID, Slot: slot})
	if errors.Is(err, sql.ErrNoRows) {
		return LibraryEmail{}, false, nil
	}
	if err != nil {
		return LibraryEmail{}, false, err
	}
	e, err := toLibraryEmail(row)
	return e, err == nil, err
}

func (r *repo) UpdateLibraryEmail(ctx context.Context, e LibraryEmail) (bool, error) {
	blocks, styles, err := marshalLibrary(e)
	if err != nil {
		return false, err
	}
	n, err := r.q.UpdateLibraryEmail(ctx, db.UpdateLibraryEmailParams{
		Name: e.Name, Subject: e.Subject, Preheader: e.Preheader, Blocks: blocks, Styles: styles, GroupID: nullUUIDp(e.GroupID), ID: e.ID,
	})
	return n > 0, err
}

func (r *repo) ClearSlot(ctx context.Context, channelID uuid.UUID, slot string) error {
	return r.q.ClearLibraryEmailSlot(ctx, db.ClearLibraryEmailSlotParams{ChannelID: channelID, Slot: slot})
}

func (r *repo) AssignSlot(ctx context.Context, id uuid.UUID, slot string) error {
	return r.q.AssignLibraryEmailSlot(ctx, db.AssignLibraryEmailSlotParams{ID: id, Slot: slot})
}

func (r *repo) DeleteLibraryEmail(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteLibraryEmail(ctx, id)
}

// --- email groups ---

func (r *repo) ListEmailGroups(ctx context.Context) ([]EmailGroup, error) {
	rows, err := r.q.ListEmailGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]EmailGroup, 0, len(rows))
	for _, g := range rows {
		out = append(out, toEmailGroup(g))
	}
	return out, nil
}

func (r *repo) GetEmailGroup(ctx context.Context, id uuid.UUID) (EmailGroup, error) {
	g, err := r.q.GetEmailGroup(ctx, id)
	if err != nil {
		return EmailGroup{}, err
	}
	return toEmailGroup(g), nil
}

func (r *repo) CreateEmailGroup(ctx context.Context, name string) (EmailGroup, error) {
	g, err := r.q.CreateEmailGroup(ctx, db.CreateEmailGroupParams{Slug: slugifyGroup(name), Name: name, SortOrder: 100})
	if err != nil {
		return EmailGroup{}, err
	}
	return toEmailGroup(g), nil
}

func (r *repo) RenameEmailGroup(ctx context.Context, id uuid.UUID, name string) (EmailGroup, error) {
	g, err := r.q.RenameEmailGroup(ctx, db.RenameEmailGroupParams{ID: id, Name: name})
	if err != nil {
		return EmailGroup{}, err
	}
	return toEmailGroup(g), nil
}

func (r *repo) DeleteEmailGroup(ctx context.Context, id uuid.UUID) error {
	_, err := r.q.DeleteEmailGroup(ctx, id)
	return err
}

func (r *repo) CountEmailsInGroup(ctx context.Context, groupID uuid.UUID) (int64, error) {
	return r.q.CountEmailsInGroup(ctx, uuid.NullUUID{UUID: groupID, Valid: true})
}

// --- revisions ---

func (r *repo) InsertRevision(ctx context.Context, e LibraryEmail, adminID *uuid.UUID, keep int) error {
	blocks, styles, err := marshalLibrary(e)
	if err != nil {
		return err
	}
	if err := r.q.InsertEmailRevision(ctx, db.InsertEmailRevisionParams{
		EmailID: e.ID, Name: e.Name, Subject: e.Subject, Preheader: e.Preheader,
		Blocks: blocks, Styles: styles, AuthorAdminID: nullUUIDp(adminID),
	}); err != nil {
		return err
	}
	return r.q.PruneEmailRevisions(ctx, db.PruneEmailRevisionsParams{EmailID: e.ID, Keep: int32(keep)})
}

func (r *repo) ListRevisions(ctx context.Context, emailID uuid.UUID, limit int32) ([]EmailRevision, error) {
	rows, err := r.q.ListEmailRevisions(ctx, db.ListEmailRevisionsParams{EmailID: emailID, Lim: limit})
	if err != nil {
		return nil, err
	}
	out := make([]EmailRevision, 0, len(rows))
	for _, row := range rows {
		rev, err := toRevision(row.ID, row.EmailID, row.Name, row.Subject, row.Preheader, row.Blocks, row.Styles, row.AuthorAdminID, row.CreatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, rev)
	}
	return out, nil
}

func (r *repo) GetRevision(ctx context.Context, id uuid.UUID) (EmailRevision, bool, error) {
	row, err := r.q.GetEmailRevision(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return EmailRevision{}, false, nil
	}
	if err != nil {
		return EmailRevision{}, false, err
	}
	rev, err := toRevision(row.ID, row.EmailID, row.Name, row.Subject, row.Preheader, row.Blocks, row.Styles, row.AuthorAdminID, row.CreatedAt)
	return rev, err == nil, err
}

func toEmailGroup(g db.EmailGroup) EmailGroup {
	return EmailGroup{ID: g.ID, Slug: g.Slug, Name: g.Name, IsSystem: g.IsSystem, SortOrder: int(g.SortOrder)}
}

func slugifyGroup(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "group"
	}
	return out
}

func toRevision(id, emailID uuid.UUID, name, subject, preheader string, blocksJSON, stylesJSON []byte, author uuid.NullUUID, createdAt time.Time) (EmailRevision, error) {
	rev := EmailRevision{ID: id, EmailID: emailID, Name: name, Subject: subject, Preheader: preheader, CreatedAt: createdAt}
	if author.Valid {
		a := author.UUID
		rev.AuthorAdminID = &a
	}
	if len(blocksJSON) > 0 {
		if err := json.Unmarshal(blocksJSON, &rev.Blocks); err != nil {
			return EmailRevision{}, err
		}
	}
	if len(stylesJSON) > 0 {
		if err := json.Unmarshal(stylesJSON, &rev.Styles); err != nil {
			return EmailRevision{}, err
		}
	}
	return rev, nil
}

func nullUUIDp(p *uuid.UUID) uuid.NullUUID {
	if p == nil || *p == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *p, Valid: true}
}

func marshalLibrary(e LibraryEmail) (blocks, styles json.RawMessage, err error) {
	b, err := json.Marshal(e.Blocks)
	if err != nil {
		return nil, nil, err
	}
	st, err := json.Marshal(e.Styles)
	if err != nil {
		return nil, nil, err
	}
	return b, st, nil
}

func toLibraryEmail(row db.EmailLibrary) (LibraryEmail, error) {
	e := LibraryEmail{ID: row.ID, ChannelID: row.ChannelID, Name: row.Name, Subject: row.Subject, Preheader: row.Preheader, Slot: row.Slot, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.GroupID.Valid {
		g := row.GroupID.UUID
		e.GroupID = &g
	}
	if len(row.Blocks) > 0 {
		if err := json.Unmarshal(row.Blocks, &e.Blocks); err != nil {
			return LibraryEmail{}, err
		}
	}
	if len(row.Styles) > 0 {
		if err := json.Unmarshal(row.Styles, &e.Styles); err != nil {
			return LibraryEmail{}, err
		}
	}
	return e, nil
}
