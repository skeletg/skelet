// Package checkout locates the Skelet checkout a command acts on
package checkout

import (
	"os"
	"path/filepath"
)

// Find walks up from dir to the first directory holding bin/_lib and skelet - the checkout you stand in wins, the
// same rule the shim applies, so an installed binary acts on the repo you're in
func Find(dir string) (string, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for d := abs; d != string(filepath.Separator); d = filepath.Dir(d) {
		if isFile(filepath.Join(d, "bin", "_lib")) && isFile(filepath.Join(d, "skelet")) {
			return d, true
		}
	}
	return "", false
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}
