package cms

import (
	"github.com/skeletg/skelet/apps/backend/core/internal/revalidate"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// Editorial-workflow policy in the config registry, so admins flip it on /settings without a redeploy, read at request time by the publish path
const CacheSurface = "cms"

func init() {
	config.RegisterSection(config.Section{Slug: "cms", Label: "CMS", SortOrder: 11})
	config.RegisterGroup(config.Group{
		Section:   "cms",
		Slug:      "workflow",
		Label:     "Editorial Workflow",
		SortOrder: 1,
	})
	config.Register(config.Definition{
		Path:    cfgRequireApproval,
		Type:    config.TypeBool,
		Default: "false",
		Label:   "Require approval before publishing",
	})

	revalidate.RegisterSurface(revalidate.Surface{Key: CacheSurface, Label: "CMS pages", AllTag: "cms:all", DefaultEnabled: true})
}
