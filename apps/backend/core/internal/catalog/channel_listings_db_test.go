package catalog

import (
	"context"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestTheChannelBatchReadsOnlyThatChannelsListings(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	var typeID, productID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'fx') RETURNING id`, "lst-"+suffix).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, "LST-"+suffix).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	chans := make([]uuid.UUID, 3)
	for i := range chans {
		if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'fx', 'USD') RETURNING id`, "lst-"+suffix+string(rune('a'+i))).Scan(&chans[i]); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published) VALUES ($1, $2, true)`, productID, chans[i]); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM catalog_products WHERE id = $1`, productID)
		_, _ = conn.Exec(`DELETE FROM catalog_product_types WHERE id = $1`, typeID)
		for _, c := range chans {
			_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, c)
		}
	})

	repo := &postgresRepository{queries: db.New(conn)}
	got, err := repo.ListProductChannelListingsByProductIDsForChannel(ctx, []uuid.UUID{productID}, chans[1])
	if err != nil {
		t.Fatal(err)
	}
	if rows := got[productID]; len(rows) != 1 || rows[0].ChannelID != chans[1] {
		t.Fatalf("channel batch = %+v, want only the one channel's listing", rows)
	}
}
