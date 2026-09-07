package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/campaign"
	"github.com/skeletg/skelet/apps/backend/internal/flow"
)

func (r *mutationResolver) CreateFlow(ctx context.Context, input model.SaveFlowInput) (*model.Flow, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "flows.manage"); err != nil {
		return nil, err
	}
	in, err := flowInput(input)
	if err != nil {
		return nil, err
	}
	f, err := r.FlowSvc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	return toFlowModel(f), nil
}

func (r *mutationResolver) UpdateFlow(ctx context.Context, id string, input model.SaveFlowInput) (*model.Flow, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "flows.manage"); err != nil {
		return nil, err
	}
	fid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	in, err := flowInput(input)
	if err != nil {
		return nil, err
	}
	f, err := r.FlowSvc.Update(ctx, fid, in)
	if err != nil {
		return nil, err
	}
	return toFlowModel(f), nil
}

func (r *mutationResolver) DeleteFlow(ctx context.Context, id string) (bool, error) {
	if err := r.requireSignals(); err != nil {
		return false, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "flows.manage"); err != nil {
		return false, err
	}
	fid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.FlowSvc.SoftDelete(ctx, fid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) RestoreFlow(ctx context.Context, id string) (bool, error) {
	if err := r.requireSignals(); err != nil {
		return false, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "flows.manage"); err != nil {
		return false, err
	}
	fid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.FlowSvc.Restore(ctx, fid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SetFlowStatus(ctx context.Context, id string, status string) (*model.Flow, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "flows.manage"); err != nil {
		return nil, err
	}
	fid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	f, err := r.FlowSvc.SetStatus(ctx, fid, status)
	if err != nil {
		return nil, err
	}
	return toFlowModel(f), nil
}

func (r *queryResolver) Flows(ctx context.Context, channelID string, includeDeleted *bool) ([]*model.Flow, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "flows.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	list, err := r.FlowSvc.List(ctx, cid, boolOrFalse(includeDeleted))
	if err != nil {
		return nil, err
	}
	out := make([]*model.Flow, len(list))
	for i, f := range list {
		out[i] = toFlowModel(f)
	}
	return out, nil
}

func (r *queryResolver) Flow(ctx context.Context, id string) (*model.Flow, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "flows.view"); err != nil {
		return nil, err
	}
	fid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	f, ok, err := r.FlowSvc.Get(ctx, fid)
	if err != nil || !ok {
		return nil, err
	}
	return toFlowModel(f), nil
}

func (r *queryResolver) FlowTriggerTypes(ctx context.Context) ([]string, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "flows.view"); err != nil {
		return nil, err
	}
	return flow.WiredTriggerTypes(), nil
}

func (r *queryResolver) FlowMergeTags(ctx context.Context) ([]string, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "flows.view"); err != nil {
		return nil, err
	}
	// Flows expose everything campaigns do, plus the cart-recovery link and the
	// browsed-product tags for abandoned-cart / browse-abandonment flows.
	return append(campaign.CampaignMergeTags(), "cart.recoveryUrl", "product.name", "product.image", "product.url"), nil
}

func (r *queryResolver) FlowConditionKinds(ctx context.Context) ([]string, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "flows.view"); err != nil {
		return nil, err
	}
	return r.FlowSvc.ConditionKinds(), nil
}
