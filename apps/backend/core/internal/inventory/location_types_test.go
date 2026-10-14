package inventory

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type locationTypeRepo struct {
	Repository
	current      Location
	created      CreateLocationInput
	updated      UpdateLocationInput
	pickupCalled bool
}

func (r *locationTypeRepo) CreateLocation(_ context.Context, input CreateLocationInput) (Location, error) {
	r.created = input
	return Location{Type: input.Type}, nil
}
func (r *locationTypeRepo) GetLocationByID(context.Context, uuid.UUID) (Location, error) {
	return r.current, nil
}
func (r *locationTypeRepo) UpdateLocation(_ context.Context, input UpdateLocationInput) (Location, error) {
	r.updated = input
	return Location{}, nil
}
func (r *locationTypeRepo) SetLocationPickup(context.Context, uuid.UUID, bool, string) (Location, error) {
	r.pickupCalled = true
	return Location{}, nil
}
func TestLocationTypesAndCustomerAccess(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"Retail store", LocationRetail}, {"warehouse", LocationWarehouse}, {"storage", LocationStorage}, {"dropshipper", LocationDropshipper}, {"fulfillment_partner", LocationFulfillment}, {"", LocationUnclassified}} {
		got, err := NormalizeLocationType(tc.input)
		if err != nil || got != tc.want {
			t.Fatalf("normalize %q=%q %v", tc.input, got, err)
		}
	}
	if _, err := NormalizeLocationType("arbitrary-purpose"); !errors.Is(err, ErrInvalidLocationType) {
		t.Fatal(err)
	}
	repo := &locationTypeRepo{current: Location{Type: LocationWarehouse}}
	svc := &Service{repo: repo}
	if _, err := svc.CreateLocation(t.Context(), CreateLocationInput{Slug: "test", Name: "Store", Type: "arbitrary"}); !errors.Is(err, ErrInvalidLocationType) {
		t.Fatal(err)
	}
	if _, err := svc.CreateLocation(t.Context(), CreateLocationInput{Slug: "test", Name: "Warehouse", Type: LocationWarehouse, StorefrontVisible: true}); !errors.Is(err, ErrRetailLocationRequired) {
		t.Fatal(err)
	}
	if _, err := svc.CreateLocation(t.Context(), CreateLocationInput{Slug: "test", Name: "Store", Type: LocationRetail, StorefrontVisible: true}); err != nil {
		t.Fatal(err)
	}
	if repo.created.PickupEnabled || !repo.created.StorefrontVisible {
		t.Fatal("walk-in visibility must not require pickup")
	}
	if _, err := svc.SetLocationPickup(t.Context(), uuid.New(), true, ""); !errors.Is(err, ErrRetailLocationRequired) || repo.pickupCalled {
		t.Fatal("warehouse offered pickup", err)
	}
	retail := LocationRetail
	warehouse := LocationWarehouse
	yes := true
	if _, err := svc.UpdateLocation(t.Context(), UpdateLocationInput{ID: uuid.New(), Type: &warehouse, StorefrontVisible: &yes, PickupEnabled: &yes}); err != nil {
		t.Fatal(err)
	}
	if *repo.updated.StorefrontVisible || *repo.updated.PickupEnabled {
		t.Fatal("non-retail conversion retained customer access")
	}
	if _, err := svc.UpdateLocation(t.Context(), UpdateLocationInput{ID: uuid.New(), StorefrontVisible: &yes}); !errors.Is(err, ErrRetailLocationRequired) {
		t.Fatal(err)
	}
	if _, err := svc.UpdateLocation(t.Context(), UpdateLocationInput{ID: uuid.New(), Type: &retail, StorefrontVisible: &yes}); err != nil {
		t.Fatal(err)
	}
}
