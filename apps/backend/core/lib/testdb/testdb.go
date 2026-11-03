// Package testdb is the one gate every database-backed test goes through: a DSN alone is not consent, since tests write real rows and the e2e suite drains every due outbox row
// They run only against a database carrying ALTER DATABASE <name> SET skelet.e2e = 'allowed', set once per throwaway database and never by any deploy path
// Each test binary works in its own clone of that database (CREATE DATABASE ... TEMPLATE), so packages running in parallel never see each other's rows - SKELET_TESTDB_SHARED=1 uses the template itself, for a suite whose database another process shares
package testdb

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // the driver production runs
	"github.com/lib/pq"
)

const marker = "skelet.e2e=allowed"

// Read only so go test's cache keys on it: skelet test sets it to a hash of the migration files, so a schema change
// reruns the database tests whose cached pass would otherwise vouch for the old one
const SchemaEnv = "SKELET_TEST_SCHEMA"

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
	dsn, reason := URL(envVar)
	if dsn == "" {
		return nil, reason
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Sprintf("open %s: %v", envVar, err)
	}
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Sprintf("database unreachable: %v", err)
	}
	return conn, ""
}

type clone struct {
	once   sync.Once
	dsn    string
	reason string
}

var clones sync.Map

// URL is the DSN this process should use for envVar - the same clone on every call, made on the first - so a
// harness that opens its own pool lands in the database the tests write to
func URL(envVar string) (dsn, reason string) {
	_ = os.Getenv(SchemaEnv)
	raw := os.Getenv(envVar)
	if raw == "" {
		return "", "set " + envVar
	}
	c, _ := clones.LoadOrStore(envVar, &clone{})
	cl := c.(*clone)
	cl.once.Do(func() { cl.dsn, cl.reason = provision(raw) })
	return cl.dsn, cl.reason
}

func provision(raw string) (string, string) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Sprintf("parse DSN: %v", err)
	}
	template := strings.TrimPrefix(u.Path, "/")
	if os.Getenv("SKELET_TESTDB_SHARED") != "" {
		return shared(raw)
	}
	admin := *u
	admin.Path = "/postgres"
	db, err := sql.Open("pgx", admin.String())
	if err != nil {
		return "", fmt.Sprintf("open maintenance database: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return "", fmt.Sprintf("database unreachable: %v", err)
	}
	// Read off the catalog rather than by connecting: a session on the template would block the clone below
	if reason := checkMarked(db, template); reason != "" {
		return "", reason
	}
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Sprintf("random suffix: %v", err)
	}
	name := template + "__pkg" + hex.EncodeToString(suffix)
	if _, err := db.Exec(fmt.Sprintf(`CREATE DATABASE %s TEMPLATE %s`, pq.QuoteIdentifier(name), pq.QuoteIdentifier(template))); err != nil {
		return "", fmt.Sprintf("clone %s: %v", template, err)
	}
	if _, err := db.Exec(fmt.Sprintf(`ALTER DATABASE %s SET skelet.e2e = 'allowed'`, pq.QuoteIdentifier(name))); err != nil {
		return "", fmt.Sprintf("mark clone %s: %v", name, err)
	}
	u.Path = "/" + name
	return u.String(), ""
}

func shared(raw string) (string, string) {
	conn, err := sql.Open("pgx", raw)
	if err != nil {
		return "", fmt.Sprintf("open: %v", err)
	}
	defer conn.Close()
	if err := conn.Ping(); err != nil {
		return "", fmt.Sprintf("database unreachable: %v", err)
	}
	var name string
	if err := conn.QueryRow(`SELECT current_database()`).Scan(&name); err != nil {
		return "", fmt.Sprintf("read database name: %v", err)
	}
	if reason := checkMarked(conn, name); reason != "" {
		return "", reason
	}
	return raw, ""
}

// checkMarked reads the marker from the catalog, since a DSN option, PGOPTIONS or a role setting could fake it on the session
func checkMarked(db *sql.DB, name string) string {
	var settings []string
	err := db.QueryRow(`SELECT s.setconfig FROM pg_db_role_setting s JOIN pg_database d ON d.oid = s.setdatabase WHERE d.datname = $1 AND s.setrole = 0`, name).Scan(pq.Array(&settings))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Sprintf("read disposable marker: %v", err)
	}
	if !slices.Contains(settings, marker) {
		return "database not marked as disposable - refusing to write; on a throwaway database run: ALTER DATABASE <name> SET skelet.e2e = 'allowed'"
	}
	return ""
}
