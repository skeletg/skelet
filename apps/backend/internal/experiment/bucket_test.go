package experiment

import (
	"fmt"
	"testing"
)

func TestBucketDeterministicAndInRange(t *testing.T) {
	// Deterministic: same (seed, anonID) → same value
	for i := range 50 {
		anon := fmt.Sprintf("anon-%d", i)
		a := bucket("seed", anon)
		b := bucket("seed", anon)
		if a != b {
			t.Errorf("bucket not deterministic for %q: %v vs %v", anon, a, b)
		}
		if a < 0 || a >= 1.0 {
			t.Errorf("bucket(%q) = %v, want in [0,1)", anon, a)
		}
	}
	// Different seeds should generally decorrelate (the coverage gate relies on this): the variation seed and coverage
	// seed must not be identical for the same anonID across a population
	differ := 0
	const n = 200
	for i := range n {
		anon := fmt.Sprintf("anon-%d", i)
		if bucket("k", anon) != bucket("k:coverage", anon) {
			differ++
		}
	}
	if differ < n/2 {
		t.Errorf("two seeds barely decorrelate: only %d/%d differed", differ, n)
	}
}

func TestAssignCoverageZeroExcludes(t *testing.T) {
	e := sampleExp()
	for _, cov := range []int{0, -1, -100} {
		e.Coverage = cov
		if v, in := e.Assign("visitor-1"); in {
			t.Errorf("coverage %d should exclude, got in=true variation=%q", cov, v.Key)
		}
	}
}

func TestAssignNoVariationsExcludes(t *testing.T) {
	e := sampleExp()
	e.Variations = nil
	if _, in := e.Assign("visitor-1"); in {
		t.Error("no variations should yield inExperiment=false")
	}
	e.Variations = []Variation{}
	if _, in := e.Assign("visitor-1"); in {
		t.Error("empty variations slice should yield inExperiment=false")
	}
}

func TestAssignAllWeightsZeroReturnsControl(t *testing.T) {
	e := sampleExp()
	e.Variations = []Variation{
		{Key: "control", Weight: 0, Value: "A"},
		{Key: "treat", Weight: 0, Value: "B"},
	}
	for i := range 100 {
		v, in := e.Assign(fmt.Sprintf("anon-%d", i))
		if !in {
			t.Fatalf("all-zero weights at 100%% coverage should still enter")
		}
		if v.Key != "control" {
			t.Fatalf("all-zero weights should return Variations[0] (control), got %q", v.Key)
		}
	}
}

func TestAssignWeightedSplit(t *testing.T) {
	e := sampleExp()
	e.Variations = []Variation{
		{Key: "control", Weight: 80, Value: "A"},
		{Key: "treat", Weight: 20, Value: "B"},
	}
	counts := map[string]int{}
	const n = 8000
	for i := range n {
		v, in := e.Assign(fmt.Sprintf("anon-%d", i))
		if !in {
			t.Fatalf("100%% coverage should always enter")
		}
		counts[v.Key]++
	}
	control := float64(counts["control"]) / n
	treat := float64(counts["treat"]) / n
	if control < 0.72 || control > 0.88 {
		t.Errorf("control got %.3f of traffic (expected ~0.80)", control)
	}
	if treat < 0.12 || treat > 0.28 {
		t.Errorf("treat got %.3f of traffic (expected ~0.20)", treat)
	}
	if counts["control"]+counts["treat"] != n {
		t.Errorf("variations don't sum to n: %v", counts)
	}
}

func TestAssignStickyAcrossRepeatedCalls(t *testing.T) {
	e := sampleExp()
	e.Variations = []Variation{
		{Key: "control", Weight: 50, Value: "A"},
		{Key: "treat", Weight: 50, Value: "B"},
	}
	for i := range 200 {
		anon := fmt.Sprintf("anon-%d", i)
		first, _ := e.Assign(anon)
		for range 5 {
			again, in := e.Assign(anon)
			if !in {
				t.Fatalf("%q dropped out on repeat call", anon)
			}
			if again.Key != first.Key {
				t.Fatalf("%q not sticky: %q then %q", anon, first.Key, again.Key)
			}
		}
	}
}

func TestAssignCoverageGateFraction(t *testing.T) {
	e := sampleExp()
	e.Coverage = 70
	in := 0
	const n = 8000
	for i := range n {
		if _, ok := e.Assign(fmt.Sprintf("anon-%d", i)); ok {
			in++
		}
	}
	frac := float64(in) / n
	if frac < 0.62 || frac > 0.78 { // ~70% admitted, generous tolerance
		t.Errorf("coverage 70%% admitted %.3f (expected ~0.70)", frac)
	}
}
