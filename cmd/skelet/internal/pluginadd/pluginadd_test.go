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
	os.Exit(m.Run())
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
// regenerates - and leaves alone the plugins that were already there
func TestAddInstallsTheUIPackagesOfWhatWasAdded(t *testing.T) {
	root := tree(t, map[string]string{
		plugins.File:                               `{"plugins": {"old": {"enabled": true, "dashboard": "@acme/old-dashboard"}}}`,
		"apps/dashboard/package.json":              `{}`,
		"apps/channels/shop/package.json":          `{}`,
		"apps/channels/shop/app/plugins/loaded.ts": `export {};`,
		"templates/astro-shop/package.json":        `{}`,
	})
	var cli []string
	var added []Package
	regenerated := false
	opsCLI = func(root string, args ...string) error {
		cli = args
		return os.WriteFile(filepath.Join(root, plugins.File), []byte(`{"plugins": {
			"old": {"enabled": true, "dashboard": "@acme/old-dashboard"},
			"fraud": {"enabled": true, "module": "github.com/acme/fraud", "dashboard": "@acme/fraud-dashboard@^1.2.0", "storefront": "@acme/fraud-shop"}
		}}`), 0o644)
	}
	pnpmIn = func(_ string, app string, args ...string) error {
		if len(args) != 2 || args[0] != "add" {
			t.Errorf("pnpm %v", args)
		}
		added = append(added, Package{App: app, Spec: args[1]})
		return nil
	}
	regen = func(string) int { regenerated = true; return 0 }

	if code := Add(root, []string{"github.com/acme/fraud@v1.2.0", "--slug", "fraud"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Join(cli, " ") != "plugin add github.com/acme/fraud@v1.2.0 --slug fraud" {
		t.Errorf("ops CLI got %q", cli)
	}
	want := []Package{{"dashboard", "@acme/fraud-dashboard@^1.2.0"}, {"shop", "@acme/fraud-shop"}}
	if !reflect.DeepEqual(added, want) {
		t.Errorf("installed %+v, want %+v - the Astro template loads no plugins, and old was there already", added, want)
	}
	if !regenerated {
		t.Error("skelet gen never ran")
	}
}

// A failed fetch stops before anything is installed
func TestAddStopsWhenTheOpsCLIFails(t *testing.T) {
	root := tree(t, map[string]string{plugins.File: `{"plugins": {}}`})
	opsCLI = func(string, ...string) error { return os.ErrNotExist }
	if code := Add(root, []string{"github.com/acme/missing"}); code == 0 {
		t.Error("a failed add reported success")
	}
}

func TestPackagesSkipsDisabledPlugins(t *testing.T) {
	set := &plugins.Set{Plugins: map[string]plugins.Entry{"off": {Dashboard: "@acme/off"}}}
	if got := Packages(t.TempDir(), []string{"off"}, set); len(got) != 0 {
		t.Errorf("a disabled plugin's packages: %+v", got)
	}
}
