package wiring

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

// What this pins: a product update that touched only its configurator skips the search upsert - the index holds none
// of it - while any other field, or an update that doesn't say what changed, still re-indexes
func TestOnlyAConfiguratorChangeSkipsTheSearchIndex(t *testing.T) {
	for _, tc := range []struct {
		changed []string
		want    bool
	}{
		{[]string{catalog.ProductFieldOptions}, false},
		{[]string{catalog.ProductFieldOptions, catalog.ProductFieldOptions}, false},
		{[]string{catalog.ProductFieldOptions, "price"}, true},
		{[]string{"name"}, true},
		{nil, true},
	} {
		if got := indexedChange(tc.changed); got != tc.want {
			t.Errorf("indexedChange(%v) = %v, want %v", tc.changed, got, tc.want)
		}
	}
}
