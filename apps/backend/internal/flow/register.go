package flow

import "github.com/skeletg/skelet/internal/permission"

func init() {
	permission.RegisterGroup(permission.Group{Slug: "flows", Name: "Automation Flows", SortOrder: 13}) // own group, so a role can be granted automation management without broader marketing/settings access
	permission.Register(permission.Definition{Group: "flows", Slug: "flows.view", Name: "View Flows", SortOrder: 1})
	permission.Register(permission.Definition{Group: "flows", Slug: "flows.manage", Name: "Manage Flows", SortOrder: 2})
}
