package admin

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// What this pins: the SKUs an email's products block saved read back as the products they name - one no longer in
// the catalog is left out rather than failing the rest - and reading them takes catalog.view
func TestProductsBySkusReadsBackWhatABlockSaved(t *testing.T) {
	conn := testdb.Require(t)
	sfx := uuid.NewString()[:8]
	var ptype uuid.UUID
	if err := conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, $1) RETURNING id`, "pbs-"+sfx).Scan(&ptype); err != nil {
		t.Fatal(err)
	}
	skus := []string{"PBS-" + sfx + "-A", "PBS-" + sfx + "-B"}
	for _, sku := range skus {
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, ptype, sku).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`INSERT INTO catalog_product_content (product_id, locale, name) VALUES ($1, 'en', $2)`, id, "Name "+sku); err != nil {
			t.Fatal(err)
		}
	}
	queries, tx := db.New(conn), txrunner.New(conn)
	r := &queryResolver{&Resolver{CatalogSvc: catalog.NewService(catalog.Deps{Repo: catalog.NewRepository(queries, tx), Tx: tx})}}

	got, err := r.ProductsBySkus(withRenderCache(adminWithCachedGrants(auth.Grant{Slug: "catalog.view"})), []string{skus[1], "PBS-" + sfx + "-GONE", skus[0]})
	if err != nil {
		t.Fatal(err)
	}
	var read []string
	for _, p := range got {
		read = append(read, p.Sku)
	}
	slices.Sort(read)
	if !slices.Equal(read, skus) {
		t.Errorf("read back %v, want %v", read, skus)
	}
	if _, err := r.ProductsBySkus(withRenderCache(adminWithCachedGrants(auth.Grant{Slug: "orders.view"})), skus); err == nil {
		t.Error("an admin without catalog.view read products by SKU")
	}
}
