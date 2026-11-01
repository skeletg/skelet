package catalog

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestProductPageFilters(t *testing.T) {
	r := newCatalogRig(t)
	ctx := context.Background()
	r.exec(`INSERT INTO currencies (code, name, decimals) VALUES ('JPY', 'Yen', 0) ON CONFLICT (code) DO NOTHING`)
	pt := r.ptype("", false)
	names := map[uuid.UUID]string{}
	mk := func(name, currency string, price int64) uuid.UUID {
		p := r.newProduct(pt.ID, KindSimple, name+"-"+r.sfx, name, currency)
		r.exec(`UPDATE catalog_products SET default_price = $2 WHERE id = $1`, p.ID, price)
		names[p.ID] = name
		return p.ID
	}
	shirt, tee, mug := mk("shirt", "USD", 1250), mk("tee", "USD", 500), mk("mug", "JPY", 1500)

	clothing := r.category(nil, "Clothing")
	shirts := r.category(&clothing, "Shirts")
	r.exec(`INSERT INTO catalog_product_categories (product_id, category_id) VALUES ($1, $2), ($3, $4)`, shirt, shirts, tee, clothing)
	col := r.id(`INSERT INTO catalog_collections (slug) VALUES ($1) RETURNING id`, "col-"+r.sfx)
	r.exec(`INSERT INTO catalog_collection_products (collection_id, product_id) VALUES ($1, $2)`, col, mug)

	shelf := r.id(`INSERT INTO inventory_locations (slug, name) VALUES ($1, 'Shelf') RETURNING id`, "shelf-"+r.sfx)
	stock := func(p uuid.UUID, onShelf, flat float64) {
		item := r.id(`INSERT INTO inventory_items (product_id, tracked, quantity) VALUES ($1, true, $2) RETURNING id`, p, flat)
		r.exec(`INSERT INTO inventory_levels (inventory_item_id, location_id, quantity) VALUES ($1, $2, $3)`, item, shelf, onShelf)
	}
	stock(shirt, 3, 0)
	stock(tee, 0, 4)
	stocked, bare := r.channel(), r.channel()
	r.exec(`INSERT INTO inventory_channel_locations (channel_id, location_id) VALUES ($1, $2)`, stocked, shelf)
	for _, ch := range []uuid.UUID{stocked, bare} {
		r.exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id) VALUES ($1, $2)`, shirt, ch)
	}

	yes, no := true, false
	for _, c := range []struct {
		name string
		f    ProductPageFilter
		want []uuid.UUID
	}{
		{"a category reaches its subcategories", ProductPageFilter{CategoryIDs: []uuid.UUID{clothing}}, []uuid.UUID{shirt, tee}},
		{"a subcategory alone", ProductPageFilter{CategoryIDs: []uuid.UUID{shirts}}, []uuid.UUID{shirt}},
		{"a collection", ProductPageFilter{CollectionIDs: []uuid.UUID{col}}, []uuid.UUID{mug}},
		{"a minimum in each product's own currency", ProductPageFilter{PriceMin: "10"}, []uuid.UUID{shirt, mug}},
		{"a maximum", ProductPageFilter{PriceMax: "12.50"}, []uuid.UUID{shirt, tee}},
		{"a yen amount isn't read as cents", ProductPageFilter{PriceMin: "1000"}, []uuid.UUID{mug}},
		{"in stock anywhere, no item counting as in stock", ProductPageFilter{InStock: &yes}, []uuid.UUID{shirt, mug}},
		{"out of stock", ProductPageFilter{InStock: &no}, []uuid.UUID{tee}},
		{"flat mode reads the item's own quantity", ProductPageFilter{InStock: &yes, FlatInventory: true}, []uuid.UUID{tee, mug}},
		{"a store counts only its own locations", ProductPageFilter{ChannelID: &stocked, InStock: &yes}, []uuid.UUID{shirt}},
		{"a store with no locations has nothing in stock", ProductPageFilter{ChannelID: &bare, InStock: &no}, []uuid.UUID{shirt}},
		{"filters combine", ProductPageFilter{CategoryIDs: []uuid.UUID{clothing}, PriceMin: "10", InStock: &yes}, []uuid.UUID{shirt}},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.f.Locale, c.f.Search, c.f.Limit = "en", r.sfx, 50
			ids, total, err := r.repo.ListProductIDsPaged(ctx, c.f)
			if err != nil {
				t.Fatal(err)
			}
			label := func(in []uuid.UUID) string {
				out := make([]string, len(in))
				for i, id := range in {
					out[i] = names[id]
				}
				slices.Sort(out)
				return strings.Join(out, ",")
			}
			if got, want := label(ids), label(c.want); got != want || total != int64(len(c.want)) {
				t.Errorf("got [%s] (total %d), want [%s]", got, total, want)
			}
		})
	}
}
