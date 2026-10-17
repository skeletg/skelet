// Package migfile reads dbmate migrations - one file per version, its halves under -- migrate:up and -- migrate:down markers - for the tests and harnesses that run them by hand
package migfile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var namePattern = regexp.MustCompile(`^(\d+)_.+\.sql$`)

type File struct {
	Path    string
	Version string
	// one entry per section, in file order - ClickHouse takes a single statement per query, so its files keep one per section
	Ups   []string
	Downs []string
}

func (f File) Up() string { return strings.Join(f.Ups, "\n") }

// Down undoes the sections last first, the reverse of how they went up
func (f File) Down() string {
	downs := slices.Clone(f.Downs)
	slices.Reverse(downs)
	return strings.Join(downs, "\n")
}

func Read(path string) (File, error) {
	m := namePattern.FindStringSubmatch(filepath.Base(path))
	if m == nil {
		return File{}, fmt.Errorf("%s is not named <version>_<name>.sql", filepath.Base(path))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	f := File{Path: path, Version: m[1]}
	var cur *[]string
	var body []string
	flush := func() {
		if cur != nil {
			*cur = append(*cur, strings.TrimSpace(strings.Join(body, "\n")))
		}
		body = nil
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		switch marker := strings.TrimSpace(line); {
		case strings.HasPrefix(marker, "-- migrate:up"):
			flush()
			cur = &f.Ups
		case strings.HasPrefix(marker, "-- migrate:down"):
			flush()
			cur = &f.Downs
		default:
			body = append(body, line)
		}
	}
	flush()
	if len(f.Ups) == 0 {
		return File{}, fmt.Errorf("%s has no -- migrate:up section", filepath.Base(path))
	}
	return f, nil
}

// List reads every migration in dir in the order dbmate applies them, by file name
func List(dir string) ([]File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []File
	seen := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !namePattern.MatchString(e.Name()) {
			continue
		}
		f, err := Read(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		if prev, dup := seen[f.Version]; dup {
			return nil, fmt.Errorf("version %s is used by both %s and %s", f.Version, prev, e.Name())
		}
		seen[f.Version] = e.Name()
		out = append(out, f)
	}
	return out, nil
}
