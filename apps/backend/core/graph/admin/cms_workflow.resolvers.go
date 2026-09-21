package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) SubmitCMSPageForReview(ctx context.Context, id string) (*model.CMSPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	p, err := r.CMSSvc.SubmitPageForReview(ctx, uid)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) ApproveCMSPage(ctx context.Context, id string) (*model.CMSPage, error) {
	// Approval is gated separately from cms.manage so an editor can't approve
	// their own work.
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.approve"); err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	p, err := r.CMSSvc.ApprovePage(ctx, uid)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) RejectCMSPage(ctx context.Context, id string, note *string) (*model.CMSPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	p, err := r.CMSSvc.RejectPage(ctx, uid, ptr.Deref(note))
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) ReopenCMSPage(ctx context.Context, id string) (*model.CMSPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	p, err := r.CMSSvc.ReopenPage(ctx, uid)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) ClaimCMSPage(ctx context.Context, id string) (*model.CMSPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	claims, ok := auth.AdminFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("an authenticated admin is required")
	}
	adminID := claims.AdminID
	p, err := r.CMSSvc.AssignPage(ctx, uid, &adminID)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) UnassignCMSPage(ctx context.Context, id string) (*model.CMSPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	p, err := r.CMSSvc.AssignPage(ctx, uid, nil)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) AddCMSPageComment(ctx context.Context, input model.CMSPageCommentInput) (*model.CMSPageComment, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	pageID, err := uuid.Parse(input.PageID)
	if err != nil {
		return nil, errInvalidID("page id", err)
	}
	c, err := r.CMSSvc.AddComment(ctx, cms.CommentInput{
		PageID:  pageID,
		Locale:  input.Locale,
		BlockID: input.BlockID,
		Body:    input.Body,
	})
	if err != nil {
		return nil, err
	}
	return toCMSPageCommentModel(c), nil
}

func (r *mutationResolver) ResolveCMSPageComment(ctx context.Context, id string, resolved bool) (*model.CMSPageComment, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	c, err := r.CMSSvc.ResolveComment(ctx, cid, resolved)
	if err != nil {
		return nil, err
	}
	return toCMSPageCommentModel(c), nil
}

func (r *mutationResolver) DeleteCMSPageComment(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return false, err
	}
	cid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.CMSSvc.DeleteComment(ctx, cid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) CreateCMSTag(ctx context.Context, name string) (*model.CMSTag, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return nil, err
	}
	t, err := r.CMSSvc.CreateTag(ctx, name)
	if err != nil {
		return nil, err
	}
	return r.tagModel(ctx, t), nil
}

func (r *mutationResolver) RenameCMSTag(ctx context.Context, slug string, name string) (*model.CMSTag, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return nil, err
	}
	t, err := r.CMSSvc.RenameTag(ctx, slug, name)
	if err != nil {
		return nil, err
	}
	return r.tagModel(ctx, t), nil
}

func (r *mutationResolver) DeleteCMSTag(ctx context.Context, slug string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return false, err
	}
	if err := r.CMSSvc.DeleteTag(ctx, slug); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) CmsPageComments(ctx context.Context, pageID string) ([]*model.CMSPageComment, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	pid, err := uuid.Parse(pageID)
	if err != nil {
		return nil, errInvalidID("page id", err)
	}
	comments, err := r.CMSSvc.ListComments(ctx, pid)
	if err != nil {
		return nil, err
	}
	return toCMSPageCommentModels(comments), nil
}

func (r *queryResolver) CmsTags(ctx context.Context) ([]*model.CMSTag, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	tags, err := r.CMSSvc.ListTags(ctx)
	if err != nil {
		return nil, err
	}
	counts, _ := r.CMSSvc.TagUsageCounts(ctx)
	out := make([]*model.CMSTag, len(tags))
	for i, t := range tags {
		out[i] = toCMSTagModel(t, counts[t.Slug])
	}
	return out, nil
}
