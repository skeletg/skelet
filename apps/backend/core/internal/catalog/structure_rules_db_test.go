package catalog

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
)

type structureRig struct {
	*catalogRig
	pt ProductType
}

func newStructureRig(t *testing.T) structureRig {
	r := newCatalogRig(t)
	r.sweep()
	return structureRig{catalogRig: r, pt: r.ptype("", false)}
}

func (r structureRig) product(kind ProductKind, name string) Product {
	r.t.Helper()
	return r.newProduct(r.pt.ID, kind, name+"-"+uuid.NewString()[:6]+"-"+r.sfx, name, "USD")
}

func (r structureRig) dropdown(productID uuid.UUID, required bool) ProductOption {
	r.t.Helper()
	o, err := r.svc.AddProductOption(context.Background(), AddProductOptionInput{
		ProductID: productID, InputType: OptionInputDropdown, IsRequired: required, Locale: "en", Title: "Pick",
	})
	if err != nil {
		r.t.Fatalf("add option: %v", err)
	}
	return o
}

func wantErr(t *testing.T, what string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("%s = %v, want %v", what, err, want)
	}
}

func TestKitComponentsCarryNamesAndValidateQuantities(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	kit := r.product(KindKit, "kit")
	a, b := r.product(KindSimple, "Bolt"), r.product(KindSimple, "Nut")
	for _, p := range []Product{a, b} {
		if _, err := r.svc.AddKitComponent(ctx, AddKitComponentInput{ProductID: kit.ID, ComponentProductID: p.ID, Quantity: 2}); err != nil {
			t.Fatalf("add component: %v", err)
		}
	}
	comps, err := r.svc.ListKitComponents(ctx, kit.ID, "en")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	names := map[uuid.UUID]string{}
	for _, c := range comps {
		names[c.ComponentProductID] = c.ComponentName
	}
	if names[a.ID] != "Bolt" || names[b.ID] != "Nut" {
		t.Errorf("component names = %v, want Bolt and Nut", names)
	}

	_, err = r.svc.AddKitComponent(ctx, AddKitComponentInput{ProductID: kit.ID, ComponentProductID: a.ID, Quantity: 1})
	wantErr(t, "duplicate component", err, ErrKitComponentDuplicate)
	c := r.product(KindSimple, "Washer")
	for _, q := range []float64{0, -1, 2e9} {
		_, err = r.svc.AddKitComponent(ctx, AddKitComponentInput{ProductID: kit.ID, ComponentProductID: c.ID, Quantity: q})
		wantErr(t, "add kit quantity", err, ErrQuantityOutOfRange)
	}
	zero := 0.0
	_, err = r.svc.UpdateKitComponent(ctx, UpdateKitComponentInput{ID: comps[0].ID, Quantity: &zero})
	wantErr(t, "update kit quantity 0", err, ErrQuantityOutOfRange)
}

func TestGroupedAndOptionValueQuantitiesMatchTheirChecks(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	parent, member := r.product(KindGrouped, "group"), r.product(KindSimple, "member")
	for _, q := range []float64{0, 0.5, 2e9} {
		_, err := r.svc.AddGroupedMember(ctx, AddGroupedMemberInput{ParentID: parent.ID, MemberID: member.ID, DefaultQuantity: q})
		wantErr(t, "add grouped quantity", err, ErrQuantityOutOfRange)
	}
	m, err := r.svc.AddGroupedMember(ctx, AddGroupedMemberInput{ParentID: parent.ID, MemberID: member.ID, DefaultQuantity: DefaultStructureQuantity})
	if err != nil {
		t.Fatalf("add grouped member: %v", err)
	}
	half := 0.5
	_, err = r.svc.UpdateGroupedMember(ctx, UpdateGroupedMemberInput{ID: m.ID, DefaultQuantity: &half})
	wantErr(t, "update grouped quantity 0.5", err, ErrQuantityOutOfRange)

	p := r.product(KindSimple, "opt")
	o := r.dropdown(p.ID, false)
	_, err = r.svc.AddProductOptionValue(ctx, AddProductOptionValueInput{OptionID: o.ID, Locale: "en", Title: "v", PriceType: OptionPriceFixed})
	wantErr(t, "add option value quantity 0", err, ErrQuantityOutOfRange)
	huge := 2e9
	_, err = r.svc.AddProductOptionValue(ctx, AddProductOptionValueInput{OptionID: o.ID, Locale: "en", Title: "v", PriceType: OptionPriceFixed, Quantity: 1, ForcedLineQty: &huge})
	wantErr(t, "forced line qty above the cap", err, ErrInvalidForcedLineQty)
}

func TestARequiredFieldCannotBecomeAnEmptyDropdown(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	p := r.product(KindSimple, "engrave")
	o, err := r.svc.AddProductOption(ctx, AddProductOptionInput{
		ProductID: p.ID, InputType: OptionInputField, IsRequired: true, Locale: "en", Title: "Text",
	})
	if err != nil {
		t.Fatalf("add field: %v", err)
	}
	dropdown := OptionInputDropdown
	_, err = r.svc.UpdateProductOption(ctx, UpdateProductOptionInput{ID: o.ID, InputType: &dropdown})
	wantErr(t, "required field -> empty dropdown", err, ErrOptionRequiredNeedsValues)
}

func TestBundleDefaultsFitTheOption(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	b := r.product(KindBundle, "bundle")
	two := int32(2)
	opt, err := r.svc.AddBundleOption(ctx, AddBundleOptionInput{ProductID: b.ID, InputType: BundleInputCheckbox, MaxSelections: &two, Locale: "en", Title: "Parts"})
	if err != nil {
		t.Fatalf("bundle option: %v", err)
	}
	parts := []Product{r.product(KindSimple, "p1"), r.product(KindSimple, "p2"), r.product(KindSimple, "p3")}
	for _, p := range parts[:2] {
		if _, err := r.svc.AddBundleSelection(ctx, AddBundleSelectionInput{OptionID: opt.ID, ProductID: p.ID, Quantity: 1, IsDefault: true}); err != nil {
			t.Fatalf("default selection: %v", err)
		}
	}
	_, err = r.svc.AddBundleSelection(ctx, AddBundleSelectionInput{OptionID: opt.ID, ProductID: parts[2].ID, Quantity: 1, IsDefault: true})
	wantErr(t, "third default on max 2", err, ErrBundleDefaultsExceedMax)
	third, err := r.svc.AddBundleSelection(ctx, AddBundleSelectionInput{OptionID: opt.ID, ProductID: parts[2].ID, Quantity: 1})
	if err != nil {
		t.Fatalf("non-default selection: %v", err)
	}
	on := true
	_, err = r.svc.UpdateBundleSelection(ctx, UpdateBundleSelectionInput{ID: third.ID, IsDefault: &on})
	wantErr(t, "flipping a third default on", err, ErrBundleDefaultsExceedMax)
	one := int32(1)
	_, err = r.svc.UpdateBundleOption(ctx, UpdateBundleOptionInput{ID: opt.ID, MaxSelections: &one})
	wantErr(t, "lowering max below the defaults", err, ErrBundleDefaultsExceedMax)
	_, err = r.svc.AddBundleSelection(ctx, AddBundleSelectionInput{OptionID: opt.ID, ProductID: parts[0].ID, Quantity: 1})
	wantErr(t, "duplicate selection", err, ErrBundleSelectionDuplicate)
	_, err = r.svc.AddBundleSelection(ctx, AddBundleSelectionInput{OptionID: opt.ID, ProductID: r.product(KindSimple, "p4").ID, Quantity: 2e9})
	wantErr(t, "selection quantity above the cap", err, ErrQuantityOutOfRange)
}

func TestConcurrentDefaultsLeaveOneDefault(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	o := r.dropdown(r.product(KindSimple, "race").ID, false)
	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for range 8 {
		wg.Go(func() {
			if _, err := r.svc.AddProductOptionValue(ctx, AddProductOptionValueInput{
				OptionID: o.ID, Locale: "en", Title: "v", PriceType: OptionPriceFixed, Quantity: 1, IsDefault: true,
			}); err == nil {
				mu.Lock()
				won++
				mu.Unlock()
			} else if !errors.Is(err, ErrOptionMultiDefaultOnSingle) {
				t.Errorf("concurrent default: %v", err)
			}
		})
	}
	wg.Wait()
	n, err := r.repo.CountProductOptionValueDefaultsForOption(ctx, o.ID)
	if err != nil || won != 1 || n != 1 {
		t.Fatalf("winners = %d, stored defaults = %d (%v), want exactly one", won, n, err)
	}
}

type fixedLocale string

func (f fixedLocale) DefaultLocale(context.Context) (string, error) { return string(f), nil }

type knownLocales map[string]bool

var errUnknownLocale = errors.New("unknown locale")

func (k knownLocales) EnsureExists(_ context.Context, code string) error {
	if !k[code] {
		return errUnknownLocale
	}
	return nil
}

func TestOptionContentLocales(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	r.exec(`INSERT INTO locales (code, name) VALUES ('de', 'German') ON CONFLICT DO NOTHING`)
	r.svc.locales = knownLocales{"en": true, "de": true}
	r.svc.defaultLocale = fixedLocale("en")
	o := r.dropdown(r.product(KindSimple, "loc").ID, false)
	v, err := r.svc.AddProductOptionValue(ctx, AddProductOptionValueInput{OptionID: o.ID, Locale: "en", Title: "Red", PriceType: OptionPriceFixed, Quantity: 1})
	if err != nil {
		t.Fatalf("add value: %v", err)
	}

	_, err = r.svc.UpsertProductOptionContent(ctx, UpsertProductOptionContentInput{OptionID: o.ID, Locale: "zz", Title: "?"})
	wantErr(t, "option content in an unknown locale", err, errUnknownLocale)
	_, err = r.svc.UpsertProductOptionValueContent(ctx, UpsertProductOptionValueContentInput{ValueID: v.ID, Locale: "zz", Title: "?"})
	wantErr(t, "value content in an unknown locale", err, errUnknownLocale)

	if _, err := r.svc.UpsertProductOptionContent(ctx, UpsertProductOptionContentInput{OptionID: o.ID, Locale: "de", Title: "Wahl"}); err != nil {
		t.Fatalf("de content: %v", err)
	}
	if _, err := r.svc.UpsertProductOptionValueContent(ctx, UpsertProductOptionValueContentInput{ValueID: v.ID, Locale: "de", Title: "Rot"}); err != nil {
		t.Fatalf("de value content: %v", err)
	}
	wantErr(t, "dropping the default option row", r.svc.DeleteProductOptionContent(ctx, o.ID, "en"), ErrCannotDeleteDefaultLocale)
	wantErr(t, "dropping the default value row", r.svc.DeleteProductOptionValueContent(ctx, v.ID, "en"), ErrCannotDeleteDefaultLocale)

	got, err := r.svc.GetProductOptions(ctx, o.ProductID, "de")
	if err != nil || len(got) != 1 || got[0].Title != "Wahl" || got[0].Values[0].Title != "Rot" {
		t.Fatalf("de read = %+v (%v), want the German titles", got, err)
	}
}

func TestAnUnreadableVisibilityRuleFailsClosed(t *testing.T) {
	r := newStructureRig(t)
	p := r.product(KindSimple, "vis")
	o := r.dropdown(p.ID, false)
	r.exec(`UPDATE catalog_product_options SET visible_if = '{"rules": "not-a-list"}' WHERE id = $1`, o.ID)
	if _, err := r.svc.GetProductOptions(context.Background(), p.ID, "en"); err == nil {
		t.Fatal("an undecodable visible_if read back as an always-visible option")
	}
}

func TestDuplicateOptionCopiesValuesAndContent(t *testing.T) {
	r := newStructureRig(t)
	ctx := context.Background()
	o := r.dropdown(r.product(KindSimple, "dup").ID, false)
	if _, err := r.svc.AddProductOptionValue(ctx, AddProductOptionValueInput{OptionID: o.ID, Locale: "en", Title: "Blue", PriceType: OptionPriceFixed, Quantity: 1}); err != nil {
		t.Fatalf("value: %v", err)
	}
	clone, err := r.svc.DuplicateProductOption(ctx, o.ID)
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	vals, err := r.repo.ListProductOptionValuesByOptionID(ctx, clone.ID)
	if err != nil || len(vals) != 1 || clone.SortOrder <= o.SortOrder {
		t.Fatalf("clone values = %d (%v), sort %d vs %d", len(vals), err, clone.SortOrder, o.SortOrder)
	}
}
