package catalog

import (
	"context"
	"testing"
)

// What this pins: a bundle's pricing is set on its own and kept apart from the listing upsert, which every listing form drives and would otherwise reset it
func TestBundlePricingIsSetOnItsOwnAndSurvivesAListingSave(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	bundle, simple := r.product(KindBundle, "box"), r.product(KindSimple, "mug")
	ch := r.channelIn("USD")

	_, err := r.svc.SetBundlePricing(ctx, bundle.ID, ch, BundlePricingSumMinusPercent, 10)
	wantErr(t, "pricing before the bundle is listed", err, ErrListingNotFound)

	listed := int64(3000) // an explicit price - inheriting one needs the channel service this rig doesn't wire
	for _, p := range []Product{bundle, simple} {
		if _, err := r.svc.SetProductListing(ctx, ProductChannelListingInput{ProductID: p.ID, ChannelID: ch, IsPublished: true, Price: &listed}); err != nil {
			t.Fatalf("list: %v", err)
		}
	}
	_, err = r.svc.SetBundlePricing(ctx, simple.ID, ch, BundlePricingFixed, 0)
	wantErr(t, "pricing a simple product", err, ErrBundleParentNotBundle)
	_, err = r.svc.SetBundlePricing(ctx, bundle.ID, ch, BundlePricingSumMinusPercent, 101)
	wantErr(t, "a discount over 100%", err, ErrInvalidBundlePricing)
	_, err = r.svc.SetBundlePricing(ctx, bundle.ID, ch, "half", 0)
	wantErr(t, "an unknown mode", err, ErrInvalidBundlePricing)

	if _, err := r.svc.SetBundlePricing(ctx, bundle.ID, ch, BundlePricingSumMinusPercent, 10); err != nil {
		t.Fatalf("set pricing: %v", err)
	}
	price := int64(4000)
	if _, err := r.svc.SetProductListing(ctx, ProductChannelListingInput{ProductID: bundle.ID, ChannelID: ch, IsPublished: true, Price: &price}); err != nil {
		t.Fatalf("relist: %v", err)
	}
	ls, err := r.svc.ListProductChannelListings(ctx, bundle.ID)
	if err != nil || len(ls) != 1 {
		t.Fatalf("listings %+v, err %v", ls, err)
	}
	l := ls[0]
	if l.BundlePricing != BundlePricingSumMinusPercent || l.BundleDiscountPercent != 10 {
		t.Fatalf("after a listing save: %s %d%%, want sum_minus_percent 10%% kept", l.BundlePricing, l.BundleDiscountPercent)
	}

	l, err = r.svc.SetBundlePricing(ctx, bundle.ID, ch, BundlePricingFixed, 10)
	if err != nil {
		t.Fatal(err)
	}
	if l.BundleDiscountPercent != 0 {
		t.Fatalf("fixed kept a %d%% discount, want it cleared", l.BundleDiscountPercent)
	}
}
