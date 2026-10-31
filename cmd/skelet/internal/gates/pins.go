package gates

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const PinsAbout = `Each pin below lives in three to six files at once, and the comments around them only ask people to
keep them in step - CI disagreeing with a dev box is the cost. Fails when a pin has two versions, or
when its pattern finds none at all, which means the pattern or the files moved.`

// A pin: one dependency version, found by pattern across the files that state it
type pin struct {
	label, pattern string
	files          func(fileSet) []string
}

type fileSet struct{ dockerfiles, compose, workflows, packageJSONs []string }

var pins = []pin{
	{"postgres", `postgres:[0-9][0-9.]*(-alpine)?`, func(f fileSet) []string { return cat(f.compose, f.workflows) }},
	{"clickhouse", `clickhouse/clickhouse-server:[0-9][0-9.]*`, func(f fileSet) []string { return cat(f.compose, f.workflows) }},
	{"dbmate", `amacneil/dbmate/v2@v[0-9][0-9.]*`, func(f fileSet) []string { return cat(f.compose, f.workflows, f.dockerfiles) }},
	{"golangci", `golangci-lint@v[0-9][0-9.]*`, func(f fileSet) []string { return cat(f.workflows, []string{"cmd/skelet/internal/lint/lint.go"}) }},
	{"node", nodePattern, func(f fileSet) []string { return cat(f.workflows, f.dockerfiles) }},
	{"alpine", `alpine3?[:.][0-9][0-9.]*`, func(fileSet) []string { return []string{"apps/backend/core/Dockerfile.prod"} }},
	{"caddy", `caddy:[0-9][0-9.]*`, func(f fileSet) []string { return f.compose }},
	// packageManager plus the images' npm install -g - an image on another pnpm than the host resolves under other
	// defaults and writes a lockfile the host then rewrites
	{"pnpm", `pnpm@[0-9][0-9.]*`, func(f fileSet) []string { return cat(f.packageJSONs, f.dockerfiles) }},
}

const nodePattern = `(node:[0-9][0-9.]*(-alpine)?|node-version:[[:space:]]*[0-9][0-9.]*)`

func Pins(root string, _ []string) int { return pinCheck(root, os.Stdout) }

func pinCheck(root string, out io.Writer) int {
	r := &report{out: out}
	files := collect(root)
	r.section("version pins agree across files")
	for _, p := range pins {
		found := versionsOf(root, p.pattern, p.files(files))
		switch len(found) {
		case 0:
			r.bad("%-16s no pin found - did the pattern or the files move?", p.label)
		case 1:
			r.pass("%-16s %s", p.label, found[0])
		default:
			r.bad("%-16s %s", p.label, strings.Join(found, " "))
			for _, hit := range grep(root, p.pattern, p.files(files)) {
				fmt.Fprintf(out, "          %s\n", hit)
			}
		}
	}

	// .nvmrc carries no prefix to match on, so it's compared rather than scanned. The workflows read it directly
	// (node-version-file), so what this guards is the Dockerfiles agreeing with it
	r.section(".nvmrc matches the node pin")
	raw, _ := os.ReadFile(filepath.Join(root, ".nvmrc"))
	nvmrc := strings.Join(strings.Fields(string(raw)), "")
	nodePin := ""
	if v := versionsOf(root, nodePattern, cat(files.workflows, files.dockerfiles)); len(v) > 0 {
		nodePin = v[0]
	}
	if nvmrc == nodePin {
		r.pass("%-16s %s", ".nvmrc", nvmrc)
	} else {
		r.bad("%-16s .nvmrc says %s, the images and workflows say %s", ".nvmrc", nvmrc, nodePin)
	}

	// The toolchain that builds the images has to be the one every go.mod declares, and CI reads go.mod directly - so a
	// Dockerfile bump is the half that gets forgotten. The CLI's build image is read off go.mod itself
	r.section("golang images match go.mod")
	var gomods []string
	for _, g := range []string{"apps/*/go.mod", "apps/backend/*/go.mod", "cmd/*/go.mod"} {
		m, _ := filepath.Glob(filepath.Join(root, g))
		for _, p := range m {
			rel, _ := filepath.Rel(root, p)
			gomods = append(gomods, rel)
		}
	}
	modVersions := versionsOf(root, `^go [0-9][0-9.]*`, gomods)
	imageVersions := versionsOf(root, `FROM golang:[0-9][0-9.]*`, files.dockerfiles)
	if len(modVersions) == 1 && len(imageVersions) == 1 && modVersions[0] == imageVersions[0] {
		r.pass("%-16s %s", "go", modVersions[0])
	} else {
		r.bad("%-16s go.mod files say %s, golang images say %s", "go", strings.Join(modVersions, " "), strings.Join(imageVersions, " "))
		for _, hit := range append(grep(root, `^go [0-9][0-9.]*`, gomods), grep(root, `FROM golang:[0-9][0-9.]*`, files.dockerfiles)...) {
			fmt.Fprintf(out, "          %s\n", hit)
		}
	}

	fmt.Fprintln(out)
	if r.failed {
		fmt.Fprintln(out, "==> pin-check FAILED - make the listed sites agree, or update cmd/skelet/internal/gates/pins.go if a pin moved")
	} else {
		fmt.Fprintln(out, "==> pin-check OK")
	}
	return r.code()
}

// collect finds the files the pins live in. Dockerfiles and package.json files are globbed under apps/ to three
// levels, so a forked storefront is covered the day it lands rather than the day someone remembers this list
func collect(root string) fileSet {
	var f fileSet
	apps := filepath.Join(root, "apps")
	_ = filepath.WalkDir(apps, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(apps, p)
		depth := len(strings.Split(rel, string(filepath.Separator)))
		if d.IsDir() {
			if d.Name() == "node_modules" || (rel != "." && depth >= 3) {
				return filepath.SkipDir
			}
			return nil
		}
		path, _ := filepath.Rel(root, p)
		switch {
		case strings.HasPrefix(d.Name(), "Dockerfile"):
			f.dockerfiles = append(f.dockerfiles, path)
		case d.Name() == "package.json":
			f.packageJSONs = append(f.packageJSONs, path)
		}
		return nil
	})
	slices.Sort(f.dockerfiles)
	slices.Sort(f.packageJSONs)
	f.compose = []string{"docker-compose.yml", "docker-compose.small.yml", "docker-compose.prod.yml"}
	f.workflows, _ = filepath.Glob(filepath.Join(root, ".github", "workflows", "*.yml"))
	for i, w := range f.workflows {
		f.workflows[i], _ = filepath.Rel(root, w)
	}
	return f
}

// versionsOf is every version pattern finds, line by line and leftmost-longest the way grep -E matches, each
// normalised to a bare version - so the same pin written as an image tag, a go install path and a workflow input
// still compares equal. Sorted, without repeats
func versionsOf(root, pattern string, files []string) []string {
	re := regexp.MustCompilePOSIX(pattern)
	var out []string
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			continue
		}
		for line := range strings.SplitSeq(string(raw), "\n") {
			for _, m := range re.FindAllString(line, -1) {
				if v := normalize(m); !slices.Contains(out, v) {
					out = append(out, v)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}

// normalize is the bash gate's sed: drop everything to the last @ or :, then spaces and a v, then an -alpine suffix
// and an alpine prefix. A go.mod line has neither @ nor :, so it keeps its "go " - stripped here
func normalize(m string) string {
	if i := strings.LastIndexAny(m, "@:"); i >= 0 {
		m = strings.TrimPrefix(strings.TrimLeft(m[i+1:], " \t"), "v")
	}
	if i := strings.Index(m, "-alpine"); i >= 0 {
		m = m[:i]
	}
	return strings.TrimPrefix(strings.TrimPrefix(m, "alpine"), "go ")
}

// grep is grep -nE over the files: file:line:text for every line the pattern matches
func grep(root, pattern string, files []string) []string {
	re := regexp.MustCompilePOSIX(pattern)
	var out []string
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if re.MatchString(line) {
				out = append(out, fmt.Sprintf("%s:%d:%s", f, i+1, line))
			}
		}
	}
	return out
}

func cat(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}
