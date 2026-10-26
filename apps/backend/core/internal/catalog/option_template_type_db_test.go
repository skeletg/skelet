package catalog

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
)

// What these pin: a product type's templates reach every simple and configurable product of the type, ahead of the
// templates a product picks itself, never another kind, and a template on both shows once; the type's list can't drop
// a template a product of it still reads unless that product picked it too; a template a type uses can't be deleted,
// detached on one product, or have a choice pulled out from under a product of the type; and every product of the
// type hears about the change

func (r structureRig) templateOption(templateID uuid.UUID, title string) (ProductOption, ProductOptionValue) {
	r.t.Helper()
	ctx := context.Background()
	o, err := r.svc.AddProductOption(ctx, AddProductOptionInput{TemplateID: templateID, InputType: OptionInputRadio, Locale: "en", Title: title})
	if err != nil {
		r.t.Fatalf("template option: %v", err)
	}
	v, err := r.svc.AddProductOptionValue(ctx, AddProductOptionValueInput{OptionID: o.ID, Locale: "en", Title: title + " yes", PriceType: OptionPriceFixed, PriceValue: 200, Quantity: 1})
	if err != nil {
		r.t.Fatalf("template choice: %v", err)
	}
	return o, v
}

func optionIDs(opts []ProductOption) []uuid.UUID {
	out := make([]uuid.UUID, len(opts))
	for i, o := range opts {
		out[i] = o.ID
	}
	return out
}

func templateNames(ts []OptionTemplate) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Name
	}
	return out
}

func TestAProductTypesTemplatesReachEveryProductOfIt(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	typeTpl, picked := r.template("Care plan"), r.template("Engraving")
	care, _ := r.templateOption(typeTpl.ID, "Care")
	engrave, _ := r.templateOption(picked.ID, "Engrave")

	tee, shirt, set := r.product(KindSimple, "tee"), r.product(KindConfigurable, "shirt"), r.product(KindBundle, "set")
	elsewhere := r.newProduct(r.ptype("", false).ID, KindSimple, "other-"+uuid.NewString()[:6]+"-"+r.sfx, "other", "USD")
	own := r.dropdown(tee.ID, false)
	if err := r.svc.SetProductOptionTemplates(ctx, tee.ID, []uuid.UUID{picked.ID}); err != nil {
		t.Fatal(err)
	}

	r.reset() // the rig records product notices already - start from this save's
	if err := r.svc.SetProductTypeOptionTemplates(ctx, r.pt.ID, []uuid.UUID{typeTpl.ID}); err != nil {
		t.Fatalf("set the type's templates: %v", err)
	}
	var told []uuid.UUID
	for _, e := range eventsOf[*ProductUpdated](r.published()) {
		told = append(told, e.ProductID)
	}
	if !slices.Contains(told, tee.ID) || !slices.Contains(told, shirt.ID) || slices.Contains(told, set.ID) || slices.Contains(told, elsewhere.ID) {
		t.Errorf("told %v, want the tee and the shirt - not the bundle, not another type's product", told)
	}

	opts, err := r.svc.GetProductOptions(ctx, tee.ID, "en")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := optionIDs(opts), []uuid.UUID{care.ID, engrave.ID, own.ID}; !slices.Equal(got, want) {
		t.Fatalf("tee options = %v, want the type's, then its own pick's, then its own", got)
	}
	if opts[0].TemplateName != typeTpl.Name || opts[0].SortOrder >= opts[1].SortOrder || opts[1].SortOrder >= opts[2].SortOrder {
		t.Fatalf("tee options = %+v, want the type's first by sort_order too", opts)
	}
	if got, _ := r.svc.GetProductOptions(ctx, shirt.ID, "en"); !slices.Equal(optionIDs(got), []uuid.UUID{care.ID}) {
		t.Errorf("configurable options = %v, want the type's", optionIDs(got))
	}
	for name, p := range map[string]Product{"a bundle of the type": set, "a product of another type": elsewhere} {
		if got, _ := r.svc.GetProductOptions(ctx, p.ID, "en"); len(got) != 0 {
			t.Errorf("%s got %v", name, optionIDs(got))
		}
	}

	if got, _ := r.svc.GetInheritedOptionTemplates(ctx, tee.ID); !slices.Equal(templateNames(got), []string{typeTpl.Name}) {
		t.Errorf("tee's inherited templates = %v", templateNames(got))
	}
	if got, _ := r.svc.GetProductOptionTemplates(ctx, tee.ID); !slices.Equal(templateNames(got), []string{picked.Name}) {
		t.Errorf("tee's own picks = %v, want only what it picked", templateNames(got))
	}
	if got, _ := r.svc.GetInheritedOptionTemplates(ctx, set.ID); len(got) != 0 {
		t.Errorf("the bundle inherits %v", templateNames(got))
	}
	if got, _ := r.svc.GetProductTypeOptionTemplates(ctx, r.pt.ID); !slices.Equal(templateNames(got), []string{typeTpl.Name}) {
		t.Errorf("the type's templates = %v", templateNames(got))
	}
	if ids, _ := r.svc.ListProductTypeIDsUsingOptionTemplate(ctx, typeTpl.ID); !slices.Equal(ids, []uuid.UUID{r.pt.ID}) {
		t.Errorf("types using the template = %v", ids)
	}
	if got, _ := r.svc.GetOptionTemplate(ctx, typeTpl.ID); got.ProductTypeCount != 1 || got.ProductCount != 0 {
		t.Errorf("counts = %d types, %d products, want 1 and 0 - the type's products didn't pick it", got.ProductTypeCount, got.ProductCount)
	}

	// Picked on the product as well as the type, it still shows once, where the type puts it
	if err := r.svc.SetProductOptionTemplates(ctx, tee.ID, []uuid.UUID{picked.ID, typeTpl.ID}); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.svc.GetProductOptions(ctx, tee.ID, "en"); !slices.Equal(optionIDs(got), []uuid.UUID{care.ID, engrave.ID, own.ID}) {
		t.Errorf("with the type's template picked too = %v, want it once, first", optionIDs(got))
	}

	rules, err := r.svc.SetOptionTemplateRules(ctx, typeTpl.ID, []ProductOptionRuleInput{{
		Condition: OptionConditionGroup{Rules: []OptionRule{{OptionID: care.ID, Op: OptionRuleFilled}}},
		Actions:   []ProductOptionRuleAction{{Type: RuleActionAdjustPrice, PriceType: OptionPriceFixed, PriceValue: 50}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := r.svc.GetProductOptionRules(ctx, shirt.ID, "en"); len(got) != 1 || got[0].ID != rules[0].ID {
		t.Errorf("the configurable's rules = %+v, want the type template's", got)
	}
}

func TestATypesTemplateGoesOnlyWhenNothingLeansOnIt(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	tpl := r.template("Gift wrap")
	wrap, gift := r.templateOption(tpl.ID, "Wrap")
	leaner, bystander := r.product(KindSimple, "mug"), r.product(KindSimple, "cup")
	if err := r.svc.SetProductTypeOptionTemplates(ctx, r.pt.ID, []uuid.UUID{tpl.ID}); err != nil {
		t.Fatal(err)
	}
	whenGift := OptionConditionGroup{Rules: []OptionRule{{OptionID: wrap.ID, Op: OptionRuleAnyOf, ValueIDs: []uuid.UUID{gift.ID}}}}
	if _, err := r.svc.AddProductOption(ctx, AddProductOptionInput{ProductID: leaner.ID, InputType: OptionInputArea, Locale: "en", Title: "Card", VisibleIf: &whenGift}); err != nil {
		t.Fatalf("an own option reading the type's template: %v", err)
	}

	wantErr(t, "dropping it from the type while a product's option reads it", r.svc.SetProductTypeOptionTemplates(ctx, r.pt.ID, nil), ErrOptionInUse)
	wantErr(t, "deleting the choice a product of the type reads", r.svc.DeleteProductOptionValue(ctx, gift.ID), ErrOptionInUse)
	wantErr(t, "deleting a template a type uses", r.svc.DeleteOptionTemplate(ctx, tpl.ID), ErrOptionTemplateInUse)
	wantErr(t, "detaching the type's template on one product", r.svc.DetachOptionTemplate(ctx, leaner.ID, tpl.ID), ErrOptionTemplateInvalid)

	// Picked on the product too, the type can let it go - the product keeps it, the bystander loses it
	if err := r.svc.SetProductOptionTemplates(ctx, leaner.ID, []uuid.UUID{tpl.ID}); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.SetProductTypeOptionTemplates(ctx, r.pt.ID, nil); err != nil {
		t.Fatalf("dropping it once the leaning product picked it itself: %v", err)
	}
	if got, _ := r.svc.GetProductOptions(ctx, leaner.ID, "en"); len(got) != 2 || got[0].ID != wrap.ID {
		t.Errorf("the product that picked it = %v, want it kept", optionIDs(got))
	}
	if got, _ := r.svc.GetProductOptions(ctx, bystander.ID, "en"); len(got) != 0 {
		t.Errorf("the bystander still shows %v", optionIDs(got))
	}

	// And the other way: on the type again, the product's own pick can go though its option reads it
	if err := r.svc.SetProductTypeOptionTemplates(ctx, r.pt.ID, []uuid.UUID{tpl.ID}); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.UnassignOptionTemplate(ctx, tpl.ID, []uuid.UUID{leaner.ID}); err != nil {
		t.Fatalf("unpicking a template the type still hands over: %v", err)
	}
	if err := r.svc.SetProductOptionTemplates(ctx, leaner.ID, []uuid.UUID{tpl.ID}); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.SetProductOptionTemplates(ctx, leaner.ID, nil); err != nil {
		t.Fatalf("replacing the picks while the type still hands it over: %v", err)
	}
	if got, _ := r.svc.GetProductOptions(ctx, leaner.ID, "en"); len(got) != 2 {
		t.Errorf("after unpicking = %v, want the type's template still there", optionIDs(got))
	}
}

func TestATypesTemplateListIsChecked(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	tpl := r.template("Warranty")
	wantErr(t, "a template listed twice", r.svc.SetProductTypeOptionTemplates(ctx, r.pt.ID, []uuid.UUID{tpl.ID, tpl.ID}), ErrOptionTemplateInvalid)
	wantErr(t, "a template that doesn't exist", r.svc.SetProductTypeOptionTemplates(ctx, r.pt.ID, []uuid.UUID{uuid.New()}), ErrOptionTemplateNotFound)
	wantErr(t, "a product type that doesn't exist", r.svc.SetProductTypeOptionTemplates(ctx, uuid.New(), []uuid.UUID{tpl.ID}), ErrProductTypeNotFound)
	many := make([]uuid.UUID, MaxOptionTemplatesPerList+1)
	for i := range many {
		many[i] = r.template("Many").ID
	}
	wantErr(t, "more templates than a list takes", r.svc.SetProductTypeOptionTemplates(ctx, r.pt.ID, many), ErrOptionTemplateInvalid)

	second := r.template("Insurance")
	if err := r.svc.SetProductTypeOptionTemplates(ctx, r.pt.ID, []uuid.UUID{second.ID, tpl.ID}); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.svc.GetProductTypeOptionTemplates(ctx, r.pt.ID); !slices.Equal(templateNames(got), []string{second.Name, tpl.Name}) {
		t.Errorf("the type's order = %v, want it as given", templateNames(got))
	}
	if err := r.svc.SetProductTypeOptionTemplates(ctx, r.pt.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.DeleteOptionTemplate(ctx, tpl.ID); err != nil {
		t.Errorf("deleting a template no type or product uses any more: %v", err)
	}
}

// An edit to a template reaches the pages of products that only get it through their type, so those have to hear
func TestATemplateEditTellsTheProductsItsTypeHandsItTo(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	tpl := r.template("Care plan")
	care, _ := r.templateOption(tpl.ID, "Care")
	mug, set := r.product(KindSimple, "mug"), r.product(KindBundle, "set")
	if err := r.svc.SetProductTypeOptionTemplates(ctx, r.pt.ID, []uuid.UUID{tpl.ID}); err != nil {
		t.Fatal(err)
	}
	r.reset() // the rig records product notices already - start from this edit's
	required := true
	if _, err := r.svc.UpdateProductOption(ctx, UpdateProductOptionInput{ID: care.ID, IsRequired: &required}); err != nil {
		t.Fatal(err)
	}
	var told []uuid.UUID
	for _, e := range eventsOf[*ProductUpdated](r.published()) {
		told = append(told, e.ProductID)
		// announced off one batched read, each still names its product the way a lookup each did
		if e.ProductID == mug.ID && (e.SKU != mug.SKU || e.ProductTypeID != r.pt.ID || !slices.Equal(e.ChangedFields, []string{ProductFieldOptions})) {
			t.Errorf("the mug's notice = %+v, want its SKU, type and the options field", e)
		}
	}
	if !slices.Contains(told, mug.ID) || slices.Contains(told, set.ID) {
		t.Errorf("told %v, want the mug the type hands it to - not the bundle", told)
	}
}

// Saving a type's list as it stands touches nothing - with thousands of products it would otherwise tell them all
func TestTheSameTypeTemplateListTellsNoOne(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	first, second := r.template("Care plan"), r.template("Gift wrap")
	r.product(KindSimple, "mug")
	if err := r.svc.SetProductTypeOptionTemplates(ctx, r.pt.ID, []uuid.UUID{first.ID, second.ID}); err != nil {
		t.Fatal(err)
	}
	r.reset() // notices land async - let the first save's arrive before counting the next
	if err := r.svc.SetProductTypeOptionTemplates(ctx, r.pt.ID, []uuid.UUID{first.ID, second.ID}); err != nil {
		t.Fatal(err)
	}
	if got := eventsOf[*ProductUpdated](r.published()); len(got) != 0 {
		t.Errorf("an unchanged list told %d products", len(got))
	}
	// a new order is a change - the options show in it
	r.reset()
	if err := r.svc.SetProductTypeOptionTemplates(ctx, r.pt.ID, []uuid.UUID{second.ID, first.ID}); err != nil {
		t.Fatal(err)
	}
	if got := eventsOf[*ProductUpdated](r.published()); len(got) != 1 {
		t.Errorf("a reorder told %d products, want the one", len(got))
	}
}

// The service checks both sides exist, but either can go before the insert - the error names the one that did
func TestATypeTemplateWriteNamesWhatWentMissing(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	tpl := r.template("Warranty")
	wantErr(t, "a template gone before the insert", r.repo.ReplaceProductTypeOptionTemplates(ctx, r.pt.ID, []uuid.UUID{uuid.New()}), ErrOptionTemplateNotFound)
	wantErr(t, "a type gone before the insert", r.repo.ReplaceProductTypeOptionTemplates(ctx, uuid.New(), []uuid.UUID{tpl.ID}), ErrProductTypeNotFound)
}

// A type's template links go with the type, so a template only a deleted type used can be deleted after
func TestDeletingAProductTypeFreesItsTemplates(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	tpl := r.template("Warranty")
	empty := r.ptype("", false)
	if err := r.svc.SetProductTypeOptionTemplates(ctx, empty.ID, []uuid.UUID{tpl.ID}); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.DeleteProductType(ctx, empty.ID); err != nil {
		t.Fatalf("deleting a product type that hands out a template: %v", err)
	}
	if got, _ := r.svc.GetOptionTemplate(ctx, tpl.ID); got.ProductTypeCount != 0 {
		t.Errorf("the template still counts %d types", got.ProductTypeCount)
	}
	if err := r.svc.DeleteOptionTemplate(ctx, tpl.ID); err != nil {
		t.Errorf("deleting the template its gone type used: %v", err)
	}
}
