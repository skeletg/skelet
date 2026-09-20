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

func (f fakePricer) ResolveBundleBySKU(context.Context, string) (BundleSnapshot, error) {
	panic("not used")
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
				// bundles keep their snapshot
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
		"set":      {UnitPrice: 9999}, // must never be consulted for a bundle line
		"stable":   {UnitPrice: 700},
	}}

	t.Run("reprices simple and configured, skips bundles and misses", func(t *testing.T) {
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
