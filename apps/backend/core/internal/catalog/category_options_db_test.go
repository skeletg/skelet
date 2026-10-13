package catalog

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// The pickers' flat list reaches past the fixed nesting of the tree query, carries each category's name and
// direct product count, and a store-scoped call keeps only what is listed on that store
func TestCategoryOptionsReachEveryDepthAndCountProducts(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]

	var channelID, typeID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'fx', 'USD') RETURNING id`, "opt-"+suffix).Scan(&channelID); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'fx') RETURNING id`, "opt-"+suffix).Scan(&typeID); err != nil {
		t.Fatal(err)
	}

	// A chain seven levels deep - two past the tree fragment's nesting
	chain := make([]uuid.UUID, 7)
	var parent *uuid.UUID
	for i := range chain {
		slug := "opt-" + suffix + "-" + string(rune('a'+i))
		if err := conn.QueryRow(`INSERT INTO catalog_categories (parent_id, slug, url_path) VALUES ($1, $2, $2) RETURNING id`, parent, slug).Scan(&chain[i]); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`INSERT INTO catalog_category_content (category_id, locale, name) VALUES ($1, 'en', $2)`, chain[i], "Level "+string(rune('1'+i))); err != nil {
			t.Fatal(err)
		}
		parent = &chain[i]
	}
	deepest := chain[len(chain)-1]

	var products []uuid.UUID
	for i := range 2 {
		var id uuid.UUID
		sku := "OPT-" + suffix + string(rune('a'+i))
		if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, sku).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`INSERT INTO catalog_product_categories (product_id, category_id) VALUES ($1, $2)`, id, deepest); err != nil {
			t.Fatal(err)
		}
		products = append(products, id)
	}
	if _, err := conn.Exec(`INSERT INTO catalog_category_channel_listings (category_id, channel_id, is_published) VALUES ($1, $2, true)`, deepest, channelID); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM catalog_products WHERE id = ANY($1)`, products)
		for _, id := range slices.Backward(chain) {
			_, _ = conn.Exec(`DELETE FROM catalog_categories WHERE id = $1`, id)
		}
		_, _ = conn.Exec(`DELETE FROM catalog_product_types WHERE id = $1`, typeID)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, channelID)
	})
	svc := NewService(Deps{Repo: NewRepository(db.New(conn), nil)})

	all, err := svc.ListCategoryOptions(ctx, nil, "en")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[uuid.UUID]CategoryOption{}
	for _, o := range all {
		byID[o.ID] = o
	}
	for i, id := range chain {
		if _, ok := byID[id]; !ok {
			t.Fatalf("level %d of the chain is missing from the options", i+1)
		}
	}
	got := byID[deepest]
	if got.ParentID == nil || *got.ParentID != chain[len(chain)-2] {
		t.Fatalf("deepest parent = %v, want %v", got.ParentID, chain[len(chain)-2])
	}
	if got.Content == nil || got.Content.Name != "Level 7" {
		t.Fatalf("deepest name = %+v, want Level 7", got.Content)
	}
	if got.ProductCount != 2 {
		t.Fatalf("deepest product count = %d, want 2", got.ProductCount)
	}
	if n := byID[chain[0]].ProductCount; n != 0 {
		t.Fatalf("root product count = %d, want 0 - counts are direct membership only", n)
	}

	scoped, err := svc.ListCategoryOptions(ctx, &channelID, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped) != 1 || scoped[0].ID != deepest || scoped[0].ProductCount != 2 {
		t.Fatalf("store-scoped options = %v, want only the listed deepest category", len(scoped))
	}
}
