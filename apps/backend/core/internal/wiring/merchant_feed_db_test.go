package wiring

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

type untrackedSalable struct{}

func (untrackedSalable) SalableByProductIDs(_ context.Context, ids []uuid.UUID, _ uuid.UUID) (map[uuid.UUID]float64, error) {
	out := make(map[uuid.UUID]float64, len(ids))
	for _, id := range ids {
		out[id] = -1
	}
	return out, nil
}

// An unknown ?locale= renders the channel default - it keys the feed cache, so a crawler can't mint an entry per made-up code or get an untitled feed
func TestTheMerchantFeedSnapsAnUnknownLocaleToTheDefault(t *testing.T) {
	conn := testdb.Require(t)
	ch, typeID, pid := uuid.New(), uuid.New(), uuid.New()
	sku := "LOC-" + ch.String()[:8]
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO channels (id, slug, name, currency_code) VALUES ($1, $2, 'loc', 'USD')`, []any{ch, "loc-" + ch.String()[:8]}},
		{`INSERT INTO catalog_product_types (id, slug, name) VALUES ($1, $2, 'loc')`, []any{typeID, "loc-" + ch.String()[:8]}},
		{`INSERT INTO catalog_products (id, product_type_id, sku, url, kind) VALUES ($1, $2, $3, $3, 'simple')`, []any{pid, typeID, sku}},
		{`INSERT INTO catalog_product_channel_listings (product_id, channel_id, price, is_published) VALUES ($1, $2, 100, TRUE)`, []any{pid, ch}},
		{`INSERT INTO catalog_product_content (product_id, locale, name) VALUES ($1, 'en', 'English title')`, []any{pid}},
	} {
		if _, err := conn.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	feed := catalog.NewMerchantFeed(db.New(conn), nil, untrackedSalable{})
	sv := &channel.Channel{ID: ch, Name: "loc", CurrencyCode: "USD", DefaultLocale: "en", PublicURL: "https://loc.test", Locales: []locale.Locale{{Code: "en", IsActive: true}}}
	req := httptest.NewRequest("GET", "/feeds/products.xml?locale=zz", nil)
	req = req.WithContext(channel.WithChannel(req.Context(), sv))
	rec := httptest.NewRecorder()
	merchantFeedHandler(feed)(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<title>English title</title>") {
		t.Errorf("an unknown locale didn't fall back to the default content:\n%s", rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != merchantFeedCacheControl {
		t.Errorf("Cache-Control = %q", got)
	}
}
