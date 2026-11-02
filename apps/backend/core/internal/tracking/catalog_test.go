package tracking_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/pluginset"
	"github.com/skeletg/skelet/apps/backend/core/internal/tracking"
	"github.com/skeletg/skelet/apps/backend/sdk"

	// Every first-party destination plugin, compiled in here only - a build includes just the enabled ones
	_ "github.com/skeletg/skelet/apps/backend/core/plugins/tracking-attentive"
	_ "github.com/skeletg/skelet/apps/backend/core/plugins/tracking-awin"
	_ "github.com/skeletg/skelet/apps/backend/core/plugins/tracking-bing"
	_ "github.com/skeletg/skelet/apps/backend/core/plugins/tracking-cj"
	_ "github.com/skeletg/skelet/apps/backend/core/plugins/tracking-ga4"
	_ "github.com/skeletg/skelet/apps/backend/core/plugins/tracking-google-ads"
	_ "github.com/skeletg/skelet/apps/backend/core/plugins/tracking-klaviyo"
	_ "github.com/skeletg/skelet/apps/backend/core/plugins/tracking-meta"
	_ "github.com/skeletg/skelet/apps/backend/core/plugins/tracking-openai"
	_ "github.com/skeletg/skelet/apps/backend/core/plugins/tracking-webhook"
)

// The catalogue the dashboard shows has to be exactly what the plugins register, and cover every tracking plugin
func TestTheCatalogueIsWhatThePluginsRegister(t *testing.T) {
	var registered []tracking.CatalogEntry
	for _, r := range sdk.Destinations() {
		registered = append(registered, tracking.CatalogEntry{Kind: r.Destination.Kind, Label: r.Destination.Label, Plugin: r.PluginSlug})
	}
	byKind := func(a, b tracking.CatalogEntry) int { return strings.Compare(a.Kind, b.Kind) }
	want := slices.Clone(tracking.FirstParty)
	slices.SortFunc(want, byKind)
	slices.SortFunc(registered, byKind)
	if !slices.Equal(want, registered) {
		t.Errorf("FirstParty drifted from the plugins:\n list   %v\n plugins %v", want, registered)
	}

	set, err := pluginset.Load("../..")
	if err != nil {
		t.Fatal(err)
	}
	for slug := range set.Plugins {
		if strings.HasPrefix(slug, "tracking-") && !slices.ContainsFunc(tracking.FirstParty, func(e tracking.CatalogEntry) bool { return e.Plugin == slug }) {
			t.Errorf("plugins.json has %s, which the catalogue doesn't list - blank-import it above and add its kinds", slug)
		}
	}
}
