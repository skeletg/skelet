package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
)

func TestInventoryLevelAvailableMath(t *testing.T) { // netReservation is NEGATIVE when stock is held, and this Available is the number the operator trusts before promising an order
	for name, tc := range map[string]struct {
		quantity, net           float64
		wantAvailable, wantResv float64
	}{
		"no reservations":            {quantity: 10, net: 0, wantAvailable: 10, wantResv: 0},
		"3 held":                     {quantity: 10, net: -3, wantAvailable: 7, wantResv: -3},
		"everything held":            {quantity: 10, net: -10, wantAvailable: 0, wantResv: -10},
		"over-held":                  {quantity: 10, net: -12, wantAvailable: 0, wantResv: -12}, // clamped - a negative availability would render as sellable -2
		"pending return inflow":      {quantity: 10, net: 4, wantAvailable: 10, wantResv: 4},    // positive net is inbound, not sellable until received
		"zero stock with a hold":     {quantity: 0, net: -2, wantAvailable: 0, wantResv: -2},
		"fractional unit of measure": {quantity: 2.5, net: -0.5, wantAvailable: 2, wantResv: -0.5},
	} {
		m := toInventoryLevelModel(inventory.Level{Quantity: tc.quantity}, inventory.Location{ID: uuid.New()}, tc.net)
		if m.Available != tc.wantAvailable {
			t.Errorf("%s: Available = %v, want %v", name, m.Available, tc.wantAvailable)
		}
		if m.ReservedQuantity != tc.wantResv {
			t.Errorf("%s: ReservedQuantity = %v, want %v", name, m.ReservedQuantity, tc.wantResv)
		}
		if m.Quantity != tc.quantity {
			t.Errorf("%s: Quantity = %v, want the raw on-hand %v", name, m.Quantity, tc.quantity)
		}
	}
}

func TestInventoryLocationHoursNeverNull(t *testing.T) { // Hours is a JSON scalar the dashboard feeds straight into an opening-hours widget that crashes on null
	m := toInventoryLocationModel(inventory.Location{ID: uuid.New(), Slug: "bg-01"})
	hours, ok := m.Hours.(map[string]any)
	if !ok || hours == nil {
		t.Errorf("Hours = %#v for a location with none stored, want an empty object", m.Hours)
	}

	withHours := toInventoryLocationModel(inventory.Location{ID: uuid.New(), Hours: []byte(`{"mon":"9-17"}`)})
	got, ok := withHours.Hours.(map[string]any)
	if !ok || got["mon"] != "9-17" {
		t.Errorf("Hours = %#v, want the stored object decoded", withHours.Hours)
	}

	malformed := toInventoryLocationModel(inventory.Location{ID: uuid.New(), Hours: []byte(`{broken`)})
	if _, ok := malformed.Hours.(map[string]any); !ok {
		t.Errorf("Hours = %#v for a malformed value, want the empty-object fallback", malformed.Hours)
	}
}

func TestInvMetadataItemsAreSortedAndStable(t *testing.T) { // map iteration order would reshuffle the admin UI's metadata list on every refetch
	m := inventory.Metadata{"zone": "A", "aisle": "7", "bin": "B3"}
	got := invMetadataItems(m)
	if len(got) != 3 {
		t.Fatalf("%d items, want 3", len(got))
	}
	for i, want := range []string{"aisle", "bin", "zone"} {
		if got[i].Key != want {
			t.Errorf("item[%d] = %q, want %q", i, got[i].Key, want)
		}
	}
	if got[0].Value != "7" {
		t.Errorf("aisle = %q, want 7", got[0].Value)
	}
	if empty := invMetadataItems(nil); empty == nil || len(empty) != 0 {
		t.Errorf("invMetadataItems(nil) = %v, want an empty non-nil list", empty)
	}
}

func TestInvMetadataFromInputSkipsNilItems(t *testing.T) { // gqlgen delivers a list of pointers, and a null element must not panic the mutation
	got := invMetadataFromInput([]*model.MetadataInput{
		{Key: "zone", Value: "A"},
		nil,
		{Key: "bin", Value: "B3"},
	})
	if len(got) != 2 || got["zone"] != "A" || got["bin"] != "B3" {
		t.Errorf("metadata = %v, want the two real items", got)
	}
	if empty := invMetadataFromInput(nil); empty == nil || len(empty) != 0 {
		t.Errorf("invMetadataFromInput(nil) = %v, want an empty non-nil map", empty)
	}
}

func TestResolveSyncProductIDByID(t *testing.T) { // the id path must not touch the catalog service - a nil service panicking would prove it did
	id := uuid.New()
	idStr := " " + id.String() + " " // ERP payloads pad things
	got, err := resolveSyncProductID(context.Background(), nil, model.SyncProductInventoryInput{ProductID: &idStr})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != id {
		t.Errorf("id = %s, want %s", got, id)
	}
}

func TestResolveSyncProductIDRefusals(t *testing.T) {
	ctx := context.Background()
	bad := "not-a-uuid"
	blank := "   "

	if _, err := resolveSyncProductID(ctx, nil, model.SyncProductInventoryInput{ProductID: &bad}); err == nil {
		t.Error("a malformed product id resolved")
	}
	if _, err := resolveSyncProductID(ctx, nil, model.SyncProductInventoryInput{}); err == nil {
		t.Error("an input with neither selector resolved")
	}
	// a whitespace-only SKU falls through to the id, so an ERP sending "sku": " " doesn't 500 on the catalog lookup
	id := uuid.New()
	idStr := id.String()
	got, err := resolveSyncProductID(ctx, nil, model.SyncProductInventoryInput{ProductSku: &blank, ProductID: &idStr})
	if err != nil || got != id {
		t.Errorf("got %s err %v, want the blank sku ignored and the id used", got, err)
	}
}
