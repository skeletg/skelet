package front

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type structureFixture struct {
	t      *testing.T
	r      *productResolver
	ch     uuid.UUID
	typeID uuid.UUID
	sfx    string
}

func newStructureFixture(t *testing.T) *structureFixture {
	conn := testdb.Require(t)
	f := &structureFixture{t: t, ch: uuid.New(), sfx: uuid.NewString()[:8]}
	if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code) VALUES ($1, $2, 'kits', 'USD')`, f.ch, "kits-"+f.sfx); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'T') RETURNING id`, "kits-"+f.sfx).Scan(&f.typeID); err != nil {
		t.Fatal(err)
	}
	q, tx := db.New(conn), txrunner.New(conn)
	f.r = &productResolver{&Resolver{
		CatalogSvc:   catalog.NewService(catalog.Deps{Repo: catalog.NewRepository(q, tx), Tx: tx}),
		InventorySvc: inventory.NewService(inventory.NewRepository(q, tx), tx),
	}}
	return f
}

// An untracked product of kind, weighing grams (0 = unset), listed on the fixture's store when listed
func (f *structureFixture) product(kind string, grams int, listed bool) (uuid.UUID, string) {
	f.t.Helper()
	conn := testdb.Require(f.t)
	sku := kind + "-" + uuid.NewString()[:8]
	var id uuid.UUID
	if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind, weight_grams) VALUES ($1, $2, $2, $3, NULLIF($4, 0)) RETURNING id`,
		f.typeID, sku, kind, grams).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	if listed {
		if _, err := conn.Exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published) VALUES ($1, $2, TRUE)`, id, f.ch); err != nil {
			f.t.Fatal(err)
		}
	}
	return id, sku
}

func (f *structureFixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := testdb.Require(f.t).Exec(query, args...); err != nil {
		f.t.Fatal(err)
	}
}

// What a kit's box holds is told whole, but only a component this store sells links to its page
func TestAKitShowsItsContentsLinkingOnlyWhatTheStoreSells(t *testing.T) {
	f := newStructureFixture(t)
	kit, _ := f.product("kit", 0, true)
	sold, _ := f.product("simple", 0, true)
	notSold, _ := f.product("simple", 0, false)
	f.exec(`INSERT INTO catalog_product_kit_components (product_id, component_product_id, quantity, sort_order) VALUES ($1, $2, 2, 0), ($1, $3, 1, 1)`, kit, sold, notSold)
	ctx := channel.WithChannel(t.Context(), &channel.Channel{ID: f.ch})

	got, err := f.r.productKitComponents(ctx, &model.Product{ID: kit.String(), Kind: model.ProductKindKit})
	if err != nil || len(got) != 2 {
		t.Fatalf("kit components = %d (%v), want both", len(got), err)
	}
	if got[0].Quantity != 2 || got[0].Component.URL == "" {
		t.Errorf("sold component = %+v (url %q), want 2 per kit with its url", got[0], got[0].Component.URL)
	}
	if got[1].Component.URL != "" {
		t.Errorf("unsold component url = %q, want none", got[1].Component.URL)
	}
}

// A kit with no size of its own is rated by what its items weigh - one that has a weight keeps it
func TestAWeightlessKitShipsAtItsItemsWeight(t *testing.T) {
	f := newStructureFixture(t)
	bare, bareSKU := f.product("kit", 0, true)
	weighed, weighedSKU := f.product("kit", 50, true)
	mug, _ := f.product("simple", 200, true)
	tea, _ := f.product("simple", 300, true)
	for _, k := range []uuid.UUID{bare, weighed} {
		f.exec(`INSERT INTO catalog_product_kit_components (product_id, component_product_id, quantity, sort_order) VALUES ($1, $2, 2, 0), ($1, $3, 1, 1)`, k, mug, tea)
	}
	dims, err := f.r.CatalogSvc.ProductDimensionsBySKUs(t.Context(), []string{bareSKU, weighedSKU})
	if err != nil {
		t.Fatal(err)
	}
	if got := dims[bareSKU].WeightGrams; got != 700 {
		t.Errorf("weightless kit = %dg, want its items' 2x200 + 300", got)
	}
	if got := dims[weighedSKU].WeightGrams; got != 50 {
		t.Errorf("weighed kit = %dg, want its own 50", got)
	}
}

// A grouped product is only buyable here through a member this store sells
func TestAGroupedProductIsInStockOnlyThroughAMemberTheStoreSells(t *testing.T) {
	f := newStructureFixture(t)
	group, _ := f.product("grouped", 0, true)
	member, _ := f.product("simple", 0, false)
	f.exec(`INSERT INTO catalog_product_grouped_members (parent_id, member_id) VALUES ($1, $2)`, group, member)
	ctx := channel.WithChannel(t.Context(), &channel.Channel{ID: f.ch})

	if in, err := f.r.wrapperInStock(ctx, model.ProductKindGrouped, group, f.ch); err != nil || in {
		t.Errorf("group whose only member isn't sold here: in stock %v (%v), want false", in, err)
	}
	f.exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published) VALUES ($1, $2, TRUE)`, member, f.ch)
	if in, err := f.r.wrapperInStock(ctx, model.ProductKindGrouped, group, f.ch); err != nil || !in {
		t.Errorf("group with its member sold here: in stock %v (%v), want true", in, err)
	}
}
