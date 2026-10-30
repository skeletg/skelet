package cart

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestBundlePicks(t *testing.T) {
	snap, mugs, coasters, mugSel, coasterSel := mugSet()
	extra := uuid.New()
	snap.Options[1].Selections = append(snap.Options[1].Selections, BundleSelectionSnapshot{ID: extra, ProductSKU: "coaster-cork", QuantityPerBundle: 1})
	min2 := snap
	min2.Options = append([]BundleOptionSnapshot(nil), snap.Options...)
	min2.Options[1].MinSelections = 2
	optional := snap
	optional.Options = append([]BundleOptionSnapshot(nil), snap.Options...)
	optional.Options[0].IsRequired = false

	for _, tc := range []struct {
		name    string
		snap    BundleSnapshot
		in      []AddBundleSelectionInput
		want    int
		wantErr error
	}{
		{"one pick per option", snap, []AddBundleSelectionInput{{OptionID: mugs, SelectionIDs: []uuid.UUID{mugSel}}, {OptionID: coasters, SelectionIDs: []uuid.UUID{coasterSel}}}, 2, nil},
		{"a radio listed twice", snap, []AddBundleSelectionInput{{OptionID: mugs, SelectionIDs: []uuid.UUID{mugSel}}, {OptionID: mugs, SelectionIDs: []uuid.UUID{mugSel}}}, 0, ErrBundleSelectionInvalid},
		{"one selection twice toward a minimum of two", min2, []AddBundleSelectionInput{{OptionID: mugs, SelectionIDs: []uuid.UUID{mugSel}}, {OptionID: coasters, SelectionIDs: []uuid.UUID{coasterSel, coasterSel}}}, 0, ErrBundleSelectionInvalid},
		{"two distinct selections meet the minimum", min2, []AddBundleSelectionInput{{OptionID: mugs, SelectionIDs: []uuid.UUID{mugSel}}, {OptionID: coasters, SelectionIDs: []uuid.UUID{coasterSel, extra}}}, 3, nil},
		{"a required option left out", snap, []AddBundleSelectionInput{{OptionID: coasters, SelectionIDs: []uuid.UUID{coasterSel}}}, 0, ErrBundleSelectionMissing},
		{"two values on a radio", snap, []AddBundleSelectionInput{{OptionID: mugs, SelectionIDs: []uuid.UUID{mugSel, uuid.New()}}}, 0, ErrBundleSelectionCardinality},
		{"a selection from nowhere", snap, []AddBundleSelectionInput{{OptionID: mugs, SelectionIDs: []uuid.UUID{uuid.New()}}}, 0, ErrBundleSelectionInvalid},
		{"an option from nowhere", snap, []AddBundleSelectionInput{{OptionID: uuid.New()}}, 0, ErrBundleOptionUnknown},
		{"every optional option left empty", optional, []AddBundleSelectionInput{{OptionID: mugs}, {OptionID: coasters}}, 0, ErrBundleSelectionMissing},
		{"no options at all", BundleSnapshot{}, nil, 0, ErrBundleSelectionMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := bundlePicks(tc.snap, tc.in, nil)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err %v, want %v", err, tc.wantErr)
			}
			if len(got) != tc.want {
				t.Fatalf("%d picks, want %d", len(got), tc.want)
			}
		})
	}
}

func TestBundlePicksAppliesDefaultsOnlyToUntouchedOptions(t *testing.T) {
	snap, mugs, coasters, mugSel, _ := mugSet()
	snap.Options[1].Selections[0].IsDefault = true
	got, err := bundlePicks(snap, []AddBundleSelectionInput{{OptionID: mugs, SelectionIDs: []uuid.UUID{mugSel}}}, nil)
	if err != nil || len(got) != 2 {
		t.Fatalf("untouched default not applied: %v %v", got, err)
	}
	got, err = bundlePicks(snap, []AddBundleSelectionInput{{OptionID: mugs, SelectionIDs: []uuid.UUID{mugSel}}, {OptionID: coasters}}, nil)
	if err != nil || len(got) != 1 {
		t.Fatalf("an explicit empty pick kept the default: %v %v", got, err)
	}
}

func TestValidQuantity(t *testing.T) {
	for _, q := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1), 0.00001, 1e10} {
		if err := validQuantity(q); !errors.Is(err, ErrQuantityInvalid) {
			t.Errorf("validQuantity(%v) = %v, want ErrQuantityInvalid", q, err)
		}
	}
	for _, q := range []float64{0.0001, 1, 2.5, 1e9} {
		if err := validQuantity(q); err != nil {
			t.Errorf("validQuantity(%v) = %v", q, err)
		}
	}
}

func TestValidateQtyBounds(t *testing.T) {
	p := SimplePricing{MinQty: ptrF(2), MaxQty: ptrF(10), QtyStep: 0.1}
	for q, ok := range map[float64]bool{1: false, 2: true, 0.3 * 10: true, 2.35: false, 10: true, 10.1: false} {
		if err := validateQtyBounds(p, q); (err == nil) != ok {
			t.Errorf("bounds(%v) = %v, want ok=%v", q, err, ok)
		}
	}
}

// A merged guest cart or two racing adds can each sum a line past its max - placement is where that gets caught
func TestCheckoutRefusesALineOutsideItsBounds(t *testing.T) {
	svc := NewService(fakeRepo{}, nil, fakePricer{prices: map[string]SimplePricing{"tee": {UnitPrice: 2500, MaxQty: ptrF(5)}}})
	cartWith := func(l CartLine) Cart { return Cart{Lines: []CartLine{l}} }
	if err := svc.ValidateCheckoutLines(context.Background(), cartWith(CartLine{ProductSku: "tee", Quantity: 5})); err != nil {
		t.Fatalf("a line at its max refused: %v", err)
	}
	if err := svc.ValidateCheckoutLines(context.Background(), cartWith(CartLine{ProductSku: "tee", Quantity: 8})); !errors.Is(err, ErrQuantityOutOfBounds) {
		t.Fatalf("a line past its max placed: %v", err)
	}
	if err := svc.ValidateCheckoutLines(context.Background(), cartWith(CartLine{ProductSku: "tee", Quantity: 8, IsGift: true})); err != nil {
		t.Fatalf("a promotion's gift line was held to the product's bounds: %v", err)
	}
	if err := svc.ValidateCheckoutLines(context.Background(), cartWith(CartLine{ProductSku: "unpriced", Quantity: 8})); err != nil {
		t.Fatalf("a pricing miss blocked placement: %v", err)
	}
}

func TestCartEmailAndNoteAreChecked(t *testing.T) {
	cartID := uuid.New()
	var stored string
	svc := NewService(fakeRepo{
		getCart:  func(uuid.UUID) (Cart, error) { return Cart{ID: cartID, Status: "active"}, nil },
		setEmail: func(e string) { stored = e },
	}, nil, nil)
	ctx := context.Background()
	for _, e := range []string{"", "a@", "@b.c", "Ana <ana@example.test>", "ana@example.test, bob@example.test", strings.Repeat("a", 250) + "@b.cd"} {
		if err := svc.SetCartEmail(ctx, cartID, nil, e); !errors.Is(err, ErrCartEmailInvalid) {
			t.Errorf("SetCartEmail(%q) = %v, want ErrCartEmailInvalid", e, err)
		}
	}
	if err := svc.SetCartEmail(ctx, cartID, nil, "  Ana@Example.test "); err != nil {
		t.Errorf("a plain address refused: %v", err)
	}
	if stored != "ana@example.test" {
		t.Errorf("stored %q, want the address normalized the way customer emails are", stored)
	}
	if err := svc.SetCartNote(ctx, cartID, nil, strings.Repeat("ž", maxNoteRunes+1)); !errors.Is(err, ErrCartNoteTooLong) {
		t.Errorf("an oversized note: %v", err)
	}
	if err := svc.SetCartNote(ctx, cartID, nil, strings.Repeat("ž", maxNoteRunes)); err != nil {
		t.Errorf("a note at the limit refused: %v", err)
	}
}

func TestAnUncappedTextOptionStillHasACeiling(t *testing.T) {
	optionID := uuid.New()
	res := ProductOptionsResolution{Options: []ProductOptionConfig{{ID: optionID, InputType: "area", Title: "Message"}}}
	if _, _, _, err := buildConfiguredLineOptions(res, []CartLineOptionInput{{OptionID: optionID, TextValue: strings.Repeat("x", maxOptionTextRunes+1)}}, nil); !errors.Is(err, ErrProductOptionInvalid) {
		t.Fatalf("an oversized message snapshotted: %v", err)
	}
	if _, _, _, err := buildConfiguredLineOptions(res, []CartLineOptionInput{{OptionID: optionID, TextValue: strings.Repeat("x", maxOptionTextRunes)}}, nil); err != nil {
		t.Fatalf("a message at the ceiling refused: %v", err)
	}
}

// A lookup failure isn't a missing cart - only a missing row reads as one
func TestAdminCartLookupsKeepDBFailures(t *testing.T) {
	dbDown := errors.New("db down")
	for _, tc := range []struct {
		err  error
		want error
	}{{sql.ErrNoRows, ErrCartNotFound}, {dbDown, dbDown}} {
		svc := NewService(fakeRepo{getCart: func(uuid.UUID) (Cart, error) { return Cart{}, tc.err }}, nil, nil)
		if _, err := svc.adminActiveCart(context.Background(), uuid.New()); !errors.Is(err, tc.want) {
			t.Errorf("adminActiveCart with %v: %v, want %v", tc.err, err, tc.want)
		}
		if _, err := svc.AdminDuplicateCart(context.Background(), uuid.New(), nil); !errors.Is(err, tc.want) {
			t.Errorf("AdminDuplicateCart with %v: %v, want %v", tc.err, err, tc.want)
		}
	}
	svc := NewService(fakeRepo{getCart: func(uuid.UUID) (Cart, error) { return Cart{Status: "converted"}, nil }}, nil, nil)
	if _, err := svc.adminActiveCart(context.Background(), uuid.New()); !errors.Is(err, ErrCartNotActive) {
		t.Errorf("a converted cart: %v, want ErrCartNotActive", err)
	}
}

type claimRepo struct {
	upsertCounter
	saved SavedItem
}

func (r *claimRepo) GetSavedItem(context.Context, uuid.UUID) (SavedItem, error) { return r.saved, nil }

// Two moves racing past the lookup - only the one whose delete took the row may add it
func TestALostSavedItemClaimAddsNothing(t *testing.T) {
	cartID := uuid.New()
	repo := &claimRepo{saved: SavedItem{ID: uuid.New(), CartID: cartID, ProductSku: "tee", Quantity: 2}}
	repo.getCart = func(uuid.UUID) (Cart, error) { return Cart{ID: cartID, Status: "active"}, nil }
	svc := NewService(repo, nil, catalogStub{prices: map[string]SimplePricing{"tee": {UnitPrice: 2500}}})
	if _, err := svc.MoveSavedToCart(context.Background(), cartID, repo.saved.ID, nil); err != nil {
		t.Fatal(err)
	}
	if repo.upserts != 0 {
		t.Fatalf("%d adds from a move that lost the claim, want none", repo.upserts)
	}
}

// Checkout re-runs the add-time rules on the line's stored picks against today's bundle
func TestStoredPicksAgainstAChangedBundle(t *testing.T) {
	snap, mugs, coasters, mugSel, coasterSel := mugSet()
	snap.Options[1].Selections[0].IsDefault = true
	line := []CartLineComponent{{BundleOptionID: mugs, BundleSelectionID: mugSel}, {BundleOptionID: coasters, BundleSelectionID: coasterSel}}
	only := func(o ...BundleOptionSnapshot) BundleSnapshot { return BundleSnapshot{Options: o} }
	noCoaster := snap.Options[1]
	noCoaster.Selections = nil
	required := BundleOptionSnapshot{ID: uuid.New(), InputType: "radio", IsRequired: true, Selections: []BundleSelectionSnapshot{{ID: uuid.New(), IsDefault: true}}}

	for _, tc := range []struct {
		name    string
		snap    BundleSnapshot
		comps   []CartLineComponent
		wantErr error
	}{
		{"unchanged", snap, line, nil},
		{"the picked selection was removed", only(snap.Options[0], noCoaster), line, ErrBundleSelectionInvalid},
		{"the picked option was removed", only(snap.Options[0]), line, ErrBundleOptionUnknown},
		{"a required option was added", only(snap.Options[0], snap.Options[1], required), line, ErrBundleSelectionMissing},
		{"an untouched default is not slipped in", snap, line[:1], nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := bundlePicks(tc.snap, picksFromComponents(tc.snap, tc.comps), nil)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err %v, want %v", err, tc.wantErr)
			}
			if err == nil && len(got) != len(tc.comps) {
				t.Fatalf("%d picks from %d stored components", len(got), len(tc.comps))
			}
		})
	}
}

// What this pins: a configurable selection stands for its variants - the customer has to name one of them, and that
// variant becomes the component the line prices and reserves
func TestBundlePicksTakeAVariantForAConfigurableSelection(t *testing.T) {
	opt, sel, small, medium := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	snap := BundleSnapshot{Options: []BundleOptionSnapshot{{ID: opt, InputType: "radio", IsRequired: true, Selections: []BundleSelectionSnapshot{{
		ID: sel, ProductID: uuid.New(), ProductSKU: "hoodie", QuantityPerBundle: 1,
		Variants: []BundleVariantSnapshot{{ProductID: small, ProductSKU: "hoodie-s", UnitPrice: 4900}, {ProductID: medium, ProductSKU: "hoodie-m", UnitPrice: 5100}},
	}}}}}
	pick := []AddBundleSelectionInput{{OptionID: opt, SelectionIDs: []uuid.UUID{sel}}}

	if _, err := bundlePicks(snap, pick, nil); !errors.Is(err, ErrBundleVariantMissing) {
		t.Fatalf("no variant = %v, want ErrBundleVariantMissing", err)
	}
	if _, err := bundlePicks(snap, pick, map[uuid.UUID]uuid.UUID{sel: uuid.New()}); !errors.Is(err, ErrBundleVariantMissing) {
		t.Fatalf("someone else's variant = %v, want ErrBundleVariantMissing", err)
	}
	got, err := bundlePicks(snap, pick, map[uuid.UUID]uuid.UUID{sel: medium})
	if err != nil || len(got) != 1 || got[0].ProductID != medium || got[0].ProductSKU != "hoodie-m" || got[0].UnitPrice != 5100 || got[0].ID != sel {
		t.Fatalf("picked %+v (%v), want selection %s holding variant hoodie-m", got, err, sel)
	}
	// A stored line rechecks the variant its component row holds
	comps := []CartLineComponent{{BundleOptionID: opt, BundleSelectionID: sel, ProductID: medium}}
	if got, err := bundlePicks(snap, picksFromComponents(snap, comps), variantsFromComponents(comps)); err != nil || got[0].ProductSKU != "hoodie-m" {
		t.Fatalf("re-resolved %+v (%v), want hoodie-m again", got, err)
	}
}
