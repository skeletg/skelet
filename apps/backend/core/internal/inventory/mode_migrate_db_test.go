package inventory

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	dbsqlc "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// The two modes read different columns and nothing syncs them, so the flip is only safe if these backfills carry the stock across - a wrong answer here reads the whole catalogue as out of stock DB-backed because the arithmetic lives in SQL, including the reserved clamp

func modeMigrateSvc(t *testing.T, conn *sql.DB) *Service {
	t.Helper()
	queries := dbsqlc.New(conn)
	txMgr := txrunner.New(conn)
	return NewService(NewRepository(queries, txMgr), txMgr)
}

// One product with two locations, so the sum across locations is what gets asserted
func seedTwoLocations(t *testing.T, conn *sql.DB, qtyA, resA, qtyB, resB float64) (itemID, locA, locB uuid.UUID, sku string) {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	sku = "MODEMIG-" + suffix

	// Reuse a product type if the database has one, else make our own - a freshly migrated database
	// has none, and skipping there would quietly mean this never runs
	var typeID uuid.UUID
	if err := conn.QueryRowContext(ctx, `SELECT id FROM catalog_product_types LIMIT 1`).Scan(&typeID); err != nil {
		if err := conn.QueryRowContext(ctx,
			`INSERT INTO catalog_product_types (slug, name, tracks_inventory)
			 VALUES ($1, $2, true) RETURNING id`,
			"modemig-type-"+suffix, "mode migrate fixture").Scan(&typeID); err != nil {
			t.Fatalf("seed product type: %v", err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM catalog_product_types WHERE id = $1`, typeID) })
	}
	var productID uuid.UUID
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO catalog_products (product_type_id, sku, url, kind)
		 VALUES ($1, $2, $3, 'simple') RETURNING id`,
		typeID, sku, "modemig-"+suffix).Scan(&productID); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO inventory_items (product_id, tracked, quantity) VALUES ($1, true, 0) RETURNING id`,
		productID).Scan(&itemID); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	for i, spec := range []struct {
		qty, res float64
		id       *uuid.UUID
	}{{qtyA, resA, &locA}, {qtyB, resB, &locB}} {
		slug := "mm-" + suffix + string(rune('a'+i))
		if err := conn.QueryRowContext(ctx,
			`INSERT INTO inventory_locations (slug, name, is_active) VALUES ($1, $2, true) RETURNING id`,
			slug, "mode migrate fixture").Scan(spec.id); err != nil {
			t.Fatalf("seed location: %v", err)
		}
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO inventory_levels (inventory_item_id, location_id, quantity, reserved_quantity)
			 VALUES ($1, $2, $3, $4)`, itemID, *spec.id, spec.qty, spec.res); err != nil {
			t.Fatalf("seed level: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM catalog_products WHERE id = $1`, productID)
		_, _ = conn.Exec(`DELETE FROM inventory_locations WHERE id = ANY($1)`, []uuid.UUID{locA, locB})
	})
	return itemID, locA, locB, sku
}

func itemQuantity(t *testing.T, conn *sql.DB, itemID uuid.UUID) float64 {
	t.Helper()
	var q float64
	if err := conn.QueryRow(`SELECT quantity FROM inventory_items WHERE id = $1`, itemID).Scan(&q); err != nil {
		t.Fatalf("read item quantity: %v", err)
	}
	return q
}

func levelQuantity(t *testing.T, conn *sql.DB, itemID, locID uuid.UUID) float64 {
	t.Helper()
	var q float64
	err := conn.QueryRow(`SELECT quantity FROM inventory_levels WHERE inventory_item_id = $1 AND location_id = $2`,
		itemID, locID).Scan(&q)
	if err == sql.ErrNoRows {
		return -1 // no row at all, distinct from a zeroed one
	}
	if err != nil {
		t.Fatalf("read level quantity: %v", err)
	}
	return q
}

func TestMigrateToFlatModeSumsSalableAcrossLocations(t *testing.T) {
	conn := testdb.Require(t)
	svc := modeMigrateSvc(t, conn)
	ctx := context.Background()

	// 10 with 3 held, plus 5 with none held, is 12 salable
	itemID, _, _, sku := seedTwoLocations(t, conn, 10, 3, 5, 0)

	rows, changed, err := svc.MigrateToFlatMode(ctx, true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if changed == 0 {
		t.Fatal("dry run reported nothing to change")
	}
	var found bool
	for _, r := range rows {
		if r.SKU == sku {
			found = true
			if r.Current != 0 || r.New != 12 {
				t.Fatalf("dry run for %s: %g -> %g, want 0 -> 12", sku, r.Current, r.New)
			}
		}
	}
	if !found {
		t.Fatalf("dry run did not mention %s", sku)
	}
	// a dry run must not have written
	if q := itemQuantity(t, conn, itemID); q != 0 {
		t.Fatalf("dry run wrote quantity = %g, want 0", q)
	}

	if _, _, err := svc.MigrateToFlatMode(ctx, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if q := itemQuantity(t, conn, itemID); q != 12 {
		t.Fatalf("quantity = %g, want 12 (salable, not raw physical 15)", q)
	}

	// idempotent - the second pass has nothing left to change
	if _, n, err := svc.MigrateToFlatMode(ctx, false); err != nil || n != 0 {
		t.Fatalf("second pass changed %d rows (err %v), want 0", n, err)
	}
}

func TestMigrateToFlatModeClampsReserved(t *testing.T) {
	conn := testdb.Require(t)
	svc := modeMigrateSvc(t, conn)

	// A drifted NEGATIVE reserved must not inflate, and reserved above quantity must not go negative
	itemID, _, _, _ := seedTwoLocations(t, conn, 4, -6, 2, 9)

	if _, _, err := svc.MigrateToFlatMode(context.Background(), false); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// 4 - max(-6,0) = 4, and max(2-9, 0) = 0
	if q := itemQuantity(t, conn, itemID); q != 4 {
		t.Fatalf("quantity = %g, want 4 (negative reserved clamped, over-reserved cell floored)", q)
	}
}

func TestMigrateToMultiLocationModeParksAtOneLocation(t *testing.T) {
	conn := testdb.Require(t)
	svc := modeMigrateSvc(t, conn)
	ctx := context.Background()

	itemID, locA, locB, _ := seedTwoLocations(t, conn, 7, 0, 3, 0)
	if _, err := conn.ExecContext(ctx, `UPDATE inventory_items SET quantity = 9 WHERE id = $1`, itemID); err != nil {
		t.Fatalf("set flat quantity: %v", err)
	}

	if _, _, err := svc.MigrateToMultiLocationMode(ctx, locA, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if q := levelQuantity(t, conn, itemID, locA); q != 9 {
		t.Fatalf("target location quantity = %g, want 9", q)
	}
	// the other location's stale stock must not add on top of what landed at the target
	if q := levelQuantity(t, conn, itemID, locB); q != 0 {
		t.Fatalf("other location quantity = %g, want 0", q)
	}

	if _, _, err := svc.MigrateToMultiLocationMode(ctx, locA, false); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if q := levelQuantity(t, conn, itemID, locA); q != 9 {
		t.Fatalf("after second pass quantity = %g, want 9 (idempotent)", q)
	}
}

func TestMigrateToMultiLocationModeRejectsInactiveLocation(t *testing.T) {
	conn := testdb.Require(t)
	svc := modeMigrateSvc(t, conn)
	ctx := context.Background()

	_, locA, _, _ := seedTwoLocations(t, conn, 1, 0, 1, 0)
	if _, err := conn.ExecContext(ctx, `UPDATE inventory_locations SET is_active = false WHERE id = $1`, locA); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, _, err := svc.MigrateToMultiLocationMode(ctx, locA, false); err == nil {
		t.Fatal("parking stock at an inactive location was accepted - it would be unsellable")
	}
}
