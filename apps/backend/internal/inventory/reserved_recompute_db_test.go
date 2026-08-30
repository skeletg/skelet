package inventory

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	dbsqlc "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/testdb"
	"github.com/skeletg/skelet/internal/tx"
)

// reserved_quantity is a denormalized cache of the ledger, and Salable reads it rather than
// re-walking rows - so when the recompute is wrong every availability answer in the store is wrong,
// silently, in whichever direction the drift went
//
// The rule these pin is that manual_adjustment rows are quantity AUDIT entries, not holds: the
// change already landed in inventory_levels.quantity via the adjust upsert, so counting them here
// once wiped real holds and manufactured phantom ones - DB-backed because the rule lives in SQL

func recomputeDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Require(t)
}

// levels and reservations both FK to inventory_items, which FKs to catalog_products, so the fixture
// builds the whole chain and drops it on cleanup - the product cascade takes the rest
func seedLevel(t *testing.T, conn *sql.DB, quantity, staleReserved float64) (itemID, locID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	suffix := uuid.NewString()[:8]

	// Reuses whatever product type the database already has rather than inventing one, so the
	// fixture stays valid as catalog requirements change
	var typeID uuid.UUID
	if err := conn.QueryRowContext(ctx, `SELECT id FROM catalog_product_types LIMIT 1`).Scan(&typeID); err != nil {
		t.Skipf("no product type to hang a fixture product off: %v", err)
	}
	var productID uuid.UUID
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO catalog_products (product_type_id, sku, url, kind)
		 VALUES ($1, $2, $3, 'simple') RETURNING id`,
		typeID, "RECOMPUTE-"+suffix, "recompute-"+suffix).Scan(&productID); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO inventory_items (product_id, tracked) VALUES ($1, true) RETURNING id`,
		productID).Scan(&itemID); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO inventory_locations (slug, name) VALUES ($1, $2) RETURNING id`,
		"rc-"+suffix, "recompute fixture").Scan(&locID); err != nil {
		t.Fatalf("seed location: %v", err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO inventory_levels (inventory_item_id, location_id, quantity, reserved_quantity)
		 VALUES ($1, $2, $3, $4)`, itemID, locID, quantity, staleReserved); err != nil {
		t.Fatalf("seed level: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM catalog_products WHERE id = $1`, productID)
		_, _ = conn.Exec(`DELETE FROM inventory_locations WHERE id = $1`, locID)
	})
	return itemID, locID
}

// A second location against an item the fixture already made
func seedSecondLocation(t *testing.T, conn *sql.DB, itemID uuid.UUID, quantity float64) uuid.UUID {
	t.Helper()
	var locID uuid.UUID
	suffix := uuid.NewString()[:8]
	if err := conn.QueryRow(
		`INSERT INTO inventory_locations (slug, name) VALUES ($1, $2) RETURNING id`,
		"rc2-"+suffix, "recompute fixture b").Scan(&locID); err != nil {
		t.Fatalf("seed second location: %v", err)
	}
	if _, err := conn.Exec(
		`INSERT INTO inventory_levels (inventory_item_id, location_id, quantity, reserved_quantity)
		 VALUES ($1, $2, $3, 0)`, itemID, locID, quantity); err != nil {
		t.Fatalf("seed second level: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM inventory_locations WHERE id = $1`, locID) })
	return locID
}

func addReservation(t *testing.T, conn *sql.DB, itemID, locID uuid.UUID, delta float64, reason Reason) {
	t.Helper()
	if _, err := conn.Exec(
		`INSERT INTO inventory_reservations (inventory_item_id, location_id, quantity_delta, reason)
		 VALUES ($1, $2, $3, $4)`, itemID, locID, delta, reason); err != nil {
		t.Fatalf("insert reservation (%s): %v", reason, err)
	}
}

func readReserved(t *testing.T, conn *sql.DB, itemID, locID uuid.UUID) float64 {
	t.Helper()
	var got float64
	if err := conn.QueryRow(
		`SELECT reserved_quantity FROM inventory_levels WHERE inventory_item_id = $1 AND location_id = $2`,
		itemID, locID).Scan(&got); err != nil {
		t.Fatalf("read reserved: %v", err)
	}
	return got
}

func recompute(t *testing.T, conn *sql.DB) {
	t.Helper()
	// Rebuilds every level, which is what the real sweep does - correct and idempotent, so other
	// rows in a dev database are left at ledger truth rather than disturbed
	repo := NewRepository(dbsqlc.New(conn), tx.New(conn))
	if err := repo.RecomputeAllReserved(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
}

// A manual adjustment sits in the same ledger as real holds but is NOT one - if the
// recompute counts it, a +10 restock reads as -10 reserved and a real hold vanishes
func TestRecomputeIgnoresManualAdjustments(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 100, 0)

	addReservation(t, conn, itemID, locID, -3, ReasonOrderPlaced)       // a real hold
	addReservation(t, conn, itemID, locID, 10, ReasonManualAdjustment)  // restock audit row
	addReservation(t, conn, itemID, locID, -25, ReasonManualAdjustment) // shrinkage audit row

	recompute(t, conn)

	// Only the -3 hold counts: reserved = -(-3) = 3. Counting the audit rows would give
	// -(-3+10-25) = 18, which would report 18 units held that nobody ordered
	if got := readReserved(t, conn, itemID, locID); got != 3 {
		t.Fatalf("reserved = %g, want 3 - manual_adjustment rows leaked into the hold total", got)
	}
}

// A manual adjustment on its own must leave nothing reserved, since the quantity column already
// carries the change
func TestRecomputeManualAdjustmentAloneReservesNothing(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 50, 7) // counter starts wrong

	addReservation(t, conn, itemID, locID, -20, ReasonManualAdjustment)

	recompute(t, conn)

	if got := readReserved(t, conn, itemID, locID); got != 0 {
		t.Fatalf("reserved = %g, want 0 - an audit-only ledger manufactured a phantom hold", got)
	}
}

// The point of the sweep: whatever the counter drifted to, the ledger wins
func TestRecomputeRepairsDriftInBothDirections(t *testing.T) {
	conn := recomputeDB(t)

	t.Run("counter too low", func(t *testing.T) {
		itemID, locID := seedLevel(t, conn, 100, 0)
		addReservation(t, conn, itemID, locID, -4, ReasonOrderPlaced)
		recompute(t, conn)
		if got := readReserved(t, conn, itemID, locID); got != 4 {
			t.Errorf("reserved = %g, want 4", got)
		}
	})

	t.Run("counter too high", func(t *testing.T) {
		itemID, locID := seedLevel(t, conn, 100, 99)
		addReservation(t, conn, itemID, locID, -4, ReasonOrderPlaced)
		recompute(t, conn)
		if got := readReserved(t, conn, itemID, locID); got != 4 {
			t.Errorf("reserved = %g, want 4 - a stale high counter would hide sellable stock", got)
		}
	})

	t.Run("no ledger rows at all", func(t *testing.T) {
		itemID, locID := seedLevel(t, conn, 100, 42)
		recompute(t, conn)
		if got := readReserved(t, conn, itemID, locID); got != 0 {
			t.Errorf("reserved = %g, want 0 - a level with no holds must not report any", got)
		}
	})
}

// A settled chain nets to zero, and a released hold must free the stock again rather than leaving
// it reserved forever
func TestRecomputeSettledChainReleasesEverything(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 30, 5)

	addReservation(t, conn, itemID, locID, -5, ReasonOrderPlaced)
	addReservation(t, conn, itemID, locID, 5, ReasonOrderCancelled)

	recompute(t, conn)

	if got := readReserved(t, conn, itemID, locID); got != 0 {
		t.Fatalf("reserved = %g, want 0 - a cancelled order left stock held", got)
	}
}

// A positive net would mean negative reserved, which would ADD phantom salable stock - the SQL
// clamps at zero, and this pins that
func TestRecomputeClampsAtZero(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 10, 0)

	// more released than was ever held - a repair or a double-release
	addReservation(t, conn, itemID, locID, -2, ReasonOrderPlaced)
	addReservation(t, conn, itemID, locID, 9, ReasonOrderCancelled)

	recompute(t, conn)

	if got := readReserved(t, conn, itemID, locID); got != 0 {
		t.Fatalf("reserved = %g, want 0 - a negative counter would invent salable stock", got)
	}
}

// Holds are per (item, location), so one location's reservations must not reduce another's
func TestRecomputeIsPerLocation(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locA := seedLevel(t, conn, 100, 0)

	locB := seedSecondLocation(t, conn, itemID, 100)

	addReservation(t, conn, itemID, locA, -6, ReasonOrderPlaced)
	addReservation(t, conn, itemID, locB, -1, ReasonOrderPlaced)

	recompute(t, conn)

	if got := readReserved(t, conn, itemID, locA); got != 6 {
		t.Errorf("location A reserved = %g, want 6", got)
	}
	if got := readReserved(t, conn, itemID, locB); got != 1 {
		t.Errorf("location B reserved = %g, want 1 - locations bled into each other", got)
	}
}

// Re-running must not move anything - the sweep is meant to be safe to schedule
func TestRecomputeIsIdempotent(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 100, 0)
	addReservation(t, conn, itemID, locID, -8, ReasonOrderPlaced)

	recompute(t, conn)
	first := readReserved(t, conn, itemID, locID)
	recompute(t, conn)
	recompute(t, conn)

	if second := readReserved(t, conn, itemID, locID); second != first {
		t.Fatalf("reserved moved across runs: %g then %g", first, second)
	}
	if first != 8 {
		t.Fatalf("reserved = %g, want 8", first)
	}
}
