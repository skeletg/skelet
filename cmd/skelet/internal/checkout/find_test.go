package checkout

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindWalksUpToTheCheckout(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"skelet", "bin/_lib"} {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	deep := filepath.Join(root, "apps", "dashboard", "src")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, deep} {
		if got, ok := Find(dir); !ok || got != root {
			t.Errorf("Find(%s) = %q, %v - want %s", dir, got, ok, root)
		}
	}
	if got, ok := Find(filepath.Dir(root)); ok {
		t.Errorf("Find above the checkout found %q", got)
	}
}

func TestLocateTellsTheKinds(t *testing.T) {
	write := func(root, rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	native, both, stranger := t.TempDir(), t.TempDir(), t.TempDir()
	write(native, "cmd/skelet/go.mod", "module github.com/skeletg/skelet/cmd/skelet\n\ngo 1.27.3\n")
	write(both, "cmd/skelet/go.mod", "module github.com/skeletg/skelet/cmd/skelet\n")
	write(both, "skelet", "")
	write(both, "bin/_lib", "")
	write(stranger, "cmd/skelet/go.mod", "module example.com/other\n")
	for root, want := range map[string]Kind{native: Native, both: Shim, stranger: None} {
		if _, got := Locate(filepath.Join(root, "cmd", "skelet")); got != want {
			t.Errorf("Locate(%s) = %v, want %v", root, got, want)
		}
	}
}
