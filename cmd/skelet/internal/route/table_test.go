package route

import "testing"

// Every route has a handler and a help text, no two answer the same command line, and the table holds every verb -
// a floor, so a refactor that drops routes fails here rather than in someone's terminal
func TestTable(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range routes {
		key := r.Verb + " " + r.Sub + " " + r.Flag
		if r.Go == nil || r.About == "" {
			t.Errorf("%q has no handler or no About", key)
		}
		if seen[key] {
			t.Errorf("%q is routed twice", key)
		}
		seen[key] = true
	}
	if len(routes) < 40 {
		t.Errorf("only %d routes - the table lost some", len(routes))
	}
}
