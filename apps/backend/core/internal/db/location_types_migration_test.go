package db

import (
	"os"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestLocationTypeMigrationClassifiesLegacyDataSafely(t *testing.T) {
	conn := testdb.Require(t)
	tx, err := conn.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, file := range []string{"migrations/000037_inventory_location_types.down.sql"} {
		sql, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(t.Context(), string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	for _, typ := range []string{"store", "warehouse", "custom-backroom", "custom-shelf", ""} {
		access := typ != "custom-shelf"
		if _, err = tx.ExecContext(t.Context(), `INSERT INTO inventory_locations(slug,name,type,storefront_visible,pickup_enabled) VALUES ($1,$1,$2,$3,$3)`, "migration-type-"+typ, typ, access); err != nil {
			t.Fatal(err)
		}
	}
	sql, err := os.ReadFile("migrations/000037_inventory_location_types.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(t.Context(), string(sql)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		raw, want string
		public    bool
	}{{"store", "retail", true}, {"warehouse", "warehouse", false}, {"custom-backroom", "retail", true}, {"custom-shelf", "unclassified", false}, {"", "retail", true}} {
		var typ, legacy string
		var visible, pickup bool
		err = tx.QueryRowContext(t.Context(), `SELECT type,storefront_visible,pickup_enabled,metadata->>'_legacy_location_type' FROM inventory_locations WHERE slug=$1`, "migration-type-"+tc.raw).Scan(&typ, &visible, &pickup, &legacy)
		if err != nil || typ != tc.want || legacy != tc.raw || visible != tc.public || pickup != tc.public {
			t.Fatalf("migration %s: %s %s %v %v %v", tc.raw, typ, legacy, visible, pickup, err)
		}
	}
	// Rolling back restores every changed type, including the untyped pickup point promoted to retail
	down, err := os.ReadFile("migrations/000037_inventory_location_types.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(t.Context(), string(down)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"store", "warehouse", "custom-backroom", "custom-shelf", ""} {
		var typ string
		var kept bool
		err = tx.QueryRowContext(t.Context(), `SELECT type, metadata ? '_legacy_location_type' FROM inventory_locations WHERE slug=$1`, "migration-type-"+raw).Scan(&typ, &kept)
		if err != nil || typ != raw || kept {
			t.Fatalf("rollback %q: got %q, legacy key kept %v, %v", raw, typ, kept, err)
		}
	}
}
