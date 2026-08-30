package customer

import (
	"context"

	"github.com/skeletg/skelet/internal/config"
)

// RegisterConfigSources backs the select/multiselect options for this domain's
// settings fields. Needs a live service, so it can't register from an init()
func RegisterConfigSources(svc *Service) {
	config.RegisterSource("customer_groups", func(ctx context.Context) ([]config.Option, error) { // These sources need live service refs, so they can't register from a package init
		groups, err := svc.ListGroups(ctx)
		if err != nil {
			return nil, err
		}
		opts := make([]config.Option, len(groups))
		for i, g := range groups {
			opts[i] = config.Option{Value: g.Slug, Label: g.Name}
		}
		return opts, nil
	})
}
