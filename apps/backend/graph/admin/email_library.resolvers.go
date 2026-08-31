package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/ai"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/emailtemplate"
)

func (r *mutationResolver) SaveLibraryEmail(ctx context.Context, input model.SaveLibraryEmailInput) (*model.LibraryEmail, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.manage"); err != nil {
		return nil, err
	}
	e, err := libraryEmailInput(input)
	if err != nil {
		return nil, err
	}
	var saved emailtemplate.LibraryEmail
	if e.ID != uuid.Nil {
		saved, err = r.EmailTmplSvc.UpdateLibraryEmail(ctx, e)
	} else {
		saved, err = r.EmailTmplSvc.CreateLibraryEmail(ctx, e)
	}
	if err != nil {
		return nil, err
	}
	// Snapshot the saved state into version history (best-effort).
	var adminID *uuid.UUID
	if claims, ok := auth.AdminFromContext(ctx); ok {
		id := claims.AdminID
		adminID = &id
	}
	_ = r.EmailTmplSvc.Snapshot(ctx, saved, adminID)
	return toLibraryEmailModel(saved), nil
}

func (r *mutationResolver) DeleteLibraryEmail(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.manage"); err != nil {
		return false, err
	}
	eid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.EmailTmplSvc.DeleteLibraryEmail(ctx, eid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) CreateEmailGroup(ctx context.Context, name string) (*model.EmailGroup, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.manage"); err != nil {
		return nil, err
	}
	g, err := r.EmailTmplSvc.CreateEmailGroup(ctx, name)
	if err != nil {
		return nil, err
	}
	return emailGroupModel(g, 0), nil
}

func (r *mutationResolver) RenameEmailGroup(ctx context.Context, id string, name string) (*model.EmailGroup, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.manage"); err != nil {
		return nil, err
	}
	gid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	g, err := r.EmailTmplSvc.RenameEmailGroup(ctx, gid, name)
	if err != nil {
		return nil, err
	}
	return emailGroupModel(g, 0), nil
}

func (r *mutationResolver) DeleteEmailGroup(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.manage"); err != nil {
		return false, err
	}
	gid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.EmailTmplSvc.DeleteEmailGroup(ctx, gid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) AssignEmailSlot(ctx context.Context, emailID string, slot string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.manage"); err != nil {
		return false, err
	}
	eid, err := uuid.Parse(emailID)
	if err != nil {
		return false, errInvalidID("emailId", err)
	}
	if err := r.EmailTmplSvc.AssignSlot(ctx, eid, slot); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) RestoreEmailRevision(ctx context.Context, revisionID string) (*model.LibraryEmail, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.manage"); err != nil {
		return nil, err
	}
	rid, err := uuid.Parse(revisionID)
	if err != nil {
		return nil, errInvalidID("revisionId", err)
	}
	var adminID *uuid.UUID
	if claims, ok := auth.AdminFromContext(ctx); ok {
		id := claims.AdminID
		adminID = &id
	}
	e, err := r.EmailTmplSvc.RestoreRevision(ctx, rid, adminID)
	if err != nil {
		return nil, err
	}
	return toLibraryEmailModel(e), nil
}

func (r *mutationResolver) SuggestEmailSubjects(ctx context.Context, context string, count *int32) ([]string, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.manage"); err != nil {
		return nil, err
	}
	if r.AISvc == nil || !r.AISvc.Enabled() {
		return nil, ai.ErrDisabled
	}
	if !r.AISvc.Available(ctx) {
		return nil, fmt.Errorf("AI copy assist is not configured")
	}
	n := 5
	if count != nil && *count > 0 {
		n = int(*count)
	}
	return r.AISvc.SuggestEmailSubjects(ctx, context, n)
}

func (r *mutationResolver) RewriteEmailText(ctx context.Context, text string, instruction *string) (string, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.manage"); err != nil {
		return "", err
	}
	if r.AISvc == nil || !r.AISvc.Enabled() {
		return "", ai.ErrDisabled
	}
	if !r.AISvc.Available(ctx) {
		return "", fmt.Errorf("AI copy assist is not configured")
	}
	instr := ""
	if instruction != nil {
		instr = *instruction
	}
	return r.AISvc.RewriteEmailText(ctx, text, instr)
}

func (r *queryResolver) LibraryEmails(ctx context.Context, channelID string, groupSlug *string) ([]*model.LibraryEmail, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	gs := ""
	if groupSlug != nil {
		gs = *groupSlug
	}
	list, err := r.EmailTmplSvc.ListLibraryEmails(ctx, cid, gs)
	if err != nil {
		return nil, err
	}
	out := make([]*model.LibraryEmail, len(list))
	for i, e := range list {
		out[i] = toLibraryEmailModel(e)
	}
	return out, nil
}

func (r *queryResolver) LibraryEmail(ctx context.Context, id string) (*model.LibraryEmail, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.view"); err != nil {
		return nil, err
	}
	eid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	e, ok, err := r.EmailTmplSvc.GetLibraryEmail(ctx, eid)
	if err != nil || !ok {
		return nil, err
	}
	return toLibraryEmailModel(e), nil
}

func (r *queryResolver) EmailGroups(ctx context.Context) ([]*model.EmailGroup, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.view"); err != nil {
		return nil, err
	}
	groups, err := r.EmailTmplSvc.ListEmailGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.EmailGroup, 0, len(groups))
	for _, g := range groups {
		count, _ := r.EmailTmplSvc.CountEmailsInGroup(ctx, g.ID)
		out = append(out, emailGroupModel(g, int(count)))
	}
	return out, nil
}

func (r *queryResolver) TransactionalSlots(ctx context.Context, channelID string) ([]*model.TransactionalSlot, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	out := make([]*model.TransactionalSlot, 0, len(emailtemplate.TransactionalSlots))
	for _, slot := range emailtemplate.TransactionalSlots {
		ts := &model.TransactionalSlot{Slot: slot, Label: slotLabel(slot)}
		if e, ok, _ := r.EmailTmplSvc.SlotEmail(ctx, cid, slot); ok {
			id := e.ID.String()
			ts.EmailID = &id
			name := e.Name
			ts.EmailName = &name
		}
		out = append(out, ts)
	}
	return out, nil
}

func (r *queryResolver) EmailPreflight(ctx context.Context, id string) ([]*model.EmailWarning, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.view"); err != nil {
		return nil, err
	}
	eid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	e, ok, err := r.EmailTmplSvc.GetLibraryEmail(ctx, eid)
	if err != nil || !ok {
		return nil, err
	}
	warnings := emailtemplate.Preflight(e.Subject, e.Preheader, e.Blocks)
	out := make([]*model.EmailWarning, 0, len(warnings))
	for _, w := range warnings {
		out = append(out, &model.EmailWarning{Severity: w.Severity, Message: w.Message})
	}
	return out, nil
}

func (r *queryResolver) EmailRevisions(ctx context.Context, id string) ([]*model.EmailRevision, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.view"); err != nil {
		return nil, err
	}
	eid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	revs, err := r.EmailTmplSvc.ListRevisions(ctx, eid, 40)
	if err != nil {
		return nil, err
	}
	out := make([]*model.EmailRevision, 0, len(revs))
	for _, rev := range revs {
		out = append(out, emailRevisionModel(rev))
	}
	return out, nil
}

func (r *queryResolver) RenderLibraryPreview(ctx context.Context, channelID string, customerID *string, subject string, preheader *string, blocks interface{}, styles interface{}) (*model.EmailPreview, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	bs, err := blocksFromJSON(blocks)
	if err != nil {
		return nil, err
	}
	st, err := stylesFromJSON(styles)
	if err != nil {
		return nil, err
	}
	ph := ""
	if preheader != nil {
		ph = *preheader
	}
	vars, err := r.previewVars(ctx, cid, customerID)
	if err != nil {
		return nil, err
	}
	subjectOut, html, _, err := r.EmailTmplSvc.RenderContent(ctx, cid, subject, ph, bs, st, vars)
	if err != nil {
		return nil, err
	}
	return &model.EmailPreview{Subject: subjectOut, HTML: html}, nil
}
