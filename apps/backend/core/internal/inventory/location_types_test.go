package inventory

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type locationTypeRepo struct {
	Repository
	created CreateLocationInput
	updated UpdateLocationInput
}

func (r *locationTypeRepo) CreateLocation(_ context.Context, input CreateLocationInput) (Location, error) {
	r.created = input
	return Location{Type: input.Type}, nil
}
func (r *locationTypeRepo) UpdateLocation(_ context.Context, input UpdateLocationInput) (Location, error) {
	r.updated = input
	return Location{}, nil
}
func TestLocationTypesAndCustomerAccess(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{LocationRetail, LocationRetail}, {LocationWarehouse, LocationWarehouse}, {LocationStorage, LocationStorage}, {LocationDropshipper, LocationDropshipper}, {LocationFulfillment, LocationFulfillment}, {"", LocationUnclassified}} {
		got, err := NormalizeLocationType(tc.input)
		if err != nil || got != tc.want {
			t.Fatalf("normalize %q=%q %v", tc.input, got, err)
		}
	}
	if _, err := NormalizeLocationType("arbitrary-purpose"); !errors.Is(err, ErrInvalidLocationType) {
		t.Fatal(err)
	}
	repo := &locationTypeRepo{}
	svc := &Service{repo: repo}
	if _, err := svc.CreateLocation(t.Context(), CreateLocationInput{Slug: "test", Name: "Store", Type: "arbitrary"}); !errors.Is(err, ErrInvalidLocationType) {
		t.Fatal(err)
	}
	if _, err := svc.CreateLocation(t.Context(), CreateLocationInput{Slug: "test", Name: "Store", Type: LocationRetail, StorefrontVisible: true}); err != nil {
		t.Fatal(err)
	}
	if repo.created.PickupEnabled || !repo.created.StorefrontVisible {
		t.Fatal("walk-in visibility must not require pickup")
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
	if _, err := svc.UpdateLocation(t.Context(), UpdateLocationInput{ID: uuid.New(), Type: &retail, StorefrontVisible: &yes}); err != nil {
		t.Fatal(err)
	}
}
