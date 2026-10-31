package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/verbtest"
)

func TestMain(m *testing.M) {
	verbtest.Main(m, map[string]func(string, []string) int{"release": Run})
}

func put(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Every image built for the platform with the build args it declares - your shell's value over the root .env's -
// then pushed, storefronts found by directory
func TestReleaseBuildsAndPushesEveryImage(t *testing.T) {
	root := t.TempDir()
	put(t, root, "apps/backend/core/Dockerfile.prod", "FROM x\n")
	put(t, root, "apps/backend/signals/Dockerfile.prod", "FROM x\n")
	put(t, root, "apps/dashboard/Dockerfile.prod", "FROM x\nARG UPLOAD_ORIGINS\nARG UNSET_ONE\n")
	put(t, root, "apps/channels/eu-shop/Dockerfile.prod", "FROM x\nARG STOREFRONT_ORIGIN\n")
	put(t, root, ".env", "UPLOAD_ORIGINS=https://old\nUPLOAD_ORIGINS=https://cdn\nSTOREFRONT_ORIGIN=https://dotenv\n")
	t.Setenv("REGISTRY", "reg.example")
	t.Setenv("STOREFRONT_ORIGIN", "https://shell")
	t.Setenv("PLATFORM", "")
	d := verbtest.NewDocker(t, "")
	out, code := d.Run("release", root, "v9")
	var cmds []string
	for _, c := range d.Calls() {
		cmd, _, _ := strings.Cut(c, "|")
		cmds = append(cmds, cmd)
	}
	want := []string{
		"build --platform linux/amd64 -f apps/backend/core/Dockerfile.prod -t reg.example/skelet-app:v9 apps/backend",
		"push reg.example/skelet-app:v9",
		"build --platform linux/amd64 -f apps/backend/signals/Dockerfile.prod -t reg.example/skelet-signals:v9 apps/backend",
		"push reg.example/skelet-signals:v9",
		"build --platform linux/amd64 --build-arg UPLOAD_ORIGINS=https://cdn -f apps/dashboard/Dockerfile.prod -t reg.example/skelet-dashboard:v9 apps/dashboard",
		"push reg.example/skelet-dashboard:v9",
		"build --platform linux/amd64 --build-arg STOREFRONT_ORIGIN=https://shell -f apps/channels/eu-shop/Dockerfile.prod -t reg.example/skelet-eu-shop:v9 apps/channels/eu-shop",
		"push reg.example/skelet-eu-shop:v9",
	}
	if code != 0 || strings.Join(cmds, "\n") != strings.Join(want, "\n") {
		t.Errorf("exit %d, docker got:\n%s\nwant:\n%s", code, strings.Join(cmds, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(out, "export EU_SHOP_IMAGE=reg.example/skelet-eu-shop:v9") {
		t.Errorf("exports:\n%s", out)
	}
}

func TestReleaseNeedsARegistryAndStopsOnAFailedBuild(t *testing.T) {
	t.Setenv("REGISTRY", "")
	if _, code := verbtest.NewDocker(t, "").Run("release", t.TempDir(), "v1"); code != 1 {
		t.Errorf("no REGISTRY: exit %d", code)
	}
	root := t.TempDir()
	put(t, root, "apps/backend/core/Dockerfile.prod", "FROM x\n")
	t.Setenv("REGISTRY", "reg")
	d := verbtest.NewDocker(t, "exit 5")
	if _, code := d.Run("release", root, "v1"); code != 5 || len(d.Calls()) != 1 {
		t.Errorf("a failed build: exit %d after %q - want 5 and no push", code, d.Calls())
	}
}
