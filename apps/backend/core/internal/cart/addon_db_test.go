package cart

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// A jacket with two optional add-ons sold as lines of their own - a protection plan, one per jacket, and a pack of two
// spare buttons - and a required gift-wrap choice sold the same way
func addOnCatalog() (catalogStub, struct{ protection, plan, buttons, pack, wrap, paper uuid.UUID }) {
	ids := struct{ protection, plan, buttons, pack, wrap, paper uuid.UUID }{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	value := func(id uuid.UUID, title, sku string, price int64, per float64, promos bool) ProductOptionValueConfig {
		return ProductOptionValueConfig{ID: id, Title: title, PriceType: "fixed", Quantity: per, HasSource: true, SourceProductID: uuid.New(),
			SourceProductPrice: price, SoldAsLine: true, SourceSKU: sku, SourceName: title, LinePromotions: promos}
	}
	return catalogStub{
		prices: map[string]SimplePricing{"jacket": {ProductName: "Jacket", UnitPrice: 10_000}, "PLAN": {ProductName: "Protection plan", UnitPrice: 1_500}},
		options: map[string]ProductOptionsResolution{"jacket": {UnitPrice: 10_000, BasePrice: 10_000, Options: []ProductOptionConfig{
			{ID: ids.protection, InputType: "checkbox", Title: "Protection", Values: []ProductOptionValueConfig{value(ids.plan, "Protection plan", "PLAN", 1_500, 1, false)}},
			{ID: ids.buttons, InputType: "checkbox", Title: "Spares", Values: []ProductOptionValueConfig{value(ids.pack, "Spare buttons", "BUTTONS", 200, 2, true)}},
			{ID: ids.wrap, InputType: "radio", Title: "Wrapping", IsRequired: true, Values: []ProductOptionValueConfig{value(ids.paper, "Gift wrap", "WRAP", 300, 1, false)}},
		}}},
	}, ids
}

// A choice sold as its own line becomes a line under the item: its own SKU and price, none of it in the item's price,
// as many as the item needs, following the item's quantity and leaving with it
func TestAnAddOnIsALineUnderItsItem(t *testing.T) {
	cat, ids := addOnCatalog()
	r := newDBRig(t, cat)
	ctx := context.Background()
	c := r.guestCart(t)
	pick := func(optionID, valueID uuid.UUID) CartLineOptionInput {
		return CartLineOptionInput{OptionID: optionID, ValueIDs: []uuid.UUID{valueID}}
	}
	all := []CartLineOptionInput{pick(ids.protection, ids.plan), pick(ids.buttons, ids.pack), pick(ids.wrap, ids.paper)}
	got, err := r.svc.AddConfiguredToCart(ctx, c.ID, nil, AddConfiguredToCartInput{ProductSku: "jacket", Quantity: 2, Options: all})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lines) != 4 || got.Lines[0].ProductSku != "jacket" {
		t.Fatalf("lines = %+v, want the jacket and its three add-ons under it", got.Lines)
	}
	jacket := got.Lines[0]
	if jacket.UnitPrice != 10_000 || len(jacket.Options) != 3 {
		t.Fatalf("the jacket: %d with %d picks, want its own 10000 - the add-ons' prices are on their lines - and the three picks", jacket.UnitPrice, len(jacket.Options))
	}
	for _, o := range jacket.Options {
		if o.PriceDelta != 0 || o.SourceProductID != nil {
			t.Errorf("pick %q adds %d to the jacket and takes stock through it (%v), want neither", o.ValueLabel, o.PriceDelta, o.SourceProductID)
		}
	}
	plan, buttons := lineBySku(t, got, "PLAN"), lineBySku(t, got, "BUTTONS")
	for _, l := range got.Lines[1:] {
		if !l.IsAddOn() || *l.ParentLineID != jacket.ID {
			t.Errorf("%s isn't under the jacket: %+v", l.ProductSku, l)
		}
	}
	if plan.UnitPrice != 1_500 || plan.Quantity != 2 || !plan.NoPromotions || plan.ProductName != "Protection plan" {
		t.Errorf("the plan line: %+v, want 2 at 1500, left alone by promotions", plan)
	}
	if buttons.UnitPrice != 200 || buttons.Quantity != 4 || buttons.NoPromotions {
		t.Errorf("the buttons line: %+v, want 4 (two per jacket) at 200, open to promotions", buttons)
	}

	// The item's quantity is the add-ons' too, and an add-on's own can't be changed
	got, err = r.svc.UpdateCartLine(ctx, c.ID, jacket.ID, nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	if lineBySku(t, got, "PLAN").Quantity != 3 || lineBySku(t, got, "BUTTONS").Quantity != 6 {
		t.Errorf("after 3 jackets: %v plans and %v buttons, want 3 and 6", lineBySku(t, got, "PLAN").Quantity, lineBySku(t, got, "BUTTONS").Quantity)
	}
	if _, err := r.svc.UpdateCartLine(ctx, c.ID, plan.ID, nil, 1); !errors.Is(err, ErrLineFollowsParent) {
		t.Errorf("an add-on's own quantity: %v, want it refused", err)
	}
	if _, err := r.svc.SaveForLater(ctx, c.ID, plan.ID, nil); !errors.Is(err, ErrLineNotSaveable) {
		t.Errorf("an add-on saved for later: %v, want it refused", err)
	}
	if repriced := r.svc.RepriceCartLines(ctx, got); lineBySku(t, repriced, "PLAN").UnitPrice != 1_500 {
		t.Errorf("an add-on was repriced at checkout, want the price it was picked at")
	}

	// The same product bought on its own is another line
	got, err = r.svc.AddToCart(ctx, c.ID, nil, "PLAN", 1)
	if err != nil {
		t.Fatal(err)
	}
	var alone, under int
	for _, l := range got.Lines {
		if l.ProductSku == "PLAN" && l.IsAddOn() {
			under++
		} else if l.ProductSku == "PLAN" {
			alone++
		}
	}
	if alone != 1 || under != 1 {
		t.Fatalf("PLAN lines: %d alone and %d under the jacket, want one of each", alone, under)
	}

	// An optional add-on can go, and takes its pick off the jacket. The only choice of a required option can't
	got, err = r.svc.RemoveFromCart(ctx, c.ID, plan.ID, nil)
	if err != nil {
		t.Fatalf("removing an optional add-on: %v", err)
	}
	jacket = got.Lines[0]
	if len(jacket.Options) != 2 {
		t.Errorf("the jacket still shows %d picks, want the plan's gone", len(jacket.Options))
	}
	if _, err := r.svc.RemoveFromCart(ctx, c.ID, lineBySku(t, got, "WRAP").ID, nil); !errors.Is(err, ErrAddOnRequired) {
		t.Errorf("removing the required option's only choice: %v, want it refused", err)
	}

	// Picking again replaces the add-ons with the new picks, and removing the item takes them all
	got, err = r.svc.AddConfiguredToCart(ctx, c.ID, nil, AddConfiguredToCartInput{ProductSku: "jacket", Quantity: 1, Options: []CartLineOptionInput{pick(ids.wrap, ids.paper)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lines) != 3 || lineBySku(t, got, "WRAP").Quantity != 1 { // jacket, its wrap, and the PLAN bought alone
		t.Fatalf("after picking again: %+v, want the jacket with only its wrap", got.Lines)
	}
	got, err = r.svc.RemoveFromCart(ctx, c.ID, got.Lines[0].ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lines) != 1 || got.Lines[0].IsAddOn() {
		t.Fatalf("after removing the jacket: %+v, want only the plan bought on its own", got.Lines)
	}
}

// An item moves to another cart with its add-ons, each under the copy
func TestAddOnsMoveWithTheirItem(t *testing.T) {
	cat, ids := addOnCatalog()
	r := newDBRig(t, cat)
	ctx := context.Background()
	guest := r.guestCart(t)
	picks := []CartLineOptionInput{{OptionID: ids.protection, ValueIDs: []uuid.UUID{ids.plan}}, {OptionID: ids.wrap, ValueIDs: []uuid.UUID{ids.paper}}}
	if _, err := r.svc.AddConfiguredToCart(ctx, guest.ID, nil, AddConfiguredToCartInput{ProductSku: "jacket", Quantity: 2, Options: picks}); err != nil {
		t.Fatal(err)
	}
	customerID := r.customer(t)
	own, err := r.svc.GetOrCreateCart(ctx, customerID, r.channel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AddToCart(ctx, own.ID, &customerID, "PLAN", 1); err != nil { // so the two carts are copied together rather than one adopted
		t.Fatal(err)
	}
	merged, err := r.svc.MergeGuestCart(ctx, customerID, guest.ID)
	if err != nil {
		t.Fatal(err)
	}
	jacket := lineBySku(t, merged, "jacket")
	var under []CartLine
	for _, l := range merged.Lines {
		if l.IsAddOn() {
			under = append(under, l)
			if *l.ParentLineID != jacket.ID || l.Quantity != 2 {
				t.Errorf("%s after the merge: %+v, want 2 under the copied jacket", l.ProductSku, l)
			}
		}
	}
	if len(under) != 2 || len(merged.Lines) != 4 {
		t.Fatalf("merged lines: %+v, want the plan bought alone, the jacket and its two add-ons", merged.Lines)
	}
}
