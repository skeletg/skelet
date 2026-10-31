package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/verbtest"
)

func TestMain(m *testing.M) {
	verbtest.Main(m, map[string]func(string, []string) int{
		"psql": Shell, "migrate": Migrate, "migrate-signals": MigrateSignals, "export": Export, "import": Import,
	})
}

func last(d *verbtest.Docker) string {
	calls := d.Calls()
	if len(calls) == 0 {
		return ""
	}
	cmd, _, _ := strings.Cut(calls[len(calls)-1], "|")
	return cmd
}

func TestDatabaseVerbs(t *testing.T) {
	for verb, want := range map[string]string{
		"psql":            "compose exec db psql -U skelet -d skelet -c select 1",
		"migrate":         `compose exec app sh -c dbmate --no-dump-schema --migrations-dir internal/db/migrations "$@" -- -c select 1`,
		"migrate-signals": `--env SIGNALS_DB_URL --migrations-dir /var/www/backend/signals/internal/db/migrations "$@" -- -c select 1`,
	} {
		d := verbtest.NewDocker(t, "")
		if _, code := d.Run(verb, t.TempDir(), "-c", "select 1"); code != 0 || !strings.Contains(last(d), want) {
			t.Errorf("%s: exit %d, docker got %q", verb, code, last(d))
		}
	}
}

// The dump lands in <file>.partial first and is renamed only when pg_dump succeeds; a failed one leaves nothing
func TestExport(t *testing.T) {
	dir := t.TempDir()
	d := verbtest.NewDocker(t, `echo "DUMP $*"`)
	out, code := d.Run("export", dir, filepath.Join(dir, "x.sql"))
	raw, _ := os.ReadFile(filepath.Join(dir, "x.sql"))
	if code != 0 || !strings.Contains(string(raw), "pg_dump -U skelet -d skelet") || strings.Contains(string(raw), "-Fc") || !strings.Contains(out, "exported →") {
		t.Errorf("exit %d, file %q, out %q", code, raw, out)
	}
	if _, code := d.Run("export", dir, filepath.Join(dir, "y.dump")); code != 0 || !strings.Contains(last(d), "-Fc") {
		t.Errorf("a .dump is pg_dump -Fc: exit %d, %q", code, last(d))
	}
	failing := verbtest.NewDocker(t, "exit 4")
	if _, code := failing.Run("export", dir, filepath.Join(dir, "z.dump")); code != 4 {
		t.Errorf("a failed dump: exit %d, want pg_dump's 4", code)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "z.dump*")); len(left) != 0 {
		t.Errorf("a failed dump left %v", left)
	}
	if _, code := verbtest.NewDocker(t, "").Run("export", dir); code != 0 {
		t.Error("the default name failed")
	} else if made, _ := filepath.Glob(filepath.Join(dir, "backups", "skelet-*.dump")); len(made) != 1 {
		t.Errorf("default export made %v", made)
	}
}

// Import overwrites the database, so without -y it needs a terminal to ask on - and the child here has none
func TestImport(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "x.sql")
	if err := os.WriteFile(dump, []byte("select 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := verbtest.NewDocker(t, `cat >/dev/null`)
	for args, code := range map[string]int{"": 1, "missing.sql": 1} {
		if _, got := d.Run("import", dir, strings.Fields(args)...); got != code {
			t.Errorf("import %q: exit %d, want %d", args, got, code)
		}
	}
	if _, code := d.Run("import", dir, "-y", dump); code != 0 || last(d) != "compose exec -T db psql -U skelet -d skelet" {
		t.Errorf("-y: exit %d, docker got %q", code, last(d))
	}
	if _, code := d.Run("import", dir, "--yes", filepath.Join(dir, "x.sql")); code != 0 {
		t.Errorf("--yes: exit %d", code)
	}
}
