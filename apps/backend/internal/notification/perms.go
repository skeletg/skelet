package notification

import "github.com/skeletg/skelet/apps/backend/internal/permission"

func init() {
	permission.RegisterGroup(permission.Group{Slug: "notifications", Name: "Notifications", SortOrder: 14})
	permission.Register(permission.Definition{Group: "notifications", Slug: "notifications.view", Name: "View Notifications", SortOrder: 1})
}
