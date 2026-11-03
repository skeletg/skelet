package cart

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type bundlePricer struct { // fixed bundle snapshot plus per-SKU channel pricing
	fakePricer
	snap BundleSnapshot
}

func (b bundlePricer) ResolveBundleBySKU(context.Context, uuid.UUID, string) (BundleSnapshot, error) {
	return b.snap, nil
}

// AddBundleToCart must price components through the channel pricing stack, not the snapshot's catalog defaults - listing overrides and specials are what the shopper was shown, and what the bundle line has to charge
func TestAddBundleToCartUsesChannelPricing(t *testing.T) {
	cartID, channelID := uuid.New(), uuid.New()
	optID, selPricedID, selFallbackID := uuid.New(), uuid.New(), uuid.New()

	snap := BundleSnapshot{
		ProductID:   uuid.New(),
		ProductName: "Starter Set",
		Options: []BundleOptionSnapshot{{
			ID:        optID,
			InputType: "checkbox",
			Selections: []BundleSelectionSnapshot{
				{ID: selPricedID, ProductID: uuid.New(), ProductSKU: "comp-priced", UnitPrice: 1000, QuantityPerBundle: 2}, // channel listing moves this one 1000 -> 1200
				{ID: selFallbackID, ProductID: uuid.New(), ProductSKU: "comp-ghost", UnitPrice: 700, QuantityPerBundle: 1}, // no resolvable pricing - the snapshot default must survive
			},
		}},
	}
	repo := &mergeRepo{
		carts: map[uuid.UUID]Cart{
			cartID: {ID: cartID, ChannelID: channelID, Status: "active"},
		},
		linesByCart:  map[uuid.UUID][]CartLine{},
		activeByCust: map[uuid.UUID]Cart{},
	}
	pricer := bundlePricer{
		prices: map[string]SimplePricing{
			"comp-priced": {UnitPrice: 1200},
		},
		snap: snap,
	}
	svc := NewService(repo, nil, pricer)

	_, err := svc.AddBundleToCart(context.Background(), cartID, nil, AddBundleToCartInput{
		ProductSku: "starter-set",
		Quantity:   1,
		Selections: []AddBundleSelectionInput{
			{OptionID: optID, SelectionIDs: []uuid.UUID{selPricedID, selFallbackID}},
		},
	})
	if err != nil {
		t.Fatalf("AddBundleToCart: %v", err)
	}
	if len(repo.bundles) != 1 {
		t.Fatalf("expected 1 bundle upsert, got %d", len(repo.bundles))
	}
	b := repo.bundles[0]
	if b.UnitPrice != 3100 { // 2 x 1200 (channel price) + 1 x 700 (snapshot fallback)
		t.Fatalf("bundle unit price = %d, want 3100 (channel-priced components)", b.UnitPrice)
	}
	prices := map[string]int64{}
	for _, c := range b.Components {
		prices[c.ProductSku] = c.UnitPrice
	}
	if prices["comp-priced"] != 1200 {
		t.Fatalf("priced component = %d, want the 1200 channel price", prices["comp-priced"])
	}
	if prices["comp-ghost"] != 700 {
		t.Fatalf("unresolvable component = %d, want the 700 snapshot fallback", prices["comp-ghost"])
	}
}

// What this pins: each pricing mode turns the picks' sum into the bundle's price - the percent off the whole, rounded half up
func TestBundleUnitPriceModes(t *testing.T) {
	for _, c := range []struct {
		name string
		snap BundleSnapshot
		sum  int64
		want int64
	}{
		{"no mode is the sum", BundleSnapshot{}, 3100, 3100},
		{"sum", BundleSnapshot{Pricing: "sum", DiscountPercent: 10}, 3100, 3100},
		{"fixed ignores the picks", BundleSnapshot{Pricing: "fixed", FixedPrice: 2500}, 3100, 2500},
		{"10% off", BundleSnapshot{Pricing: "sum_minus_percent", DiscountPercent: 10}, 3100, 2790},
		{"a half cent of discount rounds up", BundleSnapshot{Pricing: "sum_minus_percent", DiscountPercent: 15}, 1010, 858}, // 151.5 off -> 152
		{"100% off is free", BundleSnapshot{Pricing: "sum_minus_percent", DiscountPercent: 100}, 999, 0},
	} {
		if got := c.snap.UnitPrice(c.sum); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

// What this pins: add-to-cart charges the discounted price, and a mode's price is not mistaken for a hook's
func TestAddBundleToCartAppliesThePricingMode(t *testing.T) {
	cartID, optID, selID := uuid.New(), uuid.New(), uuid.New()
	repo := &mergeRepo{carts: map[uuid.UUID]Cart{cartID: {ID: cartID, ChannelID: uuid.New(), Status: "active"}}, linesByCart: map[uuid.UUID][]CartLine{}, activeByCust: map[uuid.UUID]Cart{}}
	snap := BundleSnapshot{ProductID: uuid.New(), Pricing: "sum_minus_percent", DiscountPercent: 10, Options: []BundleOptionSnapshot{{ID: optID, InputType: "radio", IsRequired: true,
		Selections: []BundleSelectionSnapshot{{ID: selID, ProductID: uuid.New(), ProductSKU: "mug", QuantityPerBundle: 2}}}}}
	svc := NewService(repo, nil, bundlePricer{prices: map[string]SimplePricing{"mug": {UnitPrice: 1000}}, snap: snap})
	if _, err := svc.AddBundleToCart(context.Background(), cartID, nil, AddBundleToCartInput{ProductSku: "set", Quantity: 1, Selections: []AddBundleSelectionInput{{OptionID: optID, SelectionIDs: []uuid.UUID{selID}}}}); err != nil {
		t.Fatal(err)
	}
	if b := repo.bundles[0]; b.UnitPrice != 1800 || b.HookPriced {
		t.Fatalf("price %d, hook-priced %v - want 2000 less 10%% = 1800, not hook-priced", b.UnitPrice, b.HookPriced)
	}
}

// What this pins: an add-on deal comes off its component's price, and a customer-quantity selection takes the shopper's
// amount while every other selection ignores one
func TestBundleSelectionDealsAndCustomerQuantities(t *testing.T) {
	cartID, optID, mugSel, teaSel := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := &mergeRepo{carts: map[uuid.UUID]Cart{cartID: {ID: cartID, ChannelID: uuid.New(), Status: "active"}}, linesByCart: map[uuid.UUID][]CartLine{}, activeByCust: map[uuid.UUID]Cart{}}
	snap := BundleSnapshot{ProductID: uuid.New(), Options: []BundleOptionSnapshot{{ID: optID, InputType: "checkbox", IsRequired: true, Selections: []BundleSelectionSnapshot{
		{ID: mugSel, ProductID: uuid.New(), ProductSKU: "mug", QuantityPerBundle: 1, DiscountPercent: 25},
		{ID: teaSel, ProductID: uuid.New(), ProductSKU: "tea", QuantityPerBundle: 1, CustomerQuantity: true},
	}}}}
	svc := NewService(repo, nil, bundlePricer{prices: map[string]SimplePricing{"mug": {UnitPrice: 1000}, "tea": {UnitPrice: 300}}, snap: snap})
	add := func(q map[uuid.UUID]float64) error {
		_, err := svc.AddBundleToCart(context.Background(), cartID, nil, AddBundleToCartInput{
			ProductSku: "set", Quantity: 2, Selections: []AddBundleSelectionInput{{OptionID: optID, SelectionIDs: []uuid.UUID{mugSel, teaSel}}}, Quantities: q,
		})
		return err
	}
	if err := add(map[uuid.UUID]float64{teaSel: 3, mugSel: 5}); err != nil {
		t.Fatal(err)
	}
	b := repo.bundles[len(repo.bundles)-1]
	if b.UnitPrice != 750+900 { // the mug at 25% off, three teas - the mug's 5 is ignored, it isn't the customer's to set
		t.Fatalf("unit price %d, want 750 + 3 x 300 = 1650", b.UnitPrice)
	}
	qty := map[string]float64{}
	for _, c := range b.Components {
		qty[c.ProductSku] = c.Quantity
	}
	if qty["mug"] != 2 || qty["tea"] != 6 {
		t.Fatalf("component quantities %v, want 2 mugs and 6 teas for two bundles", qty)
	}
	if err := add(map[uuid.UUID]float64{teaSel: 0}); !errors.Is(err, ErrBundleSelectionInvalid) {
		t.Fatalf("zero teas = %v, want ErrBundleSelectionInvalid", err)
	}
}
