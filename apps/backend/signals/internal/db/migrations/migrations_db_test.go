package migrations

import (
	"context"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/migrate"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestBootFindsADatabaseDbmateMigratedCurrent(t *testing.T) {
	applied, err := migrate.Postgres(context.Background(), testdb.RequireSignals(t), "signals", FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Errorf("applied %v to a database dbmate had already migrated", applied)
	}
}
