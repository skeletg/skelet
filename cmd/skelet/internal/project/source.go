// Package project makes a Skelet project: a checkout of one release that is yours from its first commit, with Skelet
// as the upstream it updates from
package project

import (
	"context"
	"slices"
	"strconv"
	"strings"
)

// Upstream is where releases come from unless --repo names a fork or a mirror
const Upstream = "https://github.com/skeletg/skelet.git"

// Source is how a project's files arrive. Git is the only one so far - a release served as images rather than source
// would be the second
type Source interface {
	// Latest is the newest release's tag
	Latest(ctx context.Context) (string, error)
	// Create puts the release tagged version into dir, which is missing or empty
	Create(ctx context.Context, version, dir string) error
}

// IsRelease reports whether tag names a release: vMAJOR.MINOR.PATCH, nothing before or after
func IsRelease(tag string) bool {
	_, ok := parse(tag)
	return ok
}

func parse(tag string) ([3]int, bool) {
	var v [3]int
	rest, ok := strings.CutPrefix(tag, "v")
	parts := strings.Split(rest, ".")
	if !ok || len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		if p == "" || (len(p) > 1 && p[0] == '0') || strings.Trim(p, "0123456789") != "" {
			return v, false
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// newest is the highest release among tags - a pre-release or a module tag like apps/backend/core/v0.1.0 never wins
func newest(tags []string) (string, bool) {
	var best string
	var bestV [3]int
	for _, t := range tags {
		if v, ok := parse(t); ok && (best == "" || slices.Compare(v[:], bestV[:]) > 0) {
			best, bestV = t, v
		}
	}
	return best, best != ""
}
