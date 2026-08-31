// Package pluginset owns which plugins a build compiles in - plugins/plugins.json is the decision file,
// and plugins/firstparty is generated from it
//
// Two kinds of plugin, differing only in where the source lives. An IN-TREE plugin sits at plugins/<slug>/
// and is part of this module; a MODULE plugin is a Go module dependency named by its import path, fetched
// by `go get` and living read-only in the module cache. Everything downstream of the import path is
// identical, which is the point
//
// The scan discovers, the file decides: a plugin directory appearing in the tree never enables itself, because a core
// upgrade that adds a payment gateway must not silently compile it into someone's binary
package pluginset

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	modulePath = "github.com/skeletg/skelet/apps/backend"
	pluginsDir = "plugins"
	// StateFile is hand-edited (or edited through `bin/cli plugin enable|disable`), GenFile never is
	StateFile = "plugins/plugins.json"
	GenFile   = "plugins/firstparty/firstparty.go"
	// StageDir holds what a module plugin ships beside its Go code - its GraphQL SDL - copied out of the
	// read-only module cache so codegen reaches it by a stable relative path. Derived, git-ignored
	StageDir = "plugins/.modules"
)

// Entry is one plugin's decision. Module empty means in-tree at plugins/<slug>/; set, it names the Go
// module the plugin ships as, and go.mod holds the version that actually gets built
type Entry struct {
	Enabled bool   `json:"enabled"`
	Module  string `json:"module,omitempty"`
}

// Accepts the old shape (a bare 0 or 1) as well as the object, so a tree written before module plugins
// existed still loads - Save always writes the object form
func (e *Entry) UnmarshalJSON(raw []byte) error {
	var flag int
	if err := json.Unmarshal(raw, &flag); err == nil {
		e.Enabled = flag == 1
		return nil
	}
	type entry Entry // shed the method, or this recurses
	var v entry
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	*e = Entry(v)
	return nil
}

// Set is the decision file - enabled compiles the plugin in, disabled leaves it out of the build
type Set struct {
	Plugins map[string]Entry `json:"plugins"`
}

func (s *Set) Enabled(slug string) bool { return s.Plugins[slug].Enabled }

// ImportPath is what firstparty blank-imports: the module path for a module plugin, this module's own
// plugins/<slug> for an in-tree one
func (s *Set) ImportPath(slug string) string {
	if m := s.Plugins[slug].Module; m != "" {
		return m
	}
	return modulePath + "/" + pluginsDir + "/" + slug
}

// InTree reports whether the slug's source lives in this repo rather than the module cache - the
// directory-shaped checks (Discover, Park) only apply to those
func (s *Set) InTree(slug string) bool { return s.Plugins[slug].Module == "" }

// ModuleSlugs is the enabled subset needing staging before codegen can see their SDL
func (s *Set) ModuleSlugs() []string {
	var out []string
	for _, slug := range s.EnabledSlugs() {
		if !s.InTree(slug) {
			out = append(out, slug)
		}
	}
	return out
}

// EnabledSlugs is what the generator writes and what the SDL list should eventually read
func (s *Set) EnabledSlugs() []string {
	out := make([]string, 0, len(s.Plugins))
	for slug, e := range s.Plugins {
		if e.Enabled {
			out = append(out, slug)
		}
	}
	sort.Strings(out)
	return out
}

// Set flips a plugin on or off, keeping whatever module path it already had
func (s *Set) Set(slug string, on bool) {
	if s.Plugins == nil {
		s.Plugins = map[string]Entry{}
	}
	e := s.Plugins[slug]
	e.Enabled = on
	s.Plugins[slug] = e
}

// SetModule records a plugin that ships as a Go module, enabled
func (s *Set) SetModule(slug, module string) {
	if s.Plugins == nil {
		s.Plugins = map[string]Entry{}
	}
	s.Plugins[slug] = Entry{Enabled: true, Module: module}
}

func Load(root string) (*Set, error) {
	raw, err := os.ReadFile(filepath.Join(root, StateFile))
	if err != nil {
		return nil, err
	}
	var s Set
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", StateFile, err)
	}
	if s.Plugins == nil {
		s.Plugins = map[string]Entry{}
	}
	return &s, nil
}

// Marshal sorts map keys, so the file stays diff-stable across writes
func (s *Set) Save(root string) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, StateFile), append(raw, '\n'), 0o644)
}

// Discover finds every directory under plugins/ that actually registers - structural, so plugins/firstparty and any
// other helper package in there is never mistaken for a plugin
func Discover(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, pluginsDir))
	if err != nil {
		return nil, err
	}
	var found []string
	for _, e := range entries {
		// The go tool ignores directories beginning with _ or ., so a plugin parked under one is out of the build
		// entirely - that is how a plugin whose resolvers reference core's generated models gets disabled at all
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		files, err := filepath.Glob(filepath.Join(root, pluginsDir, e.Name(), "*.go"))
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

// Generate rewrites GenFile from StateFile - pure, so running it twice changes nothing and never edits the decision file
func Generate(root string) error {
	set, err := Load(root)
	if err != nil {
		return err
	}
	found, err := Discover(root)
	if err != nil {
		return err
	}

	var undeclared []string
	for _, slug := range found {
		if _, ok := set.Plugins[slug]; !ok {
			undeclared = append(undeclared, slug)
		}
	}
	if len(undeclared) > 0 {
		return fmt.Errorf("%s does not mention %s - run `bin/cli plugin enable|disable <slug>` to decide",
			StateFile, strings.Join(undeclared, ", "))
	}

	present := map[string]bool{}
	for _, slug := range found {
		present[slug] = true
	}
	for _, slug := range set.EnabledSlugs() {
		// A module plugin has no directory to find - go build is what proves it resolves, and it says
		// so far more clearly than a guess here could
		if !set.InTree(slug) {
			continue
		}
		if !present[slug] {
			return fmt.Errorf("%s enables %q, but plugins/%s registers no plugin", StateFile, slug, slug)
		}
	}

	paths := make([]string, 0, len(set.EnabledSlugs()))
	for _, slug := range set.EnabledSlugs() {
		paths = append(paths, set.ImportPath(slug))
	}
	src, err := render(paths)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, GenFile), src, 0o644)
}

func render(importPaths []string) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by cmd/plugingen from plugins/plugins.json. DO NOT EDIT.\n\n")
	b.WriteString("// Package firstparty compiles the enabled plugins into the binary - each import runs that\n")
	b.WriteString("// plugin's init(), which registers with the SDK for the host to replay during boot\n")
	b.WriteString("//\n")
	b.WriteString("// To add or drop one, edit plugins/plugins.json - `bin/cli plugin enable|disable <slug>` flips the\n")
	b.WriteString("// 1/0 and regenerates this file, and it is the only core file an installation ever edits\n")
	b.WriteString("package firstparty\n")
	if len(importPaths) > 0 {
		b.WriteString("\nimport (\n")
		for _, p := range importPaths {
			fmt.Fprintf(&b, "\t_ %q\n", p)
		}
		b.WriteString(")\n")
	}
	return format.Source(b.Bytes())
}

// Park moves a disabled plugin under an underscore, where the go tool ignores it entirely, and back again on
// enable - the only way to disable a plugin whose resolvers return core's generated model types, because
// go build ./... compiles every package in the module whether the binary links it or not
func Park(root, slug string, enabled bool) error {
	live := filepath.Join(root, pluginsDir, slug)
	parked := filepath.Join(root, pluginsDir, "_"+slug)
	from, to := live, parked
	if enabled {
		from, to = parked, live
	}
	if _, err := os.Stat(from); err != nil {
		return nil // already where it belongs
	}
	if _, err := os.Stat(to); err == nil {
		return fmt.Errorf("both %s and %s exist - resolve that by hand", live, parked)
	}
	return os.Rename(from, to)
}

// Parked reports the plugins sitting under an underscore, which Discover cannot see
func Parked(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, pluginsDir))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "_") {
			out = append(out, strings.TrimPrefix(e.Name(), "_"))
		}
	}
	sort.Strings(out)
	return out, nil
}
