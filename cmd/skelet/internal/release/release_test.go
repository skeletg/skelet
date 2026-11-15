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

func TestReleaseBuildsAndPushesOnlyTheAppsNamed(t *testing.T) {
	root := t.TempDir()
	put(t, root, "apps/backend/core/Dockerfile.prod", "FROM x\n")
	put(t, root, "apps/backend/signals/Dockerfile.prod", "FROM x\nARG UPLOAD_ORIGINS\nARG UNSET_ONE\n")
	put(t, root, ".env", "UPLOAD_ORIGINS=https://old\nUPLOAD_ORIGINS=https://cdn\n")
	t.Setenv("REGISTRY", "reg.example")
	t.Setenv("PLATFORM", "")
	d := verbtest.NewDocker(t, "")
	out, code := d.Run("release", root, "signals", "--tag", "v9")
	var cmds []string
	for _, c := range d.Calls() {
		cmd, _, _ := strings.Cut(c, "|")
		cmds = append(cmds, cmd)
	}
	want := []string{
		"build --platform linux/amd64 --build-arg UPLOAD_ORIGINS=https://cdn -f apps/backend/signals/Dockerfile.prod -t reg.example/skelet-signals:v9 apps/backend",
		"push reg.example/skelet-signals:v9",
	}
	if code != 0 || strings.Join(cmds, "\n") != strings.Join(want, "\n") {
		t.Errorf("exit %d, docker got:\n%s\nwant:\n%s", code, strings.Join(cmds, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(out, "apps/backend/signals/.env") || !strings.Contains(out, "SIGNALS_IMAGE=reg.example/skelet-signals:v9") || strings.Contains(out, "CORE_IMAGE") {
		t.Errorf("output:\n%s", out)
	}
	if _, code := verbtest.NewDocker(t, "").Run("release", root, "dashboard"); code != 2 {
		t.Errorf("dashboard: exit %d, want 2", code)
	}
}

func TestReleaseNeedsAnAppARegistryAndStopsOnAFailedBuild(t *testing.T) {
	root := t.TempDir()
	put(t, root, "apps/backend/core/Dockerfile.prod", "FROM x\n")
	t.Setenv("REGISTRY", "reg")
	if _, code := verbtest.NewDocker(t, "").Run("release", root); code != 2 {
		t.Errorf("no app: exit %d", code)
	}
	if _, code := verbtest.NewDocker(t, "").Run("release", root, "nope"); code != 2 {
		t.Errorf("unknown app: exit %d", code)
	}
	t.Setenv("REGISTRY", "")
	if _, code := verbtest.NewDocker(t, "").Run("release", root, "core", "--tag", "v1"); code != 1 {
		t.Errorf("no REGISTRY: exit %d", code)
	}
	t.Setenv("REGISTRY", "reg")
	d := verbtest.NewDocker(t, "exit 5")
	if _, code := d.Run("release", root, "core", "--tag=v1"); code != 5 || len(d.Calls()) != 1 {
		t.Errorf("a failed build: exit %d after %q - want 5 and no push", code, d.Calls())
	}
}
