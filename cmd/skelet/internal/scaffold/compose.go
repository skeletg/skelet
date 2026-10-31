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

// rename is a copied block made the new storefront's: every mention of the source's name and image variable, then
// the published port, whose pattern differs by compose file
func rename(blk []string, src, name string, port *regexp.Regexp, portTo string) []string {
	out := make([]string, len(blk))
	for i, l := range blk {
		l = strings.ReplaceAll(l, src, name)
		l = strings.ReplaceAll(l, imageVar(src), imageVar(name))
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
	alias    = regexp.MustCompile(`\*[a-zA-Z][a-zA-Z0-9_-]*`)
	xAnchors = regexp.MustCompile(`^x-[a-zA-Z0-9_-]+:\s*&([a-zA-Z0-9_-]+)\s*$`)
)

// anchors is the x- definitions the block's aliases need, out of the base file. Anchors don't cross compose files -
// each is parsed on its own - so a block carrying logging: *logging needs that definition beside it. Only those
// the override doesn't define already
func anchors(blk []string, base, override string) []string {
	var names []string
	for _, l := range blk {
		for _, m := range alias.FindAllString(l, -1) {
			names = append(names, m[1:])
		}
	}
	slices.Sort(names)
	var out []string
	for _, a := range slices.Compact(names) {
		if regexp.MustCompile(`&` + regexp.QuoteMeta(a) + `(\s|$)`).MatchString(override) {
			continue
		}
		in := false
		for _, l := range lines(base) {
			switch {
			case xAnchors.MatchString(l) && xAnchors.FindStringSubmatch(l)[1] == a:
				in = true
				out = append(out, l)
			case in && l != "" && !isSpace(l[0]):
				in = false
			case in:
				out = append(out, l)
			}
		}
	}
	return out
}

var (
	servicesKey = regexp.MustCompile(`^services:\s*$`)
	volumesKey  = regexp.MustCompile(`^volumes:\s*$`)
)

// override is the git-ignored override with the storefront and its two volumes added - created whole when there is
// none, else spliced into its sections, since YAML allows one services: and one volumes:. ok is false when an
// existing file has no services: to put it in
func override(existing *string, blk, anc []string, name string) (string, bool) {
	vols := []string{"  " + name + "-node-modules:", "  " + name + "-next-cache:"}
	if existing == nil {
		out := []string{
			"# Per-clone compose additions - git-ignored, auto-merged by docker compose.",
			"# Storefronts scaffolded by skelet add channel land here so a pull from upstream skelet",
			"# never conflicts with your own shops - see docs/workflow.md",
			"",
		}
		out = append(append(append(out, anc...), "services:"), blk...)
		return join(append(append(out, "", "volumes:"), vols...)), true
	}
	var out []string
	inServices, inVolumes, sawServices, sawVolumes := false, false, false, false
	for i, l := range lines(*existing) {
		if i == 0 {
			out = append(out, anc...)
		}
		switch {
		case servicesKey.MatchString(l):
			// A volumes: section above it ends here too, or its new lines would land under services: - they go in
			// before the blank lines that set the two apart
			if inVolumes {
				end := len(out)
				for end > 0 && strings.TrimSpace(out[end-1]) == "" {
					end--
				}
				out, inVolumes = append(append(slices.Clone(out[:end]), vols...), out[end:]...), false
			}
			inServices, sawServices = true, true
		case volumesKey.MatchString(l):
			if inServices {
				out, inServices = append(out, blk...), false
			}
			inVolumes, sawVolumes = true, true
		// Any other key at the margin closes whichever section was open - a comment there doesn't
		case l != "" && !isSpace(l[0]) && l[0] != '#':
			if inServices {
				out, inServices = append(out, blk...), false
			}
			if inVolumes {
				out, inVolumes = append(out, vols...), false
			}
		}
		out = append(out, l)
	}
	if inServices {
		out = append(out, blk...)
	}
	if inVolumes {
		out = append(out, vols...)
	}
	if !sawVolumes {
		out = append(append(out, "", "volumes:"), vols...)
	}
	return join(out), sawServices
}

// addVolumes declares the new storefront's two volumes after the source's in a tracked dev file
func addVolumes(text, src, name string) string {
	var out []string
	prefix := "  " + src + "-next-cache:"
	for _, l := range lines(text) {
		if rest, ok := strings.CutPrefix(l, prefix); ok {
			out = append(out, prefix, "  "+name+"-node-modules:", "  "+name+"-next-cache:"+rest)
			continue
		}
		out = append(out, l)
	}
	return join(out)
}

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
