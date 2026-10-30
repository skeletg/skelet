// Package chtest gates ClickHouse tests on a database commented 'skelet.e2e=allowed', and gives each test binary its own freshly migrated clone
package chtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"github.com/skeletg/skelet/apps/backend/core/lib/migfile"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

const marker = "skelet.e2e=allowed"

var (
	once   sync.Once
	dsn    string
	reason string
	ident  = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
)

func DSN(t testing.TB) string {
	t.Helper()
	_ = os.Getenv(testdb.SchemaEnv) // keys go test's cache on the schema, as testdb does
	once.Do(func() { dsn, reason = provision(os.Getenv("SIGNALS_CH_TEST_DSN")) })
	if dsn == "" {
		t.Skip(reason)
	}
	return dsn
}

func provision(raw string) (string, string) {
	if raw == "" {
		return "", "set SIGNALS_CH_TEST_DSN to a disposable ClickHouse database"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Sprintf("parse SIGNALS_CH_TEST_DSN: %v", err)
	}
	opts, err := clickhouse.ParseDSN(raw)
	if err != nil {
		return "", fmt.Sprintf("parse SIGNALS_CH_TEST_DSN: %v", err)
	}
	template := opts.Auth.Database
	if !ident.MatchString(template) {
		return "", fmt.Sprintf("SIGNALS_CH_TEST_DSN names database %q - want a plain identifier", template)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn, err := clickhouse.Open(opts)
	if err != nil {
		return "", fmt.Sprintf("open clickhouse: %v", err)
	}
	defer conn.Close()
	if err := conn.Ping(ctx); err != nil {
		return "", fmt.Sprintf("clickhouse unreachable: %v", err)
	}
	var comment string
	if err := conn.QueryRow(ctx, "SELECT comment FROM system.databases WHERE name = ?", template).Scan(&comment); err != nil || comment != marker {
		return "", "ClickHouse database not marked as disposable - refusing to write; on a throwaway database run: ALTER DATABASE <name> MODIFY COMMENT '" + marker + "'"
	}

	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Sprintf("random suffix: %v", err)
	}
	name := template + "__pkg" + hex.EncodeToString(suffix)
	if err := conn.Exec(ctx, "CREATE DATABASE `"+name+"` COMMENT '"+marker+"'"); err != nil {
		return "", fmt.Sprintf("create %s: %v", name, err)
	}
	own := *opts
	own.Auth.Database = name
	db, err := clickhouse.Open(&own)
	if err != nil {
		return "", fmt.Sprintf("open %s: %v", name, err)
	}
	defer db.Close()
	if err := applySchema(ctx, db); err != nil {
		return "", err.Error()
	}
	u.Path = "/" + name
	return u.String(), ""
}

func applySchema(ctx context.Context, db clickhouse.Conn) error {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("locate the spine migrations")
	}
	files, err := migfile.List(filepath.Join(filepath.Dir(here), "..", "analytics", "spinech", "migrations"))
	if err != nil {
		return fmt.Errorf("read the spine migrations beside %s: %w", here, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no spine migrations found beside %s", here)
	}
	for _, f := range files {
		if err := ApplyUp(ctx, db, f); err != nil {
			return err
		}
	}
	return nil
}

// ApplyUp runs a spine migration's up sections one query each, as dbmate does - ClickHouse refuses a query holding several statements
func ApplyUp(ctx context.Context, db clickhouse.Conn, f migfile.File) error {
	for _, stmt := range f.Ups {
		if migfile.Blank(stmt) {
			continue
		}
		if err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply %s: %w", filepath.Base(f.Path), err)
		}
	}
	return nil
}
