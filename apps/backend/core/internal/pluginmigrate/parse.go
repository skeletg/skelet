package pluginmigrate

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
)

// Matches "0001_some_name.up.sql" and its .down.sql - the classic up/down pair naming
var migrationFilePattern = regexp.MustCompile(`^(\d+)_([a-zA-Z0-9_-]+)\.(up|down)\.sql$`)

// Walks a registration's FS, pairs up the .up.sql and .down.sql files, and returns them sorted by version ascending
// Unrecognised files are skipped silently, since plugins may keep a README.md or .gitkeep in the migrations dir
func loadMigrations(reg Registration) ([]Migration, error) {
	root := reg.Root
	if root == "" {
		root = "migrations"
	}
	entries, err := fs.ReadDir(reg.FS, root)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", root, err)
	}

	by := map[int]*Migration{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := migrationFilePattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		version, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("parse version from %s: %w", e.Name(), err)
		}
		entry, ok := by[version]
		if !ok {
			entry = &Migration{Version: version}
			by[version] = entry
		}
		full := path.Join(root, e.Name())
		body, err := fs.ReadFile(reg.FS, full)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", full, err)
		}
		if m[3] == "up" {
			if entry.UpPath != "" {
				return nil, fmt.Errorf("version %d has two .up.sql files: %s and %s", version, entry.UpPath, full)
			}
			// the up half names the migration - that's what `plugin migrate status` reports
			entry.Name, entry.UpSQL, entry.UpPath = m[2], string(body), full
			continue
		}
		if entry.DownPath != "" {
			return nil, fmt.Errorf("version %d has two .down.sql files: %s and %s", version, entry.DownPath, full)
		}
		entry.DownSQL, entry.DownPath = string(body), full
	}

	var out []Migration
	for v, p := range by {
		if p.UpPath == "" {
			return nil, fmt.Errorf("migration %d has %s but no .up.sql", v, p.DownPath)
		}
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })

	// Enforce no version gaps - a missing 0002 between 0001 and 0003 is the sign of a botched rebase
	for i := 1; i < len(out); i++ {
		if out[i].Version != out[i-1].Version+1 {
			return nil, fmt.Errorf("version gap between %04d and %04d (missing %d)",
				out[i-1].Version, out[i].Version, out[i-1].Version+1)
		}
	}
	if len(out) > 0 && out[0].Version != 1 {
		return nil, fmt.Errorf("first migration must be version 1, got %d", out[0].Version)
	}

	return out, nil
}
