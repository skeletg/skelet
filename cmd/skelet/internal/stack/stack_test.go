package stack

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/verbtest"
)

func TestMain(m *testing.M) {
	verbtest.Main(m, map[string]func(string, []string) int{"down": Down, "stop": Stop, "restart": Restart, "ps": Ps, "logs": Logs, "up": Up})
}

// Each passthrough becomes docker compose with your arguments after its own, from the checkout, carrying the
// profiles the selection files pick - the arguments the scripts passed, profiles included
func TestPassthroughs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".skelet-apps"), []byte("dashboard\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPOSE_PROFILES", "")
	_ = os.Unsetenv("COMPOSE_PROFILES")
	t.Setenv("SKELET_SIDECARS", "search")
	for verb, want := range map[string]string{
		"down":    "compose down --remove-orphans -v|search,dashboard,signals",
		"stop":    "compose stop -v|search,dashboard,signals",
		"restart": "compose restart -v|search,dashboard,signals",
		"ps":      "compose ps -v|search,dashboard,signals",
		"logs":    "compose logs -f -v|search,dashboard,signals",
	} {
		d := verbtest.NewDocker(t, "")
		if _, code := d.Run(verb, root, "-v"); code != 0 || !slices.Equal(d.Calls(), []string{want}) {
			t.Errorf("%s: exit %d, docker got %q, want %q", verb, code, d.Calls(), want)
		}
	}
}

// docker's own exit code is the verb's, as when the script exec'd it
func TestExitCodePassesThrough(t *testing.T) {
	d := verbtest.NewDocker(t, "exit 3")
	if _, code := d.Run("ps", t.TempDir()); code != 3 {
		t.Errorf("exit %d, want docker's 3", code)
	}
}
