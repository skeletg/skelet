package menu

import "github.com/skeletg/skelet/apps/backend/core/internal/revalidate"

const CacheSurface = "nav"

func init() {
	// The storefront "nav" cache surface - branding, menus, currencies, free-shipping threshold - Homed here since menu edits are the main bust trigger, and it busts whole-surface
	// Enabled by default: the header renders into every cached page, so a disabled surface strips the nav:all tag and silently turns every menu edit into a no-op purge
	revalidate.RegisterSurface(revalidate.Surface{
		Key:            CacheSurface,
		Label:          "Navigation (header, menus, currencies)",
		AllTag:         "nav:all",
		DefaultEnabled: true,
	})
}
