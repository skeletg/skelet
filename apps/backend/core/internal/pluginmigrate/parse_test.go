package pluginmigrate

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"unicode/utf8"
)

// TestLoadMigrations covers the parser surface in isolation - no DB, no goroutines - Each case builds a fstest.MapFS,
// calls loadMigrations, and asserts the parsed result (or error shape)
func TestLoadMigrations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		files       map[string]string
		wantCount   int
		wantErrSub  string // substring of the expected error; empty means no error
		wantVersion int    // version of the first parsed migration when wantCount>0
		wantDown    bool   // whether the first migration has a non-empty DownSQL
	}{
		{
			name: "well-formed up+down pair",
			files: map[string]string{
				"migrations/0001_initial.up.sql":   "CREATE TABLE t (id INT);",
				"migrations/0001_initial.down.sql": "DROP TABLE t;",
			},
			wantCount:   1,
			wantVersion: 1,
			wantDown:    true,
		},
		{
			name: "up only - down optional",
			files: map[string]string{
				"migrations/0001_initial.up.sql": "CREATE TABLE t (id INT);",
			},
			wantCount:   1,
			wantVersion: 1,
			wantDown:    false,
		},
		{
			name: "down without up rejected",
			files: map[string]string{
				"migrations/0001_initial.down.sql": "DROP TABLE t;",
			},
			wantErrSub: "no .up.sql",
		},
		{
			name: "version gap rejected",
			files: map[string]string{
				"migrations/0001_a.up.sql": "SELECT 1;",
				"migrations/0003_c.up.sql": "SELECT 3;",
			},
			wantErrSub: "version gap",
		},
		{
			name: "first version must be 1",
			files: map[string]string{
				"migrations/0002_a.up.sql": "SELECT 1;",
			},
			wantErrSub: "first migration must be version 1",
		},
		{
			name: "non-matching filenames ignored",
			files: map[string]string{
				"migrations/README.md":           "documentation",
				"migrations/.gitkeep":            "",
				"migrations/0001_initial.up.sql": "SELECT 1;",
			},
			wantCount:   1,
			wantVersion: 1,
		},
		{
			name: "empty migrations dir is valid",
			files: map[string]string{
				"migrations/README.md": "no migrations yet",
			},
			wantCount: 0,
		},
		{
			name: "multiple ordered migrations",
			files: map[string]string{
				"migrations/0001_a.up.sql": "SELECT 1;",
				"migrations/0002_b.up.sql": "SELECT 2;",
				"migrations/0003_c.up.sql": "SELECT 3;",
			},
			wantCount:   3,
			wantVersion: 1,
		},
		{
			name: "two ups for one version rejected",
			files: map[string]string{
				"migrations/0001_a.up.sql": "SELECT 1;",
				"migrations/0001_b.up.sql": "SELECT 2;",
			},
			wantErrSub: "two .up.sql",
		},
		{
			name: "zero-padded and bare version collide",
			files: map[string]string{
				"migrations/0001_a.up.sql": "SELECT 1;",
				"migrations/1_b.up.sql":    "SELECT 2;",
			},
			wantErrSub: "two .up.sql",
		},
		{
			name: "two downs for one version rejected",
			files: map[string]string{
				"migrations/0001_a.up.sql":   "SELECT 1;",
				"migrations/0001_a.down.sql": "SELECT 1;",
				"migrations/0001_b.down.sql": "SELECT 2;",
			},
			wantErrSub: "two .down.sql",
		},
		{
			name: "missing migrations dir surfaces error",
			files: map[string]string{
				"other/0001_a.up.sql": "SELECT 1;",
			},
			wantErrSub: "read migrations",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reg := Registration{
				Namespace: "test",
				FS:        fstest.MapFS(toMapFS(tc.files)),
				Root:      "migrations",
			}
			got, err := loadMigrations(reg)
			if tc.wantErrSub != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", tc.wantErrSub)
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tc.wantCount {
				t.Fatalf("got %d migrations, want %d", len(got), tc.wantCount)
			}
			if tc.wantCount > 0 {
				if got[0].Version != tc.wantVersion {
					t.Errorf("first version: got %d, want %d", got[0].Version, tc.wantVersion)
				}
				if (got[0].DownSQL != "") != tc.wantDown {
					t.Errorf("first DownSQL non-empty: got %v, want %v", got[0].DownSQL != "", tc.wantDown)
				}
			}
		})
	}
}

// TestLoadMigrationsRootDefault verifies that an empty Root defaults
// to "migrations" - plugins almost always use that convention
func TestLoadMigrationsRootDefault(t *testing.T) {
	t.Parallel()
	fs := fstest.MapFS{
		"migrations/0001_init.up.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
	}
	reg := Registration{Namespace: "test", FS: fs} // Root left empty
	got, err := loadMigrations(reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
}

// TestRegistrationLifecycle exercises the package-level registry - Register / FindRegistration / AllRegistrations /
// duplicate-panic / ResetForTest - without touching the runner
func TestRegistrationLifecycle(t *testing.T) {
	// Can't run in parallel: mutates package state - ResetForTest
	// drains it at the start so other tests don't leak in
	ResetForTest()
	t.Cleanup(ResetForTest)

	emptyFS := fstest.MapFS{}

	Register(Registration{Namespace: "a", FS: emptyFS, Root: "migrations"})
	Register(Registration{Namespace: "b", FS: emptyFS, Root: "migrations"})

	all := AllRegistrations()
	if len(all) != 2 {
		t.Fatalf("AllRegistrations: got %d, want 2", len(all))
	}
	if all[0].Namespace != "a" || all[1].Namespace != "b" {
		t.Errorf("AllRegistrations order: got [%s, %s], want [a, b]", all[0].Namespace, all[1].Namespace)
	}

	if _, ok := FindRegistration("a"); !ok {
		t.Error("FindRegistration(a) returned !ok")
	}
	if _, ok := FindRegistration("missing"); ok {
		t.Error("FindRegistration(missing) returned ok=true")
	}

	// Duplicate namespace panics
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate Register, got none")
		}
	}()
	Register(Registration{Namespace: "a", FS: emptyFS})
}

// TestErrNamespaceNotRegistered is the sentinel comparison shape
func TestErrNamespaceNotRegistered(t *testing.T) {
	t.Parallel()
	wrapped := errors.New("wrap: " + ErrNamespaceNotRegistered.Error())
	if errors.Is(wrapped, ErrNamespaceNotRegistered) {
		t.Error("wrapped error should not Is-match without %w; sanity check")
	}
}

// toMapFS converts a string→string map into the fstest.MapFS shape fstest.MapFile.Data is []byte; the helper keeps the
// test tables readable
func toMapFS(files map[string]string) map[string]*fstest.MapFile {
	out := make(map[string]*fstest.MapFile, len(files))
	for name, content := range files {
		out[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return out
}

func TestTruncateForLogKeepsRunesWhole(t *testing.T) {
	t.Parallel()
	got := truncateForLog(strings.Repeat("a", 239) + strings.Repeat("é", 10))
	if !utf8.ValidString(got) {
		t.Errorf("truncation split a rune: %q", got[230:])
	}
}
