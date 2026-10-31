package testrun

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/legacy"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const TestAbout = `go test across every module, then each frontend's typecheck and unit tests.

The Go half runs against disposable databases - skelet_test and skelet_test_signals, plus a
ClickHouse database when the spine is up - fresh each run, and every test binary clones its own copy
of them, so packages never share rows. Without them every database-backed test would skip: the dev
databases are never marked disposable, on purpose. Each module ends with a tally, and a skip count
that isn't near zero means a database went missing, not that the tests are fine. One run at a time.

  --keep          reuse last run's databases (migrations still run, so a new one lands)
  --no-frontend   skip the typechecks and unit tests
  --changed[=REF] only what a change since REF can reach - the merge base with develop by default.
                  Go keeps its test cache, which reruns the packages whose code, dependencies or
                  schema changed, and each frontend runs the unit tests that import a changed file

Anything after the flags goes to go test, and then only core's packages run.`

const (
	testDB    = "skelet_test"
	testSigDB = "skelet_test_signals"
	testDBURL = "postgres://skelet:skelet@db:5432/" + testDB + "?sslmode=disable"
	testSigDS = "postgres://skelet:skelet@signals-db:5432/" + testSigDB + "?sslmode=disable"
	testCHURL = "clickhouse://skelet:skelet@clickhouse:9000/" + testSigDB
	core      = "/var/www/backend/core"
	signals   = "/var/www/backend/signals"
)

type testOptions struct {
	frontend bool
	fresh    bool
	// changed is the --changed ref: "auto" for the merge base with develop, empty when not given
	changed string
	goArgs  []string
}

// parseTest takes flags up to the first argument that isn't one - that and everything after it go to go test
func parseTest(args []string) testOptions {
	o := testOptions{frontend: true, fresh: true}
	for i, a := range args {
		switch {
		case a == "--no-frontend" || a == "--no-dashboard":
			o.frontend = false
		case a == "--keep":
			o.fresh = false
		case a == "--fresh":
		case a == "--changed":
			o.changed = "auto"
		case strings.HasPrefix(a, "--changed="):
			o.changed = strings.TrimPrefix(a, "--changed=")
		default:
			o.goArgs = args[i:]
			return o
		}
	}
	return o
}

func Test(root string, args []string) int {
	o := parseTest(args)
	var changed []string
	if o.changed != "" {
		ref, files, code := changedSince(root, o.changed)
		if code != 0 {
			return code
		}
		short, _ := git(root, "rev-parse", "--short", ref)
		fmt.Printf("==> only what changed since %s (%d files)\n", strings.TrimSpace(short), len(files))
		changed = files
	}
	// The databases and go test live in the containers - there's no one-off fallback, since that would come up without
	// the database it's about to migrate
	lib := compose.Load(root)
	if !lib.Running("app") {
		fmt.Fprintln(os.Stderr, "dev stack not running - start it with skelet up")
		return 1
	}
	sys.HandleInterrupts()
	if lock("test", root) != nil {
		return 1
	}
	d := dbs{root: root, lib: lib}
	flush := checkpoint(d)
	sys.AtExit(flush)

	fmt.Println("==> preparing disposable test databases")
	for _, step := range []func() error{
		func() error { return d.postgres("db", testDB, o.fresh) },
		func() error { return d.postgres("signals-db", testSigDB, o.fresh) },
		func() error { return d.dropClones("db", testDB) },
		func() error { return d.dropClones("signals-db", testSigDB) },
	} {
		if err := step(); err != nil {
			return fail(err)
		}
	}
	d.unmarkDev()
	fmt.Printf("==> migrating commerce (%s)\n", testDB)
	if err := d.migrate(testDBURL, "internal/db/migrations"); err != nil {
		return sys.ExitCode(err)
	}
	fmt.Printf("==> migrating signals (%s)\n", testSigDB)
	if err := d.migrate(testSigDS, signals+"/internal/db/migrations"); err != nil {
		return sys.ExitCode(err)
	}
	// ClickHouse sits behind the signals profile, so under --lean the spine's tests skip - said out loud rather than not
	chDSN := ""
	if lib.Running("clickhouse") {
		if err := d.clickhouseDB(testSigDB, o.fresh); err != nil {
			return fail(err)
		}
		if err := d.dropClickhouseClones(testSigDB); err != nil {
			return fail(err)
		}
		fmt.Printf("==> migrating the spine (clickhouse %s)\n", testSigDB)
		if err := d.migrate(testCHURL, signals+"/internal/analytics/spinech/migrations"); err != nil {
			return sys.ExitCode(err)
		}
		chDSN = testCHURL
	} else {
		fmt.Println("==> (clickhouse not running - the spine's ClickHouse tests will skip)")
	}

	// A full run reruns everything; with --changed the test cache answers for what nothing touched, keyed on the schema
	// through SKELET_TEST_SCHEMA, which lib/testdb reads
	cache := []string{"-count=1"}
	if o.changed != "" {
		cache = nil
	}
	schema := "SKELET_TEST_SCHEMA=" + schemaHash(root)
	// Core opens no signals database and the suite opens no commerce one - a test that wants the other side's data gets
	// it the way production does, over the contract or off a pushed projection
	coreEnv := []string{"DATABASE_URL=" + testDBURL, "SIGNALS_DB_URL=", schema}
	sigEnv := []string{"DATABASE_URL=", "SIGNALS_DB_URL=" + testSigDS, "SIGNALS_CH_URL=" + chDSN, "SIGNALS_CH_TEST_DSN=" + chDSN, schema}
	t := tester{root: root, lib: lib, cache: cache}

	fmt.Println("==> backend")
	if len(o.goArgs) == 0 {
		// e2e sits out of the sweep - it writes real orders and never cleans up, so it runs against its own databases
		corePkgs, err := t.packages(core)
		if err != nil {
			return fail(err)
		}
		if code := t.goTest("core", core, coreEnv, corePkgs); code != 0 {
			return code
		}
		fmt.Println("==> signals module")
		sigPkgs, err := t.packages(signals)
		if err != nil {
			return fail(err)
		}
		if code := t.goTest("signals", signals, sigEnv, sigPkgs); code != 0 {
			return code
		}
		fmt.Println("==> sdk module")
		if code := t.goTest("sdk", "/var/www/backend/sdk", coreEnv, []string{"./..."}); code != 0 {
			return code
		}
		fmt.Println("==> skelet CLI module")
		if code := t.cliTest(); code != 0 {
			return code
		}
		fmt.Println("==> (e2e suites skipped - run them against their throwaway databases with skelet test e2e)")
	} else if code := t.goTest("core", core, coreEnv, o.goArgs); code != 0 {
		return code
	}
	flush()

	if o.frontend {
		if code := t.frontend("dashboard", "apps/dashboard", "typecheck:all", []string{"dash"}, changed, o.changed != ""); code != 0 {
			return code
		}
		// Per app, like CI - stopping at the dashboard's would leave every storefront untested
		manifests, _ := filepath.Glob(filepath.Join(root, "apps", "channels", "*", "package.json"))
		slices.Sort(manifests)
		for _, m := range manifests {
			name := filepath.Base(filepath.Dir(m))
			if code := t.frontend(name, "apps/channels/"+name, "typecheck", []string{"channel", name}, changed, o.changed != ""); code != 0 {
				return code
			}
		}
	}
	fmt.Println("==> tests OK")
	return 0
}

// changedSince resolves the --changed ref and lists what's changed since it: committed since, plus whatever isn't
// committed yet, new files included
func changedSince(root, ref string) (string, []string, int) {
	if ref == "auto" {
		ref = "HEAD"
		for _, base := range []string{"develop", "origin/develop"} {
			if out, err := git(root, "merge-base", "HEAD", base); err == nil {
				ref = strings.TrimSpace(out)
				break
			}
		}
	}
	if _, err := git(root, "rev-parse", "--verify", "-q", ref+"^{commit}"); err != nil {
		fmt.Fprintf(os.Stderr, "--changed: %s is not a commit\n", ref)
		return "", nil, 2
	}
	diff, _ := git(root, "diff", "--name-only", ref)
	untracked, _ := git(root, "ls-files", "--others", "--exclude-standard")
	files := strings.Fields(diff + "\n" + untracked)
	slices.Sort(files)
	return ref, slices.Compact(files), 0
}

func git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	return string(out), err
}

// checkpoint flushes the test databases in the background, once. Each clone the Go tests make leaves its pages for a
// checkpoint to write out, and the next run's DROP DATABASE forces it - a minute and more after a full run on a slow
// disk. Started once the Go tests are done, it runs while the frontends test or the next edit happens
func checkpoint(d dbs) func() {
	done := false
	return func() {
		if done {
			return
		}
		done = true
		for _, svc := range []string{"db", "signals-db"} {
			cmd := d.compose("exec", "-T", svc, "psql", "-U", "skelet", "-d", "postgres", "-qc", "CHECKPOINT")
			sys.Detach(cmd)
			_ = cmd.Start()
		}
	}
}

// schemaHash is every migration the test databases are built from - lib/testdb puts it in go test's cache key, so a
// migration change reruns the database-backed tests even when no Go changed
func schemaHash(root string) string {
	h := sha256.New()
	var files []string
	for _, dir := range []string{"apps/backend/core/internal/db/migrations", "apps/backend/signals/internal/db/migrations",
		"apps/backend/signals/internal/analytics/spinech/migrations"} {
		_ = filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(p, ".sql") {
				files = append(files, p)
			}
			return nil
		})
	}
	slices.Sort(files)
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		rel, _ := filepath.Rel(root, f)
		fmt.Fprintf(h, "%s\n%d\n", rel, len(raw))
		h.Write(raw)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

type tester struct {
	root  string
	lib   compose.Lib
	cache []string
}

// packages is a module's packages minus its e2e suite, which runs only under skelet test e2e. -e lists a package that
// doesn't compile too, so go test reports it in the tally rather than the listing ending the run
func (t tester) packages(workdir string) ([]string, error) {
	cmd := compose.Command(t.root, t.lib.Env(), "exec", "-T", "-w", workdir, "app", "go", "list", "-e", "./...")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var pkgs []string
	for p := range strings.FieldsSeq(string(out)) {
		if !strings.HasSuffix(p, "/internal/e2e") {
			pkgs = append(pkgs, p)
		}
	}
	return pkgs, nil
}

// goTest runs go test -json in the app container and prints it through the tally. Its exit code is go test's, so the
// first module that fails ends the run
func (t tester) goTest(label, workdir string, env, args []string) int {
	argv := append([]string{"exec", "-T"}, compose.UserFlags()...)
	argv = append(argv, "-w", workdir)
	for _, e := range env {
		argv = append(argv, "-e", e)
	}
	argv = append(append(append(argv, "app", "go", "test", "-json"), t.cache...), args...)
	return t.summarized(label, compose.Command(t.root, t.lib.Env(), argv...))
}

// cliTest runs this module's own tests where it's built - host Go, else the golang image, with the repo read-only since
// its tests read the route table and bin/
func (t tester) cliTest() int {
	args := append(append([]string{"test", "-json"}, t.cache...), "./...")
	var cmd *exec.Cmd
	if _, err := exec.LookPath("go"); err == nil {
		cmd = exec.Command("go", args...)
		cmd.Dir = filepath.Join(t.root, "cmd", "skelet")
		cmd.Env = append(sys.Env(), "GOWORK=off")
	} else {
		raw, _ := os.ReadFile(filepath.Join(t.root, "cmd", "skelet", "go.mod"))
		version := "1"
		if m := regexp.MustCompile(`(?m)^go (\S+)`).FindSubmatch(raw); m != nil {
			version = string(m[1])
		}
		cmd = exec.Command("docker", append([]string{"run", "--rm", "-v", t.root + ":/repo:ro", "-v", "skelet-cli-gocache:/root/.cache/go-build",
			"-w", "/repo/cmd/skelet", "-e", "GOFLAGS=-buildvcs=false", "golang:" + version + "-alpine", "go"}, args...)...)
	}
	return t.summarized("skelet", cmd)
}

func (t tester) summarized(label string, cmd *exec.Cmd) int {
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return fail(err)
	}
	err = sys.Child(func() error {
		if err := cmd.Start(); err != nil {
			return err
		}
		summarize(label, out, os.Stdout)
		return cmd.Wait()
	})
	return sys.ExitCode(err)
}

// Config, dependencies or the test setup reach every test, so a change to them means the app's whole suite
var touchesEverything = regexp.MustCompile(`(^|/)(package\.json|pnpm-lock\.yaml|pnpm-workspace\.yaml|tsconfig[^/]*\.json|vite(st)?\.config\.[cm]?[jt]s)$|^src/test-setup\.ts$`)

// frontend typechecks an app and runs its unit tests - all of them, or under --changed only what a change reaches
func (t tester) frontend(label, dir, typecheck string, runner, changed []string, onlyChanged bool) int {
	run := func(args ...string) int {
		if err := legacy.Run(t.root, runner[0], append(slices.Clone(runner[1:]), args...)...); err != nil {
			return max(sys.ExitCode(err), 1)
		}
		return 0
	}
	if !onlyChanged {
		fmt.Printf("==> %s typecheck\n", label)
		if code := run("run", typecheck); code != 0 {
			return code
		}
		fmt.Printf("==> %s unit tests\n", label)
		return run("run", "test")
	}
	var files []string
	for _, f := range changed {
		if rel, ok := strings.CutPrefix(f, dir+"/"); ok {
			files = append(files, rel)
		}
	}
	if len(files) == 0 {
		fmt.Printf("==> %s: nothing changed, skipped\n", label)
		return 0
	}
	fmt.Printf("==> %s typecheck\n", label)
	if code := run("run", typecheck); code != 0 {
		return code
	}
	if slices.ContainsFunc(files, touchesEverything.MatchString) {
		fmt.Printf("==> %s unit tests (config changed - all of them)\n", label)
		return run("run", "test")
	}
	var related []string
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(t.root, dir, f)); err == nil {
			related = append(related, f)
		}
	}
	// Only deletions - the typecheck already caught anything still importing them, and an empty list would run it all
	if len(related) == 0 {
		fmt.Printf("==> %s unit tests: only deletions, nothing to run\n", label)
		return 0
	}
	fmt.Printf("==> %s unit tests (those that import the %d changed files)\n", label, len(related))
	return run(append([]string{"exec", "vitest", "related", "--run", "--passWithNoTests"}, related...)...)
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "skelet test:", err)
	return 1
}
