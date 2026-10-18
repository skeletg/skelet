package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) RestoreCMSPageRevision(ctx context.Context, id string) (*model.CMSPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return nil, err
	}
	uid, err := parseID(id, "revision")
	if err != nil {
		return nil, err
	}
	rev, err := r.CMSSvc.GetRevision(ctx, uid)
	if err != nil {
		return nil, err
	}
	if err := r.requireOnPageChannels(ctx, "cms.manage", rev.PageID, rev.Locale); err != nil {
		return nil, err
	}
	p, err := r.CMSSvc.RestoreRevision(ctx, uid)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) ScheduleCMSPage(ctx context.Context, id string, scheduledAt *time.Time) (*model.CMSPage, error) {
	uid, err := parseID(id, "page")
	if err != nil {
		return nil, err
	}
	if err := r.requireOnPageChannels(ctx, "cms.manage", uid, ""); err != nil {
		return nil, err
	}
	p, err := r.CMSSvc.SchedulePage(ctx, uid, scheduledAt)
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(p), nil
}

func (r *mutationResolver) RebuildCMSPage(ctx context.Context, id string) (bool, error) {
	if r.CacheClient == nil {
		return false, errNotConfigured("cache revalidation")
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.requireOnPageChannels(ctx, "cms.manage", uid, ""); err != nil {
		return false, err
	}
	page, err := r.CMSSvc.GetPage(ctx, uid, "")
	if err != nil {
		return false, err
	}
	var actor *uuid.UUID
	if claims, ok := auth.AdminFromContext(ctx); ok {
		aid := claims.AdminID
		actor = &aid
	}
	// Invalidate this page's cache tag, then warm its URL so it re-renders +
	// re-caches now (default channel/locale; other channels rebuild on visit).
	r.CacheClient.Rebuild(ctx, "cms", []string{"cms:page:" + uid.String()}, nil, []string{page.URLPath}, "rebuild (on demand)", actor)
	return true, nil
}

func (r *queryResolver) CmsPageRevisions(ctx context.Context, pageID string, locale string) ([]*model.CMSPageRevision, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	uid, err := parseID(pageID, "page")
	if err != nil {
		return nil, err
	}
	list, err := r.CMSSvc.ListPageRevisions(ctx, uid, locale)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CMSPageRevision, len(list))
	for i, rev := range list {
		out[i] = toCMSPageRevisionModel(rev)
	}
	return out, nil
}
