package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/skeletg/skelet/internal/action"
)

// main.go calls this after the config Service is built, so the handlers close over a live reference
// Mirrors `bin/cli config orphans / prune` - the CLI path stays, this is the admin-UI surface for the same ops
func RegisterActions(svc *Service) {
	action.Register(action.Definition{
		Slug:        "config.orphans",
		Name:        "List orphaned config rows",
		Description: "Returns DB rows whose paths are no longer in the registry. Safe (read-only).",
		Group:       "Configuration",
		Permission:  "config.manage",
		Handler: func(ctx context.Context, _ action.Args) (action.Result, error) {
			orphans, err := svc.ListOrphans(ctx)
			if err != nil {
				return action.Result{}, err
			}
			lines := make([]string, 0, len(orphans))
			for _, o := range orphans {
				lines = append(lines, fmt.Sprintf("%s/%s = %q", FormatScope(o.Scope), o.Path, o.Value))
			}
			output := fmt.Sprintf("%d orphan row(s)", len(orphans))
			if len(lines) > 0 {
				output = output + "\n" + strings.Join(lines, "\n")
			}
			return action.Result{
				Output: output,
				Data:   map[string]any{"count": len(orphans), "rows": lines},
			}, nil
		},
	})

	action.Register(action.Definition{
		Slug:        "config.prune_orphans",
		Name:        "Prune orphaned config rows",
		Description: "Deletes DB rows whose paths are no longer in the registry. Destructive - cannot be undone.",
		Group:       "Configuration",
		Permission:  "config.manage",
		Handler: func(ctx context.Context, _ action.Args) (action.Result, error) {
			n, err := svc.Prune(ctx)
			if err != nil {
				return action.Result{}, err
			}
			return action.Result{
				Output: fmt.Sprintf("deleted %d orphan row(s)", n),
				Data:   map[string]any{"deleted": n},
			}, nil
		},
	})
}
