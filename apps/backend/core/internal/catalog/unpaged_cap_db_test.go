package catalog

import (
	"context"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// ListProducts carries a row cap because the uncapped list is 101k rows and ~17MB on a real catalog
// but the CSV export, the search reindexer and the URL-rewrite backfill all pass 0 and MUST still see everything, so the 0-means-unlimited sentinel is the load-bearing part

// Seeds three products of its own so a cap of 2 has something to cut on a freshly migrated database, then
// counts what the table holds - the rows other packages' tests leave behind are part of "everything" too
func capTestQueries(t *testing.T) (*db.Queries, int) {
	t.Helper()
	conn := testdb.Require(t)
	suffix := uuid.NewString()[:8]
	var typeID uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO catalog_product_types (slug, name) VALUES ($1, $2) RETURNING id`,
		"cap-type-"+suffix, "cap fixture").Scan(&typeID); err != nil {
		t.Fatalf("seed product type: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM catalog_product_types WHERE id = $1`, typeID) }) // runs after the product delete below
	for i := range 3 {
		var id uuid.UUID
		if err := conn.QueryRow(
			`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $3, 'simple') RETURNING id`,
			typeID, "CAP-"+suffix+"-"+string(rune('a'+i)), "cap-"+suffix+"-"+string(rune('a'+i))).Scan(&id); err != nil {
			t.Fatalf("seed product: %v", err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM catalog_products WHERE id = $1`, id) })
	}
	var total int
	if err := conn.QueryRow(`SELECT count(*) FROM catalog_products`).Scan(&total); err != nil {
		t.Fatalf("count products: %v", err)
	}
	return db.New(conn), total
}

func TestListProductsZeroLimitReturnsEverything(t *testing.T) {
	q, total := capTestQueries(t)
	rows, err := q.ListProducts(context.Background(), 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != total {
		t.Errorf("row_limit 0 returned %d of %d products - the export would ship a partial catalog", len(rows), total)
	}
}

func TestListProductsAppliesTheCap(t *testing.T) {
	q, total := capTestQueries(t)
	if total < 3 {
		t.Skipf("need a few products to cap, have %d", total)
	}
	rows, err := q.ListProducts(context.Background(), 2)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("row_limit 2 returned %d rows, want 2 - an uncapped list is reachable by any catalog viewer", len(rows))
	}
}

// The cap only helps if it reaches SQL - truncating in Go still makes Postgres build every row
func TestMaxUnpagedRowsIsSane(t *testing.T) {
	if MaxUnpagedRows <= 0 {
		t.Fatalf("MaxUnpagedRows = %d - a non-positive cap means unlimited, which is the bug it exists to prevent", MaxUnpagedRows)
	}
}
