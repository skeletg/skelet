package pnpm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/verbtest"
)

func TestMain(m *testing.M) {
	verbtest.Main(m, map[string]func(string, []string) int{
		"dashboard": Dashboard, "channel": Channel, "package": Package,
		"rundir": func(root string, args []string) int {
			if RunDir(root, args[0], args[1:]...) != nil {
				return 1
			}
			return 0
		},
		// What skelet gen does: the endpoint goes to a host install's codegen
		"codegen": func(root string, args []string) int {
			if Run(root, args[0], []string{"GRAPHQL_CODEGEN_URL=http://x"}, "run", "codegen") != nil {
				return 1
			}
			return 0
		},
	})
}

// app lays out a frontend with a package.json and node_modules holding tree: none, pnpm or npm
func app(t *testing.T, root, rel, tree string) {
	t.Helper()
	dir := filepath.Join(root, rel)
	files := map[string]string{"package.json": "{}"}
	switch tree {
	case "pnpm":
		files["node_modules/.modules.yaml"] = ""
	case "npm":
		files["node_modules/left-pad/index.js"] = ""
	}
	for f, body := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func cmds(d *verbtest.Docker) []string {
	var out []string
	for _, c := range d.Calls() {
		args, _, _ := strings.Cut(c, "|")
		out = append(out, args)
	}
	return out
}

const user = "-e LC_ALL=C.UTF-8 -e LANG=C.UTF-8"

func TestInTheContainer(t *testing.T) {
	t.Setenv("SKELET_FORCE_DOCKER", "")
	root := t.TempDir()
	app(t, root, "apps/dashboard", "none")
	app(t, root, "apps/channels/shop", "none")
	app(t, root, "templates/vetrina", "none")
	for _, c := range []struct {
		verb, answer string
		args         []string
		want         string
	}{
		{"dashboard", `case "$*" in "compose ps --services --status running") echo dashboard ;; esac`, []string{"run", "lint"},
			"compose exec " + user + " dashboard pnpm run lint"},
		// A storefront's container starts at the workspace root - pnpm runs in the storefront's own directory
		{"channel", "", []string{"shop", "install"}, "compose run --rm " + user + " -w /var/www/skelet/apps/channels/shop shop pnpm install"},
		{"channel", "", []string{"vetrina", "add", "x"}, "compose run --rm " + user + " -w /var/www/skelet/templates/vetrina vetrina pnpm add x"},
	} {
		d := verbtest.NewDocker(t, c.answer)
		d.Fake("pnpm", "")
		calls := func() []string { return cmds(d) }
		if _, code := d.Run(c.verb, root, c.args...); code != 0 || calls()[len(calls())-1] != c.want {
			t.Errorf("%s %q: exit %d, calls %q, want last %q", c.verb, c.args, code, calls(), c.want)
		}
	}
}

// A host install runs the host's pnpm, in the app - unless SKELET_FORCE_DOCKER says otherwise
func TestOnTheHost(t *testing.T) {
	t.Setenv("SKELET_FORCE_DOCKER", "")
	root := t.TempDir()
	app(t, root, "apps/dashboard", "pnpm")
	app(t, root, "apps/channels/shop", "pnpm")
	d := verbtest.NewDocker(t, "")
	d.Fake("pnpm", `echo "url=$GRAPHQL_CODEGEN_URL"`)
	if _, code := d.Run("dashboard", root, "run", "lint"); code != 0 || strings.Join(d.Calls(), "\n") != "pnpm run lint|"+filepath.Join(root, "apps/dashboard") {
		t.Errorf("exit %d, calls %q", code, d.Calls())
	}
	// The codegen endpoint reaches a host install
	if out, code := d.Run("codegen", root, "shop"); code != 0 || !strings.Contains(out, "url=http://x") {
		t.Errorf("codegen: exit %d, %q", code, out)
	}

	t.Setenv("SKELET_FORCE_DOCKER", "1")
	forced := verbtest.NewDocker(t, "")
	forced.Fake("pnpm", "")
	if _, code := forced.Run("dashboard", root, "run", "lint"); code != 0 || cmds(forced)[len(cmds(forced))-1] != "compose run --rm "+user+" dashboard pnpm run lint" {
		t.Errorf("forced: exit %d, calls %q", code, forced.Calls())
	}
}

// An app that isn't here, or holds an npm install pnpm won't take over without a terminal, runs nothing
func TestRefusals(t *testing.T) {
	t.Setenv("SKELET_FORCE_DOCKER", "")
	root := t.TempDir()
	app(t, root, "apps/channels/old", "npm")
	for _, args := range [][]string{{"old", "install"}, {"gone", "install"}} {
		d := verbtest.NewDocker(t, "")
		d.Fake("pnpm", "")
		if _, code := d.Run("channel", root, args...); code != 1 || len(d.Calls()) != 0 {
			t.Errorf("%q: exit %d, calls %q", args, code, d.Calls())
		}
	}
	d := verbtest.NewDocker(t, "")
	if _, code := d.Run("dashboard", root, "install"); code != 1 || len(d.Calls()) != 0 {
		t.Errorf("no dashboard: exit %d, calls %q", code, d.Calls())
	}
}

// A package has no container: the host's pnpm when the workspace is installed there, else a storefront's container
// pointed at the package
func TestRunDir(t *testing.T) {
	t.Setenv("SKELET_FORCE_DOCKER", "")
	root := t.TempDir()
	app(t, root, "templates/vetrina", "none")
	app(t, root, "packages/storefront-next", "none")
	d := verbtest.NewDocker(t, "")
	d.Fake("pnpm", "")
	if _, code := d.Run("rundir", root, "packages/storefront-next", "run", "test"); code != 0 ||
		cmds(d)[len(cmds(d))-1] != "compose run --rm "+user+" -w /var/www/skelet/packages/storefront-next vetrina pnpm run test" {
		t.Errorf("in a container: exit %d, calls %q", code, d.Calls())
	}
	// Installed on the host - the workspace root holds the record
	if err := os.WriteFile(filepath.Join(root, "pnpm-workspace.yaml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	app(t, root, ".", "pnpm")
	app(t, root, "packages/storefront-next", "none")
	if err := os.MkdirAll(filepath.Join(root, "packages/storefront-next/node_modules/vitest"), 0o755); err != nil {
		t.Fatal(err)
	}
	host := verbtest.NewDocker(t, "")
	host.Fake("pnpm", "")
	if _, code := host.Run("rundir", root, "packages/storefront-next", "run", "test"); code != 0 ||
		strings.Join(host.Calls(), "\n") != "pnpm run test|"+filepath.Join(root, "packages/storefront-next") {
		t.Errorf("on the host: exit %d, calls %q", code, host.Calls())
	}
}

// skelet pnpm <package> is the same, by the package's directory name
func TestPackage(t *testing.T) {
	t.Setenv("SKELET_FORCE_DOCKER", "")
	root := t.TempDir()
	app(t, root, "templates/vetrina", "none")
	app(t, root, "packages/storefront-core", "none")
	d := verbtest.NewDocker(t, "")
	if _, code := d.Run("package", root, "storefront-core", "test"); code != 0 ||
		cmds(d)[len(cmds(d))-1] != "compose run --rm "+user+" -w /var/www/skelet/packages/storefront-core vetrina pnpm test" {
		t.Errorf("exit %d, calls %q", code, d.Calls())
	}
	// No storefront to run it in, and no host install: refused rather than guessed at
	bare := t.TempDir()
	app(t, bare, "packages/storefront-core", "none")
	if _, code := verbtest.NewDocker(t, "").Run("package", bare, "storefront-core", "test"); code == 0 {
		t.Error("ran with no storefront and no host install")
	}
}
