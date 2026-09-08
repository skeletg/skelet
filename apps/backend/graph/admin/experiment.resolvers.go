package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
)

func (r *mutationResolver) CreateExperiment(ctx context.Context, input model.SaveExperimentInput) (*model.Experiment, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "experiments.manage"); err != nil {
		return nil, err
	}
	in, err := experimentInput(input)
	if err != nil {
		return nil, err
	}
	e, err := r.ExperimentSvc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	return toExperimentModel(e), nil
}

func (r *mutationResolver) UpdateExperiment(ctx context.Context, id string, input model.SaveExperimentInput) (*model.Experiment, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "experiments.manage"); err != nil {
		return nil, err
	}
	eid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	in, err := experimentInput(input)
	if err != nil {
		return nil, err
	}
	e, err := r.ExperimentSvc.Update(ctx, eid, in)
	if err != nil {
		return nil, err
	}
	return toExperimentModel(e), nil
}

func (r *mutationResolver) DeleteExperiment(ctx context.Context, id string) (bool, error) {
	if err := r.requireSignals(); err != nil {
		return false, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "experiments.manage"); err != nil {
		return false, err
	}
	eid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	return true, r.ExperimentSvc.Delete(ctx, eid)
}

func (r *queryResolver) Experiments(ctx context.Context, channelID string) ([]*model.Experiment, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "experiments.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	exps, err := r.ExperimentSvc.List(ctx, cid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Experiment, 0, len(exps))
	for _, e := range exps {
		out = append(out, toExperimentModel(e))
	}
	return out, nil
}

func (r *queryResolver) Experiment(ctx context.Context, id string) (*model.Experiment, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "experiments.view"); err != nil {
		return nil, err
	}
	eid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	e, ok, err := r.ExperimentSvc.Get(ctx, eid)
	if err != nil || !ok {
		return nil, err
	}
	return toExperimentModel(e), nil
}

func (r *queryResolver) ExperimentResults(ctx context.Context, id string, sinceDays *int32) (*model.ExperimentResults, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "experiments.view"); err != nil {
		return nil, err
	}
	eid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	days := int32(30)
	if sinceDays != nil && *sinceDays > 0 {
		days = *sinceDays
	}
	_, analysis, err := r.ExperimentSvc.Results(ctx, eid, time.Now().AddDate(0, 0, -int(days)))
	if err != nil {
		return nil, err
	}
	return toExperimentResults(analysis), nil
}

func (r *queryResolver) ExperimentsOverview(ctx context.Context, channelID string, sinceDays *int32) (*model.ExperimentsOverview, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "experiments.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	days := int32(30)
	if sinceDays != nil && *sinceDays > 0 {
		days = *sinceDays
	}
	ov, err := r.ExperimentSvc.Overview(ctx, cid, time.Now().AddDate(0, 0, -int(days)))
	if err != nil {
		return nil, err
	}
	return toExperimentsOverview(ov), nil
}

func (r *queryResolver) ExperimentGoalEvents(ctx context.Context, channelID string) ([]string, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "experiments.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	// 90-day lookback so events that fired a while ago are still suggestable.
	return r.ExperimentSvc.GoalEvents(ctx, cid, time.Now().AddDate(0, 0, -90))
}

func (r *queryResolver) ExperimentSegments(ctx context.Context) ([]*model.SegmentOption, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "experiments.view"); err != nil {
		return nil, err
	}
	if r.SegmentSvc == nil {
		return []*model.SegmentOption{}, nil
	}
	segs, _, err := r.SegmentSvc.List(ctx, 200, 0)
	if err != nil {
		return nil, err
	}
	out := make([]*model.SegmentOption, 0, len(segs))
	for _, s := range segs {
		if s.IsActive {
			out = append(out, &model.SegmentOption{Slug: s.Slug, Name: s.Name})
		}
	}
	return out, nil
}
