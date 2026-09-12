package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
)

func (r *mutationResolver) CreateExperiment(ctx context.Context, input model.SaveExperimentInput) (*model.Experiment, error) {
	authed, err := r.signalsScoped(ctx, "experiments.manage")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(input.ChannelID); err != nil {
		return nil, errInvalidID("channelId", err)
	}
	res, err := r.SignalsAPI.Prism.CreateExperiment(authed, &signalsv1.CreateExperimentRequest{Experiment: experimentInputProto(input)})
	if err != nil {
		return nil, signalsErr(err)
	}
	return experimentModel(res.GetExperiment()), nil
}

func (r *mutationResolver) UpdateExperiment(ctx context.Context, id string, input model.SaveExperimentInput) (*model.Experiment, error) {
	authed, err := r.signalsScoped(ctx, "experiments.manage")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, errInvalidID("id", err)
	}
	res, err := r.SignalsAPI.Prism.UpdateExperiment(authed, &signalsv1.UpdateExperimentRequest{Id: id, Experiment: experimentInputProto(input)})
	if err != nil {
		return nil, signalsErr(err)
	}
	return experimentModel(res.GetExperiment()), nil
}

func (r *mutationResolver) DeleteExperiment(ctx context.Context, id string) (bool, error) {
	authed, err := r.signalsScoped(ctx, "experiments.manage")
	if err != nil {
		return false, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return false, errInvalidID("id", err)
	}
	if _, err := r.SignalsAPI.Prism.DeleteExperiment(authed, &signalsv1.DeleteExperimentRequest{Id: id}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *queryResolver) Experiments(ctx context.Context, channelID string) ([]*model.Experiment, error) {
	authed, err := r.signalsScoped(ctx, "experiments.view")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(channelID); err != nil {
		return nil, errInvalidID("channelId", err)
	}
	res, err := r.SignalsAPI.Prism.ListExperiments(authed, &signalsv1.ListExperimentsRequest{ChannelId: channelID, PageSize: 500})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.Experiment, 0, len(res.GetExperiments()))
	for _, e := range res.GetExperiments() {
		out = append(out, experimentModel(e))
	}
	return out, nil
}

func (r *queryResolver) Experiment(ctx context.Context, id string) (*model.Experiment, error) {
	authed, err := r.signalsScoped(ctx, "experiments.view")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, errInvalidID("id", err)
	}
	res, err := r.SignalsAPI.Prism.GetExperiment(authed, &signalsv1.GetExperimentRequest{Id: id})
	if err != nil {
		if signalsNotFound(err) { // the old surface answered null rather than an error
			return nil, nil
		}
		return nil, signalsErr(err)
	}
	return experimentModel(res.GetExperiment()), nil
}

func (r *queryResolver) ExperimentResults(ctx context.Context, id string, sinceDays *int32) (*model.ExperimentResults, error) {
	authed, err := r.signalsScoped(ctx, "experiments.view")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, errInvalidID("id", err)
	}
	res, err := r.SignalsAPI.Prism.GetExperimentResults(authed, &signalsv1.GetExperimentResultsRequest{Id: id, SinceDays: signalsDays(sinceDays)})
	if err != nil {
		return nil, signalsErr(err)
	}
	return experimentResultsModel(res), nil
}

func (r *queryResolver) ExperimentsOverview(ctx context.Context, channelID string, sinceDays *int32) (*model.ExperimentsOverview, error) {
	authed, err := r.signalsScoped(ctx, "experiments.view")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(channelID); err != nil {
		return nil, errInvalidID("channelId", err)
	}
	res, err := r.SignalsAPI.Prism.GetExperimentsOverview(authed, &signalsv1.GetExperimentsOverviewRequest{ChannelId: channelID, SinceDays: signalsDays(sinceDays)})
	if err != nil {
		return nil, signalsErr(err)
	}
	return experimentsOverviewModel(res), nil
}

func (r *queryResolver) ExperimentGoalEvents(ctx context.Context, channelID string) ([]string, error) {
	authed, err := r.signalsScoped(ctx, "experiments.view")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(channelID); err != nil {
		return nil, errInvalidID("channelId", err)
	}
	res, err := r.SignalsAPI.Prism.ListGoalEvents(authed, &signalsv1.ListGoalEventsRequest{ChannelId: channelID})
	if err != nil {
		return nil, signalsErr(err)
	}
	return res.GetEvents(), nil
}
