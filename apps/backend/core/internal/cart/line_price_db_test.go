package cart

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

// A price set on a shopper's own cart: it needs a reason, may take no more off the catalog price than the store
// allows, is for no more of the item than the line held, and the shopper can still buy fewer, or none
func TestASetPriceOnAShoppersOwnCart(t *testing.T) {
	r := newDBRig(t, catalogStub{prices: map[string]SimplePricing{"sku": {ProductName: "Item", UnitPrice: 1000}}})
	ctx := context.Background()
	c := r.guestCart(t)
	c, err := r.svc.AddToCart(ctx, c.ID, nil, "sku", 2)
	if err != nil {
		t.Fatal(err)
	}
	line := c.Lines[0]
	admin, cut, until := uuid.New(), 20, time.Now().Add(48*time.Hour).Truncate(time.Second)
	set := func(cents int64, reason string) (Cart, error) {
		return r.svc.AdminSetLinePrice(ctx, c.ID, line.ID, SetLinePriceInput{UnitPrice: &cents, Reason: reason, By: &admin, MaxCutPercent: &cut, HoldsUntil: &until})
	}

	if _, err := set(900, "  "); !errors.Is(err, ErrLinePriceReasonRequired) {
		t.Fatalf("no reason: %v, want one asked for", err)
	}
	var floor *PriceFloorError
	if _, err := set(799, "price match"); !errors.As(err, &floor) || floor.FloorCents != 800 {
		t.Fatalf("a cent past the cut: %v, want it refused with the 800 that is the lowest", err)
	}
	got, err := set(800, "price match")
	if err != nil {
		t.Fatalf("the lowest price allowed: %v", err)
	}
	l := lineBySku(t, got, "sku")
	if !heldAt(l, 800, 1000) || l.PriceMaxQuantity == nil || *l.PriceMaxQuantity != 2 || l.PriceExpiresAt == nil || !l.PriceExpiresAt.Equal(until) {
		t.Fatalf("set on a shopper's cart: %+v, want 800 for up to 2 until %v", l, until)
	}

	// The shopper's own edits: fewer at the price, never more
	var over *PriceQuantityError
	if _, err := r.svc.UpdateCartLine(ctx, c.ID, line.ID, nil, 3); !errors.As(err, &over) || over.Max != 2 {
		t.Fatalf("a shopper raising the quantity: %v, want it refused at 2", err)
	}
	if _, err := r.svc.AddToCart(ctx, c.ID, nil, "sku", 1); !errors.Is(err, ErrLinePriceQuantity) {
		t.Fatalf("a shopper adding one more: %v, want it refused", err)
	}
	fewer, err := r.svc.UpdateCartLine(ctx, c.ID, line.ID, nil, 1)
	if err != nil || !heldAt(lineBySku(t, fewer, "sku"), 800, 1000) {
		t.Fatalf("a shopper buying fewer: %+v, %v - want 1 at the set price", fewer.Lines, err)
	}
	if back, err := r.svc.AddToCart(ctx, c.ID, nil, "sku", 1); err != nil || lineBySku(t, back, "sku").Quantity != 2 {
		t.Fatalf("a shopper back up to what the price is for: %v", err)
	}

	// An admin's quantity is one the price is for
	more, err := r.svc.AdminUpdateCartLine(ctx, c.ID, line.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if l = lineBySku(t, more, "sku"); !heldAt(l, 800, 1000) || *l.PriceMaxQuantity != 5 {
		t.Fatalf("an admin's quantity: %+v, want the price to be for 5 now", l)
	}
	if _, err := r.svc.UpdateCartLine(ctx, c.ID, line.ID, nil, 6); !errors.Is(err, ErrLinePriceQuantity) {
		t.Fatalf("a shopper past the admin's quantity: %v, want it refused", err)
	}

	// Past its expiry the price no longer holds, and the next reprice puts the line back on the catalog's
	past := time.Now().Add(-time.Minute)
	cents := int64(800)
	lapsed, err := r.svc.AdminSetLinePrice(ctx, c.ID, line.ID, SetLinePriceInput{UnitPrice: &cents, Reason: "price match", MaxCutPercent: &cut, HoldsUntil: &past})
	if err != nil {
		t.Fatal(err)
	}
	if l = lineBySku(t, lapsed, "sku"); l.PriceHeld() || !l.PriceSet() {
		t.Fatalf("a lapsed price: %+v, want it set but no longer holding", l)
	}
	if l = lineBySku(t, r.svc.RepriceCartLines(ctx, lapsed), "sku"); l.UnitPrice != 1000 || l.PriceSet() || l.ListUnitPrice != nil || l.PriceMaxQuantity != nil {
		t.Fatalf("repriced after it lapsed: %+v, want the catalog's 1000 and nothing kept", l)
	}
	if reread, err := r.svc.GetCart(ctx, c.ID, nil); err != nil || lineBySku(t, reread, "sku").PriceSource != PriceCatalog {
		t.Fatalf("the lifted price wasn't saved: %v", err)
	}
	if _, err := r.svc.RemoveFromCart(ctx, c.ID, line.ID, nil); err != nil {
		t.Fatalf("a shopper removing the line: %v", err)
	}
}

// A shopper who picks a product's options again is choosing something else than the price was set for
func TestAShopperPickingAgainGetsTheCatalogPrice(t *testing.T) {
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
	c := r.guestCart(t)
	pick := func(value uuid.UUID) CartLine {
		t.Helper()
		got, err := r.svc.AddConfiguredToCart(ctx, c.ID, nil, AddConfiguredToCartInput{ProductSku: "pen", Quantity: 1, Options: []CartLineOptionInput{{OptionID: optionID, ValueIDs: []uuid.UUID{value}}}})
		if err != nil {
			t.Fatal(err)
		}
		return lineBySku(t, got, "pen")
	}
	l := pick(plain)
	if _, err := r.svc.AdminSetLinePrice(ctx, c.ID, l.ID, SetLinePriceInput{UnitPrice: ptrI(600), Reason: "goodwill"}); err != nil {
		t.Fatal(err)
	}
	if l = pick(gold); l.PriceSet() || l.UnitPrice != 1150 || l.ListUnitPrice != nil || l.PriceReason != "" || l.PriceMaxQuantity != nil {
		t.Fatalf("picked again by the shopper: %+v, want the catalog's 1150 for the new pick", l)
	}
}

// A set price comes with the line when carts become one, and when a cart is copied - and the line never holds more
// than the price is for, however many the two carts had between them
func TestASetPriceFollowsItsLine(t *testing.T) {
	r := newDBRig(t, catalogStub{prices: map[string]SimplePricing{"sku": {ProductName: "Item", UnitPrice: 1000}, "other": {ProductName: "Other", UnitPrice: 500}}})
	ctx := context.Background()
	guest := r.guestCart(t)
	guest, err := r.svc.AddToCart(ctx, guest.ID, nil, "sku", 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AdminSetLinePrice(ctx, guest.ID, guest.Lines[0].ID, SetLinePriceInput{UnitPrice: ptrI(800), Reason: "price match"}); err != nil {
		t.Fatal(err)
	}

	copied, err := r.svc.AdminDuplicateCart(ctx, guest.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = r.conn.Exec(`DELETE FROM carts WHERE id = $1`, copied.ID) })
	if l := lineBySku(t, copied, "sku"); !heldAt(l, 800, 1000) || l.PriceReason != "price match" || *l.PriceMaxQuantity != 2 {
		t.Fatalf("a copied cart's line: %+v, want the set price with it", l)
	}

	// The customer already has three at the catalog price: signed in, the line is the two the price is for
	customerID := r.customer(t)
	own, err := r.svc.GetOrCreateCart(ctx, customerID, r.channel)
	if err != nil {
		t.Fatal(err)
	}
	for sku, qty := range map[string]float64{"sku": 3, "other": 1} {
		if _, err := r.svc.AddToCart(ctx, own.ID, &customerID, sku, qty); err != nil {
			t.Fatal(err)
		}
	}
	merged, err := r.svc.MergeGuestCart(ctx, customerID, guest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if l := lineBySku(t, merged, "sku"); !heldAt(l, 800, 1000) || l.Quantity != 2 {
		t.Fatalf("merged: %+v, want 2 at the set 800", l)
	}
	if l := lineBySku(t, merged, "other"); l.PriceSet() || l.Quantity != 1 {
		t.Fatalf("the line beside it: %+v, want it untouched", l)
	}
}
