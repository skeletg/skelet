package cart

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/pricing"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// catalogStub prices SKUs, hands out bundle snapshots and option sets - a SKU with no options entry is a kind that never carries them
type catalogStub struct {
	prices  map[string]SimplePricing
	bundles map[string]BundleSnapshot
	options map[string]ProductOptionsResolution
}

func (c catalogStub) ResolveSimplePricing(_ context.Context, _ uuid.UUID, sku string, _ *uuid.UUID) (SimplePricing, error) {
	p, ok := c.prices[sku]
	if !ok {
		return SimplePricing{}, ErrProductNotPurchasable
	}
	return p, nil
}

func (c catalogStub) ResolveBundleBySKU(_ context.Context, _ uuid.UUID, sku string) (BundleSnapshot, error) {
	b, ok := c.bundles[sku]
	if !ok {
		return BundleSnapshot{}, ErrProductNotPublished
	}
	return b, nil
}

func (c catalogStub) ResolveProductOptions(_ context.Context, _ uuid.UUID, sku string, _ *uuid.UUID) (ProductOptionsResolution, error) {
	o, ok := c.options[sku]
	if !ok {
		return ProductOptionsResolution{}, ErrOptionsUnsupportedKind
	}
	return o, nil
}

type dbRig struct {
	conn    *sql.DB
	svc     *Service
	channel uuid.UUID
}

func newDBRig(t *testing.T, cat catalogStub) dbRig {
	t.Helper()
	conn := testdb.Require(t)
	return dbRig{conn: conn, svc: NewService(NewRepository(db.New(conn)), txrunner.New(conn), cat), channel: seedAbandonChannel(t, conn)}
}

func (r dbRig) guestCart(t *testing.T) Cart {
	t.Helper()
	c, err := r.svc.CreateGuestCart(context.Background(), r.channel)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = r.conn.Exec(`DELETE FROM carts WHERE id = $1`, c.ID) })
	return c
}

func (r dbRig) customer(t *testing.T) uuid.UUID {
	t.Helper()
	var groupID, id uuid.UUID
	if err := r.conn.QueryRow(`SELECT id FROM customer_groups LIMIT 1`).Scan(&groupID); err != nil {
		t.Skipf("no customer group to seed against: %v", err)
	}
	if err := r.conn.QueryRow(`INSERT INTO customers (email, password_hash, customer_group_id, channel_id) VALUES ($1, 'x', $2, $3) RETURNING id`,
		"cart-"+uuid.NewString()+"@example.invalid", groupID, r.channel).Scan(&id); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	t.Cleanup(func() { _, _ = r.conn.Exec(`DELETE FROM customers WHERE id = $1`, id) })
	return id
}

func lineBySku(t *testing.T, c Cart, sku string) CartLine {
	t.Helper()
	for _, l := range c.Lines {
		if l.ProductSku == sku {
			return l
		}
	}
	t.Fatalf("no %s line in %+v", sku, c.Lines)
	return CartLine{}
}

func ptrF(f float64) *float64 { return &f }

func tiersAt(minQty float64, price int64) []pricing.Tier {
	return []pricing.Tier{{MinQty: minQty, Price: price}}
}

// A two-mug set: one required radio of mugs (x2 per set) and an optional checkbox of coasters (x4 per set)
func mugSet() (BundleSnapshot, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) {
	mugs, coasters, mugSel, coasterSel := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	return BundleSnapshot{ProductID: uuid.New(), ProductName: "Mug Set", Options: []BundleOptionSnapshot{
		{ID: mugs, InputType: "radio", IsRequired: true, Selections: []BundleSelectionSnapshot{
			{ID: mugSel, ProductID: uuid.New(), ProductSKU: "mug", UnitPrice: 1000, QuantityPerBundle: 2},
		}},
		{ID: coasters, InputType: "checkbox", Selections: []BundleSelectionSnapshot{
			{ID: coasterSel, ProductID: uuid.New(), ProductSKU: "coaster", UnitPrice: 200, QuantityPerBundle: 4},
		}},
	}}, mugs, coasters, mugSel, coasterSel
}

// Reservation reads component rows as line totals, so a quantity edit that left them alone reserved one set's worth for five
func TestABundleLineQuantityEditScalesItsComponents(t *testing.T) {
	snap, mugs, coasters, mugSel, coasterSel := mugSet()
	r := newDBRig(t, catalogStub{bundles: map[string]BundleSnapshot{"set": snap}, prices: map[string]SimplePricing{"mug": {UnitPrice: 1000}, "coaster": {UnitPrice: 200}}})
	ctx := context.Background()
	c := r.guestCart(t)
	added, err := r.svc.AddBundleToCart(ctx, c.ID, nil, AddBundleToCartInput{ProductSku: "set", Quantity: 1, Selections: []AddBundleSelectionInput{
		{OptionID: mugs, SelectionIDs: []uuid.UUID{mugSel}}, {OptionID: coasters, SelectionIDs: []uuid.UUID{coasterSel}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	line := lineBySku(t, added, "set")

	check := func(c Cart, qty float64) {
		t.Helper()
		l := lineBySku(t, c, "set")
		if l.Quantity != qty {
			t.Fatalf("line quantity %v, want %v", l.Quantity, qty)
		}
		want := map[string]float64{"mug": 2 * qty, "coaster": 4 * qty}
		for _, comp := range l.Components {
			if comp.Quantity != want[comp.ProductSku] {
				t.Errorf("at %v sets %s holds %v, want %v", qty, comp.ProductSku, comp.Quantity, want[comp.ProductSku])
			}
		}
	}
	check(added, 1)
	updated, err := r.svc.UpdateCartLine(ctx, c.ID, line.ID, nil, 5)
	if err != nil {
		t.Fatal(err)
	}
	check(updated, 5)
	updated, err = r.svc.AdminUpdateCartLine(ctx, c.ID, line.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	check(updated, 3)
}

// The hook's quantity is the one written, so the components must be built from it rather than from the request
func TestABundleHookQuantityReachesTheComponents(t *testing.T) {
	snap, mugs, _, mugSel, _ := mugSet()
	r := newDBRig(t, catalogStub{bundles: map[string]BundleSnapshot{"set": snap}, prices: map[string]SimplePricing{"mug": {UnitPrice: 1000}}})
	bus := events.NewBus()
	bus.SubscribeSync(EventBundleAdding, 0, func(_ context.Context, e events.Event) error {
		return e.(*BundleAdding).SetLineQuantity(3)
	})
	r.svc.SetEventBus(bus)
	c := r.guestCart(t)
	got, err := r.svc.AddBundleToCart(context.Background(), c.ID, nil, AddBundleToCartInput{ProductSku: "set", Quantity: 1, Selections: []AddBundleSelectionInput{{OptionID: mugs, SelectionIDs: []uuid.UUID{mugSel}}}})
	if err != nil {
		t.Fatal(err)
	}
	l := lineBySku(t, got, "set")
	if l.Quantity != 3 || len(l.Components) != 1 || l.Components[0].Quantity != 6 {
		t.Fatalf("line %v with components %+v, want 3 sets holding 6 mugs", l.Quantity, l.Components)
	}
}

// The flag is what keeps a hook's price at checkout now that a discounted bundle no longer equals its components' sum - it has to survive the round trip through the line row
func TestABundleHookPriceIsStoredAndKeptAtReprice(t *testing.T) {
	snap, mugs, _, mugSel, _ := mugSet()
	snap.Pricing, snap.DiscountPercent = "sum_minus_percent", 10
	prices := map[string]SimplePricing{"mug": {UnitPrice: 1000}}
	r := newDBRig(t, catalogStub{bundles: map[string]BundleSnapshot{"set": snap}, prices: prices})
	bus := events.NewBus()
	bus.SubscribeSync(EventBundleAdding, 0, func(_ context.Context, e events.Event) error {
		return e.(*BundleAdding).SetLineUnitPrice(1500)
	})
	r.svc.SetEventBus(bus)
	ctx := context.Background()
	c := r.guestCart(t)
	if _, err := r.svc.AddBundleToCart(ctx, c.ID, nil, AddBundleToCartInput{ProductSku: "set", Quantity: 1, Selections: []AddBundleSelectionInput{{OptionID: mugs, SelectionIDs: []uuid.UUID{mugSel}}}}); err != nil {
		t.Fatal(err)
	}
	prices["mug"] = SimplePricing{UnitPrice: 1200}
	stored, err := r.svc.GetCart(ctx, c.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if l := lineBySku(t, stored, "set"); !l.BundleHookPriced || l.UnitPrice != 1500 {
		t.Fatalf("stored line %d, hook-priced %v - want the hook's 1500, flagged", l.UnitPrice, l.BundleHookPriced)
	}
	l := lineBySku(t, r.svc.RepriceCartLines(ctx, stored), "set")
	if l.UnitPrice != 1500 || l.Components[0].UnitPrice != 1200 {
		t.Fatalf("repriced line %d, component %d - want the hook's 1500 kept and the component refreshed to 1200", l.UnitPrice, l.Components[0].UnitPrice)
	}
}

// Line, component delete and component inserts are separate statements - a failure part-way used to leave a full-priced line short of components
func TestAFailedBundleAddLeavesNoHalfWrittenLine(t *testing.T) {
	snap, mugs, coasters, mugSel, coasterSel := mugSet()
	snap.Options[1].Selections[0].QuantityPerBundle = 0 // the second component insert trips the quantity > 0 check
	r := newDBRig(t, catalogStub{bundles: map[string]BundleSnapshot{"set": snap}, prices: map[string]SimplePricing{"mug": {UnitPrice: 1000}, "coaster": {UnitPrice: 200}}})
	ctx := context.Background()
	c := r.guestCart(t)
	_, err := r.svc.AddBundleToCart(ctx, c.ID, nil, AddBundleToCartInput{ProductSku: "set", Quantity: 1, Selections: []AddBundleSelectionInput{
		{OptionID: mugs, SelectionIDs: []uuid.UUID{mugSel}}, {OptionID: coasters, SelectionIDs: []uuid.UUID{coasterSel}},
	}})
	if err == nil {
		t.Fatal("a zero-quantity component was stored")
	}
	var n int
	if err := r.conn.QueryRow(`SELECT count(*) FROM cart_lines WHERE cart_id = $1`, c.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d lines survived the failed add, want the whole add rolled back", n)
	}
}

func TestAPlainAddChecksWhatTheLineWillHold(t *testing.T) {
	optionID := uuid.New()
	r := newDBRig(t, catalogStub{
		prices: map[string]SimplePricing{
			"tee":  {ProductName: "Tee", UnitPrice: 2500, MaxQty: ptrF(5)},
			"mug":  {ProductName: "Mug", UnitPrice: 1000},
			"sign": {ProductName: "Sign", UnitPrice: 3000},
		},
		options: map[string]ProductOptionsResolution{
			"mug":  {UnitPrice: 1000, Options: []ProductOptionConfig{{ID: optionID, InputType: "field", Title: "Name"}}},
			"sign": {UnitPrice: 3000, Options: []ProductOptionConfig{{ID: uuid.New(), InputType: "field", Title: "Text", IsRequired: true}}},
		},
	})
	ctx := context.Background()
	c := r.guestCart(t)

	got, err := r.svc.AddToCart(ctx, c.ID, nil, "tee", 4)
	if err != nil {
		t.Fatal(err)
	}
	if l := lineBySku(t, got, "tee"); l.UnitPrice != 2500 || l.ProductName != "Tee" {
		t.Errorf("line priced %d named %q, want the catalog's 2500 Tee", l.UnitPrice, l.ProductName)
	}
	if _, err := r.svc.AddToCart(ctx, c.ID, nil, "tee", 2); !errors.Is(err, ErrQuantityOutOfBounds) {
		t.Errorf("4 + 2 past a max of 5: %v, want ErrQuantityOutOfBounds", err)
	}
	if _, err := r.svc.AddToCart(ctx, c.ID, nil, "sign", 1); !errors.Is(err, ErrProductOptionsRequired) {
		t.Errorf("a required-options product added plainly: %v", err)
	}
	if _, err := r.svc.AddConfiguredToCart(ctx, c.ID, nil, AddConfiguredToCartInput{ProductSku: "mug", Quantity: 1, Options: []CartLineOptionInput{{OptionID: optionID, TextValue: "Ana"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AddToCart(ctx, c.ID, nil, "mug", 1); !errors.Is(err, ErrLineConfigured) {
		t.Errorf("a plain add onto a configured line: %v, want ErrLineConfigured", err)
	}
	for _, q := range []float64{0, -3, 0.00001, 2e9} {
		if _, err := r.svc.AddToCart(ctx, c.ID, nil, "tee", q); !errors.Is(err, ErrQuantityInvalid) {
			t.Errorf("AddToCart(%v): %v, want ErrQuantityInvalid", q, err)
		}
	}
	tee := lineBySku(t, got, "tee")
	if _, err := r.svc.UpdateCartLine(ctx, c.ID, tee.ID, nil, 0); !errors.Is(err, ErrQuantityInvalid) {
		t.Errorf("UpdateCartLine(0): %v, want ErrQuantityInvalid", err)
	}
	final, err := r.svc.GetCart(ctx, c.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if l := lineBySku(t, final, "tee"); l.Quantity != 4 {
		t.Errorf("tee holds %v after the refused adds, want 4", l.Quantity)
	}
}

// Crossing a tier moves the base price either way, and a configured line keeps its option surcharge on top
func TestAQuantityEditRepricesAcrossTiersAndKeepsSurcharges(t *testing.T) {
	optionID, valueID, forcedValue := uuid.New(), uuid.New(), uuid.New()
	tiered := SimplePricing{UnitPrice: 1000, Tiers: tiersAt(10, 800)}
	r := newDBRig(t, catalogStub{
		prices: map[string]SimplePricing{"pen": tiered, "set": {UnitPrice: 500}},
		options: map[string]ProductOptionsResolution{
			"pen": {UnitPrice: 1000, Options: []ProductOptionConfig{{ID: optionID, InputType: "radio", Title: "Ink",
				Values: []ProductOptionValueConfig{{ID: valueID, Title: "Gold", PriceValue: 150, PriceType: "fixed", Quantity: 1}}}}},
			"set": {UnitPrice: 500, Options: []ProductOptionConfig{{ID: optionID, InputType: "radio", Title: "Pack",
				Values: []ProductOptionValueConfig{{ID: forcedValue, Title: "Four", Quantity: 1, ForcedLineQty: ptrF(4)}}}}},
		},
	})
	ctx := context.Background()
	c := r.guestCart(t)
	got, err := r.svc.AddConfiguredToCart(ctx, c.ID, nil, AddConfiguredToCartInput{ProductSku: "pen", Quantity: 1, Options: []CartLineOptionInput{{OptionID: optionID, ValueIDs: []uuid.UUID{valueID}}}})
	if err != nil {
		t.Fatal(err)
	}
	pen := lineBySku(t, got, "pen")
	for _, step := range []struct {
		qty  float64
		want int64
	}{{10, 950}, {2, 1150}} {
		got, err = r.svc.UpdateCartLine(ctx, c.ID, pen.ID, nil, step.qty)
		if err != nil {
			t.Fatal(err)
		}
		if l := lineBySku(t, got, "pen"); l.UnitPrice != step.want {
			t.Errorf("at %v the unit is %d, want %d", step.qty, l.UnitPrice, step.want)
		}
	}
	got, err = r.svc.AddConfiguredToCart(ctx, c.ID, nil, AddConfiguredToCartInput{ProductSku: "set", Quantity: 1, Options: []CartLineOptionInput{{OptionID: optionID, ValueIDs: []uuid.UUID{forcedValue}}}})
	if err != nil {
		t.Fatal(err)
	}
	set := lineBySku(t, got, "set")
	if set.Quantity != 4 {
		t.Fatalf("a Four pack holds %v", set.Quantity)
	}
	if _, err := r.svc.UpdateCartLine(ctx, c.ID, set.ID, nil, 5); !errors.Is(err, ErrProductOptionQtyForced) {
		t.Errorf("a forced quantity edited: %v", err)
	}
}

// The break is read against the line's prospective total, on a plain add and a configured one alike
func TestAddsPriceAtTheTierTheLineReaches(t *testing.T) {
	optionID, valueID := uuid.New(), uuid.New()
	r := newDBRig(t, catalogStub{
		prices: map[string]SimplePricing{"pen": {UnitPrice: 1000, Tiers: tiersAt(10, 800)}},
		options: map[string]ProductOptionsResolution{
			"pen": {UnitPrice: 1000, Options: []ProductOptionConfig{{ID: optionID, InputType: "radio", Title: "Ink",
				Values: []ProductOptionValueConfig{{ID: valueID, Title: "Gold", PriceValue: 150, PriceType: "fixed", Quantity: 1}}}}},
		},
	})
	ctx := context.Background()
	c := r.guestCart(t)
	if _, err := r.svc.AddToCart(ctx, c.ID, nil, "pen", 9); err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.AddToCart(ctx, c.ID, nil, "pen", 1)
	if err != nil {
		t.Fatal(err)
	}
	if l := lineBySku(t, got, "pen"); l.UnitPrice != 800 {
		t.Errorf("9 + 1 pens priced at %d, want the 10-unit break 800", l.UnitPrice)
	}

	c2 := r.guestCart(t)
	got, err = r.svc.AddConfiguredToCart(ctx, c2.ID, nil, AddConfiguredToCartInput{ProductSku: "pen", Quantity: 10, Options: []CartLineOptionInput{{OptionID: optionID, ValueIDs: []uuid.UUID{valueID}}}})
	if err != nil {
		t.Fatal(err)
	}
	if l := lineBySku(t, got, "pen"); l.UnitPrice != 950 {
		t.Errorf("10 gold pens priced at %d, want the break plus the surcharge, 950", l.UnitPrice)
	}
}

// The SDK setters and a sync webhook swap the event's pointer rather than write through it - read back from the event, their override used to vanish
func TestALineAddingOverrideReachesTheLine(t *testing.T) {
	r := newDBRig(t, catalogStub{prices: map[string]SimplePricing{"tee": {UnitPrice: 2500}}})
	bus := events.NewBus()
	bus.SubscribeSync(EventLineAdding, 0, func(_ context.Context, e events.Event) error {
		ev := e.(*LineAdding)
		if err := ev.SetLineUnitPrice(1999); err != nil {
			return err
		}
		_, _, err := ev.ApplyWebhookResponse([]byte(`{"quantity": 3}`))
		return err
	})
	r.svc.SetEventBus(bus)
	c := r.guestCart(t)
	got, err := r.svc.AddToCart(context.Background(), c.ID, nil, "tee", 1)
	if err != nil {
		t.Fatal(err)
	}
	if l := lineBySku(t, got, "tee"); l.UnitPrice != 1999 || l.Quantity != 3 {
		t.Fatalf("line at %d x %v, want the hook's 1999 x 3", l.UnitPrice, l.Quantity)
	}
}

// A hook that moves the quantity doesn't get to move it past the bounds the request had to pass
func TestAHookQuantityOnAnEditIsCheckedToo(t *testing.T) {
	r := newDBRig(t, catalogStub{prices: map[string]SimplePricing{"tee": {UnitPrice: 2500, MaxQty: ptrF(5)}}})
	bus := events.NewBus()
	bus.SubscribeSync(EventLineUpdating, 0, func(_ context.Context, e events.Event) error {
		return e.(*LineUpdating).SetLineQuantity(9)
	})
	ctx := context.Background()
	c := r.guestCart(t)
	got, err := r.svc.AddToCart(ctx, c.ID, nil, "tee", 1)
	if err != nil {
		t.Fatal(err)
	}
	r.svc.SetEventBus(bus)
	if _, err := r.svc.UpdateCartLine(ctx, c.ID, lineBySku(t, got, "tee").ID, nil, 2); !errors.Is(err, ErrQuantityOutOfBounds) {
		t.Fatalf("a hook pushed the line past its max: %v", err)
	}
}

func TestMovingASavedItemTwiceAddsItOnce(t *testing.T) {
	r := newDBRig(t, catalogStub{prices: map[string]SimplePricing{"tee": {ProductName: "Tee", UnitPrice: 2500}}})
	ctx := context.Background()
	c := r.guestCart(t)
	got, err := r.svc.AddToCart(ctx, c.ID, nil, "tee", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got, err = r.svc.SaveForLater(ctx, c.ID, lineBySku(t, got, "tee").ID, nil); err != nil || len(got.Lines) != 0 {
		t.Fatalf("save for later left %d lines: %v", len(got.Lines), err)
	}
	saved, err := r.svc.ListSavedItems(ctx, c.ID, nil)
	if err != nil || len(saved) != 1 {
		t.Fatalf("saved %v: %v", saved, err)
	}
	if _, err := r.svc.MoveSavedToCart(ctx, c.ID, saved[0].ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.MoveSavedToCart(ctx, c.ID, saved[0].ID, nil); !errors.Is(err, ErrSavedItemNotFound) {
		t.Fatalf("a second move of the same item: %v, want ErrSavedItemNotFound", err)
	}
	final, err := r.svc.GetCart(ctx, c.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if l := lineBySku(t, final, "tee"); l.Quantity != 2 {
		t.Errorf("tee holds %v, want the 2 saved", l.Quantity)
	}
}

// A refused add must not eat the saved item it claimed
func TestARefusedMovePutsTheSavedItemBack(t *testing.T) {
	cat := catalogStub{prices: map[string]SimplePricing{"tee": {ProductName: "Tee", UnitPrice: 2500}}}
	r := newDBRig(t, cat)
	ctx := context.Background()
	c := r.guestCart(t)
	if _, err := r.svc.repo.SaveItem(ctx, c.ID, "gone", "Gone", 1); err != nil {
		t.Fatal(err)
	}
	saved, _ := r.svc.ListSavedItems(ctx, c.ID, nil)
	if _, err := r.svc.MoveSavedToCart(ctx, c.ID, saved[0].ID, nil); err == nil {
		t.Fatal("an unsellable saved item moved into the cart")
	}
	if again, _ := r.svc.ListSavedItems(ctx, c.ID, nil); len(again) != 1 || again[0].ProductSku != "gone" {
		t.Fatalf("saved items after the refused move: %+v", again)
	}
}

func TestDuplicatingACart(t *testing.T) {
	optionID := uuid.New()
	r := newDBRig(t, catalogStub{
		prices:  map[string]SimplePricing{"mug": {ProductName: "Mug", UnitPrice: 1000}, "tee": {ProductName: "Tee", UnitPrice: 2500}},
		options: map[string]ProductOptionsResolution{"mug": {UnitPrice: 1000, Options: []ProductOptionConfig{{ID: optionID, InputType: "field", Title: "Name"}}}},
	})
	ctx := context.Background()
	src := r.guestCart(t)
	if _, err := r.svc.AddToCart(ctx, src.ID, nil, "mug", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AddToCart(ctx, src.ID, nil, "tee", 1); err != nil {
		t.Fatal(err)
	}
	customerID := r.customer(t)
	dest, err := r.svc.GetOrCreateCart(ctx, customerID, r.channel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AddConfiguredToCart(ctx, dest.ID, &customerID, AddConfiguredToCartInput{ProductSku: "mug", Quantity: 1, Options: []CartLineOptionInput{{OptionID: optionID, TextValue: "Ana"}}}); err != nil {
		t.Fatal(err)
	}

	got, err := r.svc.AdminDuplicateCart(ctx, src.ID, &customerID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != dest.ID {
		t.Fatalf("duplicated onto %s, want the customer's active cart %s", got.ID, dest.ID)
	}
	if mug := lineBySku(t, got, "mug"); mug.Quantity != 1 || len(mug.Options) != 1 {
		t.Errorf("the customer's configured mug became %v with %d options - a plain copy summed onto it", mug.Quantity, len(mug.Options))
	}
	if tee := lineBySku(t, got, "tee"); tee.Quantity != 1 {
		t.Errorf("tee copied as %v", tee.Quantity)
	}
	if _, err := r.svc.AdminDuplicateCart(ctx, dest.ID, &customerID); !errors.Is(err, ErrDuplicateOntoSource) {
		t.Fatalf("a cart duplicated onto itself: %v", err)
	}
	again, _ := r.svc.GetCart(ctx, dest.ID, &customerID)
	if tee := lineBySku(t, again, "tee"); tee.Quantity != 1 {
		t.Errorf("the refused self-duplicate still doubled tee to %v", tee.Quantity)
	}
	if _, err := r.svc.AdminDuplicateCart(ctx, uuid.New(), nil); !errors.Is(err, ErrCartNotFound) {
		t.Errorf("duplicating a missing cart: %v, want ErrCartNotFound", err)
	}
}

// Two carts on one channel, three lines in the first - the page is cut before the lines are summed, so the page must still carry the right totals
func TestTheAdminCartListSumsEachCartsLines(t *testing.T) {
	r := newDBRig(t, catalogStub{prices: map[string]SimplePricing{"a": {UnitPrice: 250}, "b": {UnitPrice: 1000}}})
	ctx := context.Background()
	full, empty := r.guestCart(t), r.guestCart(t)
	for sku, qty := range map[string]float64{"a": 2, "b": 3} {
		if _, err := r.svc.AddToCart(ctx, full.ID, nil, sku, qty); err != nil {
			t.Fatal(err)
		}
	}
	items, err := r.svc.repo.ListCarts(ctx, CartFilter{ChannelID: r.channel, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	got := map[uuid.UUID]CartListItem{}
	for _, it := range items {
		got[it.ID] = it
	}
	if f := got[full.ID]; f.ItemCount != 2 || f.Subtotal != 3500 {
		t.Errorf("full cart listed %d items at %d, want 2 at 3500", f.ItemCount, f.Subtotal)
	}
	if e, ok := got[empty.ID]; !ok || e.ItemCount != 0 || e.Subtotal != 0 {
		t.Errorf("empty cart listed as %+v (present %v)", e, ok)
	}
	page, err := r.svc.repo.ListCarts(ctx, CartFilter{ChannelID: r.channel, Limit: 1})
	if err != nil || len(page) != 1 || page[0] != got[page[0].ID] {
		t.Fatalf("a one-cart page gave %v (%v), want one cart with the totals the full list had", page, err)
	}
}

func TestTwoTabsGetTheSameCustomerCart(t *testing.T) {
	r := newDBRig(t, catalogStub{})
	customerID := r.customer(t)
	ctx := context.Background()
	carts := make(chan uuid.UUID, 8)
	errs := make(chan error, 8)
	for range 8 {
		go func() {
			c, err := r.svc.GetOrCreateCart(ctx, customerID, r.channel)
			if err != nil {
				errs <- err
				return
			}
			carts <- c.ID
		}()
	}
	seen := map[uuid.UUID]bool{}
	for range 8 {
		select {
		case err := <-errs:
			t.Fatalf("a concurrent first visit failed: %v", err)
		case id := <-carts:
			seen[id] = true
		}
	}
	if len(seen) != 1 {
		t.Fatalf("%d carts for one customer, want one: %v", len(seen), fmt.Sprint(seen))
	}
}

// What this pins: one bundle in two different mixes is two lines, and re-adding a mix rewrites its own line - the other
// keeps its picks, and repricing touches each in place
func TestOneBundleInSeveralConfigurations(t *testing.T) {
	snap, mugs, coasters, mugSel, coasterSel := mugSet()
	r := newDBRig(t, catalogStub{bundles: map[string]BundleSnapshot{"set": snap}, prices: map[string]SimplePricing{"mug": {UnitPrice: 1000}, "coaster": {UnitPrice: 200}}})
	ctx := context.Background()
	c := r.guestCart(t)
	plain := []AddBundleSelectionInput{{OptionID: mugs, SelectionIDs: []uuid.UUID{mugSel}}, {OptionID: coasters}}
	full := []AddBundleSelectionInput{{OptionID: mugs, SelectionIDs: []uuid.UUID{mugSel}}, {OptionID: coasters, SelectionIDs: []uuid.UUID{coasterSel}}}
	add := func(sels []AddBundleSelectionInput, qty float64) Cart {
		t.Helper()
		got, err := r.svc.AddBundleToCart(ctx, c.ID, nil, AddBundleToCartInput{ProductSku: "set", Quantity: qty, Selections: sels})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	add(plain, 1)
	got := add(full, 2)
	if len(got.Lines) != 2 {
		t.Fatalf("%d lines, want the two mixes apart", len(got.Lines))
	}
	got = add(plain, 3)
	byComps := map[int]float64{}
	for _, l := range got.Lines {
		byComps[len(l.Components)] = l.Quantity
	}
	if len(got.Lines) != 2 || byComps[1] != 3 || byComps[2] != 2 {
		t.Fatalf("lines by component count %v, want the mug-only mix at 3 and the full mix still at 2", byComps)
	}
	if again := r.svc.RepriceCartLines(ctx, got); len(again.Lines) != 2 {
		t.Fatalf("repricing left %d lines, want 2", len(again.Lines))
	}
}
