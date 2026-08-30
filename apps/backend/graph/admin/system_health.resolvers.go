package admin

import (
	"context"
	"time"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/auth"
)

// Probes run concurrently behind the registry, each with its own timeout, so one wedged subsystem costs the slowest
// single probe rather than the sum - an operator opens this page precisely when something is already wrong
func (r *queryResolver) SystemHealth(ctx context.Context) (*model.SystemHealth, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "settings.manage"); err != nil {
		return nil, err
	}
	report := r.Health.Run(ctx, time.Now())
	out := &model.SystemHealth{
		Status:    toHealthStatus(report.Status),
		CheckedAt: report.CheckedAt,
		Groups:    make([]*model.HealthGroup, 0, len(report.Groups)),
	}
	for _, g := range report.Groups {
		group := &model.HealthGroup{Key: g.Key, Label: g.Label, Checks: make([]*model.HealthCheck, 0, len(g.Checks))}
		for _, c := range g.Checks {
			check := &model.HealthCheck{
				Key:    c.Key,
				Label:  c.Label,
				Status: toHealthStatus(c.Result.Status),
				Detail: c.Result.Detail,
			}
			if c.Result.Hint != "" { // nil rather than "" so the UI renders a hint only when there is one
				hint := c.Result.Hint
				check.Hint = &hint
			}
			group.Checks = append(group.Checks, check)
		}
		out.Groups = append(out.Groups, group)
	}
	return out, nil
}
