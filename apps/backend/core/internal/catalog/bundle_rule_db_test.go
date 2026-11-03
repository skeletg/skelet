package catalog

import (
	"context"
	"testing"
)

// What this pins: an option with a rule offers every simple product in the category on each read - after its hand-picked
// selections and without repeating them, under ids that stay put, picking up a product added later, and gone once cleared
func TestABundleOptionRuleFillsItselfFromACategory(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	bundle := r.product(KindBundle, "box")
	tea, mug, cup := r.product(KindSimple, "tea"), r.product(KindSimple, "mug"), r.product(KindSimple, "cup")
	cat := r.category(nil, "Drinkware "+r.sfx)
	for _, p := range []Product{mug, cup} {
		r.exec(`INSERT INTO catalog_product_categories (product_id, category_id) VALUES ($1, $2)`, p.ID, cat)
	}
	opt, err := r.svc.AddBundleOption(ctx, AddBundleOptionInput{ProductID: bundle.ID, InputType: BundleInputCheckbox, Locale: "en", Title: "Cups"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AddBundleSelection(ctx, AddBundleSelectionInput{OptionID: opt.ID, ProductID: mug.ID, Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.SetBundleOptionRule(ctx, opt.ID, &BundleOptionRule{CategoryID: cat, Quantity: 2}); err != nil {
		t.Fatal(err)
	}
	read := func() []BundleSelection {
		t.Helper()
		opts, err := r.svc.GetBundleOptions(ctx, bundle.ID, "en")
		if err != nil || len(opts) != 1 {
			t.Fatalf("options %+v (%v)", opts, err)
		}
		return opts[0].Selections
	}

	sels := read()
	if len(sels) != 2 || sels[0].ProductID != mug.ID || sels[0].FromRule || sels[1].ProductID != cup.ID || !sels[1].FromRule || sels[1].Quantity != 2 {
		t.Fatalf("selections %+v, want the hand-picked mug, then the rule's cup at 2 each", sels)
	}
	if sels[1].ID != RuleSelectionID(opt.ID, cup.ID) || sels[1].ProductSKU != cup.SKU {
		t.Fatalf("rule selection %+v, want the stable id and the cup's display fields", sels[1])
	}

	r.exec(`INSERT INTO catalog_product_categories (product_id, category_id) VALUES ($1, $2)`, tea.ID, cat)
	if sels = read(); len(sels) != 3 {
		t.Fatalf("after tea joined the category: %d selections, want 3", len(sels))
	}

	if _, err := r.svc.SetBundleOptionRule(ctx, opt.ID, nil); err != nil {
		t.Fatal(err)
	}
	if sels = read(); len(sels) != 1 || sels[0].ProductID != mug.ID {
		t.Fatalf("after clearing the rule: %+v, want only the mug", sels)
	}
}
