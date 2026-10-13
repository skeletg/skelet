package inventory

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// The single-product read path - separate from the batched one, with its own remote, flat-mode and sentinel branches
type fakeSalableRepo struct {
	Repository
	item      Item
	itemErr   error
	locations []Location
	locErr    error
	levels    []Level
	levelsErr error
}

func (f fakeSalableRepo) GetItemByProductID(context.Context, uuid.UUID) (Item, error) {
	return f.item, f.itemErr
}

func (f fakeSalableRepo) ListChannelLocations(context.Context, uuid.UUID) ([]Location, error) {
	return f.locations, f.locErr
}

func (f fakeSalableRepo) ListLevelsByItems(context.Context, []uuid.UUID) ([]Level, error) {
	return f.levels, f.levelsErr
}

type stubFlatMode struct{ on bool }

func (s stubFlatMode) FlatModeEnabled(context.Context) bool { return s.on }

type stubRemoteAvail struct {
	qty map[uuid.UUID]float64
	ok  bool
}

func (s stubRemoteAvail) Salable(context.Context, []uuid.UUID, uuid.UUID) (map[uuid.UUID]float64, bool) {
	return s.qty, s.ok
}

func salableSvc(repo Repository) *Service { return &Service{repo: repo} }

// A product with no inventory item at all is "always in stock" - the sentinel every purchasability caller checks for, and returning 0 instead would make every untracked product unbuyable
func TestSalableReturnsTheAlwaysInStockSentinel(t *testing.T) {
	repo := fakeSalableRepo{itemErr: ErrItemNotFound}
	got, err := salableSvc(repo).Salable(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("Salable: %v", err)
	}
	if got != -1 {
		t.Errorf("Salable with no item = %v, want the -1 sentinel", got)
	}
}

// Any other lookup failure is NOT the sentinel - failing open there would sell stock nobody counted
func TestSalableSurfacesARealLookupError(t *testing.T) {
	boom := errors.New("connection refused")
	got, err := salableSvc(fakeSalableRepo{itemErr: boom}).Salable(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, boom) {
		t.Errorf("Salable = (%v, %v), want the error surfaced", got, err)
	}
	if got != 0 {
		t.Errorf("Salable returned %v alongside the error, want 0 rather than the in-stock sentinel", got)
	}
}

// Tracked stock is physical minus the reserved counter, summed only over the locations the channel serves
func TestSalableSubtractsReservedAcrossChannelLocations(t *testing.T) {
	locA, locB, outside := loc("a"), loc("b"), loc("outside")
	item := tracked(false)
	repo := fakeSalableRepo{
		item:      item,
		locations: []Location{locA, locB},
		levels: []Level{
			{InventoryItemID: item.ID, LocationID: locA.ID, Quantity: 10, ReservedQuantity: 3},
			{InventoryItemID: item.ID, LocationID: locB.ID, Quantity: 5, ReservedQuantity: 1},
			// stock the channel can't ship from must not count
			{InventoryItemID: item.ID, LocationID: outside.ID, Quantity: 100, ReservedQuantity: 0},
		},
	}

	got, err := salableSvc(repo).Salable(context.Background(), item.ProductID, uuid.New())
	if err != nil {
		t.Fatalf("Salable: %v", err)
	}
	if got != 11 { // (10-3) + (5-1)
		t.Errorf("Salable = %v, want 11 from the channel's two locations only", got)
	}
}

// An untracked item sells off raw physical stock - subtracting reservations there would strand it
func TestSalableIgnoresReservationsForUntrackedItems(t *testing.T) {
	locA := loc("a")
	item := Item{ID: uuid.New(), ProductID: uuid.New(), Tracked: false}
	repo := fakeSalableRepo{
		item:      item,
		locations: []Location{locA},
		levels:    []Level{{InventoryItemID: item.ID, LocationID: locA.ID, Quantity: 8, ReservedQuantity: 5}},
	}

	got, err := salableSvc(repo).Salable(context.Background(), item.ProductID, uuid.New())
	if err != nil {
		t.Fatalf("Salable: %v", err)
	}
	if got != 8 {
		t.Errorf("Salable for an untracked item = %v, want the raw 8", got)
	}
}

// A drifted negative counter must not inflate salable into an oversell - clampReserved floors it
func TestSalableClampsADriftedNegativeReservation(t *testing.T) {
	locA := loc("a")
	item := tracked(false)
	repo := fakeSalableRepo{
		item:      item,
		locations: []Location{locA},
		levels:    []Level{{InventoryItemID: item.ID, LocationID: locA.ID, Quantity: 10, ReservedQuantity: -4}},
	}

	got, err := salableSvc(repo).Salable(context.Background(), item.ProductID, uuid.New())
	if err != nil {
		t.Fatalf("Salable: %v", err)
	}
	if got != 10 {
		t.Errorf("Salable with a -4 reserved counter = %v, want the drift clamped to 10 rather than 14", got)
	}
}

// A channel with no locations can't ship anything, so it sells nothing - not the in-stock sentinel
func TestSalableIsZeroForAChannelWithNoLocations(t *testing.T) {
	item := tracked(false)
	repo := fakeSalableRepo{item: item, locations: nil}

	got, err := salableSvc(repo).Salable(context.Background(), item.ProductID, uuid.New())
	if err != nil {
		t.Fatalf("Salable: %v", err)
	}
	if got != 0 {
		t.Errorf("Salable with no channel locations = %v, want 0", got)
	}
}

// Flat mode is channel-agnostic and reads the item row, so it must not consult locations or levels
func TestSalableInFlatModeReadsTheItemRow(t *testing.T) {
	item := tracked(false)
	item.Quantity = 42
	repo := fakeSalableRepo{
		item: item,
		// deliberately contradicting - flat mode must not reach these
		locations: []Location{loc("a")},
		levels:    []Level{{InventoryItemID: item.ID, LocationID: uuid.New(), Quantity: 999}},
	}
	svc := salableSvc(repo)
	svc.SetFlatModeToggle(stubFlatMode{on: true})

	got, err := svc.Salable(context.Background(), item.ProductID, uuid.New())
	if err != nil {
		t.Fatalf("Salable: %v", err)
	}
	if got != 42 {
		t.Errorf("Salable in flat mode = %v, want the item row's 42", got)
	}
}

// An untracked item in flat mode is unlimited rather than however many happen to be counted
func TestSalableInFlatModeSentinelsUntracked(t *testing.T) {
	item := Item{ID: uuid.New(), ProductID: uuid.New(), Tracked: false, Quantity: 3}
	svc := salableSvc(fakeSalableRepo{item: item})
	svc.SetFlatModeToggle(stubFlatMode{on: true})

	got, err := svc.Salable(context.Background(), item.ProductID, uuid.New())
	if err != nil {
		t.Fatalf("Salable: %v", err)
	}
	if got != -1 {
		t.Errorf("Salable for an untracked flat item = %v, want the -1 sentinel", got)
	}
}

// A remote WMS is authoritative when it answers - native stock must not override an ERP that owns it
func TestSalablePrefersTheRemoteProvider(t *testing.T) {
	locA := loc("a")
	item := tracked(false)
	repo := fakeSalableRepo{
		item:      item,
		locations: []Location{locA},
		levels:    []Level{{InventoryItemID: item.ID, LocationID: locA.ID, Quantity: 10}},
	}
	svc := salableSvc(repo)
	svc.SetRemoteAvailability(stubRemoteAvail{qty: map[uuid.UUID]float64{item.ProductID: 77}, ok: true})

	got, err := svc.Salable(context.Background(), item.ProductID, uuid.New())
	if err != nil {
		t.Fatalf("Salable: %v", err)
	}
	if got != 77 {
		t.Errorf("Salable = %v, want the remote provider's 77", got)
	}
}

// The remote path always fails open to native - an ERP blip must not take the storefront's stock to zero
func TestSalableFallsBackToNativeWhenTheRemoteDeclines(t *testing.T) {
	locA := loc("a")
	item := tracked(false)
	repo := fakeSalableRepo{
		item:      item,
		locations: []Location{locA},
		levels:    []Level{{InventoryItemID: item.ID, LocationID: locA.ID, Quantity: 10, ReservedQuantity: 2}},
	}

	for name, remote := range map[string]stubRemoteAvail{
		"provider says no":        {ok: false},
		"provider omits the item": {qty: map[uuid.UUID]float64{uuid.New(): 5}, ok: true},
		"provider answers empty":  {qty: map[uuid.UUID]float64{}, ok: true},
	} {
		svc := salableSvc(repo)
		svc.SetRemoteAvailability(remote)
		got, err := svc.Salable(context.Background(), item.ProductID, uuid.New())
		if err != nil {
			t.Errorf("%s: Salable: %v", name, err)
			continue
		}
		if got != 8 {
			t.Errorf("%s: Salable = %v, want the native 8", name, got)
		}
	}
}

// A remote answer of -1 is "unlimited" and has to reach the caller as the sentinel, not as a negative quantity
func TestSalableCarriesTheRemoteUnlimitedSentinel(t *testing.T) {
	item := tracked(false)
	svc := salableSvc(fakeSalableRepo{item: item})
	svc.SetRemoteAvailability(stubRemoteAvail{qty: map[uuid.UUID]float64{item.ProductID: -1}, ok: true})

	got, err := svc.Salable(context.Background(), item.ProductID, uuid.New())
	if err != nil {
		t.Fatalf("Salable: %v", err)
	}
	if got != -1 {
		t.Errorf("Salable = %v, want the remote's -1 unlimited sentinel", got)
	}
}

// Errors from the location and level reads must surface rather than reading as "nothing salable", which would silently unpublish the catalog during a database blip
func TestSalablePropagatesReadErrors(t *testing.T) {
	boom := errors.New("db down")
	item := tracked(false)

	for name, repo := range map[string]fakeSalableRepo{
		"locations fail": {item: item, locErr: boom},
		"levels fail":    {item: item, locations: []Location{loc("a")}, levelsErr: boom},
	} {
		got, err := salableSvc(repo).Salable(context.Background(), item.ProductID, uuid.New())
		if !errors.Is(err, boom) {
			t.Errorf("%s: Salable = (%v, %v), want the error surfaced", name, got, err)
		}
		if got != 0 {
			t.Errorf("%s: Salable returned %v with the error, want 0", name, got)
		}
	}
}

// NetReservationsForItem is what renders reservedQuantity per location, and it negates the stored counter - a sign flip would show every location as over-reserved
func TestNetReservationsForItemNegatesTheStoredCounter(t *testing.T) {
	locA, locB := loc("a"), loc("b")
	item := tracked(false)
	repo := fakeLevelsRepo{levels: []Level{
		{InventoryItemID: item.ID, LocationID: locA.ID, Quantity: 10, ReservedQuantity: 3},
		{InventoryItemID: item.ID, LocationID: locB.ID, Quantity: 5, ReservedQuantity: 0},
	}}

	got, err := salableSvc(repo).NetReservationsForItem(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("NetReservationsForItem: %v", err)
	}
	if got[locA.ID] != -3 {
		t.Errorf("net for a reserved location = %v, want -3", got[locA.ID])
	}
	if got[locB.ID] != 0 {
		t.Errorf("net for an unreserved location = %v, want 0", got[locB.ID])
	}
	if len(got) != 2 {
		t.Errorf("%d locations reported, want one per level", len(got))
	}
}

type fakeLevelsRepo struct {
	Repository
	levels []Level
	err    error
}

func (f fakeLevelsRepo) ListLevelsByItem(context.Context, uuid.UUID) ([]Level, error) {
	return f.levels, f.err
}

func (f fakeSalableRepo) ListItemsByProductIDs(_ context.Context, pids []uuid.UUID) (map[uuid.UUID]Item, error) {
	if errors.Is(f.itemErr, ErrItemNotFound) {
		return map[uuid.UUID]Item{}, nil
	}
	if f.itemErr != nil {
		return nil, f.itemErr
	}
	out := map[uuid.UUID]Item{}
	for _, pid := range pids {
		out[pid] = f.item
	}
	return out, nil
}
