package inventory

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// Embeds Repository (nil) so it satisfies the interface without 29 methods - allocateLine only calls GetLevelForUpdate and NetReservation, so any other call panics loudly
type fakeAllocRepo struct {
	Repository
	item    Item
	itemErr error
	levels  map[uuid.UUID]float64 // locationID → on-hand quantity (absent = no level row)
	nets    map[uuid.UUID]float64 // locationID → net reservation (negative = held)
}

// itemsFor builds the preloaded item map allocateLine now reads from, mirroring what ListItemsByProductIDs returns for
// the fake: an entry only when there's a real item (itemErr == ErrItemNotFound → no entry)
func itemsFor(repo fakeAllocRepo) map[uuid.UUID]Item {
	if repo.itemErr != nil {
		return map[uuid.UUID]Item{}
	}
	return map[uuid.UUID]Item{repo.item.ProductID: repo.item}
}

// stockedFor mirrors the ReserveTx preload: a cell exists exactly where the fake has a level row
func stockedFor(repo fakeAllocRepo) map[allocKey]bool {
	out := make(map[allocKey]bool, len(repo.levels))
	for locID := range repo.levels {
		out[allocKey{item: repo.item.ID, loc: locID}] = true
	}
	return out
}

func (f fakeAllocRepo) GetLevelForUpdate(_ context.Context, _ uuid.UUID, locationID uuid.UUID) (Level, error) {
	q, ok := f.levels[locationID]
	if !ok {
		return Level{}, ErrLevelNotFound
	}
	return Level{InventoryItemID: f.item.ID, LocationID: locationID, Quantity: q}, nil
}

func (f fakeAllocRepo) NetReservation(_ context.Context, _ uuid.UUID, locationID uuid.UUID) (float64, error) {
	return f.nets[locationID], nil
}

func tracked(backorderable bool) Item {
	return Item{ID: uuid.New(), ProductID: uuid.New(), Tracked: true, Backorderable: backorderable}
}

func loc(slug string) Location { return Location{ID: uuid.New(), Slug: slug, IsActive: true} }

func runAllocate(t *testing.T, repo fakeAllocRepo, locs []Location, qty float64) ([]ReserveAllocation, error) {
	t.Helper()
	s := &Service{}
	line := ReserveLine{ProductID: repo.item.ProductID, OrderLineID: uuid.New(), Quantity: qty}
	return s.allocateLine(context.Background(), repo, line, locs, map[allocKey]float64{}, itemsFor(repo), stockedFor(repo))
}

func TestAllocateLine_UntrackedItemSkipped(t *testing.T) {
	repo := fakeAllocRepo{item: Item{ID: uuid.New(), Tracked: false}}
	plan, err := runAllocate(t, repo, []Location{loc("a")}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan != nil {
		t.Errorf("untracked item should produce no allocations, got %+v", plan)
	}
}

func TestAllocateLine_MissingItemSkipped(t *testing.T) {
	repo := fakeAllocRepo{itemErr: ErrItemNotFound}
	plan, err := runAllocate(t, repo, []Location{loc("a")}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan != nil {
		t.Errorf("missing item should produce no allocations, got %+v", plan)
	}
}

func TestAllocateLine_SingleLocationSufficient(t *testing.T) {
	item := tracked(false)
	a := loc("a")
	repo := fakeAllocRepo{item: item, levels: map[uuid.UUID]float64{a.ID: 10}}
	plan, err := runAllocate(t, repo, []Location{a}, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertPlan(t, plan, []ReserveAllocation{{InventoryItemID: item.ID, LocationID: a.ID, Quantity: 3}})
}

func TestAllocateLine_SplitsAcrossLocationsInPriorityOrder(t *testing.T) {
	item := tracked(false)
	a, b := loc("a"), loc("b")
	repo := fakeAllocRepo{item: item, levels: map[uuid.UUID]float64{a.ID: 2, b.ID: 10}}
	plan, err := runAllocate(t, repo, []Location{a, b}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Drains the first location (2) then takes the remainder (3) from the second
	assertPlan(t, plan, []ReserveAllocation{
		{InventoryItemID: item.ID, LocationID: a.ID, Quantity: 2},
		{InventoryItemID: item.ID, LocationID: b.ID, Quantity: 3},
	})
}

func TestAllocateLine_NetReservationsReduceAvailable(t *testing.T) {
	item := tracked(false)
	a, b := loc("a"), loc("b")
	// a has 10 on hand but 8 already held → only 2 available
	repo := fakeAllocRepo{
		item:   item,
		levels: map[uuid.UUID]float64{a.ID: 10, b.ID: 10},
		nets:   map[uuid.UUID]float64{a.ID: -8},
	}
	plan, err := runAllocate(t, repo, []Location{a, b}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertPlan(t, plan, []ReserveAllocation{
		{InventoryItemID: item.ID, LocationID: a.ID, Quantity: 2},
		{InventoryItemID: item.ID, LocationID: b.ID, Quantity: 3},
	})
}

func TestAllocateLine_AvailableClampedAtZero(t *testing.T) {
	item := tracked(false)
	a, b := loc("a"), loc("b")
	// a is oversold on paper (held more than on hand) → clamps to 0, not negative,
	// so it must not "give back" stock to the pool
	repo := fakeAllocRepo{
		item:   item,
		levels: map[uuid.UUID]float64{a.ID: 5, b.ID: 4},
		nets:   map[uuid.UUID]float64{a.ID: -10},
	}
	plan, err := runAllocate(t, repo, []Location{a, b}, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertPlan(t, plan, []ReserveAllocation{
		{InventoryItemID: item.ID, LocationID: b.ID, Quantity: 4},
	})
}

func TestAllocateLine_MissingLevelTreatedAsZero(t *testing.T) {
	item := tracked(false)
	a := loc("a") // no level row for a at all
	repo := fakeAllocRepo{item: item, levels: map[uuid.UUID]float64{}}
	_, err := runAllocate(t, repo, []Location{a}, 1)
	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("got %v, want ErrInsufficientStock", err)
	}
}

func TestAllocateLine_InsufficientNotBackorderable(t *testing.T) {
	item := tracked(false)
	a := loc("a")
	repo := fakeAllocRepo{item: item, levels: map[uuid.UUID]float64{a.ID: 2}}
	_, err := runAllocate(t, repo, []Location{a}, 5)
	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("got %v, want ErrInsufficientStock", err)
	}
}

func TestAllocateLine_BackorderParksShortfallAtFirstLocation(t *testing.T) {
	item := tracked(true) // backorderable
	a, b := loc("a"), loc("b")
	repo := fakeAllocRepo{item: item, levels: map[uuid.UUID]float64{a.ID: 2}}
	plan, err := runAllocate(t, repo, []Location{a, b}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Takes 2 real units from a, then parks the 3-unit shortfall at the
	// highest-priority location (a = locations[0])
	assertPlan(t, plan, []ReserveAllocation{
		{InventoryItemID: item.ID, LocationID: a.ID, Quantity: 2},
		{InventoryItemID: item.ID, LocationID: a.ID, Quantity: 3},
	})
}

func TestAllocateLine_PlannedDiscountsAcrossLines(t *testing.T) {
	// Two lines for the SAME product (e.g. a component shared by two bundles) against a single location holding 5 units - Threading one
	// `planned` map through both allocateLine calls must stop the second line from re-reading the full 5 - otherwise the order oversells
	item := tracked(false) // not backorderable: the shortfall must error
	a := loc("a")
	repo := fakeAllocRepo{item: item, levels: map[uuid.UUID]float64{a.ID: 5}}
	s := &Service{}
	planned := map[allocKey]float64{}
	locs := []Location{a}

	// Line 1 takes 4 of the 5 on hand
	line1 := ReserveLine{ProductID: item.ProductID, OrderLineID: uuid.New(), Quantity: 4}
	plan1, err := s.allocateLine(context.Background(), repo, line1, locs, planned, itemsFor(repo), stockedFor(repo))
	if err != nil {
		t.Fatalf("line 1: unexpected error: %v", err)
	}
	assertPlan(t, plan1, []ReserveAllocation{{InventoryItemID: item.ID, LocationID: a.ID, Quantity: 4}})

	// Line 2 wants 3 more, but only 1 real unit remains (5 − 4 planned)
	// Not backorderable → must fail rather than allocate a phantom 3
	line2 := ReserveLine{ProductID: item.ProductID, OrderLineID: uuid.New(), Quantity: 3}
	_, err = s.allocateLine(context.Background(), repo, line2, locs, planned, itemsFor(repo), stockedFor(repo))
	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("line 2: got %v, want ErrInsufficientStock (planned should have discounted line 1's take)", err)
	}
}

func TestAllocateLine_PlannedAllowsSecondLineWithinRemaining(t *testing.T) {
	// Same setup, but line 2 fits in the leftover unit - it should succeed
	// and take exactly the 1 remaining unit, not the full 5
	item := tracked(false)
	a := loc("a")
	repo := fakeAllocRepo{item: item, levels: map[uuid.UUID]float64{a.ID: 5}}
	s := &Service{}
	planned := map[allocKey]float64{}
	locs := []Location{a}

	line1 := ReserveLine{ProductID: item.ProductID, OrderLineID: uuid.New(), Quantity: 4}
	if _, err := s.allocateLine(context.Background(), repo, line1, locs, planned, itemsFor(repo), stockedFor(repo)); err != nil {
		t.Fatalf("line 1: unexpected error: %v", err)
	}
	line2 := ReserveLine{ProductID: item.ProductID, OrderLineID: uuid.New(), Quantity: 1}
	plan2, err := s.allocateLine(context.Background(), repo, line2, locs, planned, itemsFor(repo), stockedFor(repo))
	if err != nil {
		t.Fatalf("line 2: unexpected error: %v", err)
	}
	assertPlan(t, plan2, []ReserveAllocation{{InventoryItemID: item.ID, LocationID: a.ID, Quantity: 1}})
}

func TestAllocateLine_PropagatesUnexpectedLevelError(t *testing.T) {
	item := tracked(false)
	a := loc("a")
	boom := errors.New("db exploded")
	repo := fakeAllocErrRepo{fakeAllocRepo{item: item, levels: map[uuid.UUID]float64{a.ID: 5}}, boom} // the cell must read as stocked, or the preload skip means the erroring level read is never reached
	s := &Service{}
	line := ReserveLine{ProductID: item.ProductID, OrderLineID: uuid.New(), Quantity: 1}
	_, err := s.allocateLine(context.Background(), repo, line, []Location{a}, map[allocKey]float64{}, itemsFor(repo.fakeAllocRepo), stockedFor(repo.fakeAllocRepo))
	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want the underlying db error", err)
	}
}

// fakeAllocErrRepo overrides GetLevelForUpdate to return a non-sentinel
// error, exercising the "unexpected error propagates" branch
type fakeAllocErrRepo struct {
	fakeAllocRepo
	levelErr error
}

func (f fakeAllocErrRepo) GetLevelForUpdate(context.Context, uuid.UUID, uuid.UUID) (Level, error) {
	return Level{}, f.levelErr
}

func assertPlan(t *testing.T, got, want []ReserveAllocation) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("allocation count: got %d %+v, want %d %+v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i].LocationID != want[i].LocationID || got[i].Quantity != want[i].Quantity {
			t.Errorf("allocation[%d]: got {loc=%s qty=%g}, want {loc=%s qty=%g}",
				i, got[i].LocationID, got[i].Quantity, want[i].LocationID, want[i].Quantity)
		}
	}
}
