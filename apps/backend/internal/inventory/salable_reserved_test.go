package inventory

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// fakeReadRepo embeds Repository (nil) and implements only what the salable read path touches: items by product, the
// channel's locations, levels by item - Reserved comes off the level row - no ledger aggregation
type fakeReadRepo struct {
	Repository
	items     map[uuid.UUID]Item // productID → item
	locations []Location         // the channel's locations
	levels    []Level            // levels across the items
}

func (f fakeReadRepo) ListItemsByProductIDs(_ context.Context, pids []uuid.UUID) (map[uuid.UUID]Item, error) {
	out := map[uuid.UUID]Item{}
	for _, p := range pids {
		if it, ok := f.items[p]; ok {
			out[p] = it
		}
	}
	return out, nil
}

func (f fakeReadRepo) ListChannelLocations(context.Context, uuid.UUID) ([]Location, error) {
	return f.locations, nil
}

func (f fakeReadRepo) ListLevelsByItems(context.Context, []uuid.UUID) ([]Level, error) {
	return f.levels, nil
}

func TestSalableByProductIDs_UsesReservedCounter(t *testing.T) {
	locA, locB, locC := loc("a"), loc("b"), loc("c")
	item := tracked(false) // tracked, not backorderable (helpers from allocate_test.go)
	untrackedPID := uuid.New()

	repo := fakeReadRepo{
		items:     map[uuid.UUID]Item{item.ProductID: item},
		locations: []Location{locA, locB}, // channel serves A + B, NOT C
		levels: []Level{
			{InventoryItemID: item.ID, LocationID: locA.ID, Quantity: 10, ReservedQuantity: 3},
			{InventoryItemID: item.ID, LocationID: locB.ID, Quantity: 5, ReservedQuantity: 1},
			// Stock at a location outside the channel must be ignored
			{InventoryItemID: item.ID, LocationID: locC.ID, Quantity: 100, ReservedQuantity: 0},
		},
	}
	s := &Service{repo: repo} // flatMode nil → multi-location read path

	got, err := s.SalableByProductIDs(context.Background(),
		[]uuid.UUID{item.ProductID, untrackedPID}, uuid.New())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// (10-3) + (5-1) = 11; locC (out of channel) excluded
	if got[item.ProductID] != 11 {
		t.Fatalf("tracked salable = %v, want 11 (Σ quantity-reserved over channel locations)", got[item.ProductID])
	}
	// No inventory item row → "always in stock" sentinel
	if got[untrackedPID] != -1 {
		t.Fatalf("product without an item row = %v, want -1 sentinel", got[untrackedPID])
	}
}

func TestSalableByProductIDs_ClampsNegativeReserved(t *testing.T) {
	locA := loc("a")
	item := tracked(false)
	repo := fakeReadRepo{
		items:     map[uuid.UUID]Item{item.ProductID: item},
		locations: []Location{locA},
		levels: []Level{
			// A drifted negative counter must NOT inflate salable past physical
			{InventoryItemID: item.ID, LocationID: locA.ID, Quantity: 8, ReservedQuantity: -4},
		},
	}
	s := &Service{repo: repo}
	got, err := s.SalableByProductIDs(context.Background(), []uuid.UUID{item.ProductID}, uuid.New())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got[item.ProductID] != 8 {
		t.Fatalf("salable = %v, want 8 (negative reserved clamped to 0, never > physical)", got[item.ProductID])
	}
}

func TestClampReserved(t *testing.T) {
	if clampReserved(-5) != 0 {
		t.Fatal("negative reserved must clamp to 0")
	}
	if clampReserved(3) != 3 {
		t.Fatal("non-negative reserved must pass through")
	}
}
