package wiring

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/action"
)

func TestConfigOrphanActionsNeedTheGlobalGrant(t *testing.T) {
	if _, ok := action.Find("config.orphans"); !ok {
		registerConfigActions(nil)
	}
	for _, slug := range []string{"config.orphans", "config.prune_orphans"} {
		def, ok := action.Find(slug)
		if !ok {
			t.Fatalf("%s is not registered", slug)
		}
		if !def.RequireGlobalPermission {
			t.Errorf("%s runs on a channel-scoped config.manage, but it reads and deletes every channel's values", slug)
		}
	}
}
