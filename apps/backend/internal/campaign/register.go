package campaign

import "github.com/skeletg/skelet/internal/permission"

func init() {
	permission.RegisterGroup(permission.Group{Slug: "campaigns", Name: "Email Campaigns", SortOrder: 12})
	permission.Register(permission.Definition{Group: "campaigns", Slug: "campaigns.view", Name: "View Campaigns", SortOrder: 1})
	permission.Register(permission.Definition{Group: "campaigns", Slug: "campaigns.manage", Name: "Manage Campaigns", SortOrder: 2})
}
