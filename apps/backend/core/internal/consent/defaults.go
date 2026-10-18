package consent

import "github.com/skeletg/skelet/apps/backend/core/lib/consentkind"

func init() {
	for _, k := range consentkind.All() {
		Register(Definition{
			Slug: k.Slug, Name: k.Name, Description: k.Description,
			Purpose: Purpose(k.Purpose), Channel: Channel(k.Channel),
			Required: k.Required, DefaultOptIn: k.DefaultOptIn,
		})
	}
}
