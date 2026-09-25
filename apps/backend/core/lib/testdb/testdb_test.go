package testdb

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/lib/pq"
)

func TestSharedIgnoresSessionMarker(t *testing.T) {
	raw := os.Getenv("DATABASE_URL")
	if raw == "" {
		t.Skip("set DATABASE_URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	admin := *u
	admin.Path = "/postgres"
	db, err := sql.Open("postgres", admin.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Skipf("database unreachable: %v", err)
	}
	if reason := checkMarked(db, strings.TrimPrefix(u.Path, "/")); reason != "" {
		t.Skip(reason)
	}

	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	name := strings.TrimPrefix(u.Path, "/") + "__pkg" + hex.EncodeToString(suffix)
	if _, err := db.Exec(fmt.Sprintf(`CREATE DATABASE %s`, pq.QuoteIdentifier(name))); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, pq.QuoteIdentifier(name)))
	})

	target := *u
	target.Path = "/" + name
	q := target.Query()
	q.Set("options", "-c skelet.e2e=allowed")
	target.RawQuery = q.Encode()
	if dsn, _ := shared(target.String()); dsn != "" {
		t.Fatal("shared accepted a database whose only marker came from the DSN's session options")
	}

	if _, err := db.Exec(fmt.Sprintf(`ALTER DATABASE %s SET skelet.e2e = 'allowed'`, pq.QuoteIdentifier(name))); err != nil {
		t.Fatal(err)
	}
	if dsn, reason := shared(target.String()); dsn == "" {
		t.Fatalf("shared refused a marked database: %s", reason)
	}
}
