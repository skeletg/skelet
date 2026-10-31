// Package plugins edits plugins.json, the decision file of which plugins compile in. It writes exactly what core's
// pluginset writes, and setup only merges its gateway picks into it - every other decision is left as it was
package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/skeletg/skelet/apps/skelet/internal/envfile"
)

const (
	Dir  = "apps/backend/core/plugins"
	File = Dir + "/plugins.json"
)

// Entry mirrors pluginset.Entry - Module set means the plugin ships as a Go module, not a directory here
type Entry struct {
	Enabled bool   `json:"enabled"`
	Module  string `json:"module,omitempty"`
}

// Accepts the old bare 0 or 1 as well as the object, like pluginset does - and like it, always writes the object
func (e *Entry) UnmarshalJSON(raw []byte) error {
	var flag int
	if err := json.Unmarshal(raw, &flag); err == nil {
		e.Enabled = flag == 1
		return nil
	}
	type entry Entry
	var v entry
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	*e = Entry(v)
	return nil
}

type Set struct {
	Plugins map[string]Entry `json:"plugins"`
}

// Load returns the set and the file's bytes as they were; a missing file is an empty set
func Load(root string) (*Set, []byte, error) {
	raw, err := os.ReadFile(filepath.Join(root, File))
	if errors.Is(err, fs.ErrNotExist) {
		return &Set{Plugins: map[string]Entry{}}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var s Set
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", File, err)
	}
	if s.Plugins == nil {
		s.Plugins = map[string]Entry{}
	}
	return &s, raw, nil
}

// Marshal is pluginset.Save's encoding: sorted keys, two-space indent, a trailing newline
func (s *Set) Marshal() ([]byte, error) {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// Decide records the gateway picks. Every discovered gateway is on exactly when picked, any other plugin found here
// without a decision gets one (off) since codegen refuses to run without it, and entries named like a parked
// directory are dropped - the go tool ignores _ and . directories, and pluginset rejects those names
func (s *Set) Decide(found, picked []string) (dropped []string) {
	for slug := range s.Plugins {
		if strings.HasPrefix(slug, "_") || strings.HasPrefix(slug, ".") {
			delete(s.Plugins, slug)
			dropped = append(dropped, slug)
		}
	}
	sort.Strings(dropped)
	for _, slug := range found {
		e, decided := s.Plugins[slug]
		switch {
		case IsGateway(slug):
			e.Enabled = slices.Contains(picked, slug)
		case decided:
			continue
		}
		s.Plugins[slug] = e
	}
	return dropped
}

func IsGateway(slug string) bool { return strings.HasPrefix(slug, "payment-") }

// Save writes data when it differs from what the file holds, reporting whether it did
func Save(root string, data, before []byte) (bool, error) {
	if before != nil && bytes.Equal(before, data) {
		return false, nil
	}
	return true, envfile.WriteAtomic(filepath.Join(root, File), data, 0o644)
}

// Discover is pluginset.Discover: a directory counts when one of its non-test Go files registers a plugin, and
// _ and . directories are parked out of the build
func Discover(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, Dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var found []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		files, err := filepath.Glob(filepath.Join(root, Dir, e.Name(), "*.go"))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			if bytes.Contains(src, []byte("sdk.RegisterPlugin(")) {
				found = append(found, e.Name())
				break
			}
		}
	}
	sort.Strings(found)
	return found, nil
}
