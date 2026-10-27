package spinech

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/skeletg/skelet/apps/backend/core/lib/migfile"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// No transactions in ClickHouse: the caller holds a lock elsewhere, and a file that fails halfway re-runs its earlier statements on the next boot
func Migrate(ctx context.Context, dsn string) ([]string, error) {
	files, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	all, err := migfile.ListFS(files)
	if err != nil {
		return nil, fmt.Errorf("read the spine migrations: %w", err)
	}
	conn, _, err := open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version String, ts DateTime DEFAULT now(), applied UInt8 DEFAULT 1)
		ENGINE = ReplacingMergeTree(ts) PRIMARY KEY version ORDER BY version`); err != nil {
		return nil, fmt.Errorf("create the spine schema_migrations: %w", err)
	}
	applied, err := recordedVersions(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("read the spine schema_migrations: %w", err)
	}

	var done []string
	for _, f := range all {
		if applied[f.Version] || !f.HasSQL() {
			continue
		}
		for _, stmt := range f.Ups {
			if migfile.Blank(stmt) {
				continue
			}
			if err := conn.Exec(ctx, stmt); err != nil {
				return done, fmt.Errorf("apply %s: %w", f.Path, err)
			}
		}
		if err := conn.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, f.Version); err != nil {
			return done, fmt.Errorf("record %s: %w", f.Path, err)
		}
		done = append(done, f.Version)
	}
	return done, nil
}

func recordedVersions(ctx context.Context, conn driver.Conn) (map[string]bool, error) {
	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations FINAL WHERE applied`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}
