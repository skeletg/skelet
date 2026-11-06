package admin

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/tracking"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// The import preview offers a row exactly when the import would accept it: an unmappable one has a problem and no
// target, and a mapped one names its target, with a problem only when this build lacks the plugin
func TestTheImportPreviewMatchesWhatTheImportAccepts(t *testing.T) {
	bad := importCandidate(&signalsv1.EventDestination{Id: "x", Kind: "ftp", TargetUrl: "https://example.com"}, nil)
	if bad.Problem == nil || bad.TargetKind != nil {
		t.Errorf("an unmappable destination: problem %v, target %v", bad.Problem, bad.TargetKind)
	}
	l := &signalsv1.EventDestination{Id: "y", Kind: "ga4_mp", TargetUrl: "https://www.google-analytics.com/mp/collect?measurement_id=G-1"}
	c := importCandidate(l, nil)
	_, _, err := legacyTarget(l)
	_, installed := tracking.KindFor("ga4")
	if c.TargetKind == nil || *c.TargetKind != "ga4" || (c.Problem == nil) != installed || (err == nil) != installed {
		t.Errorf("GA4 (plugin in this build: %v): target %v, problem %v, import error %v", installed, c.TargetKind, c.Problem, err)
	}
}
