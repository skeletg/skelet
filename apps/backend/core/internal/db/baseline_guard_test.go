package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The generated migrations are pg_dump output, and pg_dump emits a search_path reset that has to be stripped every time one is regenerated. migrate reuses ONE connection for the whole run, so an empty search_path leaks into every migration after it and unqualified names stop resolving
// the dumps themselves are public-qualified throughout and survive, but the next hand-written migration that says `DELETE FROM config_values` would not
func TestGeneratedMigrationsKeepSearchPathStripped(t *testing.T) {
	generated := []string{
		"migrations/000001_schema.up.sql",
		"migrations/000002_reference_data.up.sql",
		"../../../signals/internal/db/migrations/000001_schema.up.sql", // the suite's stream, generated the same way
		"../../../signals/internal/db/migrations/000002_reference_data.up.sql",
	}
	for _, path := range generated {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		if strings.Contains(string(raw), "set_config('search_path'") {
			t.Errorf("%s carries pg_dump's search_path reset - strip it, or the first unqualified "+
				"name in a later migration fails on a fresh database (see the comment above)", filepath.Base(path))
		}
	}
}
