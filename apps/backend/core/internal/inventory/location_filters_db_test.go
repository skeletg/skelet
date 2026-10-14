package inventory

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func TestLocationClassificationFiltersAndStoreConversion(t *testing.T) {
	conn := testdb.Require(t)
	tx := txrunner.New(conn)
	queries := db.New(conn)
	channels := channel.NewService(channel.NewRepository(queries, tx))
	ch, err := channels.Create(t.Context(), channel.CreateChannelInput{Slug: "types-" + uuid.NewString()[:8], Name: "Types", CurrencyCode: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	if err = queries.SeedStoreAvailabilityDemo(t.Context(), ch.ID); err != nil {
		t.Fatal(err)
	}
	svc := NewService(NewRepository(queries, tx), tx)
	yes, no := true, false
	store, err := svc.GetLocationBySlug(t.Context(), "store-demo-belgrade")
	if err != nil {
		t.Fatal(err)
	}
	rows, count, err := svc.FilteredLocationsPage(t.Context(), "Demo", LocationFilter{Types: []string{LocationRetail}, City: "belgrade", StorefrontVisible: &yes, PickupEnabled: &no}, 1, 0)
	if err != nil || len(rows) != 1 || count != 1 || rows[0].ID != store.ID {
		t.Fatalf("walk-in filter: %+v %d %v", rows, count, err)
	}
	if _, err = svc.SetLocationPickup(t.Context(), store.ID, true, "Collect here"); err != nil {
		t.Fatal(err)
	}
	rows, count, err = svc.FilteredLocationsPage(t.Context(), "Demo", LocationFilter{PickupEnabled: &yes}, 10, 0)
	if err != nil || count != 1 || rows[0].ID != store.ID {
		t.Fatalf("pickup filter: %+v %d %v", rows, count, err)
	}
	private, err := svc.GetLocationBySlug(t.Context(), "store-demo-private")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.UpdateLocation(t.Context(), UpdateLocationInput{ID: private.ID, StorefrontVisible: &yes}); !errors.Is(err, ErrRetailLocationRequired) {
		t.Fatalf("warehouse publication=%v", err)
	}
	warehouse := LocationWarehouse
	if _, err = svc.UpdateLocation(t.Context(), UpdateLocationInput{ID: store.ID, Type: &warehouse}); err != nil {
		t.Fatal(err)
	}
	current, err := svc.GetLocation(t.Context(), store.ID)
	if err != nil || current.StorefrontVisible || current.PickupEnabled {
		t.Fatalf("converted location=%+v %v", current, err)
	}
	page, err := svc.PublicStores(t.Context(), StoreQuery{ChannelID: ch.ID, PreferredStoreID: &store.ID})
	if err != nil || page.TotalCount != 2 || page.PreferredStore != nil {
		t.Fatalf("warehouse leaked: %+v %v", page, err)
	}
	rows, count, err = svc.FilteredLocationsPage(t.Context(), "Demo", LocationFilter{Types: []string{LocationWarehouse}}, 1, 1)
	if err != nil || count != 2 || len(rows) != 1 {
		t.Fatalf("paged warehouse filter: %+v %d %v", rows, count, err)
	}
}
