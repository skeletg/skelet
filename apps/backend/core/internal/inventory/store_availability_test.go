package inventory

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"
)

type publicStoreRepo struct {
	Repository
	locations []Location
	levels    []Level
	item      Item
	itemErr   error
	channel   uuid.UUID
}

func (f *publicStoreRepo) ListPublicLocations(_ context.Context, channel uuid.UUID) ([]Location, error) {
	f.channel = channel
	return f.locations, nil
}
func (f *publicStoreRepo) GetItemByProductID(context.Context, uuid.UUID) (Item, error) {
	return f.item, f.itemErr
}
func (f *publicStoreRepo) ListLevelsByItems(context.Context, []uuid.UUID) ([]Level, error) {
	return f.levels, nil
}
func storePtr[T any](v T) *T { return &v }

func TestPhysicalStoreStock(t *testing.T) {
	for _, tc := range []struct {
		name                                         string
		tracked                                      bool
		quantity, reserved, threshold, step, minimum float64
		want                                         StoreStock
	}{
		{"normal", true, 10, 2, 3, 1, 0, StoreInStock},
		{"held", true, 3, 3, 2, 1, 0, StoreOutOfStock},
		{"low", true, 4, 2, 3, 1, 0, StoreLowStock},
		{"untracked", false, 4, 4, 0, 1, 0, StoreInStock},
		{"negative hold", true, 0, -10, 0, 1, 0, StoreOutOfStock},
		{"zero", true, 0, 0, 0, 1, 0, StoreOutOfStock},
		{"fractional", true, .25, 0, 0, .25, 0, StoreInStock},
		{"below step", true, .24, 0, 0, .25, 0, StoreOutOfStock},
		{"minimum rounded", true, .6, 0, 0, .25, .6, StoreOutOfStock},
		{"minimum met", true, .75, 0, 0, .25, .6, StoreInStock},
		{"threshold off", true, 1, 0, 0, 1, 0, StoreInStock},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := storeStockStatus(Item{Tracked: tc.tracked, Backorderable: true, LowStockThreshold: tc.threshold}, Level{Quantity: tc.quantity, ReservedQuantity: tc.reserved}, StoreProduct{QuantityStep: tc.step, MinOrderQty: &tc.minimum})
			if got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestPublicStoreFiltersAndMissingRecords(t *testing.T) {
	a, b, c, hidden := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := &publicStoreRepo{locations: []Location{{ID: b, Name: "Beta", City: "Novi Sad", PostalCode: "21000"}, {ID: c, Name: "Gamma", City: "Belgrade"}, {ID: a, Name: "Alpha", City: "Belgrade", Line1: "Knez 1"}}, item: Item{Tracked: true}, levels: []Level{{LocationID: a, Quantity: 1}, {LocationID: b, Quantity: 0}, {LocationID: hidden, Quantity: 99}}}
	svc := &Service{repo: repo}
	q := StoreQuery{ChannelID: uuid.New(), Product: &StoreProduct{ID: uuid.New(), QuantityStep: 1}, PreferredStoreID: &b, Limit: 2}
	got, err := svc.PublicStores(t.Context(), q)
	if err != nil {
		t.Fatal(err)
	}
	if repo.channel != q.ChannelID || !got.ProductEligible || got.TotalCount != 3 || len(got.Items) != 2 || got.Items[0].Location.ID != a || *got.Items[1].Status != StoreOutOfStock || got.PreferredStore.Location.ID != b {
		t.Fatalf("unexpected page %+v", got)
	}
	if len(got.Cities) != 2 {
		t.Fatalf("cities=%v", got.Cities)
	}
	q.InStockOnly = true
	got, err = svc.PublicStores(t.Context(), q)
	if err != nil || got.TotalCount != 1 {
		t.Fatalf("in stock: %+v %v", got, err)
	}
	q.InStockOnly = false
	q.City = "belgrade"
	q.Search = "KNEZ"
	got, err = svc.PublicStores(t.Context(), q)
	if err != nil || got.TotalCount != 1 || got.Items[0].Location.ID != a || got.PreferredStore == nil {
		t.Fatalf("filtered: %+v %v", got, err)
	}
	q.City = ""
	q.Search = ""
	q.Offset = 2
	got, err = svc.PublicStores(t.Context(), q)
	if err != nil || *got.Items[0].Status != StoreUnknown {
		t.Fatalf("missing: %+v %v", got, err)
	}
	repo.levels = []Level{{LocationID: hidden, Quantity: 99}}
	got, err = svc.PublicStores(t.Context(), q)
	if err != nil || got.ProductEligible {
		t.Fatalf("private stock made product eligible: %+v %v", got, err)
	}
	repo.itemErr = ErrItemNotFound
	got, err = svc.PublicStores(t.Context(), q)
	if err != nil || got.ProductEligible {
		t.Fatalf("unlimited sentinel: %+v %v", got, err)
	}
	repo.itemErr = errors.New("offline")
	if _, err = svc.PublicStores(t.Context(), q); err == nil {
		t.Fatal("lookup failure swallowed")
	}
}

func TestPublicStoresFlatModeAndDistance(t *testing.T) {
	near, far, noCoords := uuid.New(), uuid.New(), uuid.New()
	repo := &publicStoreRepo{locations: []Location{{ID: far, Name: "Alpha", Latitude: storePtr(45.), Longitude: storePtr(20.)}, {ID: noCoords, Name: "Beta"}, {ID: near, Name: "Gamma", Latitude: storePtr(44.), Longitude: storePtr(20.)}}}
	svc := &Service{repo: repo, flatMode: stubFlatMode{on: true}}
	q := StoreQuery{ChannelID: uuid.New(), Product: &StoreProduct{ID: uuid.New()}, Latitude: storePtr(44.), Longitude: storePtr(20.), Limit: 500, Offset: -1}
	got, err := svc.PublicStores(t.Context(), q)
	if err != nil {
		t.Fatal(err)
	}
	if got.StockCheckingAvailable || got.ProductEligible || got.Items[0].Status != nil || got.Items[0].Location.ID != near || got.Items[2].Location.ID != noCoords {
		t.Fatalf("flat/distance page=%+v", got)
	}
	if math.Abs(*got.Items[1].DistanceKm-111.19) > .1 {
		t.Fatalf("distance=%v", *got.Items[1].DistanceKm)
	}
	q.Latitude = storePtr(math.NaN())
	if _, err = svc.PublicStores(t.Context(), q); !errors.Is(err, ErrInvalidCoordinates) {
		t.Fatalf("invalid=%v", err)
	}
	q.Latitude = nil
	if _, err = svc.PublicStores(t.Context(), q); !errors.Is(err, ErrInvalidCoordinates) {
		t.Fatalf("incomplete=%v", err)
	}
}

func TestPublicStorePickupFilterKeepsWalkInStoresEligible(t *testing.T) {
	retail, pickup := uuid.New(), uuid.New()
	repo := &publicStoreRepo{locations: []Location{{ID: retail, Name: "Walk in"}, {ID: pickup, Name: "Pickup", PickupEnabled: true}}}
	svc := &Service{repo: repo}
	page, err := svc.PublicStores(t.Context(), StoreQuery{ChannelID: uuid.New()})
	if err != nil || page.TotalCount != 2 || !page.HasPickupStores {
		t.Fatalf("walk-in page=%+v %v", page, err)
	}
	page, err = svc.PublicStores(t.Context(), StoreQuery{ChannelID: uuid.New(), PickupOnly: true})
	if err != nil || page.TotalCount != 1 || page.Items[0].Location.ID != pickup {
		t.Fatalf("pickup filter=%+v %v", page, err)
	}
}
