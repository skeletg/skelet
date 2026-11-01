package agenttools

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func TestSearchProductsQueriesTheWholeCatalogNewestFirst(t *testing.T) {
	conn := testdb.Require(t)
	sfx := strings.ToUpper(uuid.NewString()[:8])
	typeID, older, newer := uuid.New(), uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO catalog_product_types (id, slug, name) VALUES ($1, $2, 'Probes')`, []any{typeID, "mcps-" + sfx}},
		{`INSERT INTO catalog_products (id, product_type_id, sku, url, kind, created_at) VALUES ($1, $2, $3, $3, 'simple', now() - interval '1 day')`, []any{older, typeID, "MCPS-" + sfx + "-OLD"}},
		{`INSERT INTO catalog_products (id, product_type_id, sku, url, kind) VALUES ($1, $2, $3, $3, 'simple')`, []any{newer, typeID, "MCPS-" + sfx + "-NEW"}},
		{`INSERT INTO catalog_product_content (product_id, locale, name) VALUES ($1, $2, $3)`, []any{newer, channel.FallbackLocale, "Quokka lamp " + sfx}},
	} {
		if _, err := conn.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM catalog_products WHERE product_type_id = $1`, typeID)
		_, _ = conn.Exec(`DELETE FROM catalog_product_types WHERE id = $1`, typeID)
	})
	queries, tx := db.New(conn), txrunner.New(conn)
	deps := &Deps{Catalog: catalog.NewService(catalog.Deps{Repo: catalog.NewRepository(queries, tx), Tx: tx})}

	bySKU := toolSearchProducts(context.Background(), deps, toolArgs{Query: "mcps-" + strings.ToLower(sfx)})
	iNew, iOld := strings.Index(bySKU, newer.String()), strings.Index(bySKU, older.String())
	if iNew < 0 || iOld < 0 || iNew > iOld {
		t.Fatalf("a SKU search should find both, newest first, got:\n%s", bySKU)
	}
	byName := toolSearchProducts(context.Background(), deps, toolArgs{Query: "quokka lamp " + sfx})
	if !strings.Contains(byName, newer.String()) || strings.Contains(byName, older.String()) {
		t.Fatalf("a name search should find only the named product, got:\n%s", byName)
	}
}
