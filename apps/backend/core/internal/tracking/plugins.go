package tracking

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// RegisterPluginKinds turns every destination a compiled-in plugin registered into a kind, offered on a channel only
// where that plugin is enabled. Safe to call again - boot can wire the service more than once, as tests do
func RegisterPluginKinds(regs []sdk.RegisteredDestination, enabled func(ctx context.Context, slug string, channelID uuid.UUID) bool) {
	for _, reg := range regs {
		if _, done := KindFor(reg.Destination.Kind); done {
			continue
		}
		slug := reg.PluginSlug
		RegisterKind(Kind{
			Destination: reg.Destination, Plugin: slug,
			Available: func(ctx context.Context, channelID uuid.UUID) bool { return enabled(ctx, slug, channelID) },
		})
	}
}
