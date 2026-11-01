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
	p := filepath.Join(dir, File)
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

func TestWriteKeepsTheOrderAndRoundTrips(t *testing.T) {
	p := filepath.Join(t.TempDir(), File)
	in := Profile{"B": "2", "A": "", "C": "x,y"}
	if err := Write(p, in, []string{"B", "A", "C"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	want := "# Written by skelet setup - your answers, so a re-run defaults to them\nB=2\nA=\nC=x,y\n"
	if string(raw) != want {
		t.Errorf("wrote %q", raw)
	}
	back, _ := Load(p)
	if v, ok := back.Get("A"); !ok || v != "" {
		t.Errorf("an empty answer didn't survive: %q, %v", v, ok)
	}
}
