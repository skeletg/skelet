package emailtemplate

import "github.com/skeletg/skelet/apps/backend/internal/permission"

func init() {
	permission.RegisterGroup(permission.Group{Slug: "emails", Name: "Email Library", SortOrder: 14})
	permission.Register(permission.Definition{Group: "emails", Slug: "emails.view", Name: "View Email Library", SortOrder: 1})
	permission.Register(permission.Definition{Group: "emails", Slug: "emails.manage", Name: "Manage Email Library", SortOrder: 2})
}
