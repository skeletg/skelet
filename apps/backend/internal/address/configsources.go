package address

import (
	"context"
	"sort"

	"github.com/skeletg/skelet/internal/config"
)

// RegisterConfigSources backs the address-validator select. Registration is a live, plugin-extensible
// map, so the options are read at request time rather than baked in - no live service needed, unlike
// the other domains' sources
func RegisterConfigSources() {
	config.RegisterSource("address_validators", func(context.Context) ([]config.Option, error) {
		mu.RLock()
		slugs := make([]string, 0, len(validators))
		for slug := range validators {
			slugs = append(slugs, slug)
		}
		mu.RUnlock()
		sort.Strings(slugs)
		// "none" is a sentinel Validate() honours directly, not a registered validator, so it is prepended
		opts := []config.Option{{Value: "none", Label: "None - skip address checking"}}
		for _, s := range slugs {
			opts = append(opts, config.Option{Value: s, Label: s})
		}
		return opts, nil
	})
}
