package tracking_test

import (
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/tracking"
)

// The dashboard keys the catalogue by kind and builds the install command from the slug, so each kind appears once and
// each slug is a published tracking plugin's
func TestTheCatalogueListsEachKindOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range tracking.FirstParty {
		if seen[e.Kind] {
			t.Errorf("%s is listed twice", e.Kind)
		}
		seen[e.Kind] = true
		if !strings.HasPrefix(e.Plugin, "skelet-tracking-") || e.Label == "" {
			t.Errorf("%+v: want a skelet-tracking-<name> slug and a label", e)
		}
	}
}
