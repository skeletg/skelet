package legacy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
)

// Working out compose's environment in the CLI must leave a script's alone - neither the computed profiles nor the
// lean switches may reach it, and what the user exported still does
func TestScriptsGetOnlyTheCallersEnvironment(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "env"), []byte("#!/bin/sh\nenv\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".skelet-apps"), []byte("dashboard\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"COMPOSE_PROFILES", "SKELET_SIDECARS", "SKELET_TOOLS"} {
		if v, ok := os.LookupEnv(k); ok {
			t.Cleanup(func() { _ = os.Setenv(k, v) })
			_ = os.Unsetenv(k)
		}
	}
	t.Setenv("SKELET_SHIM", "1")
	t.Setenv("MINE", "kept")
	lib := compose.Load(root)
	_ = lib.Env()

	out, err := Command(root, "env").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"COMPOSE_PROFILES=", "SKELET_SIDECARS=", "SKELET_TOOLS=", "SKELET_SHIM="} {
		if strings.Contains(string(out), "\n"+leaked) || strings.HasPrefix(string(out), leaked) {
			t.Errorf("%s reached the script", leaked)
		}
	}
	if !strings.Contains(string(out), "MINE=kept") {
		t.Error("the caller's own environment didn't reach the script")
	}
}
