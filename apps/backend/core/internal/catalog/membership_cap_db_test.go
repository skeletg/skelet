package catalog

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// A category and a collection holding the same three products, positioned 0-2
func seedMemberships(t *testing.T, conn *sql.DB) (categoryID, collectionID uuid.UUID, products []uuid.UUID) {
	t.Helper()
	suffix := uuid.NewString()[:8]
	var typeID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'membership fixture') RETURNING id`, "mem-type-"+suffix).Scan(&typeID); err != nil {
		t.Fatalf("seed type: %v", err)
	}
	if err := conn.QueryRow(`INSERT INTO catalog_categories (slug, url_path) VALUES ($1, $1) RETURNING id`, "mem-cat-"+suffix).Scan(&categoryID); err != nil {
		t.Fatalf("seed category: %v", err)
	}
	if err := conn.QueryRow(`INSERT INTO catalog_collections (slug) VALUES ($1) RETURNING id`, "mem-col-"+suffix).Scan(&collectionID); err != nil {
		t.Fatalf("seed collection: %v", err)
	}
	for i := range 3 {
		var id uuid.UUID
		sku := "MEM-" + suffix + "-" + string(rune('a'+i))
		if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, sku).Scan(&id); err != nil {
			t.Fatalf("seed product: %v", err)
		}
		if _, err := conn.Exec(`INSERT INTO catalog_product_categories (product_id, category_id, position) VALUES ($1, $2, $3)`, id, categoryID, i); err != nil {
			t.Fatalf("seed category membership: %v", err)
		}
		if _, err := conn.Exec(`INSERT INTO catalog_collection_products (collection_id, product_id, sort_order) VALUES ($1, $2, $3)`, collectionID, id, i); err != nil {
			t.Fatalf("seed collection membership: %v", err)
		}
		products = append(products, id)
	}
	return categoryID, collectionID, products
}

func TestMembershipListsHonourTheirCap(t *testing.T) {
	conn := testdb.Require(t)
	repo := &postgresRepository{queries: db.New(conn)}
	ctx := context.Background()
	cat, col, want := seedMemberships(t, conn)

	for name, list := range map[string]func(int32) ([]Product, error){
		"category":   func(n int32) ([]Product, error) { return repo.ListProductsByCategory(ctx, cat, n) },
		"collection": func(n int32) ([]Product, error) { return repo.ListProductsByCollection(ctx, col, n) },
	} {
		capped, err := list(2)
		if err != nil {
			t.Fatalf("%s capped: %v", name, err)
		}
		if len(capped) != 2 || capped[0].ID != want[0] || capped[1].ID != want[1] {
			t.Errorf("%s with a cap of 2 = %d products, want the first two in merchandising order", name, len(capped))
		}
		all, err := list(0)
		if err != nil {
			t.Fatalf("%s uncapped: %v", name, err)
		}
		if len(all) != 3 {
			t.Errorf("%s with a cap of 0 = %d products, want all 3", name, len(all))
		}
	}
}

func TestDeleteCategoryStillRefusesOneWithProducts(t *testing.T) {
	conn := testdb.Require(t)
	svc := NewService(Deps{Repo: NewRepository(db.New(conn), nil)})
	cat, _, _ := seedMemberships(t, conn)
	if err := svc.DeleteCategory(context.Background(), cat); !errors.Is(err, ErrCategoryHasProducts) {
		t.Fatalf("err = %v, want ErrCategoryHasProducts", err)
	}
}
