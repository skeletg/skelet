package stack

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The root .env wins over the caller's shell, as set -a; . .env did, and the dev defaults only fill what's still
// missing or empty
func TestServeEnv(t *testing.T) {
	root := t.TempDir()
	body := "STOREFRONT_CHANNEL_ID=from-dotenv\nSHELL_SET=dotenv-wins\nNEXT_PUBLIC_BACKEND_URL=\n"
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL_SET", "shell")
	t.Setenv("STOREFRONT_REVALIDATE_SECRET", "mine")
	t.Setenv("SKELET_ROOT", "/x")
	env, err := serveEnv(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"STOREFRONT_CHANNEL_ID=from-dotenv", "SHELL_SET=dotenv-wins",
		"NEXT_PUBLIC_BACKEND_URL=http://localhost:8080", "STOREFRONT_REVALIDATE_SECRET=mine", "LC_ALL=C.UTF-8",
	} {
		if !slices.Contains(env, want) {
			t.Errorf("no %s in the dev server's environment", want)
		}
	}
	if slices.Contains(env, "SKELET_ROOT=/x") {
		t.Error("the CLI's own variable reached the dev server")
	}
}
