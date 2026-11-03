package profile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
)

var oldFiles = []string{".skelet-profile", ".skelet-apps", ".skelet-tools"}

func Migrate(root string) (bool, error) {
	var found []string
	for _, f := range oldFiles {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			found = append(found, f)
		}
	}
	if len(found) == 0 {
		return false, nil
	}
	env := EnvPath(root)
	if _, ok, err := envfile.Get(env, EnvTarget); err != nil {
		return false, err
	} else if !ok {
		old, err := Load(filepath.Join(root, oldFiles[0]))
		if err != nil {
			return false, err
		}
		docker, hasDocker := listFile(root, oldFiles[1], old, DockerKey)
		tools, hasTools := listFile(root, oldFiles[2], old, ToolsKey)
		target := old[TargetKey]
		if target == "" {
			target = "dev"
		}
		set := [][2]string{{EnvTarget, target}}
		if apps, ok := old[AppsKey]; ok {
			set = append(set, [2]string{EnvApps, apps})
		}
		if _, ok, _ := envfile.Get(env, EnvProfiles); target == "dev" && !ok && (hasDocker || hasTools) {
			set = append(set, [2]string{EnvProfiles, DevProfiles(docker, tools, SignalsOn(root))})
		}
		for _, kv := range set {
			if err := envfile.Set(env, kv[0], kv[1]); err != nil {
				return false, err
			}
		}
	}
	for _, f := range found {
		if err := os.Remove(filepath.Join(root, f)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return true, err
		}
	}
	return true, nil
}

// The list file wins over the profile's copy of it - rm channel only ever edited the file
func listFile(root, name string, old Profile, key string) ([]string, bool) {
	if raw, err := os.ReadFile(filepath.Join(root, name)); err == nil {
		return split(strings.Join(strings.Fields(string(raw)), "")), true
	}
	return old.List(key)
}
