package catalog

import (
	"slices"
	"testing"

	"github.com/google/uuid"
)

// What this pins: a variant takes its configurable parent's options and rules only where it doesn't carry them already
// - a template on both, or on their shared product type, would otherwise be asked and charged twice - and the parent's
// lists are left as they were, since the loaders share them across a request

func TestAVariantTakesOnlyTheParentsOptionsAndRulesItLacks(t *testing.T) {
	shared, parentOnly, variantOnly := uuid.New(), uuid.New(), uuid.New()
	variant := []ProductOption{{ID: variantOnly}, {ID: shared}}
	parent := []ProductOption{{ID: shared}, {ID: parentOnly}}
	if got := ParentOptionsNotOn(variant, parent); len(got) != 1 || got[0].ID != parentOnly {
		t.Fatalf("parent options taken = %v, want only the one the variant lacks", got)
	}
	if parent[0].ID != shared || len(parent) != 2 {
		t.Fatalf("the parent's list changed under it: %v", parent)
	}

	sharedRule, parentRule := uuid.New(), uuid.New()
	vRules := []ProductOptionRule{{ID: sharedRule}}
	pRules := []ProductOptionRule{{ID: parentRule}, {ID: sharedRule}}
	got := ParentRulesNotOn(vRules, pRules)
	if ids := []uuid.UUID{got[0].ID}; len(got) != 1 || !slices.Equal(ids, []uuid.UUID{parentRule}) {
		t.Fatalf("parent rules taken = %v, want only the one the variant lacks", got)
	}
	if pRules[1].ID != sharedRule || len(pRules) != 2 {
		t.Fatalf("the parent's rules changed under it: %v", pRules)
	}
	if got := ParentOptionsNotOn(nil, parent); len(got) != 2 {
		t.Errorf("a variant with no options of its own takes %d of the parent's 2", len(got))
	}
}
