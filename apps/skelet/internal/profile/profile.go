// Package profile reads and writes .skelet-profile, the answers setup was last given. It is parsed, never sourced, so
// a profile replayed with --profile can't run anything
package profile

import (
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/skeletg/skelet/apps/skelet/internal/envfile"
)

const File = ".skelet-profile"

// Profile maps a key to its value. A key that is present but empty differs from one that is absent -
// SKELET_APPS_SELECTED= means backend only, a missing one means setup hasn't asked yet
type Profile map[string]string

// Load reads path; a missing file is an empty profile
func Load(path string) (Profile, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Profile{}, nil
	}
	if err != nil {
		return nil, err
	}
	p := Profile{}
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
		p[strings.TrimSpace(key)] = val
	}
	return p, nil
}

func (p Profile) Get(key string) (string, bool) {
	v, ok := p[key]
	return v, ok
}

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
	return envfile.WriteAtomic(path, []byte(b.String()), 0o644)
}
