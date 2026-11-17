package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/plugins"
)

func uiTree(t *testing.T, files map[string]string) string {
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

// The import lists hold the enabled plugins' packages without their versions, in slug order, each in its app's quoting
func TestPluginUIWritesTheEnabledPluginsPackages(t *testing.T) {
	root := uiTree(t, map[string]string{
		plugins.File: `{"plugins": {
			"zeta": {"enabled": true, "dashboard": "@acme/zeta@^2.0.0", "storefront": "acme-zeta-shop"},
			"alpha": {"enabled": true, "dashboard": "@acme/alpha"},
			"off": {"enabled": false, "dashboard": "@acme/off"}
		}}`,
		"apps/dashboard/package.json":              `{"dependencies": {"@acme/zeta": "^2.0.0"}, "devDependencies": {"@acme/alpha": "1.0.0"}}`,
		"apps/dashboard/src/plugins/packages.ts":   ``,
		"apps/channels/shop/package.json":          `{"dependencies": {"acme-zeta-shop": "^1.0.0"}}`,
		"apps/channels/shop/app/plugins/loaded.ts": `export {};`,
		"templates/astro-shop/package.json":        `{}`,
	})
	if err := pluginUI(root); err != nil {
		t.Fatal(err)
	}
	dash, _ := os.ReadFile(filepath.Join(root, dashboardPackages))
	if !strings.Contains(string(dash), "import '@acme/alpha'\nimport '@acme/zeta'\n") || strings.Contains(string(dash), "@acme/off") {
		t.Errorf("the dashboard's list:\n%s", dash)
	}
	shop, _ := os.ReadFile(filepath.Join(root, "apps/channels/shop", storefrontPackages))
	if !strings.Contains(string(shop), `import "acme-zeta-shop";`+"\n") {
		t.Errorf("the storefront's list:\n%s", shop)
	}
	if _, err := os.Stat(filepath.Join(root, "templates/astro-shop", storefrontPackages)); err == nil {
		t.Error("a storefront with no plugin loader got an import list")
	}
}

// An import of a package the app doesn't have fails its build with nothing saying which plugin wanted it
func TestPluginUINamesThePackageAnAppIsMissing(t *testing.T) {
	root := uiTree(t, map[string]string{
		plugins.File:                  `{"plugins": {"fraud": {"enabled": true, "dashboard": "@acme/fraud-dashboard@^1.2.0"}}}`,
		"apps/dashboard/package.json": `{}`,
		dashboardPackages:             ``,
	})
	err := pluginUI(root)
	if err == nil || !strings.Contains(err.Error(), "skelet pnpm dashboard add @acme/fraud-dashboard@^1.2.0") {
		t.Fatalf("got %v - want the command that installs it", err)
	}
	if list, _ := os.ReadFile(filepath.Join(root, dashboardPackages)); strings.Contains(string(list), "@acme/fraud") {
		t.Errorf("the missing package was imported anyway:\n%s", list)
	}
}
