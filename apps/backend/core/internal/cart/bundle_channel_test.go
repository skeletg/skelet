package cart

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type channelBundles struct {
	fakePricer
	snap      BundleSnapshot
	published map[uuid.UUID]bool
	unlisted  map[string]bool
}

func (b channelBundles) ResolveBundleBySKU(_ context.Context, channelID uuid.UUID, _ string) (BundleSnapshot, error) {
	if !b.published[channelID] {
		return BundleSnapshot{}, ErrProductNotPublished
	}
	return b.snap, nil
}

func (b channelBundles) ResolveSimplePricing(ctx context.Context, ch uuid.UUID, sku string, c *uuid.UUID) (SimplePricing, error) {
	if b.unlisted[sku] {
		return SimplePricing{}, ErrProductNotPublished
	}
	return b.fakePricer.ResolveSimplePricing(ctx, ch, sku, c)
}

func TestABundleOnlySellsWhereItAndItsComponentsAreListed(t *testing.T) {
	ctx := context.Background()
	home, other := uuid.New(), uuid.New()
	optID, selID := uuid.New(), uuid.New()
	snap := BundleSnapshot{ProductID: uuid.New(), Options: []BundleOptionSnapshot{{
		ID: optID, InputType: "checkbox",
		Selections: []BundleSelectionSnapshot{{ID: selID, ProductID: uuid.New(), ProductSKU: "comp", UnitPrice: 1000, QuantityPerBundle: 1}},
	}}}
	add := func(channelID uuid.UUID, pricer ProductResolver) error {
		cartID := uuid.New()
		repo := &mergeRepo{
			carts:        map[uuid.UUID]Cart{cartID: {ID: cartID, ChannelID: channelID, Status: "active"}},
			linesByCart:  map[uuid.UUID][]CartLine{},
			activeByCust: map[uuid.UUID]Cart{},
		}
		_, err := NewService(repo, nil, pricer).AddBundleToCart(ctx, cartID, nil, AddBundleToCartInput{
			ProductSku: "set", Quantity: 1, Selections: []AddBundleSelectionInput{{OptionID: optID, SelectionIDs: []uuid.UUID{selID}}},
		})
		return err
	}
	listed := channelBundles{prices: map[string]SimplePricing{"comp": {UnitPrice: 1200}}, snap: snap, published: map[uuid.UUID]bool{home: true}}

	if err := add(home, listed); err != nil {
		t.Fatalf("a listed bundle on its own channel: %v", err)
	}
	if err := add(other, listed); !errors.Is(err, ErrProductNotPublished) {
		t.Fatalf("a bundle not sold on this channel was added: %v", err)
	}
	unlistedComp := listed
	unlistedComp.unlisted = map[string]bool{"comp": true}
	if err := add(home, unlistedComp); !errors.Is(err, ErrProductNotPublished) {
		t.Fatalf("a component not sold on this channel was added at its catalog default: %v", err)
	}

	line := Cart{ChannelID: home, Lines: []CartLine{{ProductSku: "set", Components: []CartLineComponent{{ProductSku: "comp"}}}}}
	if err := NewService(&mergeRepo{}, nil, listed).ValidateBundleLines(ctx, line); err != nil {
		t.Fatalf("a still-listed bundle line failed placement: %v", err)
	}
	if err := NewService(&mergeRepo{}, nil, unlistedComp).ValidateBundleLines(ctx, line); !errors.Is(err, ErrProductNotPublished) {
		t.Fatalf("a bundle line whose component was pulled placed anyway: %v", err)
	}
	line.ChannelID = other
	if err := NewService(&mergeRepo{}, nil, listed).ValidateBundleLines(ctx, line); !errors.Is(err, ErrProductNotPublished) {
		t.Fatalf("a bundle line for a bundle pulled from the channel placed anyway: %v", err)
	}
}
