package testrun

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/skeletg/skelet/cmd/skelet/internal/verbtest"
)

func TestMain(m *testing.M) {
	healthTries, healthPause = 3, time.Millisecond
	verbtest.Main(m, map[string]func(string, []string) int{"test": Test, "e2e": E2E, "storefront": Storefront})
}

// cmds is each docker call's arguments, without the profiles verbtest logs beside them
func cmds(d *verbtest.Docker) []string {
	var out []string
	for _, c := range d.Calls() {
		args, _, _ := strings.Cut(c, "|")
		out = append(out, args)
	}
	return out
}

// inOrder reports the first of want that no call from the previous match on contains
func inOrder(calls, want []string) string {
	i := 0
	for _, w := range want {
		for i < len(calls) && !strings.Contains(calls[i], w) {
			i++
		}
		if i == len(calls) {
			return w
		}
	}
	return ""
}

func anyContains(calls []string, s string) bool {
	return slices.ContainsFunc(calls, func(c string) bool { return strings.Contains(c, s) })
}

func TestParseTest(t *testing.T) {
	cases := []struct {
		args            []string
		frontend, fresh bool
		changed         string
		goArgs          []string
	}{
		{nil, true, true, "", nil},
		{[]string{"--no-frontend", "--keep"}, false, false, "", nil},
		{[]string{"--no-dashboard", "--fresh"}, false, true, "", nil},
		{[]string{"--changed"}, true, true, "auto", nil},
		{[]string{"--changed=HEAD~2", "./internal/x"}, true, true, "HEAD~2", []string{"./internal/x"}},
		// The first argument that isn't a flag of ours ends them - after it everything is go test's, ours included
		{[]string{"-run", "X", "--keep"}, true, true, "", []string{"-run", "X", "--keep"}},
		{[]string{"--keep", "./...", "--no-frontend"}, true, false, "", []string{"./...", "--no-frontend"}},
	}
	for _, c := range cases {
		o := parseTest(c.args)
		if o.frontend != c.frontend || o.fresh != c.fresh || o.changed != c.changed || !slices.Equal(o.goArgs, c.goArgs) {
			t.Errorf("%q -> %+v", c.args, o)
		}
	}
}

func TestTouchesEverything(t *testing.T) {
	for f, want := range map[string]bool{
		"package.json": true, "pnpm-lock.yaml": true, "pnpm-workspace.yaml": true, "tsconfig.json": true,
		"tsconfig.app.json": true, "vite.config.ts": true, "vitest.config.mts": true, "src/test-setup.ts": true,
		"packages/x/package.json": true, "src/app.tsx": false, "src/a/test-setup.ts": false, "tsconfig/x.ts": false,
		"src/package.json.ts": false,
	} {
		if touchesEverything.MatchString(f) != want {
			t.Errorf("%s: want %v", f, want)
		}
	}
}

func gitRepo(t *testing.T, branch string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", branch)
	write("a.txt", "a")
	write(".gitignore", "ignored\n")
	run("add", ".")
	run("commit", "-qm", "base")
	run("checkout", "-qb", "feat")
	write("b.txt", "b")
	run("add", ".")
	run("commit", "-qm", "feat")
	write("a.txt", "changed")
	write("c.txt", "new")
	write("ignored", "x")
	return dir
}

// Committed since the merge base with develop, plus what isn't committed yet - new files in, ignored ones out
func TestChangedSince(t *testing.T) {
	dir := gitRepo(t, "develop")
	ref, files, code := changedSince(dir, "auto")
	base, _ := git(dir, "rev-parse", "develop")
	if code != 0 || ref != strings.TrimSpace(base) || !slices.Equal(files, []string{"a.txt", "b.txt", "c.txt"}) {
		t.Errorf("auto -> %s %v exit %d, want develop %s", ref, files, code, base)
	}
	if _, files, _ := changedSince(dir, "HEAD"); !slices.Equal(files, []string{"a.txt", "c.txt"}) {
		t.Errorf("HEAD -> %v", files)
	}
	if _, _, code := changedSince(dir, "nope"); code != 2 {
		t.Errorf("an unknown ref: exit %d, want 2", code)
	}
	// No develop to measure from: what isn't committed yet
	if ref, files, _ := changedSince(gitRepo(t, "main"), "auto"); ref != "HEAD" || !slices.Equal(files, []string{"a.txt", "c.txt"}) {
		t.Errorf("without develop -> %s %v", ref, files)
	}
}

func TestLock(t *testing.T) {
	root := t.TempDir()
	dir := lockPath("test", root)
	t.Cleanup(func() { _ = os.RemoveAll(dir); _ = os.RemoveAll(dir + ".takeover") })
	if err := lock("test", root); err != nil {
		t.Fatal(err)
	}
	if readPid(dir) != os.Getpid() {
		t.Errorf("the lock holds pid %d", readPid(dir))
	}
	// A live holder - this process - keeps it
	if err := lock("test", root); !errors.Is(err, errLocked) {
		t.Errorf("a held lock: %v", err)
	}
	if lockPath("test-e2e", root) == dir || lockPath("test", t.TempDir()) == dir {
		t.Error("the lock isn't per suite and checkout")
	}

	// A dead holder's lock is taken over
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pid"), []byte(strconv.Itoa(dead.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir+".takeover", 0o700); err != nil {
		t.Fatal(err)
	}
	// ...but not while another run is taking it over
	if err := lock("test", root); !errors.Is(err, errLocked) {
		t.Errorf("a takeover in progress: %v", err)
	}
	_ = os.Remove(dir + ".takeover")
	if err := lock("test", root); err != nil || readPid(dir) != os.Getpid() {
		t.Errorf("a dead holder's lock: %v, pid %d", err, readPid(dir))
	}
	if _, err := os.Stat(dir + ".takeover"); !os.IsNotExist(err) {
		t.Error("the takeover directory was left behind")
	}
}

// The fake docker for a run with the stack up: the databases exist, and go test reports a pass and a skip
const stackUp = `case "$*" in
"compose ps --services --status running") printf 'app\nclickhouse\n' ;;
*"SELECT 1 FROM pg_database"*) echo 1 ;;
*"go test -json"*) printf '%s\n' '{"Action":"pass","Package":"p","Test":"TestA"}' \
	'{"Action":"output","Package":"p","Test":"TestB","Output":"    x_test.go:3: no database\n"}' \
	'{"Action":"skip","Package":"p","Test":"TestB"}' ;;
esac`

func TestTestRunsAgainstDisposableDatabases(t *testing.T) {
	root := t.TempDir()
	d := verbtest.NewDocker(t, stackUp)
	out, code := d.Run("test", root, "--no-frontend", "./internal/x", "-run", "X")
	calls := cmds(d)
	if code != 0 || !strings.Contains(out, "==> core: 1 passed, 0 failed, 1 skipped") ||
		!strings.Contains(out, "1 skipped: no database") || !strings.Contains(out, "==> tests OK") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if missing := inOrder(calls, []string{
		"DROP DATABASE skelet_test WITH (FORCE)",
		"CREATE DATABASE skelet_test",
		"ALTER DATABASE skelet_test SET skelet.e2e = 'allowed'",
		"CREATE DATABASE skelet_test_signals",
		"ALTER DATABASE skelet RESET skelet.e2e",
		"dbmate --no-dump-schema --url postgres://skelet:skelet@db:5432/skelet_test?sslmode=disable --migrations-dir internal/db/migrations up",
		"--url postgres://skelet:skelet@signals-db:5432/skelet_test_signals",
		"DROP DATABASE IF EXISTS skelet_test_signals",
		"--url clickhouse://skelet:skelet@clickhouse:9000/skelet_test_signals",
		"-w /var/www/backend/core -e DATABASE_URL=postgres://skelet:skelet@db:5432/skelet_test?sslmode=disable -e SIGNALS_DB_URL= -e SKELET_TEST_SCHEMA=",
		"app go test -json -count=1 ./internal/x -run X",
	}); missing != "" {
		t.Errorf("no %q in order in:\n%s", missing, strings.Join(calls, "\n"))
	}
	// Never the dev databases
	if anyContains(calls, "DROP DATABASE skelet ") || anyContains(calls, "DROP DATABASE skelet_signals") {
		t.Errorf("a dev database was dropped:\n%s", strings.Join(calls, "\n"))
	}
	if _, err := os.Stat(lockPath("test", root)); !os.IsNotExist(err) {
		t.Error("the run left its lock behind")
	}

	// --changed lets go's test cache answer for what nothing touched
	changed := verbtest.NewDocker(t, stackUp)
	out, code = changed.Run("test", gitRepo(t, "develop"), "--changed", "--no-frontend", "./x")
	if code != 0 || !strings.Contains(out, "==> only what changed since") || !strings.Contains(out, "(3 files)") ||
		!anyContains(cmds(changed), "app go test -json ./x") {
		t.Errorf("--changed: exit %d:\n%s\n%s", code, out, strings.Join(cmds(changed), "\n"))
	}

	keep := verbtest.NewDocker(t, stackUp)
	if _, code := keep.Run("test", root, "--keep", "--no-frontend", "./internal/x"); code != 0 || anyContains(cmds(keep), "DROP DATABASE skelet_test") {
		t.Errorf("--keep: exit %d, dropped a database:\n%s", code, strings.Join(cmds(keep), "\n"))
	}
}

// The full sweep runs each module's packages minus its e2e suite, which needs databases of its own
func TestTestSweepLeavesE2EOut(t *testing.T) {
	d := verbtest.NewDocker(t, stackUp+`
case "$*" in *"go list -e ./..."*) printf 'x/a\nx/internal/e2e\nx/b\n' ;; esac`)
	// The temp root has no CLI module, so the run stops at it - after the three modules in the containers
	_, _ = d.Run("test", t.TempDir(), "--no-frontend")
	calls := cmds(d)
	if missing := inOrder(calls, []string{
		"-w /var/www/backend/core", "app go test -json -count=1 x/a x/b",
		"-w /var/www/backend/signals", "SIGNALS_DB_URL=postgres://skelet:skelet@signals-db:5432/skelet_test_signals",
		"app go test -json -count=1 x/a x/b",
		"-w /var/www/backend/sdk", "app go test -json -count=1 ./...",
	}); missing != "" || anyContains(calls, "x/internal/e2e x/b") {
		t.Errorf("no %q in order in:\n%s", missing, strings.Join(calls, "\n"))
	}
}

func TestTestRefusals(t *testing.T) {
	root := t.TempDir()
	// The stack down: nothing to migrate into, so nothing is touched
	down := verbtest.NewDocker(t, "")
	if _, code := down.Run("test", root); code != 1 || len(down.Calls()) != 1 {
		t.Errorf("stack down: exit %d, calls %q", code, down.Calls())
	}
	// A bad --changed ref fails before the stack is asked anything
	bad := verbtest.NewDocker(t, stackUp)
	if _, code := bad.Run("test", gitRepo(t, "develop"), "--changed=nope"); code != 2 || len(bad.Calls()) != 0 {
		t.Errorf("--changed=nope: exit %d, calls %q", code, bad.Calls())
	}
	// Another run holds the databases
	if err := lock("test", root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(lockPath("test", root)) })
	held := verbtest.NewDocker(t, stackUp)
	if _, code := held.Run("test", root, "--no-frontend", "./x"); code != 1 || anyContains(cmds(held), "DATABASE") {
		t.Errorf("a held lock: exit %d, calls %q", code, held.Calls())
	}
}

// A failing module ends the run with go test's code, and the tally still says what failed
func TestTestFails(t *testing.T) {
	d := verbtest.NewDocker(t, `case "$*" in
"compose ps --services --status running") echo app ;;
*"go test -json"*) printf '%s\n' '{"Action":"output","Package":"p","Test":"TestA","Output":"    boom\n"}' '{"Action":"fail","Package":"p","Test":"TestA"}'; exit 3 ;;
esac`)
	out, code := d.Run("test", t.TempDir(), "--no-frontend", "./x")
	if code != 3 || !strings.Contains(out, "boom") || !strings.Contains(out, "0 passed, 1 failed") || strings.Contains(out, "tests OK") {
		t.Errorf("exit %d:\n%s", code, out)
	}
	// Under --lean there's no ClickHouse, and the run says its tests will skip
	if !strings.Contains(out, "clickhouse not running") || anyContains(cmds(d), "clickhouse-client") {
		t.Errorf("without clickhouse:\n%s", out)
	}
}

// The frontend half under --changed: skipped when the app didn't change, everything on a config change, otherwise
// the unit tests that import a changed file - and nothing for deletions alone
// The frontend half under --changed: skipped when the app didn't change, everything on a config change, otherwise
// the unit tests that import a changed file - and nothing for deletions alone
func TestFrontendChanged(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "apps", "dashboard", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "apps", "dashboard", "src", "a.ts"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var ran strings.Builder
	tr := tester{root: root, pnpm: func(_, app string, _ []string, args ...string) error {
		fmt.Fprintf(&ran, "%s: %s\n", app, strings.Join(args, " "))
		return nil
	}}
	for _, c := range []struct {
		changed []string
		only    bool
		want    string
	}{
		{nil, false, "dashboard: run typecheck:all\ndashboard: run test\n"},
		{[]string{"apps/backend/x.go"}, true, ""},
		{[]string{"apps/dashboard/package.json", "apps/dashboard/src/a.ts"}, true, "dashboard: run typecheck:all\ndashboard: run test\n"},
		{[]string{"apps/dashboard/src/a.ts", "apps/dashboard/src/gone.ts"}, true, "dashboard: run typecheck:all\ndashboard: exec vitest related --run --passWithNoTests src/a.ts\n"},
		{[]string{"apps/dashboard/src/gone.ts"}, true, "dashboard: run typecheck:all\n"},
	} {
		ran.Reset()
		if code := tr.frontend("dashboard", "apps/dashboard", "typecheck:all", c.changed, c.only); code != 0 {
			t.Errorf("%v: exit %d", c.changed, code)
		}
		if ran.String() != c.want {
			t.Errorf("%v ran %q, want %q", c.changed, ran.String(), c.want)
		}
	}
	// A failing typecheck stops the app there, with a failing code
	failing := tester{root: root, pnpm: func(string, string, []string, ...string) error { return errors.New("no") }}
	if code := failing.frontend("dashboard", "apps/dashboard", "typecheck:all", nil, false); code == 0 {
		t.Error("a failing typecheck passed")
	}
}

func TestSchemaHashFollowsTheMigrations(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "apps/backend/core/internal/db/migrations")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("1_a.sql", "create table a ();")
	before := schemaHash(root)
	write("README.md", "not a migration")
	if schemaHash(root) != before {
		t.Error("a non-.sql file changed the hash")
	}
	write("1_a.sql", "create table b ();")
	if schemaHash(root) == before {
		t.Error("an edited migration kept the hash")
	}
	write("1_a.sql", "create table a ();")
	write("2_b.sql", "")
	if schemaHash(root) == before {
		t.Error("an empty new migration kept the hash")
	}
}
