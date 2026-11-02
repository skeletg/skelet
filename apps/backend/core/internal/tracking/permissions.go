package tracking

import "github.com/skeletg/skelet/apps/backend/core/internal/permission"

func init() {
	permission.RegisterGroup(permission.Group{Slug: "tracking", Name: "Tracking", SortOrder: 91})
	permission.Register(permission.Definition{Group: "tracking", Slug: "tracking.view", Name: "View tracking destinations and their delivery log", SortOrder: 1})
	permission.Register(permission.Definition{Group: "tracking", Slug: "tracking.manage", Name: "Set up tracking destinations (credentials, test sends, retries, imports)", SortOrder: 2})
}
