package segment

import (
	"os"
	"regexp"
	"testing"
)

// A stage the recompute writes but rules reject is an audience nobody can target, and one rules accept but it never
// writes is a segment that silently matches nobody - both failed silently until someone wondered why a list was empty
func TestLifecycleStagesMatchWhatTheRecomputeWrites(t *testing.T) {
	const query = "../db/query/traits.sql"
	sql, err := os.ReadFile(query)
	if err != nil {
		t.Fatalf("read %s: %v", query, err)
	}
	// The recompute assigns them as quoted literals in the arms of its lifecycle CASE, the default one included
	emitted := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?:THEN|ELSE) '(prospect|new|active|at_risk|lapsed|dormant)'`).FindAllStringSubmatch(string(sql), -1) {
		emitted[m[1]] = true
	}
	if len(emitted) == 0 {
		t.Fatalf("found no lifecycle literals in %s - the check would pass vacuously", query)
	}
	for stage := range emitted {
		if !lifecycleStages[stage] {
			t.Errorf("the recompute writes %q but a segment rule cannot select it", stage)
		}
	}
	for stage := range lifecycleStages {
		if !emitted[stage] {
			t.Errorf("a segment rule accepts %q but the recompute never writes it, so it matches nobody", stage)
		}
	}
}
