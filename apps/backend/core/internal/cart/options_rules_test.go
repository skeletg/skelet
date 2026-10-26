package cart

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

// What these pin: a fired rule requires, frees, fixes the quantity or moves the price, a hidden option stays exempt,
// and a persisted line re-reads its rules from its own picks

func whenPicked(opt ProductOptionConfig, v ProductOptionValueConfig) OptionVisibilityGroup {
	return OptionVisibilityGroup{Rules: []OptionVisibilityRule{{OptionID: opt.ID, Op: "anyOf", ValueIDs: []uuid.UUID{v.ID}}}}
}

func pick(opt ProductOptionConfig, v ProductOptionValueConfig) CartLineOptionInput {
	return CartLineOptionInput{OptionID: opt.ID, ValueIDs: []uuid.UUID{v.ID}}
}

func giftSetup() (ProductOptionConfig, ProductOptionValueConfig, ProductOptionValueConfig, ProductOptionConfig) {
	yes, no := val(0, 1), val(0, 1)
	gift := selOpt(false, "radio", yes, no)
	note := ProductOptionConfig{ID: uuid.New(), InputType: "area", Title: "Note"}
	return gift, yes, no, note
}

func TestRules_RequireOnlyWhenTheConditionHolds(t *testing.T) {
	gift, yes, no, note := giftSetup()
	r := res(1000, gift, note)
	r.Rules = []ProductOptionRuleConfig{{ID: uuid.New(), Condition: whenPicked(gift, yes), Actions: []OptionRuleActionConfig{{Type: "require", OptionID: note.ID}}}}

	if _, _, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{pick(gift, yes)}, nil); !errors.Is(err, ErrProductOptionMissing) {
		t.Fatalf("gift picked without a note: want ErrProductOptionMissing, got %v", err)
	}
	if _, _, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{pick(gift, no)}, nil); err != nil {
		t.Fatalf("no gift: the note stays optional, got %v", err)
	}
	if _, _, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{pick(gift, yes), {OptionID: note.ID, TextValue: "Happy birthday"}}, nil); err != nil {
		t.Fatalf("gift with a note: %v", err)
	}
}

func TestRules_OptionalFreesARequiredOptionButRequireWins(t *testing.T) {
	gift, yes, no, note := giftSetup()
	note.IsRequired = true
	r := res(1000, gift, note)
	r.Rules = []ProductOptionRuleConfig{{ID: uuid.New(), Condition: whenPicked(gift, no), Actions: []OptionRuleActionConfig{{Type: "optional", OptionID: note.ID}}}}
	if _, _, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{pick(gift, no)}, nil); err != nil {
		t.Fatalf("an optional rule should free the note: %v", err)
	}
	if _, _, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{pick(gift, yes)}, nil); !errors.Is(err, ErrProductOptionMissing) {
		t.Fatalf("the rule didn't fire, so the note is still required: got %v", err)
	}
	r.Rules = append(r.Rules, ProductOptionRuleConfig{ID: uuid.New(), Condition: whenPicked(gift, no), Actions: []OptionRuleActionConfig{{Type: "require", OptionID: note.ID}}})
	if _, _, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{pick(gift, no)}, nil); !errors.Is(err, ErrProductOptionMissing) {
		t.Fatalf("require beats optional: want ErrProductOptionMissing, got %v", err)
	}
}

func TestRules_AHiddenOptionIsNeverRequired(t *testing.T) {
	gift, yes, no, note := giftSetup()
	note.VisibleIf = &OptionVisibilityGroup{Rules: []OptionVisibilityRule{{OptionID: gift.ID, Op: "anyOf", ValueIDs: []uuid.UUID{yes.ID}}}}
	r := res(1000, gift, note)
	r.Rules = []ProductOptionRuleConfig{{ID: uuid.New(), Condition: whenPicked(gift, no), Actions: []OptionRuleActionConfig{{Type: "require", OptionID: note.ID}}}}
	if _, _, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{pick(gift, no)}, nil); err != nil {
		t.Fatalf("the note is hidden, so requiring it can't apply: %v", err)
	}
}

func TestRules_SetQuantityAndItsConflicts(t *testing.T) {
	single, pair := val(0, 1), val(0, 1)
	pack := selOpt(true, "radio", single, pair)
	r := res(1000, pack)
	r.Rules = []ProductOptionRuleConfig{{ID: uuid.New(), Condition: whenPicked(pack, pair), Actions: []OptionRuleActionConfig{{Type: "set_quantity", Quantity: 2}}}}
	_, _, forced, err := buildConfiguredLineOptions(r, []CartLineOptionInput{pick(pack, pair)}, nil)
	if err != nil || forced == nil || *forced != 2 {
		t.Fatalf("want quantity fixed at 2, got %v err %v", forced, err)
	}

	r.Rules = append(r.Rules, ProductOptionRuleConfig{ID: uuid.New(), Condition: whenPicked(pack, pair), Actions: []OptionRuleActionConfig{{Type: "set_quantity", Quantity: 3}}})
	if _, _, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{pick(pack, pair)}, nil); !errors.Is(err, ErrProductOptionQtyConflict) {
		t.Fatalf("two rules, two quantities: want ErrProductOptionQtyConflict, got %v", err)
	}

	forcing := fval(0, 4)
	pack2 := selOpt(true, "radio", forcing)
	r2 := res(1000, pack2)
	r2.Rules = []ProductOptionRuleConfig{{ID: uuid.New(), Condition: whenPicked(pack2, forcing), Actions: []OptionRuleActionConfig{{Type: "set_quantity", Quantity: 2}}}}
	if _, _, _, err := buildConfiguredLineOptions(r2, []CartLineOptionInput{pick(pack2, forcing)}, nil); !errors.Is(err, ErrProductOptionQtyConflict) {
		t.Fatalf("a rule against a choice's own quantity: want ErrProductOptionQtyConflict, got %v", err)
	}
}

func TestRules_AdjustPriceAddsALabelledRow(t *testing.T) {
	gift, yes, _, _ := giftSetup()
	r := res(2000, gift)
	rule := ProductOptionRuleConfig{ID: uuid.New(), Label: "Gift set", Condition: whenPicked(gift, yes), Actions: []OptionRuleActionConfig{
		{Type: "adjust_price", PriceType: "fixed", PriceValue: -500},
		{Type: "adjust_price", PriceType: "percent", PriceValue: 10},
	}}
	unnamed := ProductOptionRuleConfig{ID: uuid.New(), Condition: whenPicked(gift, yes), Actions: []OptionRuleActionConfig{{Type: "adjust_price", PriceType: "fixed", PriceValue: 50}}}
	r.Rules = []ProductOptionRuleConfig{rule, unnamed}

	rows, delta, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{pick(gift, yes)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if delta != -500+200+50 {
		t.Fatalf("delta = %d, want -250", delta)
	}
	if len(rows) != 3 {
		t.Fatalf("want the pick plus a row per priced rule, got %+v", rows)
	}
	if got := rows[1]; got.InputType != OptionRuleInputType || got.ProductOptionID != rule.ID || got.OptionTitle != "Gift set" || got.PriceDelta != -300 {
		t.Fatalf("named rule row = %+v", got)
	}
	if rows[2].OptionTitle != "Adjustment" {
		t.Fatalf("an unnamed rule's row should read Adjustment, got %q", rows[2].OptionTitle)
	}

	r.Rules = []ProductOptionRuleConfig{{ID: uuid.New(), Condition: whenPicked(gift, yes), Actions: []OptionRuleActionConfig{{Type: "adjust_price", PriceType: "fixed", PriceValue: 300, PriceUnavailable: true}}}}
	if _, _, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{pick(gift, yes)}, nil); !errors.Is(err, ErrOptionPriceUnavailable) {
		t.Fatalf("a rule with no amount in this currency: want ErrOptionPriceUnavailable, got %v", err)
	}
}

func TestRules_APlainAddIsRefusedWhenAnEmptyAnswerFiresARule(t *testing.T) {
	gift, _, _, note := giftSetup()
	r := res(1000, gift, note)
	if HasVisiblyRequiredOptions(r) {
		t.Fatal("nothing required and no rules - a plain add is fine")
	}
	r.Rules = []ProductOptionRuleConfig{{ID: uuid.New(), Condition: OptionVisibilityGroup{Rules: []OptionVisibilityRule{{OptionID: gift.ID, Op: "empty"}}}, Actions: []OptionRuleActionConfig{{Type: "require", OptionID: note.ID}}}}
	if !HasVisiblyRequiredOptions(r) {
		t.Fatal("a rule requiring the note on no answers must refuse the plain add")
	}
	r.Rules[0].Actions = []OptionRuleActionConfig{{Type: "adjust_price", PriceType: "fixed", PriceValue: 100}}
	if !HasVisiblyRequiredOptions(r) {
		t.Fatal("a rule pricing the empty answer must refuse the plain add, which can't charge it")
	}
}

func TestRules_APersistedLineReReadsItsQuantityRule(t *testing.T) {
	single, pair := val(0, 1), val(0, 1)
	pack := selOpt(true, "radio", single, pair)
	r := res(1000, pack)
	rule := ProductOptionRuleConfig{ID: uuid.New(), Condition: whenPicked(pack, pair), Actions: []OptionRuleActionConfig{{Type: "set_quantity", Quantity: 2}}}
	r.Rules = []ProductOptionRuleConfig{rule}
	snapshot := []CartLineOption{
		{ProductOptionID: pack.ID, OptionValueID: &pair.ID, InputType: "radio"},
		{ProductOptionID: uuid.New(), InputType: OptionRuleInputType, OptionTitle: "Pair deal", PriceDelta: -100},
	}
	if q := forcedQtyForSnapshot(r, snapshot); q == nil || *q != 2 {
		t.Fatalf("want the line locked at 2, got %v", q)
	}
	snapshot[0].OptionValueID = &single.ID
	if q := forcedQtyForSnapshot(r, snapshot); q != nil {
		t.Fatalf("a single pick fires no rule, got %v", *q)
	}
}
