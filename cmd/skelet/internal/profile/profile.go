// Package profile reads and writes .skelet/profile, the answers setup was last given. It is parsed, never sourced, so
// a profile replayed with --profile can't run anything
package profile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
)

// Beside the CLI's own build cache, so a clone keeps one directory of Skelet state rather than files in its root
const File = ".skelet/profile"

const (
	AppsKey   = "SKELET_APPS_SELECTED"
	DockerKey = "SKELET_DOCKER_APPS"
	ToolsKey  = "SKELET_TOOLS_SELECTED"
	TargetKey = "SKELET_TARGET"
)

func Path(root string) string { return filepath.Join(root, filepath.FromSlash(File)) }

// Profile maps a key to its value. A key that is present but empty differs from one that is absent -
// SKELET_APPS_SELECTED= means backend only, a missing one means setup hasn't asked yet
type Profile map[string]string

// Load reads path; a missing file is an empty profile
func Load(path string) (Profile, error) {
	p, _, err := load(path)
	return p, err
}

func load(path string) (Profile, []string, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Profile{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	p := Profile{}
	var keys []string
	for line := range strings.Lines(string(raw)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			continue
		}
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		key = strings.TrimSpace(key)
		if _, seen := p[key]; !seen {
			keys = append(keys, key)
		}
		p[key] = val
	}
	return p, keys, nil
}

func (p Profile) Get(key string) (string, bool) {
	v, ok := p[key]
	return v, ok
}

func (p Profile) List(key string) ([]string, bool) {
	v, ok := p[key]
	var items []string
	for item := range strings.SplitSeq(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items, ok
}

func (p Profile) Apps() ([]string, bool) { return p.List(AppsKey) }

// Write puts keys in the order given, under a header saying where the file came from
func Write(path string, p Profile, keys []string) error {
	var b strings.Builder
	b.WriteString("# Written by skelet setup - your answers, so a re-run defaults to them\n")
	for _, k := range keys {
		b.WriteString(k + "=" + p[k] + "\n")
	}
	if old, err := os.ReadFile(path); err == nil && string(old) == b.String() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return envfile.WriteAtomic(path, []byte(b.String()), 0o644)
}

func Update(path string, change func(Profile)) error {
	p, keys, err := load(path)
	if err != nil {
		return err
	}
	change(p)
	var added []string
	for k := range p {
		if !slices.Contains(keys, k) {
			added = append(added, k)
		}
	}
	slices.Sort(added)
	return Write(path, p, append(keys, added...))
}
