package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CreateSignupForm(ctx context.Context, input model.SaveSignupFormInput) (*model.SignupForm, error) {
	cid, err := uuid.Parse(input.ChannelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "forms.manage", cid, nil); err != nil {
		return nil, err
	}
	f, err := r.FormSvc.Create(ctx, saveInputToForm(input, cid))
	if err != nil {
		return nil, err
	}
	return formToModel(f, 0), nil
}

func (r *mutationResolver) UpdateSignupForm(ctx context.Context, id string, input model.SaveSignupFormInput) (*model.SignupForm, error) {
	fid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	existing, err := r.FormSvc.Get(ctx, fid)
	if err != nil {
		return nil, errNotFound("form")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "forms.manage", existing.ChannelID, nil); err != nil {
		return nil, err
	}
	f, err := r.FormSvc.Update(ctx, fid, saveInputToForm(input, existing.ChannelID))
	if err != nil {
		return nil, err
	}
	count, _ := r.FormSvc.SubmissionCount(ctx, fid)
	return formToModel(f, int(count)), nil
}

func (r *mutationResolver) DeleteSignupForm(ctx context.Context, id string) (bool, error) {
	fid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	existing, err := r.FormSvc.Get(ctx, fid)
	if err != nil {
		return false, errNotFound("form")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "forms.manage", existing.ChannelID, nil); err != nil {
		return false, err
	}
	if err := r.FormSvc.Delete(ctx, fid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) SignupForms(ctx context.Context, channelID string) ([]*model.SignupForm, error) {
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "forms.view", cid, nil); err != nil {
		return nil, err
	}
	forms, err := r.FormSvc.List(ctx, cid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.SignupForm, 0, len(forms))
	for _, f := range forms {
		count, _ := r.FormSvc.SubmissionCount(ctx, f.ID)
		out = append(out, formToModel(f, int(count)))
	}
	return out, nil
}

func (r *queryResolver) SignupForm(ctx context.Context, id string) (*model.SignupForm, error) {
	fid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	f, err := r.FormSvc.Get(ctx, fid)
	if err != nil {
		return nil, errNotFound("form")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "forms.view", f.ChannelID, nil); err != nil {
		return nil, err
	}
	count, _ := r.FormSvc.SubmissionCount(ctx, fid)
	return formToModel(f, int(count)), nil
}

func (r *queryResolver) SignupFormSubmissions(ctx context.Context, formID string, limit *int32) ([]*model.SignupFormSubmission, error) {
	fid, err := uuid.Parse(formID)
	if err != nil {
		return nil, errInvalidID("formId", err)
	}
	f, err := r.FormSvc.Get(ctx, fid)
	if err != nil {
		return nil, errNotFound("form")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "forms.view", f.ChannelID, nil); err != nil {
		return nil, err
	}
	var lim int32 = 100
	if limit != nil && *limit > 0 {
		lim = *limit
	}
	rows, err := r.FormSvc.RecentSubmissions(ctx, fid, lim)
	if err != nil {
		return nil, err
	}
	out := make([]*model.SignupFormSubmission, 0, len(rows))
	for _, row := range rows {
		var cid *string
		if row.CustomerID.Valid {
			s := row.CustomerID.UUID.String()
			cid = &s
		}
		out = append(out, &model.SignupFormSubmission{Email: row.Email, CustomerID: cid, CreatedAt: row.CreatedAt})
	}
	return out, nil
}
