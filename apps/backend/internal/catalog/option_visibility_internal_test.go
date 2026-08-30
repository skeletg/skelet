package catalog

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func visSiblings() (parent, other ProductOption) {
	parent = ProductOption{
		ID: uuid.New(), Title: "Gift wrap", InputType: OptionInputRadio, SortOrder: 0,
		Values: []ProductOptionValue{{ID: uuid.New()}, {ID: uuid.New()}},
	}
	other = ProductOption{ID: uuid.New(), Title: "Engraving", InputType: OptionInputField, SortOrder: 1}
	return parent, other
}

func TestValidateOptionVisibility(t *testing.T) {
	parent, other := visSiblings()
	ownID := uuid.New()
	siblings := []ProductOption{parent, other}

	anyOfParent := func(vids ...uuid.UUID) *OptionConditionGroup {
		return &OptionConditionGroup{Rules: []OptionRule{{OptionID: parent.ID, Op: "anyOf", ValueIDs: vids}}}
	}

	cases := []struct {
		name     string
		group    *OptionConditionGroup
		ownOrder int32
		wantErr  string // substring; empty = valid
	}{
		{"valid anyOf on earlier select", anyOfParent(parent.Values[0].ID), 2, ""},
		{"valid filled on earlier text", &OptionConditionGroup{Rules: []OptionRule{{OptionID: other.ID, Op: "filled"}}}, 2, ""},
		{"valid nested or-group", &OptionConditionGroup{
			Logic:  "or",
			Groups: []OptionConditionGroup{{Rules: []OptionRule{{OptionID: parent.ID, Op: "empty"}}}},
		}, 2, ""},
		{"nil is always valid", nil, 0, ""},
		{"self reference", &OptionConditionGroup{Rules: []OptionRule{{OptionID: ownID, Op: "filled"}}}, 2, "cannot depend on itself"},
		{"unknown option", &OptionConditionGroup{Rules: []OptionRule{{OptionID: uuid.New(), Op: "filled"}}}, 2, "not on this product"},
		{"equal sort order rejected", anyOfParent(parent.Values[0].ID), 0, "ordered BEFORE"},
		{"forward reference rejected", &OptionConditionGroup{Rules: []OptionRule{{OptionID: other.ID, Op: "filled"}}}, 1, "ordered BEFORE"},
		{"foreign value id", anyOfParent(uuid.New()), 2, "not on option"},
		{"anyOf needs values", anyOfParent(), 2, "at least one value"},
		{"anyOf on text parent", &OptionConditionGroup{Rules: []OptionRule{{OptionID: other.ID, Op: "anyOf", ValueIDs: []uuid.UUID{uuid.New()}}}}, 2, "choice-type parent"},
		{"unknown op", &OptionConditionGroup{Rules: []OptionRule{{OptionID: parent.ID, Op: "equals"}}}, 2, "unknown op"},
		{"bad logic", &OptionConditionGroup{Logic: "xor", Rules: []OptionRule{{OptionID: parent.ID, Op: "filled"}}}, 2, "logic"},
		{"empty group", &OptionConditionGroup{}, 2, "at least one rule"},
		{"bad nested group bubbles", &OptionConditionGroup{
			Rules:  []OptionRule{{OptionID: parent.ID, Op: "filled"}},
			Groups: []OptionConditionGroup{{Rules: []OptionRule{{OptionID: ownID, Op: "filled"}}}},
		}, 2, "cannot depend on itself"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOptionVisibility(tc.group, tc.ownOrder, ownID, siblings)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("want valid, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestGroupReferencesOption(t *testing.T) {
	target := uuid.New()
	g := &OptionConditionGroup{
		Rules:  []OptionRule{{OptionID: uuid.New(), Op: "filled"}},
		Groups: []OptionConditionGroup{{Rules: []OptionRule{{OptionID: target, Op: "empty"}}}},
	}
	if !groupReferencesOption(g, target) {
		t.Error("nested reference not found")
	}
	if groupReferencesOption(g, uuid.New()) {
		t.Error("false positive")
	}
	if groupReferencesOption(nil, target) {
		t.Error("nil group should not match")
	}
}
