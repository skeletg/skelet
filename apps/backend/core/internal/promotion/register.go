package promotion

import "github.com/skeletg/skelet/apps/backend/core/internal/permission"

func init() {
	permission.RegisterGroup(permission.Group{Slug: "promotions", Name: "Promotions", SortOrder: 10})
	permission.Register(permission.Definition{Group: "promotions", Slug: "promotions.view", Name: "View Promotions", SortOrder: 1})
	permission.Register(permission.Definition{Group: "promotions", Slug: "promotions.manage", Name: "Manage Promotions", SortOrder: 2})
}
