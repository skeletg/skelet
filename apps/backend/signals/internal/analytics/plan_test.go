package analytics

import (
	"reflect"
	"testing"
)

func TestApplyPlanStripsWhatBreaksIt(t *testing.T) {
	props := map[string]any{
		"sku": "TEE", "entity_price": "25.00", "attr:color": "black", // planned
		"is_bot": false, "revenue_cents": 99999, "name": "purchase", // only the server sets these
		"entity":    7,      // wrong type
		"coupon":    "FREE", // not in the plan
		"attr:size": 42.0,   // an attribute must be a string
	}
	keep, issues := ApplyPlan(EventPageView, props)
	if !keep {
		t.Fatal("a planned event was dropped")
	}
	want := map[string]any{"sku": "TEE", "entity_price": "25.00", "attr:color": "black"}
	if !reflect.DeepEqual(props, want) {
		t.Fatalf("props after the plan = %v, want %v", props, want)
	}
	got := map[string]string{}
	for _, is := range issues {
		if is.Event != EventPageView {
			t.Errorf("issue on %q, want page_view", is.Event)
		}
		got[is.Prop] = is.Kind
	}
	wantIssues := map[string]string{
		"is_bot": IssueServerProp, "revenue_cents": IssueServerProp, "name": IssueServerProp,
		"entity": IssueBadType, "attr:size": IssueBadType, "coupon": IssueUnknownProp,
	}
	if !reflect.DeepEqual(got, wantIssues) {
		t.Fatalf("issues = %v, want %v", got, wantIssues)
	}
}

func TestApplyPlanOnNamedEvents(t *testing.T) {
	if keep, issues := ApplyPlan("$made_up", map[string]any{}); keep || len(issues) != 1 || issues[0].Kind != IssueUnknownEvent {
		t.Fatalf("an unplanned $ event: keep=%v issues=%v, want it dropped and flagged", keep, issues)
	}
	// A merchant's own interaction name is open, held to the custom entry's properties
	props := map[string]any{"action": "expand", "currency": "EUR"}
	keep, issues := ApplyPlan("Size guide", props)
	if !keep || len(issues) != 1 || issues[0].Event != CustomEvent || issues[0].Prop != "currency" {
		t.Fatalf("a named interaction: keep=%v issues=%v", keep, issues)
	}
	if keep, issues := ApplyPlan("$search", map[string]any{"query": "shirt", "results": 3.0}); !keep || len(issues) != 0 {
		t.Fatalf("a clean $search: keep=%v issues=%v", keep, issues)
	}
	if _, issues := ApplyPlan("$search", map[string]any{"results": "3"}); len(issues) != 1 || issues[0].Kind != IssueBadType {
		t.Fatalf("results as a string: %v, want a bad type", issues)
	}
}

func TestEveryPlannedPropertyHasAKnownType(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range TrackingPlan {
		if seen[e.Name] {
			t.Errorf("%s is planned twice", e.Name)
		}
		seen[e.Name] = true
		if e.About == "" {
			t.Errorf("%s has no description for the docs", e.Name)
		}
		for k, typ := range e.Props {
			if typ != PropString && typ != PropNumber && typ != PropBool {
				t.Errorf("%s.%s has type %q", e.Name, k, typ)
			}
			if serverProps[k] {
				t.Errorf("%s plans %s, which only the server may set", e.Name, k)
			}
		}
	}
}
