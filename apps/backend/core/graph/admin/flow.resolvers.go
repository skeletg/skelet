package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func (r *mutationResolver) CreateFlow(ctx context.Context, input model.SaveFlowInput) (*model.Flow, error) {
	authed, err := r.signalsChannel(ctx, "flows.manage", input.ChannelID)
	if err != nil {
		return nil, err
	}
	in, err := flowInput(input)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Flows.CreateFlow(authed, &signalsv1.CreateFlowRequest{Flow: in})
	if err != nil {
		return nil, signalsErr(err)
	}
	return flowModel(res.GetFlow()), nil
}

func (r *mutationResolver) UpdateFlow(ctx context.Context, id string, input model.SaveFlowInput) (*model.Flow, error) {
	authed, err := r.signalsScoped(ctx, "flows.manage")
	if err != nil {
		return nil, err
	}
	in, err := flowInput(input)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Flows.UpdateFlow(authed, &signalsv1.UpdateFlowRequest{Id: id, Flow: in})
	if err != nil {
		return nil, signalsErr(err)
	}
	return flowModel(res.GetFlow()), nil
}

func (r *mutationResolver) DeleteFlow(ctx context.Context, id string) (bool, error) {
	authed, err := r.signalsScoped(ctx, "flows.manage")
	if err != nil {
		return false, err
	}
	if _, err := r.SignalsAPI.Flows.DeleteFlow(authed, &signalsv1.DeleteFlowRequest{Id: id}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) RestoreFlow(ctx context.Context, id string) (bool, error) {
	authed, err := r.signalsScoped(ctx, "flows.manage")
	if err != nil {
		return false, err
	}
	if _, err := r.SignalsAPI.Flows.RestoreFlow(authed, &signalsv1.RestoreFlowRequest{Id: id}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) SetFlowStatus(ctx context.Context, id string, status string) (*model.Flow, error) {
	authed, err := r.signalsScoped(ctx, "flows.manage")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Flows.SetFlowStatus(authed, &signalsv1.SetFlowStatusRequest{Id: id, Status: status})
	if err != nil {
		return nil, signalsErr(err)
	}
	return flowModel(res.GetFlow()), nil
}

func (r *queryResolver) Flows(ctx context.Context, channelID string, includeDeleted *bool) ([]*model.Flow, error) {
	authed, err := r.signalsChannel(ctx, "flows.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Flows.ListFlows(authed, &signalsv1.ListFlowsRequest{ChannelId: channelID, IncludeDeleted: boolOrFalse(includeDeleted)})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.Flow, 0, len(res.GetFlows()))
	for _, f := range res.GetFlows() {
		out = append(out, flowModel(f))
	}
	return out, nil
}

func (r *queryResolver) Flow(ctx context.Context, id string) (*model.Flow, error) {
	authed, err := r.signalsScoped(ctx, "flows.view")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Flows.GetFlowByID(authed, &signalsv1.GetFlowByIDRequest{Id: id})
	if err != nil {
		if signalsNotFound(err) {
			return nil, nil
		}
		return nil, signalsErr(err)
	}
	return flowModel(res.GetFlow()), nil
}

// The triggers are the contract's own list - core raises every one of them, so it needs no round trip
func (r *queryResolver) FlowTriggerTypes(ctx context.Context) ([]string, error) {
	if _, err := r.signalsScoped(ctx, "flows.view"); err != nil {
		return nil, err
	}
	return signalsapi.FlowTriggerTypes(), nil
}

func (r *queryResolver) FlowMergeTags(ctx context.Context) ([]string, error) {
	if _, err := r.signalsScoped(ctx, "flows.view"); err != nil {
		return nil, err
	}
	return signalsapi.FlowMergeTags(), nil
}

// What the suite can answer right now - a condition whose data is not there is not offered
func (r *queryResolver) FlowConditionKinds(ctx context.Context) ([]string, error) {
	authed, err := r.signalsScoped(ctx, "flows.view")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Flows.ListFlowConditionKinds(authed, &signalsv1.ListFlowConditionKindsRequest{})
	if err != nil {
		return nil, signalsErr(err)
	}
	return res.GetKinds(), nil
}
