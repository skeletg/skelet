// Package changelog is skelet changelog: one file per release under changelog/, CHANGELOG.md as their index, and
// pending entries as fragments in changelog/unreleased/ so two branches never edit the same lines
package changelog

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const About = `The changelog - one file per release under changelog/, CHANGELOG.md as their index, and pending
entries as fragments in changelog/unreleased/, so two branches never edit the same lines.

  skelet changelog                           the pending entries, assembled as the next release would print them
  skelet changelog check                     fragments are well-formed and the index matches the release files
  skelet changelog release <version> [date]  write changelog/<version>.md, list it, delete the fragments

A fragment is changelog/unreleased/<slug>.<section>.md holding one or more "- " entries, where section is
added, changed, deprecated, removed, fixed or security - or summary, a paragraph that opens the release.`

const (
	dir      = "changelog/unreleased"
	releases = "changelog"
	index    = "CHANGELOG.md"
	repo     = "https://github.com/skeletg/skelet"
)

// Keep a Changelog's order, which is the order a release prints them in
var sections = []string{"added", "changed", "deprecated", "removed", "fixed", "security"}

var (
	fragmentName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.[a-z]+\.md$`)
	listedLine   = regexp.MustCompile(`^- \[([0-9]+\.[0-9]+\.[0-9]+)\]\(changelog/[0-9.]+\.md\)`)
	versionShape = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

func Run(root string, args []string) int {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "", "preview":
		if out := assemble(root); out != "" {
			fmt.Println(out)
		} else {
			fmt.Printf("(no pending entries in %s/)\n", dir)
		}
		return 0
	case "check":
		fmt.Println("==> changelog")
		return Check(root, os.Stdout)
	case "release":
		return release(root, args[1:])
	case "-h", "--help":
		fmt.Println(About)
		return 0
	}
	fmt.Fprintln(os.Stderr, "skelet changelog [preview | check | release <version> [date]]")
	return 2
}

// fragments is a section's files, sorted
func fragments(root, section string) []string {
	m, _ := filepath.Glob(filepath.Join(root, dir, "*."+section+".md"))
	var out []string
	for _, p := range m {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

// trimmed is a fragment without its leading empty lines or its trailing blank ones
func trimmed(path string) string {
	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// assemble is the next release's body: summaries first, then each section with entries under its heading
func assemble(root string) string {
	var b strings.Builder
	for _, f := range fragments(root, "summary") {
		b.WriteString(trimmed(f) + "\n\n")
	}
	for _, s := range sections {
		fs := fragments(root, s)
		if len(fs) == 0 {
			continue
		}
		b.WriteString("### " + strings.ToUpper(s[:1]) + s[1:] + "\n\n")
		for _, f := range fs {
			b.WriteString(trimmed(f) + "\n\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func Latest(root string) string {
	if versions := listed(root); len(versions) > 0 {
		return versions[0]
	}
	return ""
}

// listed is the versions CHANGELOG.md lists under Releases, newest first as the index holds them
func listed(root string) []string {
	raw, _ := os.ReadFile(filepath.Join(root, index))
	var out []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		if m := listedLine.FindStringSubmatch(line); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}

// Check holds the fragments, the index and the release files to each other
func Check(root string, out io.Writer) int {
	failed := false
	bad := func(format string, a ...any) {
		failed = true
		fmt.Fprintf(out, "  FAIL  "+format+"\n", a...)
	}
	entries, _ := os.ReadDir(filepath.Join(root, dir))
	for _, e := range entries {
		base := e.Name()
		f := dir + "/" + base
		if base == "README.md" {
			continue
		}
		if !fragmentName.MatchString(base) {
			bad("%s - name it <slug>.<section>.md, e.g. filesystem-storage.added.md", f)
			continue
		}
		sec := strings.TrimSuffix(base, ".md")
		sec = sec[strings.LastIndex(sec, ".")+1:]
		if sec != "summary" && !slices.Contains(sections, sec) {
			bad("%s - \"%s\" is not one of: %s summary", f, sec, strings.Join(sections, " "))
			continue
		}
		raw, _ := os.ReadFile(filepath.Join(root, f))
		lines := strings.Split(string(raw), "\n")
		first := ""
		for _, l := range lines {
			if l != "" {
				first = l
				break
			}
		}
		switch {
		case slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, "#") }):
			bad("%s - no headings, the section comes from the file name", f)
		case sec != "summary" && !strings.HasPrefix(first, "- "):
			bad("%s - must start with a \"- \" entry", f)
		}
	}
	// Anything written straight into the index brings back the merge conflicts fragments exist to remove
	if underUnreleased(root) {
		bad("%s has entries under [Unreleased] - move them into %s/", index, dir)
	}
	have := listed(root)
	files, _ := filepath.Glob(filepath.Join(root, releases, "*.md"))
	slices.Sort(files)
	for _, f := range files {
		if v := strings.TrimSuffix(filepath.Base(f), ".md"); !slices.Contains(have, v) {
			bad("%s/%s.md is not listed under Releases in %s", releases, v, index)
		}
	}
	for _, v := range have {
		if _, err := os.Stat(filepath.Join(root, releases, v+".md")); err != nil {
			bad("%s lists %s, but %s/%s.md does not exist", index, v, releases, v)
		}
	}
	if failed {
		return 1
	}
	fmt.Fprintf(out, "  ok    %d fragments well-formed, %d releases listed, Unreleased empty\n", countFragments(root), len(have))
	return 0
}

func underUnreleased(root string) bool {
	raw, _ := os.ReadFile(filepath.Join(root, index))
	on := false
	for line := range strings.SplitSeq(string(raw), "\n") {
		switch {
		case strings.HasPrefix(line, "## [Unreleased]"):
			on = true
		case strings.HasPrefix(line, "## "):
			on = false
		case on && (strings.HasPrefix(line, "### ") || strings.HasPrefix(line, "- ")):
			return true
		}
	}
	return false
}

// countFragments counts every .md under the fragments directory but its README, subdirectories included
func countFragments(root string) int {
	n := 0
	_ = filepath.WalkDir(filepath.Join(root, dir), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".md") && d.Name() != "README.md" {
			n++
		}
		return nil
	})
	return n
}

func release(root string, args []string) int {
	version, date := "", time.Now().Format("2006-01-02")
	if len(args) > 0 {
		version = args[0]
	}
	if len(args) > 1 {
		date = args[1]
	}
	if !versionShape.MatchString(version) {
		fmt.Fprintln(os.Stderr, "skelet changelog release <x.y.z> [YYYY-MM-DD]")
		return 2
	}
	out := filepath.Join(releases, version+".md")
	if _, err := os.Stat(filepath.Join(root, out)); err == nil {
		fmt.Fprintf(os.Stderr, "%s already exists\n", out)
		return 1
	}
	if Check(root, io.Discard) != 0 {
		Check(root, os.Stdout)
		return 1
	}
	body := assemble(root)
	if body == "" {
		fmt.Fprintf(os.Stderr, "no fragments in %s/ - nothing to release\n", dir)
		return 1
	}
	link := fmt.Sprintf("[%s]: %s/releases/tag/v%s\n", version, repo, version)
	if prev := listed(root); len(prev) > 0 {
		link = fmt.Sprintf("[%s]: %s/compare/v%s...v%s\n", version, repo, prev[0], version)
	}
	text := fmt.Sprintf("# [%s] - %s\n\n%s\n\n%s", version, date, body, link)
	if err := os.WriteFile(filepath.Join(root, out), []byte(text), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := relist(root, version, date); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// Staged when this is a git checkout - the file is written either way
	_ = git(root, "add", out)
	for _, s := range append([]string{"summary"}, sections...) {
		for _, f := range fragments(root, s) {
			rel, _ := filepath.Rel(root, f)
			if git(root, "rm", "-q", rel) != nil {
				_ = os.Remove(f)
			}
		}
	}
	fmt.Printf("%s written and listed in %s, fragments removed - review the diff, then commit\n", out, index)
	return 0
}

// relist adds the release under the line after "## Releases" and points the Unreleased link at it
func relist(root, version, date string) error {
	path := filepath.Join(root, index)
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(raw), "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		switch {
		case strings.HasPrefix(l, "## Releases"):
			out = append(out, l)
			if i+1 < len(lines) {
				i++
				out = append(out, lines[i])
			}
			out = append(out, fmt.Sprintf("- [%s](changelog/%s.md) - %s", version, version, date))
		case strings.HasPrefix(l, "[Unreleased]: "):
			out = append(out, fmt.Sprintf("[Unreleased]: %s/compare/v%s...develop", repo, version))
		default:
			out = append(out, l)
		}
	}
	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
}

func git(root string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	return cmd.Run()
}
