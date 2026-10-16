package front

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/configstore"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func frontStorePtr[T any](v T) *T { return &v }

func TestPublicStoresVisibilityAndFeatureScope(t *testing.T) {
	conn := testdb.Require(t)
	q := db.New(conn)
	tx := txrunner.New(conn)
	channels := channel.NewService(channel.NewRepository(q, tx))
	ch, err := channels.Create(t.Context(), channel.CreateChannelInput{Slug: "store-vis-" + uuid.NewString()[:8], Name: "Store visibility", CurrencyCode: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := channels.Create(t.Context(), channel.CreateChannelInput{Slug: "store-other-" + uuid.NewString()[:8], Name: "Other", CurrencyCode: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	if err = q.SeedStoreAvailabilityDemo(t.Context(), ch.ID); err != nil {
		t.Fatal(err)
	}
	// Re-running fixtures must not duplicate stores, products or levels.
	if err = q.SeedStoreAvailabilityDemo(t.Context(), ch.ID); err != nil {
		t.Fatal(err)
	}
	cfg := config.NewService(configstore.New(q))
	inv := inventory.NewService(inventory.NewRepository(q, tx), tx)
	r := &queryResolver{&Resolver{ConfigSvc: cfg, InventorySvc: inv, CatalogSvc: catalog.NewService(catalog.Deps{Repo: catalog.NewRepository(q, tx), Tx: tx})}}
	ctx := channel.WithChannel(t.Context(), &ch)
	page, err := r.PublicStores(ctx, nil)
	if err != nil || page.Enabled {
		t.Fatalf("default gate=%+v %v", page, err)
	}
	if _, err = cfg.Set(ctx, "sales/store_availability/enabled", "true", config.DefaultScope); err != nil {
		t.Fatal(err)
	}
	backpack, err := r.CatalogSvc.GetProductBySKU(ctx, "STORE-DEMO-BACKPACK")
	if err != nil {
		t.Fatal(err)
	}
	input := &model.PublicStoresInput{ProductID: frontStorePtr(backpack.ID.String())}
	page, err = r.PublicStores(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Enabled || !page.ProductEligible || len(page.Items) != 3 || page.TotalCount != 3 {
		t.Fatalf("public page=%+v", page)
	}
	hoodie, err := r.CatalogSvc.GetProductBySKU(ctx, "STORE-DEMO-HOODIE")
	if err != nil {
		t.Fatal(err)
	}
	// A configurable parent has no store stock of its own, but the directory must still list the stores
	parentPage, err := r.PublicStores(ctx, &model.PublicStoresInput{ProductID: frontStorePtr(hoodie.ID.String())})
	if err != nil || parentPage.ProductEligible || parentPage.TotalCount != 3 || !parentPage.StockCheckingAvailable {
		t.Fatalf("configurable parent page=%+v %v", parentPage, err)
	}
	for _, store := range page.Items {
		if store.Slug == "store-demo-private" {
			t.Fatal("private warehouse leaked")
		}
	}
	otherCtx := channel.WithChannel(t.Context(), &other)
	otherPage, err := r.PublicStores(otherCtx, nil)
	if err != nil || len(otherPage.Items) != 0 {
		t.Fatalf("cross-channel stores=%+v %v", otherPage, err)
	}
	if _, err = r.PublicStores(otherCtx, input); !errors.Is(err, catalog.ErrProductNotFound) {
		t.Fatalf("hidden product=%v", err)
	}
	// Channel override wins over the global enabled value.
	if _, err = cfg.Set(ctx, "sales/store_availability/enabled", "false", config.Scope{Kind: config.ScopeChannel, ChannelID: &ch.ID}); err != nil {
		t.Fatal(err)
	}
	page, err = r.PublicStores(ctx, input)
	if err != nil || page.Enabled || len(page.Items) != 0 {
		t.Fatalf("channel disabled=%+v %v", page, err)
	}
	if _, err = cfg.Set(ctx, "sales/store_availability/enabled", "true", config.Scope{Kind: config.ScopeChannel, ChannelID: &ch.ID}); err != nil {
		t.Fatal(err)
	}
	public, err := inv.GetLocationBySlug(ctx, "store-demo-belgrade")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = inv.UpdateLocation(ctx, inventory.UpdateLocationInput{ID: public.ID, IsActive: frontStorePtr(false)}); err != nil {
		t.Fatal(err)
	}
	input.PreferredStoreID = frontStorePtr(public.ID.String())
	page, err = r.PublicStores(ctx, input)
	if err != nil || page.TotalCount != 2 || page.PreferredStore != nil {
		t.Fatalf("inactive/preferred=%+v %v", page, err)
	}
	if _, err = r.CatalogSvc.SetProductListingPublished(ctx, backpack.ID, ch.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = r.PublicStores(ctx, input); !errors.Is(err, catalog.ErrProductNotFound) {
		t.Fatalf("unpublished product=%v", err)
	}
}
