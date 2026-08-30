package cache

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/skeletg/skelet/internal/action"
)

// One source for both the select options and the flush targets - they were two hardcoded lists, so an option added to only one silently flushed nothing
var flushable = []struct{ Prefix, Label string }{
	{"channel", "Channel resolution"},
	{"chanpath", "Channel path lookups"},
	{"categorytree", "Category tree"},
	{"urlrewrite", "URL rewrites"},
	{"config", "Config values"},
	{"menu", "Menus"},
	{"frontresp", "Storefront response cache"},
}

func flushOptions() []action.ArgumentOption {
	opts := []action.ArgumentOption{{Value: "all", Label: "All namespaces"}}
	for _, f := range flushable {
		opts = append(opts, action.ArgumentOption{Value: f.Prefix, Label: f.Label})
	}
	return opts
}

func flushPrefixes() []string {
	out := make([]string, len(flushable))
	for i, f := range flushable {
		out[i] = f.Prefix
	}
	return out
}

// Dev-only - on a live store this is a foot-gun with no audit trail beyond the action log, and the per-service invalidation paths are the supported route
func RegisterActions(client *redis.Client, dev bool) {
	if !dev {
		return
	}
	action.Register(action.Definition{
		Slug:        "dev.flush_caches",
		Name:        "Flush Valkey caches",
		Description: "Drops cached read-through namespaces (channel resolution, channel paths, category tree, url rewrites, config, menus, storefront responses) so the next read rebuilds from Postgres. Use when hand-editing rows in psql or chasing stale-cache suspicions.",
		Group:       "Developer",
		Permission:  "config.manage",
		Arguments: []action.Argument{{
			Name:     "namespace",
			Label:    "Namespace",
			Type:     action.ArgSelect,
			Required: true,
			Default:  "all",
			Options:  flushOptions(),
		}},
		Handler: func(ctx context.Context, args action.Args) (action.Result, error) {
			ns, _ := args.String("namespace")
			targets := flushPrefixes()
			if ns != "" && ns != "all" {
				targets = []string{ns}
			}
			for _, prefix := range targets {
				New(client, prefix).DelPattern(ctx, "*")
			}
			return action.Result{Output: fmt.Sprintf("flushed: %s", strings.Join(targets, ", "))}, nil
		},
	})
}
