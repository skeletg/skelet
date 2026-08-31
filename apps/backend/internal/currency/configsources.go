package currency

import (
	"context"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/internal/config"
)

// RegisterConfigSources backs the select options for settings fields that name a currency.
// Needs a live service, so it can't register from an init()
func RegisterConfigSources(svc *Service) {
	config.RegisterSource("currencies", func(ctx context.Context) ([]config.Option, error) {
		// Active only - an archived currency is not something a setting should be able to adopt
		list, err := svc.List(ctx, true)
		if err != nil {
			return nil, err
		}
		opts := make([]config.Option, len(list))
		for i, c := range list {
			opts[i] = config.Option{Value: c.Code, Label: fmt.Sprintf("%s - %s", c.Code, c.Name)}
		}
		return opts, nil
	})
}
