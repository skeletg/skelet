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

func (r *feedRig) product(sku string) uuid.UUID {
	r.t.Helper()
	typeID := r.id(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'rig') RETURNING id`, "rig-"+sku+"-"+r.sfx)
	id := r.id(`INSERT INTO catalog_products (product_type_id, sku, url, kind, default_price) VALUES ($1, $2, $3, 'simple', 1000) RETURNING id`,
		typeID, sku+"-"+r.sfx, strings.ToLower(sku)+"-"+r.sfx)
	r.t.Cleanup(func() {
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

	feed := NewMerchantFeed(db.New(r.conn), nil, fakeSalable{ids[0]: 0, ids[1]: -1, ids[2]: 0})
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
