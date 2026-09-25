package admin

import (
	"testing"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func TestASegmentsRulesProblemReachesTheDashboard(t *testing.T) {
	m := customerSegmentModel(&signalsv1.CustomerSegment{Id: "s", RulesProblem: "condition 1: total_spent_cents needs a channelId"})
	if m.RulesProblem == nil || *m.RulesProblem == "" {
		t.Fatal("the rules problem was dropped on the way to the dashboard, so a broken segment looks healthy")
	}
	if ok := customerSegmentModel(&signalsv1.CustomerSegment{Id: "s"}); ok.RulesProblem != nil {
		t.Fatalf("a runnable segment reports %q, want no problem", *ok.RulesProblem)
	}
}
