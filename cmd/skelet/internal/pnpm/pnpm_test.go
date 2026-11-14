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
		"dashboard": Dashboard, "channel": Channel, "package": Package, "lockfile": Lockfile,
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

func TestOnTheHost(t *testing.T) {
	root := t.TempDir()
	app(t, root, "apps/dashboard", "pnpm")
	app(t, root, "apps/channels/shop", "none")
	app(t, root, "packages/storefront-core", "none")
	d := verbtest.NewDocker(t, "")
	d.Fake("pnpm", `echo "url=$GRAPHQL_CODEGEN_URL"`)
	for _, c := range []struct {
		verb string
		args []string
		want string
	}{
		{"dashboard", []string{"run", "lint"}, "pnpm run lint|" + filepath.Join(root, "apps/dashboard")},
		{"channel", []string{"shop", "add", "x"}, "pnpm add x|" + filepath.Join(root, "apps/channels/shop")},
		{"package", []string{"storefront-core", "test"}, "pnpm test|" + filepath.Join(root, "packages/storefront-core")},
		{"rundir", []string{"packages/storefront-core", "run", "test"}, "pnpm run test|" + filepath.Join(root, "packages/storefront-core")},
	} {
		before := len(d.Calls())
		if _, code := d.Run(c.verb, root, c.args...); code != 0 || len(d.Calls()) != before+1 || d.Calls()[before] != c.want {
			t.Errorf("%s %q: exit %d, calls %q, want %q", c.verb, c.args, code, d.Calls()[before:], c.want)
		}
	}
	if out, code := d.Run("codegen", root, "shop"); code != 0 || !strings.Contains(out, "url=http://x") {
		t.Errorf("codegen: exit %d, %q", code, out)
	}
	for _, c := range cmds(d) {
		if strings.HasPrefix(c, "compose ") || strings.HasPrefix(c, "run ") {
			t.Errorf("reached for a container: %q", c)
		}
	}
}

// An app that isn't here, or holds an npm install pnpm won't take over without a terminal, runs nothing
func TestRefusals(t *testing.T) {
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

func TestLockfileGoesThroughTheManifests(t *testing.T) {
	root := t.TempDir()
	app(t, root, "templates/vetrina", "none")
	for f, body := range map[string]string{
		"pnpm-workspace.yaml": "packages:\n  - templates/*\n", "pnpm-lock.yaml": "old\n", ".nvmrc": "24\n",
		"templates/vetrina/package.json": `{"packageManager": "pnpm@12.6.0"}`,
	} {
		if err := os.WriteFile(filepath.Join(root, f), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d := verbtest.NewDocker(t, `case "$*" in *--lockfile-only*)
  for a in "$@"; do case "$a" in *:/w) d="${a%:/w}";; esac; done
  echo new > "$d/pnpm-lock.yaml";;
esac`)
	if out, code := d.Run("lockfile", root); code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if last := cmds(d)[len(cmds(d))-1]; !strings.Contains(last, "node:24-alpine npx -y pnpm@12.6.0 install --lockfile-only") {
		t.Errorf("ran %q", last)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "pnpm-lock.yaml")); string(raw) != "new\n" {
		t.Errorf("pnpm-lock.yaml = %q", raw)
	}
}
