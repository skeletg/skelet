package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/ptr"
)

func (r *mutationResolver) CreateCMSSnippet(ctx context.Context, input model.CreateCMSSnippetInput) (*model.CMSSnippet, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return nil, err
	}
	sn, err := r.CMSSvc.CreateSnippet(ctx, input.Slug, input.Name, rawJSON(input.Blocks))
	if err != nil {
		return nil, err
	}
	return toCMSSnippetModel(sn), nil
}

func (r *mutationResolver) UpdateCMSSnippet(ctx context.Context, input model.UpdateCMSSnippetInput) (*model.CMSSnippet, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return nil, err
	}
	id, err := parseID(input.ID, "snippet")
	if err != nil {
		return nil, err
	}
	sn, err := r.CMSSvc.UpdateSnippet(ctx, id, ptr.Deref(input.Name), rawJSON(input.Blocks))
	if err != nil {
		return nil, err
	}
	return toCMSSnippetModel(sn), nil
}

func (r *mutationResolver) DeleteCMSSnippet(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return false, err
	}
	uid, err := parseID(id, "snippet")
	if err != nil {
		return false, err
	}
	if err := r.CMSSvc.DeleteSnippet(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) CreateCMSBlockType(ctx context.Context, input model.CreateCMSBlockTypeInput) (*model.CMSBlockType, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.types_manage"); err != nil {
		return nil, err
	}
	bt, err := r.CMSSvc.CreateBlockType(ctx, input.Key, input.Name, ptr.Deref(input.Icon), rawJSON(input.Fields), ptr.Deref(input.HTMLTemplate))
	if err != nil {
		return nil, err
	}
	return toCMSBlockTypeModel(bt), nil
}

func (r *mutationResolver) UpdateCMSBlockType(ctx context.Context, input model.UpdateCMSBlockTypeInput) (*model.CMSBlockType, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.types_manage"); err != nil {
		return nil, err
	}
	id, err := parseID(input.ID, "block type")
	if err != nil {
		return nil, err
	}
	bt, err := r.CMSSvc.UpdateBlockType(ctx, id, ptr.Deref(input.Name), ptr.Deref(input.Icon), rawJSON(input.Fields), ptr.Deref(input.HTMLTemplate))
	if err != nil {
		return nil, err
	}
	return toCMSBlockTypeModel(bt), nil
}

func (r *mutationResolver) DeleteCMSBlockType(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.types_manage"); err != nil {
		return false, err
	}
	uid, err := parseID(id, "block type")
	if err != nil {
		return false, err
	}
	if err := r.CMSSvc.DeleteBlockType(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) CmsSnippets(ctx context.Context) ([]*model.CMSSnippet, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	list, err := r.CMSSvc.ListSnippets(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CMSSnippet, len(list))
	for i, s := range list {
		out[i] = toCMSSnippetModel(s)
	}
	return out, nil
}

func (r *queryResolver) CmsSnippet(ctx context.Context, id string) (*model.CMSSnippet, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	uid, err := parseID(id, "snippet")
	if err != nil {
		return nil, err
	}
	s, err := r.CMSSvc.GetSnippet(ctx, uid)
	if err != nil {
		return nil, nil
	}
	return toCMSSnippetModel(s), nil
}

func (r *queryResolver) CmsBlockTypes(ctx context.Context) ([]*model.CMSBlockType, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	list, err := r.CMSSvc.ListBlockTypes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CMSBlockType, len(list))
	for i, b := range list {
		out[i] = toCMSBlockTypeModel(b)
	}
	return out, nil
}

func (r *queryResolver) CmsBlockType(ctx context.Context, id string) (*model.CMSBlockType, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	uid, err := parseID(id, "block type")
	if err != nil {
		return nil, err
	}
	b, err := r.CMSSvc.GetBlockType(ctx, uid)
	if err != nil {
		return nil, nil
	}
	return toCMSBlockTypeModel(b), nil
}
