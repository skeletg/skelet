package analytics

import "testing"

func TestTraitRuleEmpty(t *testing.T) {
	if !(TraitRule{}).empty() {
		t.Error("zero rule should be empty")
	}
	if (TraitRule{Lifecycle: []string{"at_risk"}}).empty() {
		t.Error("lifecycle rule should not be empty")
	}
	if (TraitRule{MinMonetaryScore: 4}).empty() {
		t.Error("monetary rule should not be empty")
	}
}
