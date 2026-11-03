package agenttools

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func TestLowStockListsTheChannelsOwnWarehouses(t *testing.T) {
	conn := testdb.Require(t)
	tx := txrunner.New(conn)
	deps := &Deps{Inventory: inventory.NewService(inventory.NewRepository(db.New(conn), tx), tx)}
	sfx := uuid.NewString()[:8]
	var ch, own, foreign, typeID, product, item uuid.UUID
	for _, q := range []struct {
		sql  string
		args []any
		into *uuid.UUID
	}{
		{`INSERT INTO channels (slug, name, currency_code, default_locale) VALUES ($1, $1, 'USD', 'en') RETURNING id`, []any{"ls-" + sfx}, &ch},
		{`INSERT INTO inventory_locations (slug, name) VALUES ($1, 'Own') RETURNING id`, []any{"ls-own-" + sfx}, &own},
		{`INSERT INTO inventory_locations (slug, name) VALUES ($1, 'Foreign') RETURNING id`, []any{"ls-foreign-" + sfx}, &foreign},
		{`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'Low') RETURNING id`, []any{"ls-" + sfx}, &typeID},
	} {
		if err := conn.QueryRow(q.sql, q.args...).Scan(q.into); err != nil {
			t.Fatal(err)
		}
	}
	if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, "LS-"+sfx).Scan(&product); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`INSERT INTO inventory_items (product_id, tracked, low_stock_threshold) VALUES ($1, true, 5) RETURNING id`, product).Scan(&item); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM inventory_items WHERE id = $1`, item)
		_, _ = conn.Exec(`DELETE FROM catalog_products WHERE id = $1`, product)
		_, _ = conn.Exec(`DELETE FROM catalog_product_types WHERE id = $1`, typeID)
		_, _ = conn.Exec(`DELETE FROM inventory_locations WHERE id IN ($1, $2)`, own, foreign)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch)
	})
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO inventory_channel_locations (channel_id, location_id) VALUES ($1, $2)`, []any{ch, own}},
		{`INSERT INTO inventory_levels (inventory_item_id, location_id, quantity) VALUES ($1, $2, 1), ($1, $3, 0)`, []any{item, own, foreign}},
	} {
		if _, err := conn.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}

	got := toolLowStock(t.Context(), deps, ch, toolArgs{})
	if !strings.Contains(got, "LS-"+sfx) || !strings.Contains(got, "Own") || strings.Contains(got, "Foreign") {
		t.Fatalf("want this channel's warehouse and nothing else, got:\n%s", got)
	}
}
