package catalog

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestTheSaleReadsOnlyThisStoresPublishedPriceCuts(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	var typeID, ch, other uuid.UUID
	mustScan(t, conn, &typeID, `INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'sale') RETURNING id`, "sale-"+suffix)
	mustScan(t, conn, &ch, `INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'sale', 'USD') RETURNING id`, "sale-"+suffix)
	mustScan(t, conn, &other, `INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'other', 'USD') RETURNING id`, "other-"+suffix)
	hour := time.Hour
	product := func(name string, price int64, compareAt *int64) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		mustScan(t, conn, &id, `INSERT INTO catalog_products (product_type_id, sku, url, kind, default_price, compare_at_price) VALUES ($1, $2, $2, 'simple', $3, $4) RETURNING id`,
			typeID, name+"-"+suffix, price, compareAt)
		return id
	}
	list := func(id, store uuid.UUID, published bool, special *int64, from, to *time.Time) {
		t.Helper()
		if _, err := conn.Exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published, special_price, special_from, special_to) VALUES ($1, $2, $3, $4, $5, $6)`,
			id, store, published, special, from, to); err != nil {
			t.Fatal(err)
		}
	}
	at := func(d time.Duration) *time.Time { v := time.Now().Add(d); return &v }
	cents := func(v int64) *int64 { return &v }

	half := product("half", 1000, nil)
	list(half, ch, true, cents(500), at(-hour), at(hour))
	quarter := product("quarter", 750, cents(1000))
	list(quarter, ch, true, nil, nil, nil)
	expired := product("expired", 1000, nil)
	list(expired, ch, true, cents(100), at(-2*hour), at(-hour))
	upcoming := product("upcoming", 1000, nil)
	list(upcoming, ch, true, cents(100), at(hour), nil)
	noise := product("noise", 1000, cents(900))
	list(noise, ch, true, nil, nil, nil)
	hidden := product("hidden", 1000, cents(5000))
	list(hidden, ch, false, nil, nil, nil)
	elsewhere := product("elsewhere", 1000, cents(5000))
	list(elsewhere, other, true, nil, nil, nil)
	parent, variant := product("parent", 1000, nil), product("variant", 1000, cents(5000))
	list(parent, ch, true, nil, nil, nil)
	list(variant, ch, true, nil, nil, nil)
	if _, err := conn.Exec(`INSERT INTO catalog_product_children (parent_id, child_id) VALUES ($1, $2)`, parent, variant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM catalog_products WHERE product_type_id = $1`, typeID)
		_, _ = conn.Exec(`DELETE FROM catalog_product_types WHERE id = $1`, typeID)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = ANY($1)`, []uuid.UUID{ch, other})
	})

	repo := &postgresRepository{queries: db.New(conn)}
	svc := &listingService{core: &core{repo: repo}, repo: repo}
	got, err := svc.SaleProductIDs(ctx, ch, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := []uuid.UUID{half, quarter}; !slices.Equal(got, want) {
		t.Fatalf("sale = %v, want %v - expired, upcoming, at-or-below, unpublished, other-store and variant prices aren't this store's sale", got, want)
	}
	cands, err := repo.ListSaleCandidates(ctx, ch, 1)
	if err != nil || len(cands) != 1 || cands[0].ProductID != half {
		t.Errorf("a capped scan kept %+v, %v - the deepest cut should come first", cands, err)
	}
}

func mustScan(t *testing.T, conn *sql.DB, into *uuid.UUID, query string, args ...any) {
	t.Helper()
	if err := conn.QueryRow(query, args...).Scan(into); err != nil {
		t.Fatal(err)
	}
}
