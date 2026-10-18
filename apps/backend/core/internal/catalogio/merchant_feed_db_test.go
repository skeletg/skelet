package catalogio

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

type feedRig struct {
	t    *testing.T
	conn *sql.DB
	sfx  string
}

func newFeedRig(t *testing.T) *feedRig {
	return &feedRig{t: t, conn: testdb.Require(t), sfx: uuid.NewString()[:8]}
}

func (r *feedRig) exec(q string, args ...any) {
	r.t.Helper()
	if _, err := r.conn.Exec(q, args...); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
}

func (r *feedRig) id(q string, args ...any) uuid.UUID {
	r.t.Helper()
	var id uuid.UUID
	if err := r.conn.QueryRow(q, args...).Scan(&id); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
	return id
}

func (r *feedRig) product(sku string) uuid.UUID { return r.productOf(sku, "simple") }

func (r *feedRig) productOf(sku, kind string) uuid.UUID {
	r.t.Helper()
	typeID := r.id(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'rig') RETURNING id`, "rig-"+sku+"-"+r.sfx)
	id := r.id(`INSERT INTO catalog_products (product_type_id, sku, url, kind, default_price) VALUES ($1, $2, $3, $4, 1000) RETURNING id`,
		typeID, sku+"-"+r.sfx, strings.ToLower(sku)+"-"+r.sfx, kind)
	r.t.Cleanup(func() {
		for _, q := range []string{
			`DELETE FROM catalog_product_children WHERE parent_id = $1 OR child_id = $1`,
			`DELETE FROM catalog_product_kit_components WHERE product_id = $1 OR component_product_id = $1`,
			`DELETE FROM catalog_product_grouped_members WHERE parent_id = $1 OR member_id = $1`,
			`DELETE FROM url_rewrites WHERE entity_id = $1`,
		} {
			_, _ = r.conn.Exec(q, id)
		}
		_, _ = r.conn.Exec(`DELETE FROM catalog_product_channel_listings WHERE product_id = $1`, id)
		_, _ = r.conn.Exec(`DELETE FROM inventory_items WHERE product_id = $1`, id)
		_, _ = r.conn.Exec(`DELETE FROM catalog_products WHERE id = $1`, id)
		_, _ = r.conn.Exec(`DELETE FROM catalog_product_types WHERE id = $1`, typeID)
	})
	return id
}

func (r *feedRig) channel() uuid.UUID {
	r.t.Helper()
	id := r.id(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'rig', 'USD') RETURNING id`, "rig-"+r.sfx)
	r.t.Cleanup(func() { _, _ = r.conn.Exec(`DELETE FROM channels WHERE id = $1`, id) })
	return id
}

type fakeSalable map[uuid.UUID]float64

func (f fakeSalable) SalableByProductIDs(_ context.Context, ids []uuid.UUID, _ uuid.UUID) (map[uuid.UUID]float64, error) {
	out := make(map[uuid.UUID]float64, len(ids))
	for _, id := range ids {
		out[id] = f[id]
	}
	return out, nil
}

func feedItem(t *testing.T, feed, sku string) string {
	t.Helper()
	for chunk := range strings.SplitSeq(feed, "<item>") {
		if strings.Contains(chunk, "<g:id>"+sku+"</g:id>") {
			return chunk
		}
	}
	t.Fatalf("no item for %s in the feed", sku)
	return ""
}

func TestTheMerchantFeedPagesPricesAndCaches(t *testing.T) {
	r := newFeedRig(t)
	ctx := context.Background()
	ch := r.channel()
	skus := []string{"FA", "FB", "FC"}
	ids := make([]uuid.UUID, len(skus))
	for i, sku := range skus {
		ids[i] = r.product(sku)
		r.exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id, price, is_published) VALUES ($1, $2, 900, TRUE)`, ids[i], ch)
	}
	r.exec(`UPDATE catalog_products SET special_price = 500 WHERE id = $1`, ids[0])
	r.exec(`UPDATE catalog_product_channel_listings SET special_price = 300 WHERE product_id = $1`, ids[0])
	r.exec(`INSERT INTO inventory_items (product_id, tracked, backorderable) VALUES ($1, TRUE, FALSE), ($2, TRUE, TRUE)`, ids[0], ids[2])
	long := strings.Repeat("ž", merchantFeedDescriptionMaxRunes+10)
	r.exec(`INSERT INTO catalog_product_content (product_id, locale, name, meta_description) VALUES ($1, 'en', 'Alpha', $2)`, ids[0], long)

	feed := NewMerchantFeed(db.New(r.conn), nil, fakeSalable{ids[0]: 0, ids[1]: -1, ids[2]: 0}, nil)
	feed.pageSize = 2
	render := func() string {
		t.Helper()
		out, _, err := feed.Render(ctx, MerchantFeedOptions{ChannelID: ch, BaseURL: "https://feed.test", CurrencyCode: "USD", Locale: "en", Title: "T"})
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	out := render()
	if n := strings.Count(out, "<item>"); n != 3 {
		t.Fatalf("feed has %d items, want all 3 across two pages", n)
	}
	a := feedItem(t, out, skus[0]+"-"+r.sfx)
	if !strings.Contains(a, "<g:price>3.00 USD</g:price>") {
		t.Errorf("the listing special must win over the product's:\n%s", a)
	}
	if !strings.Contains(a, "out_of_stock") {
		t.Errorf("a tracked product with nothing salable must be out of stock:\n%s", a)
	}
	if !strings.Contains(a, strings.Repeat("ž", merchantFeedDescriptionMaxRunes)+"</description>") || strings.Contains(a, "\uFFFD") {
		t.Errorf("the description must cut at %d characters, whole", merchantFeedDescriptionMaxRunes)
	}
	if b := feedItem(t, out, skus[1]+"-"+r.sfx); !strings.Contains(b, "in_stock") || !strings.Contains(b, "<g:price>9.00 USD</g:price>") {
		t.Errorf("an untracked product at the listing price:\n%s", b)
	}
	if c := feedItem(t, out, skus[2]+"-"+r.sfx); !strings.Contains(c, "backorder") {
		t.Errorf("a backorderable product with nothing salable:\n%s", c)
	}

	r.exec(`UPDATE catalog_product_channel_listings SET is_published = FALSE WHERE product_id = $1`, ids[1])
	if again := render(); again != out {
		t.Error("a second pull inside the TTL re-rendered instead of serving the cached feed")
	}
	feed.now = func() time.Time { return time.Now().Add(2 * merchantFeedCacheTTL) }
	if n := strings.Count(render(), "<item>"); n != 2 {
		t.Errorf("after the TTL the feed has %d items, want the unpublished one gone", n)
	}
}

func (r *feedRig) list(ch uuid.UUID, ids ...uuid.UUID) {
	r.t.Helper()
	for _, id := range ids {
		r.exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id, price, is_published) VALUES ($1, $2, 1000, TRUE)`, id, ch)
	}
}

func (r *feedRig) render(ch uuid.UUID, locale string, salable fakeSalable, locales DefaultLocaler) string {
	r.t.Helper()
	out, _, err := NewMerchantFeed(db.New(r.conn), nil, salable, locales).Render(context.Background(),
		MerchantFeedOptions{ChannelID: ch, BaseURL: "https://feed.test", CurrencyCode: "USD", Locale: locale, Title: "T"})
	if err != nil {
		r.t.Fatal(err)
	}
	return string(out)
}

func TestTheMerchantFeedSellsOnlyPurchasableKinds(t *testing.T) {
	r := newFeedRig(t)
	ch := r.channel()
	conf, v1, v2 := r.productOf("CONF", "configurable"), r.product("V1"), r.product("V2")
	hidden, hv := r.productOf("HIDDEN", "configurable"), r.product("HV")
	grouped, bundle := r.productOf("GRP", "grouped"), r.productOf("BUN", "bundle")
	kitOK, kitOut, c1, c2 := r.productOf("KITOK", "kit"), r.productOf("KITOUT", "kit"), r.product("C1"), r.product("C2")
	r.list(ch, conf, v1, v2, hv, grouped, bundle, kitOK, kitOut)
	r.exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id, price, is_published) VALUES ($1, $2, 1000, FALSE)`, hidden, ch)
	r.exec(`INSERT INTO catalog_product_children (parent_id, child_id, sort_order) VALUES ($1, $2, 0), ($1, $3, 1), ($4, $5, 0)`, conf, v1, v2, hidden, hv)
	r.exec(`INSERT INTO url_rewrites (entity_type, entity_id, request_path, redirect_type) VALUES ('product', $1, $2, 0), ('product', $3, $4, 0)`,
		conf, "conf-page-"+r.sfx, v2, "v2-page-"+r.sfx)
	r.exec(`INSERT INTO catalog_product_kit_components (product_id, component_product_id, quantity) VALUES ($1, $3, 2), ($1, $4, 1), ($2, $3, 6)`,
		kitOK, kitOut, c1, c2)

	out := r.render(ch, "en", fakeSalable{c1: 5, c2: -1, v1: -1, v2: -1}, nil)
	for _, sku := range []string{"CONF", "HIDDEN", "HV", "GRP", "BUN"} {
		if strings.Contains(out, "<g:id>"+sku+"-"+r.sfx+"</g:id>") {
			t.Errorf("%s is in the feed - parents, bundles and variants of an unpublished parent are not sold as items", sku)
		}
	}
	confSKU := "CONF-" + r.sfx
	a := feedItem(t, out, "V1-"+r.sfx)
	if !strings.Contains(a, "<g:item_group_id>"+confSKU+"</g:item_group_id>") || !strings.Contains(a, "/conf-page-"+r.sfx+"</link>") {
		t.Errorf("a variant without a path of its own groups under and links to its parent:\n%s", a)
	}
	if b := feedItem(t, out, "V2-"+r.sfx); !strings.Contains(b, "/v2-page-"+r.sfx+"</link>") {
		t.Errorf("a variant with its own path links there:\n%s", b)
	}
	if k := feedItem(t, out, "KITOK-"+r.sfx); !strings.Contains(k, availabilityInStock) {
		t.Errorf("5 of a component used twice makes 2 kits, the untracked one doesn't constrain:\n%s", k)
	}
	if k := feedItem(t, out, "KITOUT-"+r.sfx); !strings.Contains(k, availabilityOutOfStock) {
		t.Errorf("5 of a component used six times makes no kit:\n%s", k)
	}
}

type fixedFeedLocale string

func (f fixedFeedLocale) DefaultLocale(context.Context) string { return string(f) }

func TestTheMerchantFeedFallsBackToTheDefaultLocale(t *testing.T) {
	r := newFeedRig(t)
	ch := r.channel()
	id := r.product("LOC")
	r.list(ch, id)
	r.exec(`INSERT INTO locales (code, name) VALUES ('de', 'German') ON CONFLICT DO NOTHING`)
	r.exec(`INSERT INTO catalog_product_content (product_id, locale, name, meta_description) VALUES ($1, 'en', 'Only English', 'English copy')`, id)
	item := feedItem(t, r.render(ch, "de", fakeSalable{id: -1}, fixedFeedLocale("en")), "LOC-"+r.sfx)
	if !strings.Contains(item, "<title>Only English</title>") || !strings.Contains(item, "English copy") {
		t.Errorf("a product with only default-locale content rendered without it:\n%s", item)
	}
}

func TestTruncateRunesNeverSplitsACharacter(t *testing.T) {
	for _, c := range []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"žžžž", 2, "žž"},
		{"a€b", 2, "a€"},
		{"", 3, ""},
	} {
		got := truncateRunes(c.in, c.n)
		if got != c.want || !utf8.ValidString(got) {
			t.Errorf("truncateRunes(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}
