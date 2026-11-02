package profile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Where a clone kept its answers before .skelet/profile, the two lists beside it being copies rm channel kept current
var oldFiles = []struct{ file, key string }{{".skelet-profile", ""}, {".skelet-apps", DockerKey}, {".skelet-tools", ToolsKey}}

// Migrate moves a clone's old root files into .skelet/profile once, the lists winning where they disagree with it
func Migrate(root string) (bool, error) {
	var found []string
	for _, l := range oldFiles {
		if _, err := os.Stat(filepath.Join(root, l.file)); err == nil {
			found = append(found, l.file)
		}
	}
	if len(found) == 0 {
		return false, nil
	}
	if _, err := os.Stat(Path(root)); errors.Is(err, fs.ErrNotExist) {
		p, keys, err := load(filepath.Join(root, oldFiles[0].file))
		if err != nil {
			return false, err
		}
		for _, l := range oldFiles[1:] {
			raw, err := os.ReadFile(filepath.Join(root, l.file))
			if err != nil {
				continue
			}
			if _, had := p[l.key]; !had {
				keys = append(keys, l.key)
			}
			p[l.key] = strings.Join(strings.Fields(string(raw)), "")
		}
		if err := Write(Path(root), p, keys); err != nil {
			return false, err
		}
	}
	for _, f := range found {
		if err := os.Remove(filepath.Join(root, f)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return true, err
		}
	}
	return true, nil
}
