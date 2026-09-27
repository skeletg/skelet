package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CreateWebForm(ctx context.Context, channelID string, input model.SaveWebFormInput) (*model.WebForm, error) {
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "webforms.manage", cid, nil); err != nil {
		return nil, err
	}
	in, err := saveWebFormInput(input)
	if err != nil {
		return nil, err
	}
	f, err := r.WebFormSvc.Create(ctx, cid, in)
	if err != nil {
		return nil, err
	}
	return webFormToModel(f, 0), nil
}

func (r *mutationResolver) UpdateWebForm(ctx context.Context, id string, input model.SaveWebFormInput) (*model.WebForm, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webforms.manage"); err != nil {
		return nil, err
	}
	fid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	existing, err := r.WebFormSvc.Get(ctx, fid)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "webforms.manage", existing.ChannelID, nil); err != nil {
		return nil, err
	}
	in, err := saveWebFormInput(input)
	if err != nil {
		return nil, err
	}
	f, err := r.WebFormSvc.Update(ctx, fid, in)
	if err != nil {
		return nil, err
	}
	count, _ := r.WebFormSvc.CountSubmissions(ctx, fid)
	return webFormToModel(f, count), nil
}

func (r *mutationResolver) DeleteWebForm(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webforms.manage"); err != nil {
		return false, err
	}
	fid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	existing, err := r.WebFormSvc.Get(ctx, fid)
	if err != nil {
		return false, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "webforms.manage", existing.ChannelID, nil); err != nil {
		return false, err
	}
	if err := r.WebFormSvc.SoftDelete(ctx, fid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) RestoreWebForm(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webforms.manage"); err != nil {
		return false, err
	}
	fid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	// Permission is store-scoped, so the (soft-deleted) form has to be
	// loaded before the check - Get does not filter deleted rows.
	existing, err := r.WebFormSvc.Get(ctx, fid)
	if err != nil {
		return false, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "webforms.manage", existing.ChannelID, nil); err != nil {
		return false, err
	}
	if err := r.WebFormSvc.Restore(ctx, fid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) WebForms(ctx context.Context, channelID string, includeDeleted *bool) ([]*model.WebForm, error) {
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "webforms.view", cid, nil); err != nil {
		return nil, err
	}
	forms, err := r.WebFormSvc.List(ctx, cid, boolOrFalse(includeDeleted))
	if err != nil {
		return nil, err
	}
	out := make([]*model.WebForm, 0, len(forms))
	for _, f := range forms {
		count, _ := r.WebFormSvc.CountSubmissions(ctx, f.ID)
		out = append(out, webFormToModel(f, count))
	}
	return out, nil
}

func (r *queryResolver) WebForm(ctx context.Context, id string) (*model.WebForm, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webforms.view"); err != nil {
		return nil, err
	}
	fid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	f, err := r.WebFormSvc.Get(ctx, fid)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "webforms.view", f.ChannelID, nil); err != nil {
		return nil, err
	}
	count, _ := r.WebFormSvc.CountSubmissions(ctx, fid)
	return webFormToModel(f, count), nil
}

func (r *queryResolver) WebFormSubmissions(ctx context.Context, formID string, limit *int32, offset *int32) ([]*model.WebFormSubmission, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webforms.submissions_view"); err != nil {
		return nil, err
	}
	fid, err := uuid.Parse(formID)
	if err != nil {
		return nil, errInvalidID("formId", err)
	}
	f, err := r.WebFormSvc.Get(ctx, fid)
	if err != nil {
		return nil, err
	}
	// Submissions can hold PII - gate on the dedicated permission.
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "webforms.submissions_view", f.ChannelID, nil); err != nil {
		return nil, err
	}
	var lim, off int32
	if limit != nil {
		lim = *limit
	}
	if offset != nil {
		off = *offset
	}
	rows, err := r.WebFormSvc.ListSubmissions(ctx, fid, lim, off)
	if err != nil {
		return nil, err
	}
	out := make([]*model.WebFormSubmission, 0, len(rows))
	for _, row := range rows {
		out = append(out, webFormSubmissionToModel(row))
	}
	return out, nil
}

func (r *queryResolver) WebFormSubmissionCount(ctx context.Context, formID string) (int32, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webforms.view"); err != nil {
		return 0, err
	}
	fid, err := uuid.Parse(formID)
	if err != nil {
		return 0, errInvalidID("formId", err)
	}
	f, err := r.WebFormSvc.Get(ctx, fid)
	if err != nil {
		return 0, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "webforms.view", f.ChannelID, nil); err != nil {
		return 0, err
	}
	count, err := r.WebFormSvc.CountSubmissions(ctx, fid)
	if err != nil {
		return 0, err
	}
	return int32(count), nil
}
