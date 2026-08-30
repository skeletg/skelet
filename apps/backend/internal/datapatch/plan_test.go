package datapatch

import (
	"context"
	"strings"
	"testing"
)

func noop(context.Context, *Context) error { return nil }

func p(id string, deps ...string) Patch {
	return Patch{ID: id, Namespace: "core", DependsOn: deps, Apply: noop}
}

func ids(ps []Patch) []string {
	out := make([]string, len(ps))
	for i, x := range ps {
		out[i] = x.ID
	}
	return out
}

func applied(idsList ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(idsList))
	for _, id := range idsList {
		m[id] = struct{}{}
	}
	return m
}

func TestPlan_StableIDOrderWhenIndependent(t *testing.T) {
	got, err := plan([]Patch{p("c"), p("a"), p("b")}, applied())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if want := []string{"a", "b", "c"}; !equal(ids(got), want) {
		t.Errorf("order: got %v, want %v", ids(got), want)
	}
}

func TestPlan_DependenciesRunFirst(t *testing.T) {
	// b depends on c; a depends on b → c, b, a regardless of input order
	got, err := plan([]Patch{p("a", "b"), p("b", "c"), p("c")}, applied())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if want := []string{"c", "b", "a"}; !equal(ids(got), want) {
		t.Errorf("order: got %v, want %v", ids(got), want)
	}
}

func TestPlan_SkipsApplied(t *testing.T) {
	got, err := plan([]Patch{p("a"), p("b"), p("c")}, applied("a", "c"))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if want := []string{"b"}; !equal(ids(got), want) {
		t.Errorf("pending: got %v, want %v", ids(got), want)
	}
}

func TestPlan_DependencyOnAppliedPatchIsSatisfied(t *testing.T) {
	// a depends on b; b already applied → a is pending, runs fine
	got, err := plan([]Patch{p("a", "b"), p("b")}, applied("b"))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if want := []string{"a"}; !equal(ids(got), want) {
		t.Errorf("pending: got %v, want %v", ids(got), want)
	}
}

func TestPlan_AppliedDepNoLongerRegisteredIsSatisfied(t *testing.T) {
	// a depends on "gone"; "gone" isn't registered but is applied → ok
	got, err := plan([]Patch{p("a", "gone")}, applied("gone"))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if want := []string{"a"}; !equal(ids(got), want) {
		t.Errorf("pending: got %v, want %v", ids(got), want)
	}
}

func TestPlan_UnknownDependencyErrors(t *testing.T) {
	_, err := plan([]Patch{p("a", "missing")}, applied())
	if err == nil || !strings.Contains(err.Error(), "unknown patch") {
		t.Fatalf("got %v, want unknown-patch error", err)
	}
}

func TestPlan_CycleErrors(t *testing.T) {
	_, err := plan([]Patch{p("a", "b"), p("b", "a")}, applied())
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("got %v, want cycle error", err)
	}
}

func TestRegister_DuplicateIDPanics(t *testing.T) {
	resetForTests()
	defer resetForTests()
	Register(Patch{ID: "x", Apply: noop})
	defer func() {
		if recover() == nil {
			t.Error("expected panic on duplicate ID")
		}
	}()
	Register(Patch{ID: "x", Apply: noop})
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
