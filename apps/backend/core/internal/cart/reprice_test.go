package cart

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/pricing"
)

// repriceRepo records UpdateCartLineQuantityPrice writes over the inert fakeRepo
type repriceRepo struct {
	fakeRepo
	writes map[uuid.UUID]int64 // line id -> persisted unit price
	fail   bool
}

func (r *repriceRepo) UpdateCartLineQuantityPrice(_ context.Context, lineID uuid.UUID, _ float64, unitPrice int64) (CartLine, error) {
	if r.fail {
		return CartLine{}, fmt.Errorf("db down")
	}
	if r.writes == nil {
		r.writes = map[uuid.UUID]int64{}
	}
	r.writes[lineID] = unitPrice
	return CartLine{}, nil
}

func (r *repriceRepo) WithTx(*sql.Tx) Repository { return r }

type fakePricer struct {
	prices map[string]SimplePricing
}

func (f fakePricer) ResolveSimplePricing(_ context.Context, _ uuid.UUID, sku string, _ *uuid.UUID) (SimplePricing, error) {
	p, ok := f.prices[sku]
	if !ok {
		return SimplePricing{}, fmt.Errorf("no listing for %q", sku)
	}
	return p, nil
}

func (f fakePricer) ResolveBundleBySKU(_ context.Context, _ uuid.UUID, sku string) (BundleSnapshot, error) {
	return BundleSnapshot{}, fmt.Errorf("no bundle %q", sku)
}

func (f fakePricer) ResolveProductOptions(context.Context, uuid.UUID, string, *uuid.UUID) (ProductOptionsResolution, error) {
	panic("not used")
}

func TestRepriceCartLines(t *testing.T) {
	channelID := uuid.New()
	simpleID, configuredID, bundleID, staleID, missingID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()

	baseCart := func() Cart {
		return Cart{
			ID: uuid.New(), ChannelID: channelID, Status: "active",
			Lines: []CartLine{
				// catalog moved 1000 -> 1200
				{ID: simpleID, ProductSku: "plain", UnitPrice: 1000, Quantity: 2},
				// base moved 1500 -> 1300, snapshot option delta +500 rides on top
				{ID: configuredID, ProductSku: "engraved", UnitPrice: 2000, Quantity: 1,
					Options: []CartLineOption{{OptionTitle: "Engraving", PriceDelta: 500}}},
				// a bundle that can't be re-resolved keeps its snapshot
				{ID: bundleID, ProductSku: "set", UnitPrice: 4000, Quantity: 1,
					Components: []CartLineComponent{{ProductSku: "plain", UnitPrice: 1000, Quantity: 4}}},
				// price unchanged - no write
				{ID: staleID, ProductSku: "stable", UnitPrice: 700, Quantity: 1},
				// delisted since add - snapshot survives
				{ID: missingID, ProductSku: "ghost", UnitPrice: 999, Quantity: 1},
			},
		}
	}
	pricer := fakePricer{prices: map[string]SimplePricing{
		"plain":    {UnitPrice: 1200},
		"engraved": {UnitPrice: 1300},
		"set":      {UnitPrice: 9999}, // a bundle's own price is never its line price
		"stable":   {UnitPrice: 700},
	}}

	t.Run("reprices simple and configured, skips unresolvable bundles and misses", func(t *testing.T) {
		repo := &repriceRepo{}
		svc := NewService(repo, nil, pricer)
		out := svc.RepriceCartLines(context.Background(), baseCart())

		if got := out.Lines[0].UnitPrice; got != 1200 {
			t.Fatalf("simple line = %d, want 1200", got)
		}
		if got := out.Lines[1].UnitPrice; got != 1800 {
			t.Fatalf("configured line = %d, want 1800 (new base 1300 + delta 500)", got)
		}
		if got := out.Lines[2].UnitPrice; got != 4000 {
			t.Fatalf("bundle line = %d, want untouched 4000", got)
		}
		if got := out.Lines[4].UnitPrice; got != 999 {
			t.Fatalf("delisted line = %d, want snapshot 999", got)
		}
		if len(repo.writes) != 2 {
			t.Fatalf("persisted %d lines, want exactly the 2 that moved: %v", len(repo.writes), repo.writes)
		}
		if repo.writes[simpleID] != 1200 || repo.writes[configuredID] != 1800 {
			t.Fatalf("persisted prices diverge from returned cart: %v", repo.writes)
		}
	})

	t.Run("tier break applies to the repriced base", func(t *testing.T) {
		repo := &repriceRepo{}
		tiered := pricer
		tiered.prices = map[string]SimplePricing{
			"plain": {UnitPrice: 1200, Tiers: []pricing.Tier{{MinQty: 2, Price: 1100}}},
		}
		svc := NewService(repo, nil, tiered)
		c := Cart{ID: uuid.New(), ChannelID: channelID, Lines: []CartLine{
			{ID: simpleID, ProductSku: "plain", UnitPrice: 1000, Quantity: 2},
		}}
		out := svc.RepriceCartLines(context.Background(), c)
		if got := out.Lines[0].UnitPrice; got != 1100 {
			t.Fatalf("tiered reprice = %d, want 1100", got)
		}
	})

	t.Run("persist failure keeps the snapshot - returned cart never diverges from storage", func(t *testing.T) {
		repo := &repriceRepo{fail: true}
		svc := NewService(repo, nil, pricer)
		out := svc.RepriceCartLines(context.Background(), baseCart())
		if got := out.Lines[0].UnitPrice; got != 1000 {
			t.Fatalf("line = %d, want snapshot 1000 when the write fails", got)
		}
	})

	t.Run("no product resolver is a no-op", func(t *testing.T) {
		repo := &repriceRepo{}
		svc := NewService(repo, nil, nil)
		out := svc.RepriceCartLines(context.Background(), baseCart())
		if got := out.Lines[0].UnitPrice; got != 1000 {
			t.Fatalf("line = %d, want untouched snapshot", got)
		}
		if len(repo.writes) != 0 {
			t.Fatal("no-op must not write")
		}
	})
}

// bundleRepriceRepo records the bundle rewrites RepriceCartLines makes
type bundleRepriceRepo struct {
	fakeRepo
	upserts []AddBundleLineInput
}

func (r *bundleRepriceRepo) UpsertBundleCartLine(_ context.Context, _ uuid.UUID, in AddBundleLineInput) (CartLine, error) {
	r.upserts = append(r.upserts, in)
	out := CartLine{UnitPrice: in.UnitPrice, Quantity: in.Quantity}
	for _, c := range in.Components {
		out.Components = append(out.Components, CartLineComponent{BundleOptionID: c.BundleOptionID, BundleSelectionID: c.BundleSelectionID, ProductSku: c.ProductSku, UnitPrice: c.UnitPrice, Quantity: c.Quantity})
	}
	return out, nil
}

func (r *bundleRepriceRepo) WithTx(*sql.Tx) Repository { return r }

func TestRepriceBundleLines(t *testing.T) {
	ch, optID, selID := uuid.New(), uuid.New(), uuid.New()
	snap := func(perBundle float64) BundleSnapshot {
		return BundleSnapshot{Options: []BundleOptionSnapshot{{ID: optID, InputType: "radio", IsRequired: true,
			Selections: []BundleSelectionSnapshot{{ID: selID, ProductSKU: "mug", QuantityPerBundle: perBundle}}}}}
	}
	// two bundles of 2 mugs at 1000 each, stored as a 2000 bundle with a 4-mug component row
	line := func(unit int64) CartLine {
		return CartLine{ID: uuid.New(), ProductSku: "set", UnitPrice: unit, Quantity: 2,
			Components: []CartLineComponent{{BundleOptionID: optID, BundleSelectionID: selID, ProductSku: "mug", UnitPrice: 1000, Quantity: 4}}}
	}
	run := func(t *testing.T, s BundleSnapshot, mug int64, l CartLine) (CartLine, []AddBundleLineInput, int64) {
		t.Helper()
		repo := &bundleRepriceRepo{}
		svc := NewService(repo, nil, channelBundles{prices: map[string]SimplePricing{"mug": {UnitPrice: mug}}, snap: s, published: map[uuid.UUID]bool{ch: true}})
		c := Cart{ID: uuid.New(), ChannelID: ch, Lines: []CartLine{l}}
		current := svc.CurrentLinePrices(context.Background(), c)[l.ID]
		return svc.RepriceCartLines(context.Background(), c).Lines[0], repo.upserts, current
	}

	t.Run("a component price change re-sums the bundle", func(t *testing.T) {
		got, writes, current := run(t, snap(2), 1200, line(2000))
		if got.UnitPrice != 2400 || current != 2400 || len(writes) != 1 || writes[0].UnitPrice != 2400 {
			t.Fatalf("line %d, current %d, writes %+v - want 2*1200=2400 everywhere", got.UnitPrice, current, writes)
		}
		if c := got.Components[0]; c.UnitPrice != 1200 || c.Quantity != 4 {
			t.Fatalf("component = %+v, want 1200 x 4", c)
		}
	})
	t.Run("a changed quantity per bundle rescales the component and the price", func(t *testing.T) {
		got, _, _ := run(t, snap(3), 1000, line(2000))
		if got.UnitPrice != 3000 || got.Components[0].Quantity != 6 {
			t.Fatalf("line %d, component qty %v - want 3*1000=3000 and 3 per bundle x 2 = 6", got.UnitPrice, got.Components[0].Quantity)
		}
	})
	t.Run("a hook-priced bundle keeps its price while its components refresh", func(t *testing.T) {
		got, writes, current := run(t, snap(2), 1200, line(1500))
		if got.UnitPrice != 1500 || current != 1500 || len(writes) != 1 || got.Components[0].UnitPrice != 1200 {
			t.Fatalf("line %d, current %d, component %d - want the hook's 1500 kept and the component at 1200", got.UnitPrice, current, got.Components[0].UnitPrice)
		}
	})
	t.Run("an unchanged bundle is not rewritten", func(t *testing.T) {
		if _, writes, _ := run(t, snap(2), 1000, line(2000)); len(writes) != 0 {
			t.Fatalf("rewrote an unchanged line: %+v", writes)
		}
	})
}
