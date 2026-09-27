package signalsgrpc

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/segment"
)

func TestTheContractCarriesWhySegmentRulesCantRun(t *testing.T) {
	broken := segment.Segment{ID: uuid.New(), Rules: json.RawMessage(`{"conditions":[{"kind":"total_spent_cents","op":"gte","value":100}]}`)}
	if p := segmentProto(broken, 0).GetRulesProblem(); p == "" {
		t.Fatal("a money rule with no channel went over the contract with no problem, so the dashboard shows it as healthy")
	}
	ok := segment.Segment{ID: uuid.New(), Rules: json.RawMessage(`{"conditions":[{"kind":"order_count","op":"gte","value":1}]}`)}
	if p := segmentProto(ok, 0).GetRulesProblem(); p != "" {
		t.Fatalf("a runnable rule reported %q", p)
	}
}
