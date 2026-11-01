package doctor

import (
	"maps"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/changelog"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
)

// A shop on npm's packages keeps them until it's bumped, while the backend moves with every release merged in
func sharedPackages(r *report, root string) {
	release := changelog.Latest(root)
	header := false
	for _, s := range storefronts.List(root) {
		deps := storefronts.SharedDeps(root, s.Dir)
		if s.Template() || len(deps) == 0 {
			continue
		}
		if !header {
			r.section("storefront packages")
			header = true
		}
		names := slices.Sorted(maps.Keys(deps))
		var specs []string
		for _, spec := range deps {
			specs = append(specs, strings.TrimLeft(spec, "^~"))
		}
		slices.Sort(specs)
		specs = slices.Compact(specs)
		version := specs[0]
		switch {
		case len(specs) > 1:
			r.note("%s mixes @skelet/storefront-* versions (%s) - keep them on one release: %s", s.Name, strings.Join(specs, ", "), bump(s.Name, names, release))
		case strings.HasPrefix(version, "workspace:"):
			r.pass("%s links @skelet/storefront-* to packages/", s.Name)
		case release == "" || version == release:
			r.pass("%s takes @skelet/storefront-* %s from npm", s.Name, version)
		default:
			r.note("%s takes @skelet/storefront-* %s from npm, but this checkout is %s - its GraphQL may not match the backend: %s", s.Name, version, release, bump(s.Name, names, release))
		}
	}
}

func bump(shop string, names []string, release string) string {
	if release == "" {
		release = "<version>"
	}
	args := make([]string, len(names))
	for i, n := range names {
		args[i] = n + "@" + release
	}
	return "skelet pnpm " + shop + " add " + strings.Join(args, " ")
}
