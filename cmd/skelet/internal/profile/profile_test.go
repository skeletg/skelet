package profile

import (
	"os"
	"path/filepath"
	"testing"
)

// The shape a profile from an older setup has - keys that are gone, and none for the app selection
const legacy = `# Written by bin/setup - your answers, so a re-run defaults to them
SKELET_TARGET=dev
SKELET_SIGNALS=true
SKELET_B2B=true
SKELET_SEARCH=opensearch
SKELET_SPINE=clickhouse
SKELET_GATEWAYS=
export SKELET_AI="false"
SKELET_TOOLS_SELECTED='mailpit,grafana'
$(touch pwned)
`

func TestLoadParsesWithoutRunning(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "answers")
	if err := os.WriteFile(p, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	prof, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"SKELET_TARGET": "dev", "SKELET_AI": "false", "SKELET_TOOLS_SELECTED": "mailpit,grafana", "SKELET_GATEWAYS": ""} {
		if got, ok := prof.Get(k); !ok || got != want {
			t.Errorf("%s = %q, %v - want %q", k, got, ok, want)
		}
	}
	if _, ok := prof.Get("SKELET_APPS_SELECTED"); ok {
		t.Error("a key the file doesn't have reads as present")
	}
	if _, err := os.Stat(filepath.Join(dir, "pwned")); err == nil {
		t.Error("loading the profile ran a command in it")
	}
	if empty, err := Load(filepath.Join(dir, "missing")); err != nil || len(empty) != 0 {
		t.Errorf("a missing profile: %v, %v", empty, err)
	}
}
