package admin // AI-chat tools for Prism experiments - the shared registry, dispatch and schema helpers live in aichat_tools.go

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/ai"
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
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, _ toolArgs) string {
				return reuseDigest(func(b *strings.Builder) { writePrismDigest(ctx, r, cid, since, b) }, "No experiments in this window.")
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
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolExperimentResults(ctx, r, cid, since, a)
			},
		},
	}
}

func toolExperimentResults(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, a toolArgs) string {
	eid, err := uuid.Parse(strings.TrimSpace(a.ExperimentID))
	if err != nil {
		return "Provide a valid 'experiment_id' (use get_experiments)."
	}
	exp, an, err := r.ExperimentSvc.Results(ctx, eid, since)
	if err != nil {
		return "Could not load experiment results."
	}
	if exp.ID == uuid.Nil {
		return "Experiment not found."
	}
	if exp.ChannelID != cid { // an experiment_id is the one tool arg that could reach across channels
		return "That experiment belongs to a different channel."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Experiment %q (goal %q): %d exposed.\n", exp.Name, exp.GoalEvent, an.TotalExposed)
	for _, v := range an.Variations {
		tag := ""
		if v.IsControl {
			tag = " [control]"
		}
		sig := ""
		if v.Significant {
			sig = ", significant"
		}
		fmt.Fprintf(&b, "- %s%s: %d/%d converted (%.2f%%), uplift %.1f%%, chance-to-win %.0f%%%s\n",
			v.Name, tag, v.Converted, v.Exposed, v.Rate*100, v.Uplift*100, v.ChanceToWin*100, sig)
	}
	if an.Winner != "" {
		fmt.Fprintf(&b, "Winner: %s\n", an.Winner)
	} else {
		b.WriteString("No significant winner yet.\n")
	}
	return b.String()
}
