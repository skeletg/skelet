package admin // AI-chat tools for Prism experiments - the shared registry, dispatch and schema helpers live in aichat_tools.go

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func prismSignalTools() []signalTool {
	return []signalTool{
		{
			def: ai.Tool{
				Name:        "get_experiments",
				Description: "Prism A/B experiments: running vs decided counts and, per experiment, its id, leader, uplift, and chance-to-win. Use an experiment id with get_experiment_results.",
				Schema:      windowSchema(nil),
			},
			perm: func(p digestPerms) bool { return p.prism },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, _ time.Time, days int, _ toolArgs) string {
				return reuseDigest(func(b *strings.Builder) { writePrismDigest(ctx, r, cid, days, b) }, "No experiments in this window.")
			},
		},
		{
			def: ai.Tool{
				Name:        "get_experiment_results",
				Description: "Full per-variant results for one experiment (by experiment_id from get_experiments): exposed, conversions, rate, uplift, chance-to-win, significance, and the winner.",
				Schema: objSchema(map[string]any{
					"experiment_id": map[string]any{"type": "string", "description": "Experiment UUID from get_experiments."},
					"window_days":   windowProp(),
				}, "experiment_id"),
			},
			perm: func(p digestPerms) bool { return p.prism },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolExperimentResults(ctx, r, cid, days, a)
			},
		},
	}
}

func toolExperimentResults(ctx context.Context, r *Resolver, cid uuid.UUID, days int, a toolArgs) string {
	eid, err := uuid.Parse(strings.TrimSpace(a.ExperimentID))
	if err != nil {
		return "Provide a valid 'experiment_id' (use get_experiments)."
	}
	authed, err := signalsCall(ctx, r)
	if err != nil {
		return signalsUnavailable(err, "experiment results")
	}
	// The definition names the experiment and says whose channel it is - the analysis has the numbers
	exp, err := r.SignalsAPI.Prism.GetExperiment(authed, &signalsv1.GetExperimentRequest{Id: eid.String()})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return "Experiment not found."
		}
		return signalsUnavailable(err, "experiment results")
	}
	if exp.GetExperiment().GetChannelId() != cid.String() { // an experiment_id is the one tool arg that could reach across channels
		return "That experiment belongs to a different channel."
	}
	an, err := r.SignalsAPI.Prism.GetExperimentResults(authed, &signalsv1.GetExperimentResultsRequest{Id: eid.String(), SinceDays: int32(days)})
	if err != nil {
		return signalsUnavailable(err, "experiment results")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Experiment %q (goal %q): %d exposed.\n", exp.GetExperiment().GetName(), exp.GetExperiment().GetGoalEvent(), an.GetTotalExposed())
	for _, v := range an.GetVariations() {
		tag := ""
		if v.GetIsControl() {
			tag = " [control]"
		}
		sig := ""
		if v.GetSignificant() {
			sig = ", significant"
		}
		fmt.Fprintf(&b, "- %s%s: %d/%d converted (%.2f%%), uplift %.1f%%, chance-to-win %.0f%%%s\n",
			v.GetName(), tag, v.GetConverted(), v.GetExposed(), v.GetRate()*100, v.GetUplift()*100, v.GetChanceToWin()*100, sig)
	}
	if an.GetWinner() != "" {
		fmt.Fprintf(&b, "Winner: %s\n", an.GetWinner())
	} else {
		b.WriteString("No significant winner yet.\n")
	}
	return b.String()
}
