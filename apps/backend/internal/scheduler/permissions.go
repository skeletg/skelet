package scheduler

import "github.com/skeletg/skelet/internal/permission"

func init() {
	permission.RegisterGroup(permission.Group{
		Slug: "scheduler", Name: "Scheduled jobs", SortOrder: 90,
	})
	permission.Register(permission.Definition{Group: "scheduler", Slug: "scheduler.view", Name: "View scheduled jobs", SortOrder: 1})
	permission.Register(permission.Definition{Group: "scheduler", Slug: "scheduler.manage", Name: "Manage scheduled jobs (schedule, enable, run now, webhook jobs)", SortOrder: 2})
}
