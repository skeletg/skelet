package catalog

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// What these pin: a product shows its templates' options and rules first, in template currency, and its own may read
// them; nothing that leans on a template's option lets it go quietly; detaching hands the product copies its own
// options and rules now point at; a template still in use can't be deleted

func (r structureRig) template(name string) OptionTemplate {
	r.t.Helper()
	t, err := r.svc.CreateOptionTemplate(context.Background(), OptionTemplateInput{Name: name + "-" + uuid.NewString()[:6], CurrencyCode: "usd"})
	if err != nil {
		r.t.Fatalf("create template: %v", err)
	}
	return t
}

func TestOptionTemplateShowsFirstAndOwnOptionsReadIt(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	tpl := r.template("Gift wrapping")
	if tpl.CurrencyCode != "USD" {
		t.Fatalf("currency = %q, want it upper-cased", tpl.CurrencyCode)
	}
	wrap, err := r.svc.AddProductOption(ctx, AddProductOptionInput{TemplateID: tpl.ID, InputType: OptionInputRadio, Locale: "en", Title: "Wrapping", SortOrder: 5})
	if err != nil {
		t.Fatal(err)
	}
	gift, err := r.svc.AddProductOptionValue(ctx, AddProductOptionValueInput{OptionID: wrap.ID, Locale: "en", Title: "Gift", PriceType: OptionPriceFixed, PriceValue: 300, Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	premium, err := r.svc.AddProductOptionValue(ctx, AddProductOptionValueInput{OptionID: wrap.ID, Locale: "en", Title: "Premium", PriceType: OptionPriceFixed, PriceValue: 900, Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	whenGift := OptionConditionGroup{Rules: []OptionRule{{OptionID: wrap.ID, Op: OptionRuleAnyOf, ValueIDs: []uuid.UUID{gift.ID}}}}
	whenPremium := OptionConditionGroup{Rules: []OptionRule{{OptionID: wrap.ID, Op: OptionRuleAnyOf, ValueIDs: []uuid.UUID{premium.ID}}}}
	if _, err := r.svc.SetOptionTemplateRules(ctx, tpl.ID, []ProductOptionRuleInput{{
		Labels: map[string]string{"en": "Gift fee"}, Condition: whenGift,
		Actions: []ProductOptionRuleAction{{Type: RuleActionAdjustPrice, PriceType: OptionPriceFixed, PriceValue: 100}},
	}}); err != nil {
		t.Fatalf("template rules: %v", err)
	}

	p := r.product(KindSimple, "tee")
	note, err := r.svc.AddProductOption(ctx, AddProductOptionInput{ProductID: p.ID, InputType: OptionInputArea, Locale: "en", Title: "Card note", VisibleIf: &whenGift})
	wantErr(t, "an own option reading a template the product doesn't use", err, ErrOptionVisibilityInvalid)

	if err := r.svc.SetProductOptionTemplates(ctx, p.ID, []uuid.UUID{tpl.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	note, err = r.svc.AddProductOption(ctx, AddProductOptionInput{ProductID: p.ID, InputType: OptionInputArea, Locale: "en", Title: "Card note", VisibleIf: &whenGift})
	if err != nil {
		t.Fatalf("own option shown for a template answer: %v", err)
	}
	if _, err := r.svc.SetProductOptionRules(ctx, p.ID, []ProductOptionRuleInput{{Condition: whenPremium, Actions: []ProductOptionRuleAction{{Type: RuleActionRequire, OptionID: &note.ID}}}}); err != nil {
		t.Fatalf("own rule reading a template answer: %v", err)
	}

	opts, err := r.svc.GetProductOptions(ctx, p.ID, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 2 || opts[0].ID != wrap.ID || opts[0].TemplateName != tpl.Name || opts[0].Currency != "USD" || opts[1].ID != note.ID {
		t.Fatalf("options = %+v, want the template's wrapping first, then the note", opts)
	}
	if opts[0].SortOrder >= opts[1].SortOrder {
		t.Fatalf("template option sorts at %d, own at %d - anything ordering by sort_order must see the template first", opts[0].SortOrder, opts[1].SortOrder)
	}
	rules, err := r.svc.GetProductOptionRules(ctx, p.ID, "en")
	if err != nil || len(rules) != 2 || rules[0].Label != "Gift fee" || rules[0].TemplateID != tpl.ID || rules[1].TemplateID != uuid.Nil {
		t.Fatalf("rules = %+v %v, want the template's first, then the product's", rules, err)
	}

	_, err = r.svc.SetOptionTemplateRules(ctx, tpl.ID, []ProductOptionRuleInput{{Condition: whenGift, Actions: []ProductOptionRuleAction{{Type: RuleActionRequire, OptionID: &note.ID}}}})
	wantErr(t, "a template rule changing a product's option", err, ErrOptionRuleInvalid)

	err = r.svc.SetProductOptionTemplates(ctx, p.ID, nil)
	wantErr(t, "dropping a template the product's options read", err, ErrOptionInUse)
	err = r.svc.UnassignOptionTemplate(ctx, tpl.ID, []uuid.UUID{p.ID})
	wantErr(t, "unassigning it from the template side", err, ErrOptionInUse)
	err = r.svc.DeleteProductOptionValue(ctx, premium.ID)
	wantErr(t, "deleting the template choice only a product's rule reads", err, ErrOptionInUse)
	err = r.svc.DeleteOptionTemplate(ctx, tpl.ID)
	wantErr(t, "deleting a template in use", err, ErrOptionTemplateInUse)

	bundle := r.product(KindBundle, "set")
	err = r.svc.AssignOptionTemplate(ctx, tpl.ID, []uuid.UUID{bundle.ID})
	wantErr(t, "assigning to a kind that never resolves options", err, ErrOptionInputKindNotPurchasable)
}

func TestDetachOptionTemplateCopiesAndRepoints(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	tpl := r.template("Engraving")
	style, err := r.svc.AddProductOption(ctx, AddProductOptionInput{TemplateID: tpl.ID, InputType: OptionInputDropdown, Locale: "en", Title: "Style"})
	if err != nil {
		t.Fatal(err)
	}
	script, err := r.svc.AddProductOptionValue(ctx, AddProductOptionValueInput{OptionID: style.ID, Locale: "en", Title: "Script", PriceType: OptionPricePercent, PriceValue: 5, Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	whenScript := OptionConditionGroup{Rules: []OptionRule{{OptionID: style.ID, Op: OptionRuleAnyOf, ValueIDs: []uuid.UUID{script.ID}}}}
	text, err := r.svc.AddProductOption(ctx, AddProductOptionInput{TemplateID: tpl.ID, InputType: OptionInputField, Locale: "en", Title: "Text", SortOrder: 1, VisibleIf: &whenScript})
	if err != nil {
		t.Fatalf("template option reading an earlier template option: %v", err)
	}
	if _, err := r.svc.SetOptionTemplateRules(ctx, tpl.ID, []ProductOptionRuleInput{{Condition: whenScript, Actions: []ProductOptionRuleAction{{Type: RuleActionRequire, OptionID: &text.ID}}}}); err != nil {
		t.Fatal(err)
	}

	p := r.product(KindSimple, "ring")
	if err := r.svc.AssignOptionTemplate(ctx, tpl.ID, []uuid.UUID{p.ID, p.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	gift, err := r.svc.AddProductOption(ctx, AddProductOptionInput{ProductID: p.ID, InputType: OptionInputCheckbox, Locale: "en", Title: "Gift box", VisibleIf: &whenScript})
	if err != nil {
		t.Fatal(err)
	}

	if err := r.svc.DetachOptionTemplate(ctx, p.ID, tpl.ID); err != nil {
		t.Fatalf("detach: %v", err)
	}
	opts, err := r.svc.GetProductOptions(ctx, p.ID, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 3 || opts[0].Title != "Style" || opts[1].Title != "Text" || opts[2].ID != gift.ID {
		t.Fatalf("options = %+v, want the copies first, in order, then the product's own", opts)
	}
	for _, o := range opts {
		if o.TemplateID != uuid.Nil || o.ProductID != p.ID {
			t.Fatalf("%q still belongs to %v / %v after detaching", o.Title, o.TemplateID, o.ProductID)
		}
	}
	styleCopy, textCopy := opts[0], opts[1]
	if len(styleCopy.Values) != 1 || styleCopy.Values[0].ID == script.ID {
		t.Fatalf("the style copy carries %+v, want a fresh Script", styleCopy.Values)
	}
	scriptCopy := styleCopy.Values[0].ID
	if !groupReferencesValue(textCopy.VisibleIf, scriptCopy) || !groupReferencesValue(opts[2].VisibleIf, scriptCopy) {
		t.Fatalf("conditions still read the template: text %+v, gift box %+v", textCopy.VisibleIf, opts[2].VisibleIf)
	}
	rules, err := r.svc.GetProductOptionRules(ctx, p.ID, "en")
	if err != nil || len(rules) != 1 || rules[0].TemplateID != uuid.Nil || !ruleTargets(rules[0], textCopy.ID) {
		t.Fatalf("rules = %+v %v, want the template's rule copied onto the product and pointed at the copies", rules, err)
	}
	if left, _ := r.svc.GetOptionTemplateOptions(ctx, tpl.ID, "en"); len(left) != 2 {
		t.Fatalf("the template itself keeps its options, got %d", len(left))
	}
	if err := r.svc.DeleteOptionTemplate(ctx, tpl.ID); err != nil {
		t.Fatalf("a template nobody uses deletes: %v", err)
	}
}

func TestOptionTemplateNamesAreUnique(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	name := "Unique-" + uuid.NewString()[:6]
	if _, err := r.svc.CreateOptionTemplate(ctx, OptionTemplateInput{Name: name, CurrencyCode: "EUR"}); err != nil {
		t.Fatal(err)
	}
	_, err := r.svc.CreateOptionTemplate(ctx, OptionTemplateInput{Name: "  " + name + "  ", CurrencyCode: "EUR"})
	wantErr(t, "the same name again", err, ErrOptionTemplateNameTaken)
	_, err = r.svc.CreateOptionTemplate(ctx, OptionTemplateInput{Name: "x", CurrencyCode: "euro"})
	wantErr(t, "a currency that isn't a code", err, ErrOptionTemplateInvalid)
}

func TestConfiguratorLayoutDefaultsToListAndTakesSteps(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	p := r.product(KindSimple, "desk")
	if got, err := r.svc.GetProductConfiguratorLayout(ctx, p.ID); err != nil || got != ConfiguratorList {
		t.Fatalf("new product layout = %q %v, want list", got, err)
	}
	if err := r.svc.SetProductConfiguratorLayout(ctx, p.ID, ConfiguratorSteps); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.svc.GetProductConfiguratorLayout(ctx, p.ID); got != ConfiguratorSteps {
		t.Fatalf("layout = %q, want steps", got)
	}
	wantErr(t, "a layout that isn't one", r.svc.SetProductConfiguratorLayout(ctx, p.ID, "wizard"), ErrOptionTemplateInvalid)
	wantErr(t, "a kind that never shows options", r.svc.SetProductConfiguratorLayout(ctx, r.product(KindBundle, "set").ID, ConfiguratorSteps), ErrOptionInputKindNotPurchasable)
}
