// Package testdb is the one gate every database-backed test goes through: a DSN alone is not consent, since tests write real rows and the e2e suite drains every due outbox row
// They run only against a database carrying ALTER DATABASE <name> SET skelet.e2e = 'allowed', set once per throwaway database and never by any deploy path
package testdb

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/lib/pq" // tests dial before any app pool exists
)

// Require returns the commerce database, skipping when DATABASE_URL is unset, unreachable, or
// unmarked - the connection closes with the test
func Require(t *testing.T) *sql.DB { return require(t, "DATABASE_URL") }

// RequireSignals is Require for SIGNALS_DB_URL - each database carries its own marker, since a
// database can only consent for itself
func RequireSignals(t *testing.T) *sql.DB { return require(t, "SIGNALS_DB_URL") }

func require(t *testing.T, envVar string) *sql.DB {
	t.Helper()
	conn, reason := Open(envVar)
	if conn == nil {
		t.Skip(reason)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// Open is the non-skipping variant for TestMain callers, which hold no *testing.T - a nil conn
// comes back with the reason a test would have skipped on, and the caller owns the close
func Open(envVar string) (*sql.DB, string) {
	dsn := os.Getenv(envVar)
	if dsn == "" {
		return nil, "set " + envVar
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Sprintf("open %s: %v", envVar, err)
	}
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Sprintf("database unreachable: %v", err)
	}
	var marker sql.NullString
	if err := conn.QueryRow(`SELECT current_setting('skelet.e2e', true)`).Scan(&marker); err != nil {
		_ = conn.Close()
		return nil, fmt.Sprintf("read disposable marker: %v", err)
	}
	if marker.String != "allowed" {
		_ = conn.Close()
		return nil, "database not marked as disposable - refusing to write; on a throwaway database run: ALTER DATABASE <name> SET skelet.e2e = 'allowed'"
	}
	return conn, ""
}
