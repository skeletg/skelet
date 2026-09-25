package admin

import (
	"context"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func (r *mutationResolver) SaveLibraryEmail(ctx context.Context, input model.SaveLibraryEmailInput) (*model.LibraryEmail, error) {
	authed, err := r.signalsChannel(ctx, "emails.manage", input.ChannelID)
	if err != nil {
		return nil, err
	}
	blocks, err := jsonText(input.Blocks)
	if err != nil {
		return nil, errInvalidInput("blocks", err)
	}
	styles, err := jsonText(input.Styles)
	if err != nil {
		return nil, errInvalidInput("styles", err)
	}
	in := &signalsv1.LibraryEmailInput{
		Id: optionalStr(input.ID), ChannelId: input.ChannelID, Name: input.Name, Subject: input.Subject,
		Preheader: optionalStr(input.Preheader), BlocksJson: blocks, StylesJson: styles,
	}
	if input.GroupID != nil && *input.GroupID != "" {
		in.GroupId = input.GroupID
	}
	res, err := r.SignalsAPI.Builder.SaveLibraryEmail(authed, &signalsv1.SaveLibraryEmailRequest{Email: in})
	if err != nil {
		return nil, signalsErr(err)
	}
	return libraryEmailModel(res.GetEmail()), nil
}

func (r *mutationResolver) DeleteLibraryEmail(ctx context.Context, id string) (bool, error) {
	authed, err := r.signalsScoped(ctx, "emails.manage")
	if err != nil {
		return false, err
	}
	if _, err := r.SignalsAPI.Builder.DeleteLibraryEmail(authed, &signalsv1.DeleteLibraryEmailRequest{Id: id}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) CreateEmailGroup(ctx context.Context, name string) (*model.EmailGroup, error) {
	authed, err := r.signalsScoped(ctx, "emails.manage")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Builder.CreateEmailGroup(authed, &signalsv1.CreateEmailGroupRequest{Name: name})
	if err != nil {
		return nil, signalsErr(err)
	}
	return emailGroupModel(res.GetGroup()), nil
}

func (r *mutationResolver) RenameEmailGroup(ctx context.Context, id string, name string) (*model.EmailGroup, error) {
	authed, err := r.signalsScoped(ctx, "emails.manage")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Builder.RenameEmailGroup(authed, &signalsv1.RenameEmailGroupRequest{Id: id, Name: name})
	if err != nil {
		return nil, signalsErr(err)
	}
	return emailGroupModel(res.GetGroup()), nil
}

func (r *mutationResolver) DeleteEmailGroup(ctx context.Context, id string) (bool, error) {
	authed, err := r.signalsScoped(ctx, "emails.manage")
	if err != nil {
		return false, err
	}
	if _, err := r.SignalsAPI.Builder.DeleteEmailGroup(authed, &signalsv1.DeleteEmailGroupRequest{Id: id}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) AssignEmailSlot(ctx context.Context, emailID string, slot string) (bool, error) {
	authed, err := r.signalsScoped(ctx, "emails.manage")
	if err != nil {
		return false, err
	}
	if _, err := r.SignalsAPI.Builder.AssignEmailSlot(authed, &signalsv1.AssignEmailSlotRequest{EmailId: emailID, Slot: slot}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) RestoreEmailRevision(ctx context.Context, revisionID string) (*model.LibraryEmail, error) {
	authed, err := r.signalsScoped(ctx, "emails.manage")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Builder.RestoreEmailRevision(authed, &signalsv1.RestoreEmailRevisionRequest{RevisionId: revisionID})
	if err != nil {
		return nil, signalsErr(err)
	}
	return libraryEmailModel(res.GetEmail()), nil
}

func (r *mutationResolver) SeedPulseStarterContent(ctx context.Context, channelID string) (*model.PulseStarterContent, error) {
	if _, err := r.signalsChannel(ctx, "flows.manage", channelID); err != nil {
		return nil, err
	}
	authed, err := r.signalsChannel(ctx, "emails.manage", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Flows.SeedStarterContent(authed, &signalsv1.SeedStarterContentRequest{ChannelId: channelID})
	if err != nil {
		return nil, signalsErr(err)
	}
	return &model.PulseStarterContent{Blocks: res.GetBlocks(), Emails: res.GetEmails(), Flows: res.GetFlows()}, nil
}

// The two copy assists are core's - the AI service is configured here, and they read no builder state
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
	return r.AISvc.RewriteEmailText(ctx, text, optionalStr(instruction))
}

func (r *queryResolver) LibraryEmails(ctx context.Context, channelID string, groupSlug *string) ([]*model.LibraryEmail, error) {
	authed, err := r.signalsChannel(ctx, "emails.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Builder.ListLibraryEmails(authed, &signalsv1.ListLibraryEmailsRequest{ChannelId: channelID, GroupSlug: optionalStr(groupSlug)})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.LibraryEmail, 0, len(res.GetEmails()))
	for _, e := range res.GetEmails() {
		out = append(out, libraryEmailModel(e))
	}
	return out, nil
}

func (r *queryResolver) LibraryEmail(ctx context.Context, id string) (*model.LibraryEmail, error) {
	authed, err := r.signalsScoped(ctx, "emails.view")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Builder.GetLibraryEmail(authed, &signalsv1.GetLibraryEmailRequest{Id: id})
	if err != nil {
		if signalsNotFound(err) {
			return nil, nil
		}
		return nil, signalsErr(err)
	}
	return libraryEmailModel(res.GetEmail()), nil
}

func (r *queryResolver) EmailGroups(ctx context.Context) ([]*model.EmailGroup, error) {
	authed, err := r.signalsScoped(ctx, "emails.view")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Builder.ListEmailGroups(authed, &signalsv1.ListEmailGroupsRequest{})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.EmailGroup, 0, len(res.GetGroups()))
	for _, g := range res.GetGroups() {
		out = append(out, emailGroupModel(g))
	}
	return out, nil
}

func (r *queryResolver) TransactionalSlots(ctx context.Context, channelID string) ([]*model.TransactionalSlot, error) {
	authed, err := r.signalsChannel(ctx, "emails.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Builder.ListTransactionalSlots(authed, &signalsv1.ListTransactionalSlotsRequest{ChannelId: channelID})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.TransactionalSlot, 0, len(res.GetSlots()))
	for _, s := range res.GetSlots() {
		ts := &model.TransactionalSlot{Slot: s.GetSlot(), Label: s.GetLabel(), EmailID: s.EmailId}
		if s.EmailId != nil {
			name := s.GetEmailName()
			ts.EmailName = &name
		}
		out = append(out, ts)
	}
	return out, nil
}

func (r *queryResolver) EmailPreflight(ctx context.Context, id string) ([]*model.EmailWarning, error) {
	authed, err := r.signalsScoped(ctx, "emails.view")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Builder.PreflightEmail(authed, &signalsv1.PreflightEmailRequest{Id: id})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.EmailWarning, 0, len(res.GetWarnings()))
	for _, w := range res.GetWarnings() {
		out = append(out, &model.EmailWarning{Severity: w.GetSeverity(), Message: w.GetMessage()})
	}
	return out, nil
}

func (r *queryResolver) EmailRevisions(ctx context.Context, id string) ([]*model.EmailRevision, error) {
	authed, err := r.signalsScoped(ctx, "emails.view")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Builder.ListEmailRevisions(authed, &signalsv1.ListEmailRevisionsRequest{Id: id, Limit: 40})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.EmailRevision, 0, len(res.GetRevisions()))
	for _, rev := range res.GetRevisions() {
		out = append(out, &model.EmailRevision{
			ID:        rev.GetId(),
			Name:      rev.GetName(),
			Subject:   rev.GetSubject(),
			Preheader: rev.GetPreheader(),
			Blocks:    jsonValue(rev.GetBlocksJson(), "[]"),
			Styles:    jsonValue(rev.GetStylesJson(), "{}"),
			CreatedAt: protoTime(rev.GetCreatedAt()),
		})
	}
	return out, nil
}

func (r *queryResolver) RenderLibraryPreview(ctx context.Context, channelID string, customerID *string, subject string, preheader *string, blocks interface{}, styles interface{}) (*model.EmailPreview, error) {
	authed, err := r.signalsChannel(ctx, "emails.view", channelID)
	if err != nil {
		return nil, err
	}
	blocksJSON, err := jsonText(blocks)
	if err != nil {
		return nil, errInvalidInput("blocks", err)
	}
	stylesJSON, err := jsonText(styles)
	if err != nil {
		return nil, errInvalidInput("styles", err)
	}
	res, err := r.SignalsAPI.Builder.RenderLibraryPreview(authed, &signalsv1.RenderLibraryPreviewRequest{
		ChannelId: channelID, CustomerId: customerID, Subject: subject, Preheader: optionalStr(preheader),
		BlocksJson: blocksJSON, StylesJson: stylesJSON,
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	return &model.EmailPreview{Subject: res.GetSubject(), HTML: res.GetHtml()}, nil
}
