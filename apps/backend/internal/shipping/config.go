package shipping

import "github.com/skeletg/skelet/apps/backend/internal/config"

func init() {
	// The section shipping-carrier plugins register their per-carrier groups under - a plugin registers "shipping/<slug>/api_key" as a ConfigField and the host adapter auto-registers the per-carrier Group, but only if this Section exists first
	config.RegisterSection(config.Section{
		Slug:      "shipping",
		Label:     "Shipping",
		SortOrder: 55,
	})
}
