package signalsgrpc

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/experiment"
)

type prismServer struct {
	signalsv1.UnimplementedPrismServiceServer
	svc *experiment.Service
}

func (s *prismServer) ListExperiments(ctx context.Context, req *signalsv1.ListExperimentsRequest) (*signalsv1.ListExperimentsResponse, error) {
	cid, err := channelScope(ctx, "experiments.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	exps, err := s.svc.List(ctx, cid)
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.Experiment, 0, len(exps))
	for _, e := range exps {
		out = append(out, experimentProto(e))
	}
	return &signalsv1.ListExperimentsResponse{Experiments: out}, nil
}

func (s *prismServer) GetExperiment(ctx context.Context, req *signalsv1.GetExperimentRequest) (*signalsv1.GetExperimentResponse, error) {
	exp, err := s.load(ctx, "experiments.view", req.GetId())
	if err != nil {
		return nil, err
	}
	return &signalsv1.GetExperimentResponse{Experiment: experimentProto(exp)}, nil
}

func (s *prismServer) GetExperimentResults(ctx context.Context, req *signalsv1.GetExperimentResultsRequest) (*signalsv1.GetExperimentResultsResponse, error) {
	exp, err := s.load(ctx, "experiments.view", req.GetId())
	if err != nil {
		return nil, err
	}
	days := windowDays(req.GetSinceDays(), 30)
	_, analysis, err := s.svc.Results(ctx, exp.ID, time.Now().AddDate(0, 0, -int(days)))
	if err != nil {
		return nil, experimentErr(err)
	}
	vars := make([]*signalsv1.ExperimentVariationResult, 0, len(analysis.Variations))
	for _, v := range analysis.Variations {
		vars = append(vars, &signalsv1.ExperimentVariationResult{
			Variation: v.Variation, Name: v.Name,
			Exposed: int32(v.Exposed), Converted: int32(v.Converted), RevenueCents: v.RevenueCents,
			Rate: v.Rate, CiLow: v.CILow, CiHigh: v.CIHigh, Uplift: v.Uplift, ChanceToWin: v.ChanceToWin,
			IsControl: v.IsControl, Significant: v.Significant,
			AlwaysValidP: v.AlwaysValidP, SequentialSignificant: v.SequentialSignificant,
		})
	}
	return &signalsv1.GetExperimentResultsResponse{
		Variations: vars, Winner: analysis.Winner, SequentialWinner: analysis.SequentialWinner,
		SampleRatioMismatch: analysis.SampleRatioMismatch,
		TotalExposed:        int32(analysis.TotalExposed), TotalRevenue: analysis.TotalRevenueCents,
	}, nil
}

func (s *prismServer) GetExperimentsOverview(ctx context.Context, req *signalsv1.GetExperimentsOverviewRequest) (*signalsv1.GetExperimentsOverviewResponse, error) {
	cid, err := channelScope(ctx, "experiments.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	days := windowDays(req.GetSinceDays(), 30)
	ov, err := s.svc.Overview(ctx, cid, time.Now().AddDate(0, 0, -int(days)))
	if err != nil {
		return nil, err
	}
	sums := make([]*signalsv1.ExperimentSummary, 0, len(ov.Experiments))
	for _, sm := range ov.Experiments {
		sums = append(sums, &signalsv1.ExperimentSummary{
			Id: sm.Experiment.ID.String(), Key: sm.Experiment.Key, Name: sm.Experiment.Name,
			Variations: int32(len(sm.Experiment.Variations)), Coverage: int32(sm.Experiment.Coverage),
			TotalExposed: int32(sm.TotalExposed), Conversions: int32(sm.Conversions),
			Leader: sm.Leader, LeaderRate: sm.LeaderRate, LeaderUplift: sm.LeaderUplift,
			LeaderChanceToWin: sm.LeaderChanceToWin, LeaderIsControl: sm.LeaderIsControl,
			Winner: sm.Winner, SampleRatioMismatch: sm.SampleRatioMismatch,
		})
	}
	return &signalsv1.GetExperimentsOverviewResponse{
		Running: int32(ov.Running), Draft: int32(ov.Draft), Stopped: int32(ov.Stopped),
		TotalExposed: int32(ov.TotalExposed), Decided: int32(ov.Decided), Experiments: sums,
	}, nil
}

func (s *prismServer) ListGoalEvents(ctx context.Context, req *signalsv1.ListGoalEventsRequest) (*signalsv1.ListGoalEventsResponse, error) {
	cid, err := channelScope(ctx, "experiments.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	// The suggestion window is the server's to pick - it feeds a picker, not a report
	events, err := s.svc.GoalEvents(ctx, cid, time.Now().AddDate(0, 0, -90))
	if err != nil {
		return nil, err
	}
	return &signalsv1.ListGoalEventsResponse{Events: events}, nil
}

func (s *prismServer) CreateExperiment(ctx context.Context, req *signalsv1.CreateExperimentRequest) (*signalsv1.CreateExperimentResponse, error) {
	if err := requirePermission(ctx, "experiments.manage"); err != nil {
		return nil, err
	}
	in, err := experimentInput(req.GetExperiment())
	if err != nil {
		return nil, err
	}
	if err := requireChannel(ctx, "experiments.manage", in.ChannelID); err != nil {
		return nil, err
	}
	exp, err := s.svc.Create(ctx, in)
	if err != nil {
		return nil, experimentErr(err)
	}
	return &signalsv1.CreateExperimentResponse{Experiment: experimentProto(exp)}, nil
}

func (s *prismServer) UpdateExperiment(ctx context.Context, req *signalsv1.UpdateExperimentRequest) (*signalsv1.UpdateExperimentResponse, error) {
	existing, err := s.load(ctx, "experiments.manage", req.GetId())
	if err != nil {
		return nil, err
	}
	in, err := experimentInput(req.GetExperiment())
	if err != nil {
		return nil, err
	}
	in.ChannelID = existing.ChannelID
	exp, err := s.svc.Update(ctx, existing.ID, in)
	if err != nil {
		return nil, experimentErr(err)
	}
	return &signalsv1.UpdateExperimentResponse{Experiment: experimentProto(exp)}, nil
}

func (s *prismServer) DeleteExperiment(ctx context.Context, req *signalsv1.DeleteExperimentRequest) (*signalsv1.DeleteExperimentResponse, error) {
	exp, err := s.load(ctx, "experiments.manage", req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.Delete(ctx, exp.ChannelID, exp.ID); err != nil {
		return nil, err
	}
	return &signalsv1.DeleteExperimentResponse{Deleted: true}, nil
}

func (s *prismServer) load(ctx context.Context, perm, rawID string) (experiment.Experiment, error) {
	if err := requirePermission(ctx, perm); err != nil {
		return experiment.Experiment{}, err
	}
	id, err := parseID("id", rawID)
	if err != nil {
		return experiment.Experiment{}, err
	}
	exp, found, err := s.svc.Get(ctx, id)
	if err != nil {
		return experiment.Experiment{}, err
	}
	if !found {
		return experiment.Experiment{}, status.Error(codes.NotFound, "experiment not found")
	}
	if err := requireChannel(ctx, perm, exp.ChannelID); err != nil {
		return experiment.Experiment{}, err
	}
	return exp, nil
}

func experimentProto(e experiment.Experiment) *signalsv1.Experiment {
	vars := make([]*signalsv1.ExperimentVariation, 0, len(e.Variations))
	for _, v := range e.Variations {
		vars = append(vars, &signalsv1.ExperimentVariation{Key: v.Key, Name: v.Name, Weight: int32(v.Weight), Value: v.Value})
	}
	return &signalsv1.Experiment{
		Id: e.ID.String(), ChannelId: e.ChannelID.String(), Key: e.Key, Name: e.Name,
		Status: e.Status, Coverage: int32(e.Coverage), GoalEvent: e.GoalEvent, SegmentSlug: e.SegmentSlug,
		Variations: vars, CreatedAt: ts(e.CreatedAt), UpdatedAt: ts(e.UpdatedAt),
	}
}

func experimentInput(in *signalsv1.ExperimentInput) (experiment.Input, error) {
	if in == nil {
		return experiment.Input{}, status.Error(codes.InvalidArgument, "experiment is required")
	}
	cid, err := parseID("channel_id", in.GetChannelId())
	if err != nil {
		return experiment.Input{}, err
	}
	vars := make([]experiment.Variation, 0, len(in.GetVariations()))
	for _, v := range in.GetVariations() {
		vars = append(vars, experiment.Variation{Key: v.GetKey(), Name: v.GetName(), Weight: int(v.GetWeight()), Value: v.GetValue()})
	}
	return experiment.Input{
		ChannelID: cid, Key: in.GetKey(), Name: in.GetName(), Status: in.GetStatus(),
		Coverage: int(in.GetCoverage()), GoalEvent: in.GetGoalEvent(), SegmentSlug: in.GetSegmentSlug(),
		Variations: vars,
	}, nil
}

// The experiment service's sentinels as the codes core's resolvers render - anything else is the suite's own failure
func experimentErr(err error) error {
	switch {
	case errors.Is(err, experiment.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, experiment.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, experiment.ErrKeyTaken):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, experiment.ErrLocked):
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return err
}
