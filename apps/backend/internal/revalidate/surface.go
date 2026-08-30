// Package revalidate is the generic storefront cache-invalidation layer - NOT CMS-specific: any domain registers a cacheable "surface" and purges its tags through one Client, and the admin Cache page reflects every registered surface uniformly
package revalidate

import (
	"sync"

	"github.com/skeletg/skelet/internal/config"
	"github.com/skeletg/skelet/internal/permission"
)

// config.RegisterSection panics on a duplicate, but every surface lives under the one "cache" section - register it exactly once
var cacheSectionOnce sync.Once

func init() {
	permission.RegisterGroup(permission.Group{Slug: "cache", Name: "Cache", SortOrder: 90})
	permission.Register(permission.Definition{Group: "cache", Slug: "cache.view", Name: "View cache settings & activity", SortOrder: 1})
	permission.Register(permission.Definition{Group: "cache", Slug: "cache.manage", Name: "Purge cache & edit cache settings", SortOrder: 2})
}

// A cacheable storefront area - AllTag is the cache tag every cached fetch of that surface carries, so purging it busts the whole surface
type Surface struct {
	Key    string // stable id, e.g. "cms"
	Label  string // human label, e.g. "CMS pages"
	AllTag string // coarse tag, e.g. "cms:all"
	// Seeds the cache/<key>/enabled config - set false for a surface whose storefront caching isn't wired yet, so the admin doesn't show it as cached until it really is
	DefaultEnabled bool
}

var (
	registry = map[string]Surface{}
	order    []string
)

const (
	cacheSection = "cache"
)

func SurfaceConfigPaths(key string) (enabled, ttl string) {
	return cacheSection + "/" + key + "/enabled", cacheSection + "/" + key + "/ttl_seconds"
}

// Adds a surface to the registry and registers its config keys, so they appear in Settings and can be read publicly by the storefront - idempotent per key
func RegisterSurface(s Surface) {
	if _, exists := registry[s.Key]; !exists {
		order = append(order, s.Key)
	}
	registry[s.Key] = s

	cacheSectionOnce.Do(func() {
		config.RegisterSection(config.Section{Slug: cacheSection, Label: "Cache", SortOrder: 90})
	})
	config.RegisterGroup(config.Group{Section: cacheSection, Slug: s.Key, Label: s.Label, SortOrder: len(order)})
	enabledPath, ttlPath := SurfaceConfigPaths(s.Key)
	enabledDefault := "false"
	if s.DefaultEnabled {
		enabledDefault = "true"
	}
	config.Register(config.Definition{Path: enabledPath, Type: config.TypeBool, Default: enabledDefault, Label: s.Label + " - cached", Public: true})
	config.Register(config.Definition{Path: ttlPath, Type: config.TypeInt, Default: "3600", Label: s.Label + " - TTL (seconds)", Public: true})
}

func Surfaces() []Surface {
	out := make([]Surface, 0, len(order))
	for _, k := range order {
		out = append(out, registry[k])
	}
	return out
}

func Lookup(key string) (Surface, bool) {
	s, ok := registry[key]
	return s, ok
}
