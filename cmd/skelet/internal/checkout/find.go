// Package checkout locates the Skelet checkout a command acts on
package checkout

import (
	"os"
	"path/filepath"
	"strings"
)

// Kind is how a checkout runs its own CLI
type Kind int

const (
	None Kind = iota
	// Shim has the bash front door at its root - every release so far, and this tree until that's retired
	Shim
	// Native has only cmd/skelet, which the installed skelet builds and runs itself
	Native
)

// Locate walks up from dir to the first checkout - the one you stand in wins, the same rule the shim applies, so an
// installed binary acts on the repo you're in
func Locate(dir string) (string, Kind) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", None
	}
	for d := abs; d != string(filepath.Separator); d = filepath.Dir(d) {
		if k := kind(d); k != None {
			return d, k
		}
	}
	return "", None
}

// Find is Locate for callers that only need the root
func Find(dir string) (string, bool) {
	root, k := Locate(dir)
	return root, k != None
}

func kind(d string) Kind {
	if isFile(filepath.Join(d, "bin", "_lib")) && isFile(filepath.Join(d, "skelet")) {
		return Shim
	}
	// The module path, not just the file - a go.mod at cmd/skelet in some other repo isn't a Skelet checkout
	raw, err := os.ReadFile(filepath.Join(d, "cmd", "skelet", "go.mod"))
	if err == nil {
		for line := range strings.Lines(string(raw)) {
			if mod, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok && strings.HasSuffix(strings.TrimSpace(mod), "/cmd/skelet") {
				return Native
			}
		}
	}
	return None
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}
