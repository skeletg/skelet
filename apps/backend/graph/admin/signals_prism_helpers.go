package admin

// Converters between the Prism contract and the admin models

import (
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
)

func experimentInputProto(in model.SaveExperimentInput) *signalsv1.ExperimentInput {
	vars := make([]*signalsv1.ExperimentVariation, 0, len(in.Variations))
	for _, v := range in.Variations {
		vars = append(vars, &signalsv1.ExperimentVariation{Key: v.Key, Name: v.Name, Weight: v.Weight, Value: v.Value})
	}
	out := &signalsv1.ExperimentInput{
		ChannelId: in.ChannelID, Key: in.Key, Name: in.Name, Status: in.Status,
		Coverage: in.Coverage, GoalEvent: in.GoalEvent, Variations: vars,
	}
	if in.SegmentSlug != nil {
		out.SegmentSlug = *in.SegmentSlug
	}
	return out
}

func experimentModel(e *signalsv1.Experiment) *model.Experiment {
	vs := make([]*model.ExperimentVariation, 0, len(e.GetVariations()))
	for _, v := range e.GetVariations() {
		vs = append(vs, &model.ExperimentVariation{Key: v.GetKey(), Name: v.GetName(), Weight: v.GetWeight(), Value: v.GetValue()})
	}
	return &model.Experiment{
		ID: e.GetId(), ChannelID: e.GetChannelId(), Key: e.GetKey(), Name: e.GetName(),
		Status: e.GetStatus(), Coverage: e.GetCoverage(), GoalEvent: e.GetGoalEvent(), SegmentSlug: e.GetSegmentSlug(),
		Variations: vs, CreatedAt: e.GetCreatedAt().AsTime(), UpdatedAt: e.GetUpdatedAt().AsTime(),
	}
}

func experimentResultsModel(res *signalsv1.GetExperimentResultsResponse) *model.ExperimentResults {
	vs := make([]*model.ExperimentVariationResult, 0, len(res.GetVariations()))
	for _, v := range res.GetVariations() {
		vs = append(vs, &model.ExperimentVariationResult{
			Variation: v.GetVariation(), Name: v.GetName(), Exposed: v.GetExposed(), Converted: v.GetConverted(),
			RevenueCents: int(v.GetRevenueCents()), Rate: v.GetRate(), CiLow: v.GetCiLow(), CiHigh: v.GetCiHigh(),
			Uplift: v.GetUplift(), ChanceToWin: v.GetChanceToWin(), IsControl: v.GetIsControl(),
			Significant: v.GetSignificant(), AlwaysValidP: v.GetAlwaysValidP(), SequentialSignificant: v.GetSequentialSignificant(),
		})
	}
	return &model.ExperimentResults{
		Variations: vs, Winner: res.GetWinner(), SequentialWinner: res.GetSequentialWinner(),
		SampleRatioMismatch: res.GetSampleRatioMismatch(),
		TotalExposed:        res.GetTotalExposed(), TotalRevenue: int(res.GetTotalRevenue()),
	}
}

func experimentsOverviewModel(ov *signalsv1.GetExperimentsOverviewResponse) *model.ExperimentsOverview {
	exps := make([]*model.ExperimentSummary, 0, len(ov.GetExperiments()))
	for _, s := range ov.GetExperiments() {
		exps = append(exps, &model.ExperimentSummary{
			ID: s.GetId(), Key: s.GetKey(), Name: s.GetName(),
			Variations: s.GetVariations(), Coverage: s.GetCoverage(),
			TotalExposed: s.GetTotalExposed(), Conversions: s.GetConversions(), Leader: s.GetLeader(),
			LeaderRate: s.GetLeaderRate(), LeaderUplift: s.GetLeaderUplift(), LeaderChanceToWin: s.GetLeaderChanceToWin(),
			LeaderIsControl: s.GetLeaderIsControl(), Winner: s.GetWinner(), SampleRatioMismatch: s.GetSampleRatioMismatch(),
		})
	}
	return &model.ExperimentsOverview{
		Running: ov.GetRunning(), Draft: ov.GetDraft(), Stopped: ov.GetStopped(),
		TotalExposed: ov.GetTotalExposed(), Decided: ov.GetDecided(), Experiments: exps,
	}
}
