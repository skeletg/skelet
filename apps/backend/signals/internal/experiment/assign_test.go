package experiment

import (
	"fmt"
	"testing"
)

func sampleExp() Experiment {
	return Experiment{
		Key:      "hero-cta",
		Status:   StatusRunning,
		Coverage: 100,
		Variations: []Variation{
			{Key: "control", Weight: 1, Value: "Buy now"},
			{Key: "treat", Weight: 1, Value: "Shop the sale"},
		},
	}
}

func TestAssignDeterministicAndSticky(t *testing.T) {
	e := sampleExp()
	v1, in1 := e.Assign("visitor-123")
	v2, in2 := e.Assign("visitor-123")
	if !in1 || !in2 {
		t.Fatal("100% coverage should always enter")
	}
	if v1.Key != v2.Key {
		t.Fatalf("not sticky: %q vs %q", v1.Key, v2.Key)
	}
}

func TestAssignRoughlyBalanced(t *testing.T) {
	e := sampleExp()
	counts := map[string]int{}
	const n = 4000
	for i := range n {
		v, _ := e.Assign(fmt.Sprintf("anon-%d", i))
		counts[v.Key]++
	}
	for k, c := range counts {
		frac := float64(c) / n
		if frac < 0.4 || frac > 0.6 { // 50/50 split, generous tolerance
			t.Errorf("variation %q got %.2f of traffic (expected ~0.5)", k, frac)
		}
	}
}

func TestCoverageGate(t *testing.T) {
	e := sampleExp()
	e.Coverage = 30
	in := 0
	const n = 4000
	for i := range n {
		if _, ok := e.Assign(fmt.Sprintf("anon-%d", i)); ok {
			in++
		}
	}
	frac := float64(in) / n
	if frac < 0.2 || frac > 0.4 { // ~30% entered
		t.Errorf("coverage 30%% admitted %.2f", frac)
	}
}
