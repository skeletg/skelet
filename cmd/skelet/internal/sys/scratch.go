package sys

import (
	"os"
	"path/filepath"
)

// Scratch is a new dir in the checkout's .skelet/tmp, so a rename into the tree stays on one filesystem and Docker Desktop can mount it
func Scratch(root, prefix string) (string, error) {
	abs, err := filepath.Abs(filepath.Join(root, ".skelet", "tmp"))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(abs, prefix)
}
