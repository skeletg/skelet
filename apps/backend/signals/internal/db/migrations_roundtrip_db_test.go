package db

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestMigrationsRoundTrip(t *testing.T) {
	testdb.MigrationRoundTrip(t, testdb.RequireSignals(t), "migrations", 2)
}
