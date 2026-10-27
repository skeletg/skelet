package spinech

import (
	"context"
	"io/fs"
	"net/url"
	"slices"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"

	"github.com/skeletg/skelet/apps/backend/core/lib/migfile"
	"github.com/skeletg/skelet/apps/backend/signals/internal/chtest"
)

// An empty database beside the package's own, since chtest's arrives with the schema already applied and unrecorded
func emptyDatabase(t *testing.T) (dsn string, conn clickhouse.Conn) {
	t.Helper()
	base := chtest.DSN(t)
	opts, err := clickhouse.ParseDSN(base)
	if err != nil {
		t.Fatal(err)
	}
	conn, err = clickhouse.Open(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	name := opts.Auth.Database + "_migrate"
	ctx := context.Background()
	if err := conn.Exec(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Exec(context.Background(), "DROP DATABASE `"+name+"`") })
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String(), conn
}

func TestMigrateBuildsAnEmptySpineOnceInDbmatesShape(t *testing.T) {
	dsn, conn := emptyDatabase(t)
	ctx := context.Background()
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	files, err := migfile.ListFS(sub)
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, f := range files {
		want = append(want, f.Version)
	}

	got, err := Migrate(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) == 0 || !slices.Equal(got, want) {
		t.Fatalf("applied %v, want %v", got, want)
	}
	again, err := Migrate(ctx, dsn)
	if err != nil || len(again) != 0 {
		t.Fatalf("second run applied %v (%v), want nothing", again, err)
	}

	db, _ := url.Parse(dsn)
	name := db.Path[1:]
	for _, table := range []string{"analytics_events", "analytics_identities"} {
		var exists uint8
		if err := conn.QueryRow(ctx, "EXISTS TABLE `"+name+"`.`"+table+"`").Scan(&exists); err != nil || exists != 1 {
			t.Errorf("%s missing after the migration (%v)", table, err)
		}
	}
	rows, err := conn.Query(ctx, "SELECT version FROM `"+name+"`.schema_migrations FINAL WHERE applied ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var recorded []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		recorded = append(recorded, v)
	}
	if !slices.Equal(recorded, want) {
		t.Errorf("schema_migrations = %v, want %v", recorded, want)
	}
}
