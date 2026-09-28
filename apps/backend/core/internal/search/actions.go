package search

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/adminaction"
)

// Called from main.go after the search tier exists so the handler closes over the live registry - which by then holds the core indexes plus
// whatever plugins added search.reindex mirrors the CLI command so operators can run it from /tools without shell access
func RegisterActions(registry *IndexRegistry) {
	// Options are built from what is REGISTERED at action-definition time - a plugin index registered later still works
	// via "all", and by the next boot it has its own option, since plugin registrations replay before this runs
	options := []adminaction.ArgumentOption{{Value: "all", Label: "Every index"}}
	for _, e := range registry.List() {
		label := e.Slug
		if e.PluginSlug != "" {
			label = fmt.Sprintf("%s (plugin %s)", e.Slug, e.PluginSlug)
		}
		options = append(options, adminaction.ArgumentOption{Value: e.Slug, Label: label})
	}

	adminaction.Register(adminaction.Definition{
		Slug:        "search.reindex",
		Name:        "Reindex search",
		Description: "Rebuilds the chosen index from the source of truth. Core indexes (products, blog) rebuild into a fresh index and flip atomically - the store keeps serving the old docs throughout. Plugin indexes rebuild in place and ARE empty for the duration. Use after changing search/mapping/* settings (analyzer, shards, replicas); query-time settings (boosts, fuzziness, match type) apply live and never need this.",
		Group:       "Search",
		// The default 30s killed the rebuild a few hundred products in, leaving the
		// live index holding a fraction of the catalogue
		Timeout: 30 * time.Minute,
		// Drops the single shared index for EVERY channel, so a channel-scoped grant
		// must not be enough - one merchandiser could take every tenant's listings down
		Permission:              "search.manage",
		RequireGlobalPermission: true,
		Arguments: []adminaction.Argument{
			{
				Name:        "target",
				Label:       "Target index",
				Description: "Which index to rebuild.",
				Type:        adminaction.ArgSelect,
				Required:    true,
				Default:     "all",
				Options:     options,
			},
		},
		Handler: func(ctx context.Context, args adminaction.Args) (adminaction.Result, error) {
			target, ok := args.String("target")
			if !ok {
				return adminaction.Result{}, fmt.Errorf("target is required")
			}
			if len(registry.List()) == 0 {
				return adminaction.Result{Output: "Search is disabled (OPENSEARCH_URL not set) - nothing to reindex."}, nil
			}
			results, err := registry.ReindexTargets(ctx, []string{target})
			if err != nil {
				return adminaction.Result{}, err
			}
			parts := make([]string, 0, len(results))
			data := map[string]any{}
			var firstErr error
			for _, res := range results {
				data[res.Slug] = res.Count
				switch {
				case res.Skipped:
					parts = append(parts, fmt.Sprintf("%s: recreate failed", res.Slug))
				case res.Err != nil:
					parts = append(parts, fmt.Sprintf("%s: %d indexed, with errors", res.Slug, res.Count))
				default:
					parts = append(parts, fmt.Sprintf("%s: %d indexed", res.Slug, res.Count))
				}
				if res.Err != nil && firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", res.Slug, res.Err)
				}
			}
			if firstErr != nil {
				// The output still reports every index's progress - the error carries the cause
				return adminaction.Result{Output: strings.Join(parts, "; "), Data: data}, firstErr
			}
			return adminaction.Result{Output: strings.Join(parts, "; "), Data: data}, nil
		},
	})
}
