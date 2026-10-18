package catalog

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// The URL settings are read with no channel in hand, so a per-channel value would be stored and never read
func TestTheURLSettingsCatalogReadsAreStoreWide(t *testing.T) {
	for _, path := range []string{"general/urls/auto_redirect_on_change", "general/urls/url_template_fallback", "general/urls/redirect_type"} {
		if config.ScopingAllows(path, config.ScopeChannel, false) {
			t.Errorf("%s accepts a per-channel value, but the catalog only reads it store-wide", path)
		}
	}
}
