package cart

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func heldAt(l CartLine, unit, list int64) bool {
	return l.PriceHeld() && l.PriceSource == PriceAdmin && l.UnitPrice == unit && l.ListUnitPrice != nil && *l.ListUnitPrice == list
}

// A price an admin sets on a line says where it came from - who, why and the catalog price it replaced - and holds
// through more of the item, a quantity edit and checkout's repricing, with the catalog's figure kept beside it
func TestASetPriceKeepsWhereItCameFrom(t *testing.T) {
	prices := map[string]SimplePricing{"sku": {ProductName: "Item", UnitPrice: 1000}}
	r := newDBRig(t, catalogStub{prices: prices})
	ctx := context.Background()
	c := r.offer(t)
	c, err := r.svc.AdminAddToCart(ctx, c.ID, "sku", 1)
	if err != nil {
		t.Fatal(err)
	}
	line := c.Lines[0]
	if line.PriceSource != PriceCatalog || line.PriceHeld() || line.ListUnitPrice != nil || line.PriceSetBy != nil {
		t.Fatalf("a line as added: %+v, want the catalog's price and nothing kept about it", line)
	}

	admin := uuid.New()
	set := func(cents int64, reason string) (CartLine, error) {
		got, err := r.svc.AdminSetLinePrice(ctx, c.ID, line.ID, SetLinePriceInput{UnitPrice: &cents, Reason: reason, By: &admin})
		if err != nil {
			return CartLine{}, err
		}
		return lineBySku(t, got, "sku"), nil
	}
	l, err := set(700, "  price match  ")
	if err != nil {
		t.Fatal(err)
	}
	if !heldAt(l, 700, 1000) || l.PriceSetBy == nil || *l.PriceSetBy != admin || l.PriceReason != "price match" {
		t.Fatalf("a set price: %+v, want 700 by the admin for a price match, replacing 1000", l)
	}

	// The catalog moves: a price set again replaces today's figure, and its note replaces the old one
	prices["sku"] = SimplePricing{ProductName: "Item", UnitPrice: 1200}
	if l, err = set(650, ""); err != nil || !heldAt(l, 650, 1200) || l.PriceReason != "" {
		t.Fatalf("set again: %+v, %v - want 650 replacing 1200, no note", l, err)
	}

	prices["sku"] = SimplePricing{ProductName: "Item", UnitPrice: 1300}
	more, err := r.svc.AdminAddToCart(ctx, c.ID, "sku", 2)
	if err != nil {
		t.Fatal(err)
	}
	if l = lineBySku(t, more, "sku"); l.Quantity != 3 || !heldAt(l, 650, 1300) {
		t.Fatalf("more of the item: %+v, want 3 at the set 650 beside the catalog's 1300", l)
	}
	if l = lineBySku(t, r.svc.RepriceCartLines(ctx, more), "sku"); !heldAt(l, 650, 1300) {
		t.Fatalf("repriced: %+v, want the set price left alone", l)
	}
	// A reprice that read the line before the price was set writes the catalog's figure - the line keeps its own
	raced, err := r.svc.repo.UpdateCartLineQuantityPrice(ctx, line.ID, 4, 1400)
	if err != nil || raced.Quantity != 4 || !heldAt(raced, 650, 1400) {
		t.Fatalf("a late reprice: %+v, %v - want 4 at the set 650", raced, err)
	}

	back, err := r.svc.AdminSetLinePrice(ctx, c.ID, line.ID, SetLinePriceInput{By: &admin})
	if err != nil {
		t.Fatal(err)
	}
	if l = lineBySku(t, back, "sku"); l.PriceHeld() || l.PriceSource != PriceCatalog || l.UnitPrice != 1300 || l.ListUnitPrice != nil || l.PriceSetBy != nil || l.PriceReason != "" {
		t.Fatalf("put back: %+v, want the catalog's 1300 and nothing kept", l)
	}

	for name, in := range map[string]SetLinePriceInput{
		"negative":        {UnitPrice: ptrI(-1)},
		"past the most":   {UnitPrice: ptrI(MaxLinePriceCents + 1)},
		"a note too long": {UnitPrice: ptrI(500), Reason: strings.Repeat("é", MaxPriceReason+1)},
	} {
		if _, err := r.svc.AdminSetLinePrice(ctx, c.ID, line.ID, in); !errors.Is(err, ErrOfferLinePriceInvalid) {
			t.Errorf("%s: %v, want the price refused", name, err)
		}
	}
	if _, err := r.svc.AdminSetLinePrice(ctx, c.ID, line.ID, SetLinePriceInput{UnitPrice: ptrI(500), Reason: strings.Repeat("é", MaxPriceReason)}); err != nil {
		t.Errorf("a note of exactly the most: %v", err)
	}

	custom, err := r.svc.AdminAddCustomLine(ctx, c.ID, CustomLineInput{Name: "Fitting", UnitPrice: 2500, Quantity: 1, By: &admin})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range custom.Lines {
		if !l.IsCustom {
			continue
		}
		if l.PriceSource != PriceAdmin || l.ListUnitPrice != nil || l.PriceSetBy == nil || *l.PriceSetBy != admin {
			t.Errorf("a custom line: %+v, want the admin's price with no catalog price behind it", l)
		}
	}
	if _, err := r.svc.AdminAddCustomLine(ctx, c.ID, CustomLineInput{Name: "Too much", UnitPrice: MaxLinePriceCents + 1, Quantity: 1}); !errors.Is(err, ErrOfferLinePriceInvalid) {
		t.Errorf("a custom line past the most: %v", err)
	}
}

func ptrI(v int64) *int64 { return &v }

// Picking a configured line's options again prices the new picks, but a price someone set on the line stays the
// price - it used to be overwritten while the line still said it was set
func TestPickingAgainKeepsASetPrice(t *testing.T) {
	optionID, gold, plain := uuid.New(), uuid.New(), uuid.New()
	r := newDBRig(t, catalogStub{
		prices: map[string]SimplePricing{"pen": {ProductName: "Pen", UnitPrice: 1000}},
		options: map[string]ProductOptionsResolution{
			"pen": {UnitPrice: 1000, Options: []ProductOptionConfig{{ID: optionID, InputType: "radio", Title: "Ink", Values: []ProductOptionValueConfig{
				{ID: gold, Title: "Gold", PriceValue: 150, PriceType: "fixed", Quantity: 1},
				{ID: plain, Title: "Plain", PriceValue: 20, PriceType: "fixed", Quantity: 1},
			}}}},
		},
	})
	ctx := context.Background()
	c := r.offer(t)
	pick := func(value uuid.UUID) CartLine {
		t.Helper()
		got, err := r.svc.AdminAddConfiguredToCart(ctx, c.ID, AddConfiguredToCartInput{ProductSku: "pen", Quantity: 1, Options: []CartLineOptionInput{{OptionID: optionID, ValueIDs: []uuid.UUID{value}}}})
		if err != nil {
			t.Fatal(err)
		}
		return lineBySku(t, got, "pen")
	}
	l := pick(gold)
	if l.UnitPrice != 1150 || l.PriceHeld() {
		t.Fatalf("as picked: %+v, want the catalog's 1150", l)
	}
	if _, err := r.svc.AdminSetLinePrice(ctx, c.ID, l.ID, SetLinePriceInput{UnitPrice: ptrI(900)}); err != nil {
		t.Fatal(err)
	}
	if l = pick(plain); !heldAt(l, 900, 1020) || len(l.Options) != 1 || l.Options[0].ValueLabel != "Plain" {
		t.Fatalf("picked again: %+v, want the new pick at the set 900, beside the 1020 the catalog asks for it", l)
	}
}
