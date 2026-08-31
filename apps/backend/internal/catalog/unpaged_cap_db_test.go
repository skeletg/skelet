package catalog

import (
	"context"
	"testing"

	_ "github.com/lib/pq"

	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/testdb"
)

// ListProducts carries a row cap because the uncapped list is 101k rows and ~17MB on a real
// catalog - but the CSV export, the search reindexer and the URL-rewrite backfill all pass 0 and
// MUST still see everything, so the 0-means-unlimited sentinel is the load-bearing part
//
// DB-backed because the sentinel is LIMIT NULLIF($1, 0) - only Postgres can say whether it holds

func capTestQueries(t *testing.T) (*db.Queries, int) {
	t.Helper()
	conn := testdb.Require(t)
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
