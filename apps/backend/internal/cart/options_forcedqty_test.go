package cart

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func fval(price int32, forced float64) ProductOptionValueConfig {
	v := val(price, 1)
	v.ForcedLineQty = &forced
	return v
}

func TestForcedQty_PickedValueForces(t *testing.T) {
	single := val(0, 1)
	setOf4 := fval(0, 4)
	pack := ProductOptionConfig{ID: uuid.New(), InputType: "radio", Title: "Pack", Values: []ProductOptionValueConfig{single, setOf4}}
	r := res(1000, pack)

	_, _, forced, err := buildConfiguredLineOptions(r, []CartLineOptionInput{{OptionID: pack.ID, ValueIDs: []uuid.UUID{setOf4.ID}}}, nil)
	if err != nil || forced == nil || *forced != 4 {
		t.Fatalf("want forced 4, got %v err %v", forced, err)
	}
	_, _, forced, err = buildConfiguredLineOptions(r, []CartLineOptionInput{{OptionID: pack.ID, ValueIDs: []uuid.UUID{single.ID}}}, nil)
	if err != nil || forced != nil {
		t.Fatalf("plain value must not force, got %v err %v", forced, err)
	}
}

func TestForcedQty_ConflictRejected(t *testing.T) {
	a := fval(0, 2)
	b := fval(0, 4)
	multi := ProductOptionConfig{ID: uuid.New(), InputType: "multiselect", Title: "Bundles", Values: []ProductOptionValueConfig{a, b}}
	r := res(1000, multi)
	_, _, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{{OptionID: multi.ID, ValueIDs: []uuid.UUID{a.ID, b.ID}}}, nil)
	if !errors.Is(err, ErrProductOptionQtyConflict) {
		t.Fatalf("want PRODUCT_OPTION_QTY_CONFLICT, got %v", err)
	}
	// Two forcers that AGREE are fine
	c := fval(0, 4)
	multi2 := ProductOptionConfig{ID: uuid.New(), InputType: "multiselect", Title: "Bundles", Values: []ProductOptionValueConfig{b, c}}
	r2 := res(1000, multi2)
	_, _, forced, err := buildConfiguredLineOptions(r2, []CartLineOptionInput{{OptionID: multi2.ID, ValueIDs: []uuid.UUID{b.ID, c.ID}}}, nil)
	if err != nil || forced == nil || *forced != 4 {
		t.Fatalf("agreeing forcers should pass with 4, got %v err %v", forced, err)
	}
}

func TestForcedQty_HiddenOptionsForcerIgnored(t *testing.T) {
	// The forcing value sits on an option hidden by the current picks; an
	// empty husk for it is tolerated and its ForcedLineQty must not leak
	vNo := val(0, 1)
	vYes := val(0, 1)
	gate := ProductOptionConfig{ID: uuid.New(), InputType: "radio", Title: "Gate", Values: []ProductOptionValueConfig{vNo, vYes}}
	forcedVal := fval(0, 6)
	pack := ProductOptionConfig{
		ID: uuid.New(), InputType: "radio", Title: "Pack", Values: []ProductOptionValueConfig{forcedVal},
		VisibleIf: &OptionVisibilityGroup{Rules: []OptionVisibilityRule{{OptionID: gate.ID, Op: "anyOf", ValueIDs: []uuid.UUID{vYes.ID}}}},
	}
	r := res(1000, gate, pack)
	_, _, forced, err := buildConfiguredLineOptions(r, []CartLineOptionInput{{OptionID: gate.ID, ValueIDs: []uuid.UUID{vNo.ID}}}, nil)
	if err != nil || forced != nil {
		t.Fatalf("hidden option must not force qty, got %v err %v", forced, err)
	}
}

func TestForcedQty_SnapshotResolution(t *testing.T) {
	setOf4 := fval(0, 4)
	pack := ProductOptionConfig{ID: uuid.New(), InputType: "radio", Title: "Pack", Values: []ProductOptionValueConfig{setOf4}}
	r := res(1000, pack)
	vid := setOf4.ID
	snap := []CartLineOption{{ProductOptionID: pack.ID, OptionValueID: &vid}}
	if q := forcedQtyForSnapshot(r, snap); q == nil || *q != 4 {
		t.Fatalf("snapshot should resolve forced 4, got %v", q)
	}
	other := uuid.New()
	if q := forcedQtyForSnapshot(r, []CartLineOption{{ProductOptionID: pack.ID, OptionValueID: &other}}); q != nil {
		t.Fatalf("unknown value must not force, got %v", q)
	}
}
