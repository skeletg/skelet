package cart

import (
	"context"
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
