package lint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToolchain(t *testing.T) {
	dir := t.TempDir()
	for directive, want := range map[string]string{"1.27.3": "go1.27.3+auto", "1.27": "go1.27.0+auto"} {
		p := filepath.Join(dir, "go.mod")
		if err := os.WriteFile(p, []byte("module x\n\ngo "+directive+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := toolchain(p); got != want {
			t.Errorf("go %s: %s, want %s", directive, got, want)
		}
	}
	if got := toolchain(filepath.Join(dir, "missing")); got != "auto" {
		t.Errorf("no go.mod: %s", got)
	}
}
