package experiment

import "github.com/skeletg/skelet/apps/backend/internal/permission"

func init() {
	permission.RegisterGroup(permission.Group{Slug: "experiments", Name: "Experiments", SortOrder: 14}) // own group, so a role can manage optimization without broader marketing/settings access
	permission.Register(permission.Definition{Group: "experiments", Slug: "experiments.view", Name: "View Experiments", SortOrder: 1})
	permission.Register(permission.Definition{Group: "experiments", Slug: "experiments.manage", Name: "Manage Experiments", SortOrder: 2})
}
