package lens

import "github.com/skeletg/skelet/internal/permission"

func init() {
	permission.RegisterGroup(permission.Group{Slug: "lens", Name: "Web Analytics", SortOrder: 16})                         // own capability, so a role can get analytics access without broader settings/customer reach
	permission.Register(permission.Definition{Group: "lens", Slug: "lens.view", Name: "View Web Analytics", SortOrder: 1}) // no .manage twin - Lens is query-only, and the one that existed gated nothing; add it back with the write path it guards
}
