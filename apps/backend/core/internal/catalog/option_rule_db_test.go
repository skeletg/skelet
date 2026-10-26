package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// What these pin: a save keeps the rules it names, adds the new and drops the rest, refuses what would point at
// nothing, and an option or choice a rule uses can't be deleted from under it

func TestOptionRulesReplaceKeepingIdentity(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	p := r.product(KindSimple, "rules")
	gift := r.dropdown(p.ID, false)
	yes, err := r.svc.AddProductOptionValue(ctx, AddProductOptionValueInput{OptionID: gift.ID, Locale: "en", Title: "Yes", PriceType: OptionPriceFixed, Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	note, err := r.svc.AddProductOption(ctx, AddProductOptionInput{ProductID: p.ID, InputType: OptionInputArea, Locale: "en", Title: "Note", SortOrder: 1})
	if err != nil {
		t.Fatal(err)
	}
	whenYes := OptionConditionGroup{Rules: []OptionRule{{OptionID: gift.ID, Op: OptionRuleAnyOf, ValueIDs: []uuid.UUID{yes.ID}}}}
	qty := 2.0

	saved, err := r.svc.SetProductOptionRules(ctx, p.ID, []ProductOptionRuleInput{
		{Labels: map[string]string{"en": " Gift set ", "de": ""}, Condition: whenYes, Actions: []ProductOptionRuleAction{
			{Type: RuleActionRequire, OptionID: &note.ID},
			{Type: RuleActionAdjustPrice, PriceType: OptionPriceFixed, PriceValue: -500},
		}},
		{Condition: whenYes, Actions: []ProductOptionRuleAction{{Type: RuleActionSetQuantity, Quantity: &qty}}},
	})
	if err != nil {
		t.Fatalf("save rules: %v", err)
	}
	if len(saved) != 2 || saved[0].Label != "Gift set" || len(saved[0].Labels) != 1 || saved[0].Currency != "USD" {
		t.Fatalf("saved = %+v, want two rules, the first labelled Gift set with blank labels dropped", saved)
	}
	first := saved[0].ID

	again, err := r.svc.SetProductOptionRules(ctx, p.ID, []ProductOptionRuleInput{
		{ID: &first, Labels: map[string]string{"en": "Gift set"}, Condition: whenYes, Actions: []ProductOptionRuleAction{{Type: RuleActionOptional, OptionID: &note.ID}}},
	})
	if err != nil {
		t.Fatalf("resave: %v", err)
	}
	if len(again) != 1 || again[0].ID != first || again[0].Actions[0].Type != RuleActionOptional {
		t.Fatalf("resave = %+v, want the first rule kept by id and edited, the second gone", again)
	}

	_, err = r.svc.SetProductOptionRules(ctx, p.ID, []ProductOptionRuleInput{{ID: ptrUUID(uuid.New()), Condition: whenYes, Actions: []ProductOptionRuleAction{{Type: RuleActionRequire, OptionID: &note.ID}}}})
	wantErr(t, "an id from nowhere", err, ErrOptionRuleInvalid)

	err = r.svc.DeleteProductOption(ctx, note.ID)
	wantErr(t, "delete the option a rule targets", err, ErrOptionInUse)
	err = r.svc.DeleteProductOptionValue(ctx, yes.ID)
	wantErr(t, "delete the choice a rule reads", err, ErrOptionInUse)

	if _, err := r.svc.SetProductOptionRules(ctx, p.ID, nil); err != nil {
		t.Fatalf("clear rules: %v", err)
	}
	if err := r.svc.DeleteProductOption(ctx, note.ID); err != nil {
		t.Fatalf("with the rules gone the option deletes: %v", err)
	}
}

func TestOptionRulesRefuseWhatPointsAtNothing(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	p := r.product(KindSimple, "rules")
	other := r.product(KindSimple, "elsewhere")
	gift := r.dropdown(p.ID, false)
	foreign := r.dropdown(other.ID, false)
	filled := OptionConditionGroup{Rules: []OptionRule{{OptionID: gift.ID, Op: OptionRuleFilled}}}
	zero, half := 0.0, 1.5

	cases := map[string]ProductOptionRuleInput{
		"no condition":           {Actions: []ProductOptionRuleAction{{Type: RuleActionRequire, OptionID: &gift.ID}}},
		"no actions":             {Condition: filled},
		"condition elsewhere":    {Condition: OptionConditionGroup{Rules: []OptionRule{{OptionID: foreign.ID, Op: OptionRuleFilled}}}, Actions: []ProductOptionRuleAction{{Type: RuleActionRequire, OptionID: &gift.ID}}},
		"target elsewhere":       {Condition: filled, Actions: []ProductOptionRuleAction{{Type: RuleActionRequire, OptionID: &foreign.ID}}},
		"quantity zero":          {Condition: filled, Actions: []ProductOptionRuleAction{{Type: RuleActionSetQuantity, Quantity: &zero}}},
		"quantity fractional":    {Condition: filled, Actions: []ProductOptionRuleAction{{Type: RuleActionSetQuantity, Quantity: &half}}},
		"percent out of range":   {Condition: filled, Actions: []ProductOptionRuleAction{{Type: RuleActionAdjustPrice, PriceType: OptionPricePercent, PriceValue: -150}}},
		"store price on percent": {Condition: filled, Actions: []ProductOptionRuleAction{{Type: RuleActionAdjustPrice, PriceType: OptionPricePercent, PriceValue: 5, ChannelPrices: []RuleChannelPrice{{ChannelID: uuid.New(), PriceValue: 1}}}}},
		"store it isn't sold on": {Condition: filled, Actions: []ProductOptionRuleAction{{Type: RuleActionAdjustPrice, PriceType: OptionPriceFixed, PriceValue: 5, ChannelPrices: []RuleChannelPrice{{ChannelID: uuid.New(), PriceValue: 1}}}}},
		"unknown action":         {Condition: filled, Actions: []ProductOptionRuleAction{{Type: "hide"}}},
		"anyOf without a choice": {Condition: OptionConditionGroup{Rules: []OptionRule{{OptionID: gift.ID, Op: OptionRuleAnyOf}}}, Actions: []ProductOptionRuleAction{{Type: RuleActionRequire, OptionID: &gift.ID}}},
	}
	for name, in := range cases {
		_, err := r.svc.SetProductOptionRules(ctx, p.ID, []ProductOptionRuleInput{in})
		if !errors.Is(err, ErrOptionRuleInvalid) {
			t.Errorf("%s: got %v, want ErrOptionRuleInvalid", name, err)
		}
	}
	rules, err := r.svc.GetProductOptionRules(ctx, p.ID, "en")
	if err != nil || len(rules) != 0 {
		t.Fatalf("refused saves must store nothing, got %v %v", rules, err)
	}
}

func ptrUUID(id uuid.UUID) *uuid.UUID { return &id }
