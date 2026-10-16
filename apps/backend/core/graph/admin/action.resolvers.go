package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/adminaction"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) RunAdminAction(ctx context.Context, slug string, args interface{}) (*model.AdminActionResult, error) {
	start := time.Now()
	def, ok := adminaction.Find(slug)
	if !ok {
		err := fmt.Errorf("unknown admin action: %s", slug)
		r.recordAdminAction(ctx, slug, args, nil, err, time.Since(start))
		return nil, err
	}
	if err := r.requireActionPermission(ctx, def); err != nil {
		r.recordAdminAction(ctx, slug, args, nil, err, time.Since(start))
		return nil, err
	}
	res, err := r.runAdminAction(ctx, slug, args)
	r.recordAdminAction(ctx, slug, args, res, err, time.Since(start))
	return res, err
}

func (r *queryResolver) AdminActions(ctx context.Context) ([]*model.AdminAction, error) {
	if _, err := auth.RequireActor(ctx); err != nil {
		return nil, err
	}
	// The same gate running one applies, so the list never offers an action the caller would be refused
	var defs []adminaction.Definition
	for _, d := range adminaction.AllDefinitions() {
		if auth.RequirePermission(ctx, r.AdminSvc, d.Permission) == nil {
			defs = append(defs, d)
		}
	}

	out := make([]*model.AdminAction, len(defs))
	for i, d := range defs {
		out[i] = toAdminActionModel(d)
	}
	return out, nil
}
