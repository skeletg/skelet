package pluginadd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/plugins"
)

// Nothing here may reach a container, pnpm or the dev stack - every seam is swapped for the whole package
func TestMain(m *testing.M) {
	opsCLI = func(string, ...string) error { panic("a test reached the ops CLI") }
	pnpmIn = func(string, string, ...string) error { panic("a test reached pnpm") }
	regen = func(string) int { panic("a test reached skelet gen") }
	confirm = func(string) (bool, bool) { panic("a test reached the keyboard") }
	os.Exit(m.Run())
}

// seams swaps the four for one test and puts the traps back after it
type seams struct {
	cli         []string
	added       []Package
	regenerated bool
}

const withFraud = `{"plugins": {
	"old": {"enabled": true, "dashboard": "@acme/old-dashboard"},
	"fraud": {"enabled": true, "module": "github.com/acme/fraud", "dashboard": "@acme/fraud-dashboard@^1.2.0", "storefront": "@acme/fraud-shop"}
}}`

// The ops CLI records fraud, pnpm add writes the dependency the way pnpm does unless failOn names the app
func fake(t *testing.T, failOn string) *seams {
	t.Helper()
	s := &seams{}
	oldCLI, oldPnpm, oldRegen, oldConfirm := opsCLI, pnpmIn, regen, confirm
	t.Cleanup(func() { opsCLI, pnpmIn, regen, confirm = oldCLI, oldPnpm, oldRegen, oldConfirm })
	opsCLI = func(root string, args ...string) error {
		s.cli = args
		return os.WriteFile(filepath.Join(root, plugins.File), []byte(withFraud), 0o644)
	}
	pnpmIn = func(root, app string, args ...string) error {
		if len(args) != 2 || args[0] != "add" {
			t.Errorf("pnpm %v", args)
		}
		if app == failOn {
			return os.ErrDeadlineExceeded
		}
		s.added = append(s.added, Package{App: app, Spec: args[1]})
		dir := "apps/dashboard"
		if app != "dashboard" {
			dir = "apps/channels/" + app
		}
		manifest := `{"dependencies": {"` + plugins.PackageName(args[1]) + `": "^1.2.0", "@acme/old-dashboard": "^1.0.0"}}`
		return os.WriteFile(filepath.Join(root, filepath.FromSlash(dir), "package.json"), []byte(manifest), 0o644)
	}
	regen = func(string) int { s.regenerated = true; return 0 }
	return s
}

func checkout(t *testing.T) string {
	return tree(t, map[string]string{
		plugins.File:                               `{"plugins": {"old": {"enabled": true, "dashboard": "@acme/old-dashboard"}}}`,
		"apps/dashboard/package.json":              `{"dependencies": {"@acme/old-dashboard": "^1.0.0"}}`,
		"apps/channels/shop/package.json":          `{}`,
		"apps/channels/shop/app/plugins/loaded.ts": `export {};`,
		"templates/astro-shop/package.json":        `{}`,
	})
}

func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// The ops CLI records the plugin and its UI packages; the verb installs each into the apps that load plugins and
// regenerates - and leaves alone what an app already has
func TestAddInstallsWhatTheAppsAreMissing(t *testing.T) {
	root := checkout(t)
	s := fake(t, "")
	if code := Add(root, []string{"github.com/acme/fraud@v1.2.0", "--yes", "--slug", "fraud"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Join(s.cli, " ") != "plugin add github.com/acme/fraud@v1.2.0 --slug fraud" {
		t.Errorf("ops CLI got %q - --yes is this verb's own", s.cli)
	}
	want := []Package{{"dashboard", "@acme/fraud-dashboard@^1.2.0"}, {"shop", "@acme/fraud-shop"}}
	if !reflect.DeepEqual(s.added, want) {
		t.Errorf("installed %+v, want %+v - the Astro template loads no plugins, and old was there already", s.added, want)
	}
	if !s.regenerated {
		t.Error("skelet gen never ran")
	}
}

// pnpm failing halfway leaves the plugin recorded and one app without its package - the same command again installs
// what's still missing and nothing else, where a diff of plugins.json would find nothing changed and skip it
func TestAddAgainFinishesAnInstallThatStoppedHalfway(t *testing.T) {
	root := checkout(t)
	s := fake(t, "shop")
	if code := Add(root, []string{"github.com/acme/fraud", "-y"}); code == 0 || s.regenerated {
		t.Fatalf("exit %d, regenerated %v - a failed pnpm add went on to skelet gen", code, s.regenerated)
	}
	s = fake(t, "")
	if code := Add(root, []string{"github.com/acme/fraud", "-y"}); code != 0 {
		t.Fatalf("the second run: exit %d", code)
	}
	if want := []Package{{"shop", "@acme/fraud-shop"}}; !reflect.DeepEqual(s.added, want) || !s.regenerated {
		t.Errorf("the second run installed %+v (regenerated %v), want only %+v", s.added, s.regenerated, want)
	}
}

// Nothing is installed and nothing of the plugin's runs until someone says so
func TestAddAsksBeforeInstallingAndRunning(t *testing.T) {
	for name, answer := range map[string][2]bool{"declined": {false, true}, "no terminal": {false, false}} {
		t.Run(name, func(t *testing.T) {
			root := checkout(t)
			s := fake(t, "")
			confirm = func(string) (bool, bool) { return answer[0], answer[1] }
			if code := Add(root, []string{"github.com/acme/fraud"}); code == 0 || len(s.added) != 0 || s.regenerated {
				t.Errorf("exit %d, installed %+v, regenerated %v", code, s.added, s.regenerated)
			}
		})
	}
	root := checkout(t)
	s := fake(t, "")
	confirm = func(string) (bool, bool) { return true, true }
	if code := Add(root, []string{"github.com/acme/fraud"}); code != 0 || len(s.added) != 2 || !s.regenerated {
		t.Errorf("after a yes: exit %d, installed %+v, regenerated %v", code, s.added, s.regenerated)
	}
}

// A checkout that left the dashboard out still gets its storefronts' packages
func TestAddWithoutTheDashboard(t *testing.T) {
	root := checkout(t)
	if err := os.RemoveAll(filepath.Join(root, "apps", "dashboard")); err != nil {
		t.Fatal(err)
	}
	s := fake(t, "")
	if code := Add(root, []string{"github.com/acme/fraud", "--yes"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if want := []Package{{"shop", "@acme/fraud-shop"}}; !reflect.DeepEqual(s.added, want) {
		t.Errorf("installed %+v, want %+v", s.added, want)
	}
}

// A failed fetch stops before anything is installed
func TestAddStopsWhenTheOpsCLIFails(t *testing.T) {
	root := tree(t, map[string]string{plugins.File: `{"plugins": {}}`})
	fake(t, "")
	opsCLI = func(string, ...string) error { return os.ErrNotExist }
	if code := Add(root, []string{"github.com/acme/missing", "--yes"}); code == 0 {
		t.Error("a failed add reported success")
	}
}

// A manifest can add a package to an app and can't move one the app has: naming next at an old version, or zod,
// installs nothing
func TestMissingNeverTouchesWhatAnAppAlreadyHas(t *testing.T) {
	root := tree(t, map[string]string{
		"apps/dashboard/package.json":                    `{"dependencies": {"zod": "^4.0.0", "@acme/a": "catalog:"}, "devDependencies": {"vite": "8.3.4"}}`,
		"templates/nextjs-vetrina/package.json":          `{"dependencies": {"next": "16.0.3"}}`,
		"templates/nextjs-vetrina/app/plugins/loaded.ts": `export {};`,
	})
	set := &plugins.Set{Plugins: map[string]plugins.Entry{
		"a":   {Enabled: true, Dashboard: "zod", Storefront: "next@13.5.0"},
		"b":   {Enabled: true, Dashboard: "vite@7.0.0"},
		"c":   {Enabled: true, Dashboard: "@acme/a@^2.0.0"},
		"d":   {Enabled: true, Dashboard: "@acme/new@latest"},
		"off": {Dashboard: "@acme/off"},
	}}
	missing, clashes, err := Missing(root, set)
	if err != nil {
		t.Fatal(err)
	}
	if want := []Package{{"dashboard", "@acme/new@latest"}}; !reflect.DeepEqual(missing, want) {
		t.Errorf("missing %+v, want %+v", missing, want)
	}
	if len(clashes) != 2 || !strings.Contains(clashes[0], "vite") || !strings.Contains(clashes[1], "next") {
		t.Errorf("clashes %q - want vite and next reported, zod and the catalog entry left quiet", clashes)
	}
}
