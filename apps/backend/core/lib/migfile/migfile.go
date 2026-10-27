// Package migfile reads dbmate migrations - one file per version, its halves under -- migrate:up and -- migrate:down markers - for the boot-time runner and the tests that run them by hand
package migfile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var namePattern = regexp.MustCompile(`^(\d+)_.+\.sql$`)

type File struct {
	Path          string
	Version       string
	Ups           []string
	Downs         []string
	NoTransaction bool
}

func (f File) Up() string { return strings.Join(f.Ups, "\n") }

// HasSQL is false while the up half is comments only, like the template `dbmate new` writes
func (f File) HasSQL() bool {
	return slices.ContainsFunc(f.Ups, func(s string) bool { return !Blank(s) })
}

func (f File) Down() string {
	downs := slices.Clone(f.Downs)
	slices.Reverse(downs)
	return strings.Join(downs, "\n")
}

func Blank(section string) bool {
	for line := range strings.SplitSeq(section, "\n") {
		if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "--") {
			return false
		}
	}
	return true
}

func Read(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	return parse(path, raw)
}

func List(dir string) ([]File, error) {
	return list(os.DirFS(dir), func(name string) string { return filepath.Join(dir, name) })
}

func ListFS(fsys fs.FS) ([]File, error) {
	return list(fsys, func(name string) string { return name })
}

func list(fsys fs.FS, path func(string) string) ([]File, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var out []File
	seen := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !namePattern.MatchString(e.Name()) {
			continue
		}
		raw, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		f, err := parse(path(e.Name()), raw)
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

func parse(path string, raw []byte) (File, error) {
	m := namePattern.FindStringSubmatch(filepath.Base(path))
	if m == nil {
		return File{}, fmt.Errorf("%s is not named <version>_<name>.sql", filepath.Base(path))
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
			if slices.Contains(strings.Fields(marker), "transaction:false") {
				f.NoTransaction = true
			}
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
