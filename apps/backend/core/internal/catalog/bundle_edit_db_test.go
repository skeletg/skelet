package catalog

import (
	"context"
	"testing"
)

// What this pins: a staged bundle save is one transaction - a later edit failing takes the earlier ones back with it,
// while a whole edit lands, the selection finding the option an earlier step created
func TestEditBundleLandsWholeOrNotAtAll(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	bundle, mug, nested := r.product(KindBundle, "box"), r.product(KindSimple, "mug"), r.product(KindBundle, "inner")

	err := r.svc.EditBundle(ctx, func(e BundleEditor) error {
		opt, err := e.AddBundleOption(ctx, AddBundleOptionInput{ProductID: bundle.ID, InputType: BundleInputRadio, IsRequired: true, Locale: "en", Title: "Mug"})
		if err != nil {
			return err
		}
		_, err = e.AddBundleSelection(ctx, AddBundleSelectionInput{OptionID: opt.ID, ProductID: nested.ID, Quantity: 1})
		return err
	})
	wantErr(t, "a bundle inside a bundle", err, ErrBundleSelectionNotSimple)
	if opts, _ := r.svc.GetBundleOptions(ctx, bundle.ID, "en"); len(opts) != 0 {
		t.Fatalf("the failed edit left %d option(s) behind, want none", len(opts))
	}

	if err := r.svc.EditBundle(ctx, func(e BundleEditor) error {
		opt, err := e.AddBundleOption(ctx, AddBundleOptionInput{ProductID: bundle.ID, InputType: BundleInputRadio, IsRequired: true, Locale: "en", Title: "Mug"})
		if err != nil {
			return err
		}
		_, err = e.AddBundleSelection(ctx, AddBundleSelectionInput{OptionID: opt.ID, ProductID: mug.ID, Quantity: 2, IsDefault: true})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	opts, err := r.svc.GetBundleOptions(ctx, bundle.ID, "en")
	if err != nil || len(opts) != 1 || len(opts[0].Selections) != 1 || opts[0].Selections[0].Quantity != 2 {
		t.Fatalf("options %+v (%v), want the one option holding 2 mugs", opts, err)
	}
}
