package wiring

import (
	"context"
	"fmt"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/internal/adminaction"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// The Tools-page surface over the config service - the Tools registry is core's, config is plumbing
// Mirrors `bin/cli config orphans / prune`
func registerConfigActions(svc *config.Service) {
	adminaction.Register(adminaction.Definition{
		Slug:                    "config.orphans",
		Name:                    "List orphaned config rows",
		Description:             "Returns DB rows whose paths are no longer in the registry. Safe (read-only).",
		Group:                   "Configuration",
		Permission:              "config.manage",
		RequireGlobalPermission: true,
		Handler: func(ctx context.Context, _ adminaction.Args) (adminaction.Result, error) {
			orphans, err := svc.ListOrphans(ctx)
			if err != nil {
				return adminaction.Result{}, err
			}
			lines := make([]string, 0, len(orphans))
			for _, o := range orphans {
				lines = append(lines, fmt.Sprintf("%s/%s = %q", config.FormatScope(o.Scope), o.Path, o.Value))
			}
			output := fmt.Sprintf("%d orphan row(s)", len(orphans))
			if len(lines) > 0 {
				output = output + "\n" + strings.Join(lines, "\n")
			}
			return adminaction.Result{
				Output: output,
				Data:   map[string]any{"count": len(orphans), "rows": lines},
			}, nil
		},
	})

	adminaction.Register(adminaction.Definition{
		Slug:        "config.prune_orphans",
		Name:        "Prune orphaned config rows",
		Description: "Deletes DB rows whose paths are no longer in the registry. Destructive - cannot be undone.",
		Group:       "Configuration",
		Permission:  "config.manage",
		// Orphans span every channel's values
		RequireGlobalPermission: true,
		Handler: func(ctx context.Context, _ adminaction.Args) (adminaction.Result, error) {
			n, err := svc.Prune(ctx)
			if err != nil {
				return adminaction.Result{}, err
			}
			return adminaction.Result{
				Output: fmt.Sprintf("deleted %d orphan row(s)", n),
				Data:   map[string]any{"deleted": n},
			}, nil
		},
	})
}
