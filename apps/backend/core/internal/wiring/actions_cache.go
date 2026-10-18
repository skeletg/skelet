package wiring

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/skeletg/skelet/apps/backend/core/internal/adminaction"
	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
)

// The read-through namespaces core keeps in Valkey - one list for both the select options and the flush targets
var flushableCaches = []struct{ Prefix, Label string }{
	{"channel", "Channel resolution"},
	{"chanpath", "Channel path lookups"},
	{"categorytree", "Category tree"},
	{"urlrewrite", "URL rewrites"},
	{"config", "Config values"},
	{"menu", "Menus"},
	{"frontresp", "Storefront response cache"},
}

func flushOptions() []adminaction.ArgumentOption {
	opts := []adminaction.ArgumentOption{{Value: "all", Label: "All namespaces"}}
	for _, f := range flushableCaches {
		opts = append(opts, adminaction.ArgumentOption{Value: f.Prefix, Label: f.Label})
	}
	return opts
}

func flushPrefixes() []string {
	out := make([]string, len(flushableCaches))
	for i, f := range flushableCaches {
		out[i] = f.Prefix
	}
	return out
}

// Dev-only - on a live store this is a foot-gun with no audit trail beyond the action log, and the per-service invalidation paths are the supported route
func registerCacheActions(client *redis.Client, dev bool) {
	if !dev {
		return
	}
	adminaction.Register(adminaction.Definition{
		Slug:        "dev.flush_caches",
		Name:        "Flush Valkey caches",
		Description: "Drops cached read-through namespaces (channel resolution, channel paths, category tree, url rewrites, config, menus, storefront responses) so the next read rebuilds from Postgres. Use when hand-editing rows in psql or chasing stale-cache suspicions.",
		Group:       "Developer",
		Permission:  "config.manage",
		Arguments: []adminaction.Argument{{
			Name:     "namespace",
			Label:    "Namespace",
			Type:     adminaction.ArgSelect,
			Required: true,
			Default:  "all",
			Options:  flushOptions(),
		}},
		Handler: func(ctx context.Context, args adminaction.Args) (adminaction.Result, error) {
			ns, _ := args.String("namespace")
			targets := flushPrefixes()
			if ns != "" && ns != "all" {
				targets = []string{ns}
			}
			for _, prefix := range targets {
				cache.New(client, prefix).Flush(ctx)
			}
			return adminaction.Result{Output: fmt.Sprintf("flushed: %s", strings.Join(targets, ", "))}, nil
		},
	})
}
