package catalog

import (
	"context"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/testdb"
)

// A facet panel shows at most MaxValues buckets, so which ones get cut matters - ordering by sort_order then truncating drops by POSITION, which on a high-cardinality attribute silently hides the biggest buckets: measured on a 141-value attribute it kept a 193-product value and dropped a 901-product one while the index (a terms agg, so count-ordered) kept the big one, two engines disagreeing about which filters exist - gated on DATABASE_URL because the ordering is in the SQL, and the engine-level test couldn't see this because its fake catalog replaces the very query that does the ordering
func TestFacetCountsKeepsTheBiggestBuckets(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()

	var channelID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel: %v", err)
	}
	var productTypeID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM catalog_product_types LIMIT 1`).Scan(&productTypeID); err != nil {
		t.Skipf("no product type: %v", err)
	}

	txn, err := conn.BeginTx(ctx, nil) // always rolled back, so the test leaves no rows behind and can run against a populated database
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = txn.Rollback() }()

	var attrID uuid.UUID
	if err := txn.QueryRow(
		`INSERT INTO catalog_attributes (slug, name, input_type, filterable)
		 VALUES ('facet-order-test', 'Facet Order Test', 'dropdown', true) RETURNING id`,
	).Scan(&attrID); err != nil {
		t.Fatal(err)
	}

	// Value i gets i products AND sort_order i so the two orderings disagree as much as possible - by position the first buckets are the emptiest, by popularity the fullest, and a fixture where the two agree would pass either way
	const values = 12
	valueIDs := make([]uuid.UUID, values+1)
	for i := 1; i <= values; i++ {
		if err := txn.QueryRow(
			`INSERT INTO catalog_attribute_values (attribute_id, slug, name, value, sort_order)
			 VALUES ($1, 'v' || $2::text, 'V' || $2::text, 'V' || $2::text, $3) RETURNING id`,
			attrID, i, i,
		).Scan(&valueIDs[i]); err != nil {
			t.Fatal(err)
		}
		for n := 0; n < i; n++ {
			var pid uuid.UUID
			if err := txn.QueryRow(
				`INSERT INTO catalog_products (product_type_id, sku, url, kind)
				 VALUES ($1, 'FACETORD-' || $2::text || '-' || $3::text, 'facetord-' || $2::text || '-' || $3::text, 'simple')
				 RETURNING id`, productTypeID, i, n,
			).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			if _, err := txn.Exec(
				`INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published)
				 VALUES ($1, $2, true)`, pid, channelID); err != nil {
				t.Fatal(err)
			}
			if _, err := txn.Exec(
				`INSERT INTO catalog_product_attribute_values (product_id, attribute_id, attribute_value_id)
				 VALUES ($1, $2, $3)`, pid, attrID, valueIDs[i]); err != nil {
				t.Fatal(err)
			}
		}
	}

	const maxValues = 4
	rows, err := db.New(txn).BrowseFacetCountsFallback(ctx, db.BrowseFacetCountsFallbackParams{
		TargetAttributeID: attrID,
		ChannelID:         channelID,
		Kinds:             []string{},
		Search:            "",
		OtherValueIds:     []uuid.UUID{},
		OtherAttrCount:    0,
		MaxValues:         maxValues,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != maxValues {
		t.Fatalf("got %d buckets, want %d", len(rows), maxValues)
	}
	// The four biggest are values 12, 11, 10, 9 - and they must come back in that order, since the index returns count-descending and the tiers have to agree
	want := []int64{12, 11, 10, 9}
	for n, row := range rows {
		if row.ProductCount != want[n] {
			t.Errorf("bucket %d = %s with %d products, want %d - truncated by position rather than popularity",
				n, row.Slug, row.ProductCount, want[n])
		}
	}
}
