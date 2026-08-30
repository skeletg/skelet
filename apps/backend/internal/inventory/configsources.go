package inventory

import (
	"context"

	"github.com/skeletg/skelet/internal/config"
)

// RegisterConfigSources backs the select/multiselect options for this domain's
// settings fields. Needs a live service, so it can't register from an init()
func RegisterConfigSources(svc *Service) {
	config.RegisterSource("inventory_locations", func(ctx context.Context) ([]config.Option, error) {
		locs, err := svc.ListLocations(ctx)
		if err != nil {
			return nil, err
		}
		opts := make([]config.Option, len(locs))
		for i, l := range locs {
			opts[i] = config.Option{Value: l.Slug, Label: l.Name}
		}
		return opts, nil
	})
}
