package inventory

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// fakeLocRepo counts what reaches the repository, so the service's clamping is observed at the seam
type fakeLocRepo struct {
	Repository
	sawSearch        string
	sawLimit, sawOff int32
	lowLimit         int32
	flatCalled       bool
	levelCalled      bool
}

func (f *fakeLocRepo) ListLocationsPage(_ context.Context, search string, limit, offset int32) ([]Location, error) {
	f.sawSearch, f.sawLimit, f.sawOff = search, limit, offset
	return []Location{}, nil
}

func (f *fakeLocRepo) CountLocations(_ context.Context, search string) (int64, error) {
	f.sawSearch = search
	return 42, nil
}

func (f *fakeLocRepo) ListLowStockLevels(_ context.Context, limit int32) ([]LowStockLevel, error) {
	f.levelCalled, f.lowLimit = true, limit
	return nil, nil
}

func (f *fakeLocRepo) ListLowStockItemsFlat(_ context.Context, limit int32) ([]LowStockLevel, error) {
	f.flatCalled, f.lowLimit = true, limit
	return nil, nil
}

func TestListLocationsPageClampsItsInputs(t *testing.T) { // limit and offset arrive off a public-ish admin API, and an unclamped limit is a full-table SELECT again
	for name, tc := range map[string]struct {
		limit, offset         int32
		wantLimit, wantOffset int32
	}{
		"unset":          {0, 0, 50, 0},
		"negative":       {-5, -10, 50, 0},
		"normal":         {20, 40, 20, 40},
		"over the cap":   {5000, 0, 500, 0},
		"exactly at cap": {500, 0, 500, 0},
	} {
		repo := &fakeLocRepo{}
		s := &Service{repo: repo}
		if _, _, err := s.ListLocationsPage(context.Background(), "", tc.limit, tc.offset); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if repo.sawLimit != tc.wantLimit || repo.sawOff != tc.wantOffset {
			t.Errorf("%s: repo saw limit=%d offset=%d, want %d/%d", name, repo.sawLimit, repo.sawOff, tc.wantLimit, tc.wantOffset)
		}
	}
}

func TestListLocationsPageTrimsTheSearch(t *testing.T) { // "  bg  " must match what "bg" matches, and both the page and the count must filter identically or the pager shows phantom pages
	repo := &fakeLocRepo{}
	s := &Service{repo: repo}
	_, total, err := s.ListLocationsPage(context.Background(), "  novi sad  ", 10, 0)
	if err != nil {
		t.Fatalf("ListLocationsPage: %v", err)
	}
	if repo.sawSearch != "novi sad" {
		t.Errorf("repo saw search %q, want it trimmed", repo.sawSearch)
	}
	if total != 42 {
		t.Errorf("total = %d, want the count's 42", total)
	}
}

func TestLowStockLimitClampsAndDefaults(t *testing.T) { // the list is sorted most-urgent first, so the cap keeps the worst cases - but it must never be zero, which would render an empty reorder page
	for name, tc := range map[string]struct {
		in   int32
		want int32
	}{
		"unset":    {0, 200},
		"negative": {-1, 200},
		"normal":   {50, 50},
		"huge":     {100000, 1000},
	} {
		repo := &fakeLocRepo{}
		s := &Service{repo: repo}
		if _, err := s.ListLowStockLevels(context.Background(), tc.in); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if repo.lowLimit != tc.want {
			t.Errorf("%s: repo saw limit %d, want %d", name, repo.lowLimit, tc.want)
		}
		if !repo.levelCalled || repo.flatCalled {
			t.Errorf("%s: flat=%v level=%v, want the per-location query outside flat mode", name, repo.flatCalled, repo.levelCalled)
		}
	}
}

// countingAllocRepo panics if a lockless cell is FOR-UPDATEd - the preload skip is the whole point
type countingAllocRepo struct {
	fakeAllocRepo
	lockCalls int
}

func (c *countingAllocRepo) GetLevelForUpdate(ctx context.Context, itemID, locationID uuid.UUID) (Level, error) {
	c.lockCalls++
	return c.fakeAllocRepo.GetLevelForUpdate(ctx, itemID, locationID)
}

func TestAllocateLineSkipsUnstockedCellsWithoutLocking(t *testing.T) { // the preload makes empty locations free, where a per-line lookup costs 1000 in-tx queries for 500 of them
	item := tracked(false)
	stockedLoc := loc("stocked")
	repo := &countingAllocRepo{
		item:   item,
		levels: map[uuid.UUID]float64{stockedLoc.ID: 10}}

	locs := []Location{loc("empty-1"), loc("empty-2"), loc("empty-3"), stockedLoc}
	s := &Service{}
	line := ReserveLine{ProductID: item.ProductID, OrderLineID: uuid.New(), Quantity: 4}
	plan, err := s.allocateLine(context.Background(), repo, line, locs, map[allocKey]float64{}, itemsFor(repo.fakeAllocRepo), stockedFor(repo.fakeAllocRepo))
	if err != nil {
		t.Fatalf("allocateLine: %v", err)
	}
	if len(plan) != 1 || plan[0].Quantity != 4 || plan[0].LocationID != stockedLoc.ID {
		t.Fatalf("plan = %+v, want the full 4 from the stocked location", plan)
	}
	if repo.lockCalls != 1 {
		t.Errorf("GetLevelForUpdate ran %d times, want once - the empty cells must not be locked", repo.lockCalls)
	}
}

func TestStockedCellsMapsEveryLevelRow(t *testing.T) {
	itemA, itemB := tracked(false), tracked(false)
	locX, locY := uuid.New(), uuid.New()
	repo := &levelsListRepo{levels: []Level{
		{InventoryItemID: itemA.ID, LocationID: locX},
		{InventoryItemID: itemB.ID, LocationID: locY},
	}}

	got, err := stockedCells(context.Background(), repo, map[uuid.UUID]Item{itemA.ProductID: itemA, itemB.ProductID: itemB})
	if err != nil {
		t.Fatalf("stockedCells: %v", err)
	}
	if !got[allocKey{item: itemA.ID, loc: locX}] || !got[allocKey{item: itemB.ID, loc: locY}] {
		t.Errorf("cells = %v, want both real cells present", got)
	}
	if got[allocKey{item: itemA.ID, loc: locY}] {
		t.Error("a cross-pairing (itemA at itemB's location) reads as stocked")
	}
}

type levelsListRepo struct {
	Repository
	levels []Level
}

func (l *levelsListRepo) ListLevelsByItems(_ context.Context, _ []uuid.UUID) ([]Level, error) {
	return l.levels, nil
}

func TestLocationInputErrorsAreTyped(t *testing.T) {
	svc := NewService(&fakeLocRepo{}, nil)
	ctx := context.Background()
	if _, err := svc.CreateLocation(ctx, CreateLocationInput{Slug: "wh", Name: " "}); !errors.Is(err, ErrLocationNameRequired) {
		t.Errorf("blank name = %v, want ErrLocationNameRequired", err)
	}
	bad := 91.0
	if _, err := svc.CreateLocation(ctx, CreateLocationInput{Slug: "wh", Name: "WH", Latitude: &bad}); !errors.Is(err, ErrInvalidCoordinates) {
		t.Errorf("latitude 91 = %v, want ErrInvalidCoordinates", err)
	}
	blank := ""
	if _, err := svc.UpdateLocation(ctx, UpdateLocationInput{Name: &blank}); !errors.Is(err, ErrLocationNameRequired) {
		t.Errorf("update to a blank name = %v, want ErrLocationNameRequired", err)
	}
}
