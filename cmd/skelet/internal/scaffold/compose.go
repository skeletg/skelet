// Package scaffold is skelet add channel and skelet rm channel: forking a storefront into the compose files and taking
// one back out. The compose files are edited as text, a line at a time, so every comment and blank line someone wrote
// stays where it was - a YAML round trip would rewrite the whole file
package scaffold

import (
	"regexp"
	"slices"
	"strings"
)

// lines splits a file the way awk reads it: a final newline ends the last line rather than starting another
func lines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// join is lines written back, every one ending in a newline
func join(ls []string) string {
	if len(ls) == 0 {
		return ""
	}
	return strings.Join(ls, "\n") + "\n"
}

// declares says whether a compose file has a line starting the service
func declares(text, service string) bool {
	return slices.ContainsFunc(lines(text), func(l string) bool { return strings.HasPrefix(l, "  "+service+":") })
}

// endsBlock is the line after a service block: the next thing at two-space indent - a sibling, or a comment there - or
// anything at the margin, since the last service in a file is followed by volumes:, not by another name
func endsBlock(l string) bool {
	return (len(l) > 2 && l[:2] == "  " && l[2] != ' ') || (l != "" && !isSpace(l[0]))
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

// block is a service's lines, from its `  <name>:` line to the line that ends it - blank lines before the next
// sibling included
func block(text, service string) []string {
	var out []string
	in := false
	for _, l := range lines(text) {
		switch {
		case l == "  "+service+":":
			in = true
			out = append(out, l)
		case in && endsBlock(l):
			in = false
		case in:
			out = append(out, l)
		}
	}
	return out
}

// move is a storefront becoming another: from a template or a storefront of the project's own, into apps/channels
type move struct {
	srcDir, src, dir, name string
}

func newMove(srcDir, src, name string) move {
	return move{srcDir: srcDir, src: src, dir: "apps/channels/" + name, name: name}
}

func (m move) apply(s string) string { return m.replace(s, volumeSuffixes) }

func (m move) applyFile(s string) string { return m.replace(s, nil) }

var (
	namePrefixes   = []string{"skelet-", "prod-", "small-"}
	volumeSuffixes = []string{"-node-modules", "-next-cache"}
)

func (m move) replace(s string, suffixes []string) string {
	s = whole(s, m.srcDir, m.dir, nil, nil)
	s = whole(s, imageVar(m.src), imageVar(m.name), nil, nil)
	return whole(s, m.src, m.name, namePrefixes, suffixes)
}

// whole replaces old only where no name character touches it - but for a prefix ending right before or a suffix starting right after
func whole(s, old, new string, prefixes, suffixes []string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, old)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		end := i + len(old)
		before := i == 0 || !nameChar(s[i-1]) || slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasSuffix(s[:i], p) })
		after := end == len(s) || !nameChar(s[end]) || slices.ContainsFunc(suffixes, func(p string) bool { return strings.HasPrefix(s[end:], p) })
		if before && after {
			b.WriteString(s[:i] + new)
			s = s[end:]
			continue
		}
		b.WriteString(s[:i+1])
		s = s[i+1:]
	}
}

func nameChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// rename is a copied block made the new storefront's, then the published port, whose pattern differs by compose file
func rename(blk []string, m move, port *regexp.Regexp, portTo string) []string {
	out := make([]string, len(blk))
	for i, l := range blk {
		l = m.apply(l)
		if port != nil {
			if loc := port.FindStringIndex(l); loc != nil {
				l = l[:loc[0]] + portTo + l[loc[1]:]
			}
		}
		out[i] = l
	}
	return out
}

// imageVar is the *_IMAGE variable a storefront's prod image is read from
func imageVar(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_IMAGE"
}

// spliceAfter puts add after the source's block, then a blank line, keeping related services together
func spliceAfter(text, src string, add []string) string {
	var out []string
	in := false
	for _, l := range lines(text) {
		switch {
		case l == "  "+src+":":
			in = true
		case in && endsBlock(l):
			in = false
			out = append(append(out, add...), "")
		}
		out = append(out, l)
	}
	if in {
		out = append(out, add...)
	}
	return join(out)
}

var (
	servicesKey = regexp.MustCompile(`^services:\s*$`)
)

// strip takes a service's block and its two volume declarations out of a compose file
func strip(text, name string) string {
	var out []string
	in := false
	for _, l := range lines(text) {
		switch {
		case l == "  "+name+":":
			in = true
			continue
		case in && endsBlock(l):
			in = false
		case in:
			continue
		}
		if l == "  "+name+"-node-modules:" || l == "  "+name+"-next-cache:" {
			continue
		}
		out = append(out, l)
	}
	return join(out)
}

// hasServices says an override still declares a service - with none, its services: is empty, which compose rejects
func hasServices(text string) bool {
	in := false
	for _, l := range lines(text) {
		switch {
		case servicesKey.MatchString(l):
			in = true
		case in && l != "" && !isSpace(l[0]):
			in = false
		case in && len(l) > 2 && l[:2] == "  " && l[2] >= 'a' && l[2] <= 'z':
			return true
		}
	}
	return false
}
