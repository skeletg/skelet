package node

import (
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A workspace member's node_modules holds links and no install record - the record is in the workspace root's
func TestTree(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "pnpm-workspace.yaml")
	app := filepath.Join(root, "templates", "vetrina")
	if err := os.MkdirAll(filepath.Join(app, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Tree(app); got != "none" {
		t.Errorf("empty: %s", got)
	}
	// Links into a workspace install in a container's volume - the host has none of its own
	if err := os.Symlink("../../../node_modules/.pnpm/react@19/node_modules/react", filepath.Join(app, "node_modules", "react")); err != nil {
		t.Fatal(err)
	}
	if got := Tree(app); got != "none" {
		t.Errorf("links into a tree the host can't see: %s", got)
	}
	touch(t, app, "node_modules/.package-lock.json")
	if got := Tree(app); got != "npm" {
		t.Errorf("an npm tree in a workspace member: %s", got)
	}
	touch(t, root, "node_modules/.modules.yaml")
	if got := Tree(app); got != "pnpm" {
		t.Errorf("a workspace member: %s", got)
	}
	// A standalone project still reads its own record, and a tree without one is npm's
	solo := t.TempDir()
	touch(t, solo, "node_modules/react/package.json")
	if got := Tree(solo); got != "npm" {
		t.Errorf("standalone npm: %s", got)
	}
	touch(t, solo, "node_modules/.modules.yaml")
	if got := Tree(solo); got != "pnpm" {
		t.Errorf("standalone: %s", got)
	}
	if WorkspaceRoot(app) != root || WorkspaceRoot(solo) != "" {
		t.Errorf("WorkspaceRoot: %q %q", WorkspaceRoot(app), WorkspaceRoot(solo))
	}
}
