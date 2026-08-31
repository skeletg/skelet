package admin

import (
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/experiment"
)

// experimentInput parses a SaveExperimentInput into the domain Input. Kept out
// of the resolver file so gqlgen doesn't evict it on regeneration.
func experimentInput(in model.SaveExperimentInput) (experiment.Input, error) {
	cid, err := uuid.Parse(in.ChannelID)
	if err != nil {
		return experiment.Input{}, errInvalidID("channelId", err)
	}
	vars := make([]experiment.Variation, 0, len(in.Variations))
	for _, v := range in.Variations {
		vars = append(vars, experiment.Variation{Key: v.Key, Name: v.Name, Weight: int(v.Weight), Value: v.Value})
	}
	segmentSlug := ""
	if in.SegmentSlug != nil {
		segmentSlug = *in.SegmentSlug
	}
	return experiment.Input{
		ChannelID: cid, Key: in.Key, Name: in.Name, Status: in.Status,
		Coverage: int(in.Coverage), GoalEvent: in.GoalEvent, SegmentSlug: segmentSlug, Variations: vars,
	}, nil
}

func toExperimentModel(e experiment.Experiment) *model.Experiment {
	vs := make([]*model.ExperimentVariation, 0, len(e.Variations))
	for _, v := range e.Variations {
		vs = append(vs, &model.ExperimentVariation{Key: v.Key, Name: v.Name, Weight: int32(v.Weight), Value: v.Value})
	}
	return &model.Experiment{
		ID: e.ID.String(), ChannelID: e.ChannelID.String(), Key: e.Key, Name: e.Name,
		Status: e.Status, Coverage: int32(e.Coverage), GoalEvent: e.GoalEvent, SegmentSlug: e.SegmentSlug,
		Variations: vs, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}
}

func toExperimentResults(a experiment.Analysis) *model.ExperimentResults {
	vs := make([]*model.ExperimentVariationResult, 0, len(a.Variations))
	for _, v := range a.Variations {
		vs = append(vs, &model.ExperimentVariationResult{
			Variation: v.Variation, Name: v.Name, Exposed: int32(v.Exposed), Converted: int32(v.Converted),
			RevenueCents: int(v.RevenueCents), Rate: v.Rate, CiLow: v.CILow, CiHigh: v.CIHigh, Uplift: v.Uplift,
			ChanceToWin: v.ChanceToWin, IsControl: v.IsControl, Significant: v.Significant,
			AlwaysValidP: v.AlwaysValidP, SequentialSignificant: v.SequentialSignificant,
		})
	}
	return &model.ExperimentResults{
		Variations: vs, Winner: a.Winner,
		SequentialWinner: a.SequentialWinner, SampleRatioMismatch: a.SampleRatioMismatch,
		TotalExposed: int32(a.TotalExposed), TotalRevenue: int(a.TotalRevenueCents),
	}
}

func toExperimentsOverview(ov experiment.Overview) *model.ExperimentsOverview {
	exps := make([]*model.ExperimentSummary, 0, len(ov.Experiments))
	for _, s := range ov.Experiments {
		exps = append(exps, &model.ExperimentSummary{
			ID: s.Experiment.ID.String(), Key: s.Experiment.Key, Name: s.Experiment.Name,
			Variations: int32(len(s.Experiment.Variations)), Coverage: int32(s.Experiment.Coverage),
			TotalExposed: int32(s.TotalExposed), Conversions: int32(s.Conversions), Leader: s.Leader,
			LeaderRate: s.LeaderRate, LeaderUplift: s.LeaderUplift, LeaderChanceToWin: s.LeaderChanceToWin,
			LeaderIsControl: s.LeaderIsControl, Winner: s.Winner, SampleRatioMismatch: s.SampleRatioMismatch,
		})
	}
	return &model.ExperimentsOverview{
		Running: int32(ov.Running), Draft: int32(ov.Draft), Stopped: int32(ov.Stopped),
		TotalExposed: int32(ov.TotalExposed), Decided: int32(ov.Decided), Experiments: exps,
	}
}
