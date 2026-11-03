package agenttools // Agent tools for Prism experiments - the shared registry, dispatch and schema helpers live in tools.go

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

func prismTools() []tool {
	return []tool{
		{
			def: ai.Tool{
				Name:        "get_experiments",
				Description: "Prism A/B experiments: running vs decided counts and, per experiment, its id, leader, uplift, and chance-to-win. Use an experiment id with get_experiment_results.",
				Schema:      windowSchema(nil),
			},
			perm: func(p Perms) bool { return p.Prism },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time, days int, _ toolArgs) string {
				return prismDigest.tool(ctx, deps, cid, since, days)
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
			perm: func(p Perms) bool { return p.Prism },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolExperimentResults(ctx, deps, cid, days, a)
			},
		},
	}
}

func toolExperimentResults(ctx context.Context, deps *Deps, cid uuid.UUID, days int, a toolArgs) string {
	eid, err := uuid.Parse(strings.TrimSpace(a.ExperimentID))
	if err != nil {
		return "Provide a valid 'experiment_id' (use get_experiments)."
	}
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "experiment results")
	}
	// The definition names the experiment and says whose channel it is - the analysis has the numbers
	exp, err := deps.Signals.Prism.GetExperiment(authed, &signalsv1.GetExperimentRequest{Id: eid.String()})
	if err != nil {
		switch status.Code(err) {
		case codes.NotFound:
			return "Experiment not found."
		case codes.PermissionDenied:
			return "That experiment belongs to a channel you cannot view."
		}
		return signalsUnavailable(ctx, err, "experiment results")
	}
	if exp.GetExperiment().GetChannelId() != cid.String() { // an experiment_id is the one tool arg that could reach across channels
		return "That experiment belongs to a different channel."
	}
	an, err := deps.Signals.Prism.GetExperimentResults(authed, &signalsv1.GetExperimentResultsRequest{Id: eid.String(), SinceDays: int32(days)})
	if err != nil {
		return signalsUnavailable(ctx, err, "experiment results")
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
