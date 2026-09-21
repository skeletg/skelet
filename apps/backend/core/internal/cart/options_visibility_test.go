package cart

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

// The canonical fixture - a required radio ("Gift wrap" No / Yes +300) then a required text option ("Gift message" +200) visible only when wrap = Yes, returned with the IDs needed to drive picks
func giftPair() (r ProductOptionsResolution, wrap ProductOptionConfig, yes, no uuid.UUID, msg ProductOptionConfig) {
	vNo := ProductOptionValueConfig{ID: uuid.New(), Title: "No", PriceValue: 0, PriceType: "fixed", Quantity: 1}
	vYes := ProductOptionValueConfig{ID: uuid.New(), Title: "Yes", PriceValue: 300, PriceType: "fixed", Quantity: 1}
	wrap = ProductOptionConfig{ID: uuid.New(), InputType: "radio", IsRequired: true, Title: "Gift wrap", Values: []ProductOptionValueConfig{vNo, vYes}}
	price := int32(200)
	msg = ProductOptionConfig{
		ID: uuid.New(), InputType: "field", IsRequired: true, Title: "Gift message",
		PriceValue: &price, PriceType: "fixed",
		VisibleIf: &OptionVisibilityGroup{Rules: []OptionVisibilityRule{{OptionID: wrap.ID, Op: "anyOf", ValueIDs: []uuid.UUID{vYes.ID}}}},
	}
	r = res(1000, wrap, msg)
	return r, wrap, vYes.ID, vNo.ID, msg
}

func TestVisibility_AnyOfShowsAndHides(t *testing.T) {
	r, wrap, yes, no, msg := giftPair()
	v := visibleOptionIDs(r, []CartLineOptionInput{{OptionID: wrap.ID, ValueIDs: []uuid.UUID{yes}}})
	if !v[wrap.ID] || !v[msg.ID] {
		t.Errorf("wrap=Yes should show both, got %v", v)
	}
	v = visibleOptionIDs(r, []CartLineOptionInput{{OptionID: wrap.ID, ValueIDs: []uuid.UUID{no}}})
	if !v[wrap.ID] || v[msg.ID] {
		t.Errorf("wrap=No should hide the message, got %v", v)
	}
	v = visibleOptionIDs(r, nil) // no pick at all on the parent: anyOf cannot match
	if v[msg.ID] {
		t.Error("unpicked parent should not satisfy anyOf")
	}
}

func TestVisibility_HiddenRequiredNotEnforced(t *testing.T) {
	r, wrap, _, no, _ := giftPair()
	rows, delta, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{{OptionID: wrap.ID, ValueIDs: []uuid.UUID{no}}}, nil)
	if err != nil {
		t.Fatalf("hidden required option must not be enforced: %v", err)
	}
	if delta != 0 {
		t.Errorf("delta = %d, want 0 (No carries no surcharge, message hidden)", delta)
	}
	if len(rows) != 1 {
		t.Errorf("want only the wrap row, got %+v", rows)
	}
}

func TestVisibility_VisibleRequiredStillEnforced(t *testing.T) {
	r, wrap, yes, _, _ := giftPair()
	_, _, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{{OptionID: wrap.ID, ValueIDs: []uuid.UUID{yes}}}, nil)
	if !errors.Is(err, ErrProductOptionMissing) {
		t.Errorf("wrap=Yes without message should be PRODUCT_OPTION_MISSING, got %v", err)
	}
}

func TestVisibility_VisiblePathPricesBothDeltas(t *testing.T) {
	r, wrap, yes, _, msg := giftPair()
	in := []CartLineOptionInput{
		{OptionID: wrap.ID, ValueIDs: []uuid.UUID{yes}},
		{OptionID: msg.ID, TextValue: "happy birthday"},
	}
	rows, delta, _, err := buildConfiguredLineOptions(r, in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delta != 500 {
		t.Errorf("delta = %d, want 500 (300 wrap + 200 message)", delta)
	}
	if len(rows) != 2 {
		t.Errorf("want 2 snapshot rows, got %+v", rows)
	}
}

func TestVisibility_PickOnHiddenOptionRejected(t *testing.T) {
	r, wrap, _, no, msg := giftPair()
	in := []CartLineOptionInput{
		{OptionID: wrap.ID, ValueIDs: []uuid.UUID{no}},
		{OptionID: msg.ID, TextValue: "sneaky"},
	}
	_, _, _, err := buildConfiguredLineOptions(r, in, nil)
	if !errors.Is(err, ErrProductOptionHidden) {
		t.Errorf("pick on hidden option should be PRODUCT_OPTION_HIDDEN, got %v", err)
	}
}

func TestVisibility_EmptyHuskOnHiddenOptionTolerated(t *testing.T) { // PDPs may post the full option list with empty values - an empty pick on a hidden option is not an attack and must not block the add
	r, wrap, _, no, msg := giftPair()
	in := []CartLineOptionInput{
		{OptionID: wrap.ID, ValueIDs: []uuid.UUID{no}},
		{OptionID: msg.ID, TextValue: "   "},
	}
	_, delta, _, err := buildConfiguredLineOptions(r, in, nil)
	if err != nil {
		t.Fatalf("empty pick on hidden option should be ignored: %v", err)
	}
	if delta != 0 {
		t.Errorf("delta = %d, want 0", delta)
	}
}

func TestVisibility_CascadeHiddenParentHidesChild(t *testing.T) {
	// A (radio yes/no) -> B (dropdown, visible when A=yes) -> C (text, visible when B filled) - with A=no a smuggled B pick is rejected outright, and even for pure evaluation C stays hidden because hidden options contribute no picks
	vNo := val(0, 1)
	vYes := val(0, 1)
	a := ProductOptionConfig{ID: uuid.New(), InputType: "radio", Title: "A", Values: []ProductOptionValueConfig{vNo, vYes}}
	bv := val(100, 1)
	b := ProductOptionConfig{
		ID: uuid.New(), InputType: "dropdown", Title: "B", Values: []ProductOptionValueConfig{bv},
		VisibleIf: &OptionVisibilityGroup{Rules: []OptionVisibilityRule{{OptionID: a.ID, Op: "anyOf", ValueIDs: []uuid.UUID{vYes.ID}}}},
	}
	c := ProductOptionConfig{
		ID: uuid.New(), InputType: "field", Title: "C",
		VisibleIf: &OptionVisibilityGroup{Rules: []OptionVisibilityRule{{OptionID: b.ID, Op: "filled"}}},
	}
	r := res(1000, a, b, c)

	in := []CartLineOptionInput{
		{OptionID: a.ID, ValueIDs: []uuid.UUID{vNo.ID}},
		{OptionID: b.ID, ValueIDs: []uuid.UUID{bv.ID}}, // smuggled
	}
	v := visibleOptionIDs(r, in)
	if v[b.ID] || v[c.ID] {
		t.Errorf("hidden B must not cascade C visible, got %v", v)
	}
	_, _, _, err := buildConfiguredLineOptions(r, in, nil)
	if !errors.Is(err, ErrProductOptionHidden) {
		t.Errorf("smuggled B pick should be PRODUCT_OPTION_HIDDEN, got %v", err)
	}

	in = []CartLineOptionInput{ // happy cascade: A=yes, B picked, C filled
		{OptionID: a.ID, ValueIDs: []uuid.UUID{vYes.ID}},
		{OptionID: b.ID, ValueIDs: []uuid.UUID{bv.ID}},
		{OptionID: c.ID, TextValue: "engrave me"},
	}
	v = visibleOptionIDs(r, in)
	if !v[a.ID] || !v[b.ID] || !v[c.ID] {
		t.Errorf("full cascade should be visible, got %v", v)
	}
	rows, delta, _, err := buildConfiguredLineOptions(r, in, nil)
	if err != nil || delta != 100 || len(rows) != 3 {
		t.Errorf("cascade add: rows=%d delta=%d err=%v, want 3/100/nil", len(rows), delta, err)
	}
}

func TestVisibility_NoneOfAndEmptyOps(t *testing.T) {
	vA := val(0, 1)
	vB := val(0, 1)
	parent := ProductOptionConfig{ID: uuid.New(), InputType: "dropdown", Title: "P", Values: []ProductOptionValueConfig{vA, vB}}
	notA := ProductOptionConfig{
		ID: uuid.New(), InputType: "field", Title: "notA",
		VisibleIf: &OptionVisibilityGroup{Rules: []OptionVisibilityRule{{OptionID: parent.ID, Op: "noneOf", ValueIDs: []uuid.UUID{vA.ID}}}},
	}
	whenEmpty := ProductOptionConfig{
		ID: uuid.New(), InputType: "field", Title: "whenEmpty",
		VisibleIf: &OptionVisibilityGroup{Rules: []OptionVisibilityRule{{OptionID: parent.ID, Op: "empty"}}},
	}
	r := res(1000, parent, notA, whenEmpty)

	v := visibleOptionIDs(r, []CartLineOptionInput{{OptionID: parent.ID, ValueIDs: []uuid.UUID{vA.ID}}})
	if v[notA.ID] || v[whenEmpty.ID] {
		t.Errorf("picked A: noneOf(A) and empty should both hide, got %v", v)
	}
	v = visibleOptionIDs(r, []CartLineOptionInput{{OptionID: parent.ID, ValueIDs: []uuid.UUID{vB.ID}}})
	if !v[notA.ID] || v[whenEmpty.ID] {
		t.Errorf("picked B: noneOf(A) shows, empty hides, got %v", v)
	}
	v = visibleOptionIDs(r, nil)
	if !v[notA.ID] || !v[whenEmpty.ID] {
		t.Errorf("no pick: noneOf and empty both show, got %v", v)
	}
}

func TestVisibility_AndOrLogic(t *testing.T) {
	vYes1 := val(0, 1)
	p1 := ProductOptionConfig{ID: uuid.New(), InputType: "radio", Title: "p1", Values: []ProductOptionValueConfig{vYes1}}
	p2 := ProductOptionConfig{ID: uuid.New(), InputType: "field", Title: "p2"}
	and := ProductOptionConfig{
		ID: uuid.New(), InputType: "field", Title: "and",
		VisibleIf: &OptionVisibilityGroup{Logic: "and", Rules: []OptionVisibilityRule{
			{OptionID: p1.ID, Op: "anyOf", ValueIDs: []uuid.UUID{vYes1.ID}},
			{OptionID: p2.ID, Op: "filled"},
		}},
	}
	or := ProductOptionConfig{
		ID: uuid.New(), InputType: "field", Title: "or",
		VisibleIf: &OptionVisibilityGroup{Logic: "or", Rules: []OptionVisibilityRule{
			{OptionID: p1.ID, Op: "anyOf", ValueIDs: []uuid.UUID{vYes1.ID}},
			{OptionID: p2.ID, Op: "filled"},
		}},
	}
	r := res(1000, p1, p2, and, or)

	in := []CartLineOptionInput{{OptionID: p1.ID, ValueIDs: []uuid.UUID{vYes1.ID}}} // only p1 satisfied
	v := visibleOptionIDs(r, in)
	if v[and.ID] || !v[or.ID] {
		t.Errorf("one of two: and hides, or shows; got and=%v or=%v", v[and.ID], v[or.ID])
	}
	in = append(in, CartLineOptionInput{OptionID: p2.ID, TextValue: "x"}) // both satisfied
	v = visibleOptionIDs(r, in)
	if !v[and.ID] || !v[or.ID] {
		t.Errorf("both: and+or show; got and=%v or=%v", v[and.ID], v[or.ID])
	}
}

func TestVisibility_UnknownOpFailsClosed(t *testing.T) {
	p := ProductOptionConfig{ID: uuid.New(), InputType: "field", Title: "p"}
	child := ProductOptionConfig{
		ID: uuid.New(), InputType: "field", Title: "child",
		VisibleIf: &OptionVisibilityGroup{Rules: []OptionVisibilityRule{{OptionID: p.ID, Op: "equalsMaybe"}}},
	}
	r := res(1000, p, child)
	if visibleOptionIDs(r, []CartLineOptionInput{{OptionID: p.ID, TextValue: "x"}})[child.ID] {
		t.Error("unknown op must hide, not show")
	}
}
