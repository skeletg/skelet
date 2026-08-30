package pluginmigrate

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
)

// Matches "0001_some_name.up.sql" and its .down.sql - mirrors golang-migrate's naming so existing muscle memory transfers
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

	type partial struct {
		Migration
		hasUp, hasDown bool
	}
	by := map[int]*partial{}
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
			entry = &partial{Version: version, Name: m[2]}
			by[version] = entry
		}
		full := path.Join(root, e.Name())
		switch m[3] {
		case "up":
			body, err := fs.ReadFile(reg.FS, full)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", full, err)
			}
			entry.UpSQL = string(body)
			entry.UpPath = full
			entry.hasUp = true
		case "down":
			body, err := fs.ReadFile(reg.FS, full)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", full, err)
			}
			entry.DownSQL = string(body)
			entry.DownPath = full
			entry.hasDown = true
		}
		// Names may differ between the up and down halves - keep the up version's, since that's what `bin/cli plugin-migrate status` reports
		if entry.Name == "" || (m[3] == "up" && entry.Name != m[2]) {
			entry.Name = m[2]
		}
	}

	var out []Migration
	for v, p := range by {
		if !p.hasUp {
			// A down without a paired up - skip for now, since the runner shouldn't half-apply a migration whose forward half is missing
			return nil, fmt.Errorf("migration %d has %s but no .up.sql", v, p.DownPath)
		}
		out = append(out, p.Migration)
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
