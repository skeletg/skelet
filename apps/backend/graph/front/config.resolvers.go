package front

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	model1 "github.com/skeletg/skelet/apps/backend/graph/front/model"
)

// Unauthenticated on purpose - the storefront loads store name, currency, "allow guest checkout" and the like from here
func (r *queryResolver) PublicConfig(ctx context.Context) ([]*model.ConfigValue, error) {
	entries, err := r.ConfigSvc.ListPublic(ctx, requestConfigScope(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]*model.ConfigValue, len(entries))
	for i, e := range entries {
		out[i] = toConfigValueModel(ctx, e)
	}
	return out, nil
}

func (r *queryResolver) PublicConfigSections(ctx context.Context) ([]*model.ConfigSection, error) {
	entries, err := r.ConfigSvc.ListPublic(ctx, requestConfigScope(ctx))
	if err != nil {
		return nil, err
	}
	return buildConfigSections(ctx, entries), nil
}

func (r *queryResolver) Features(ctx context.Context) (*model1.StoreFeatures, error) {
	return &model1.StoreFeatures{Signals: r.SignalsEnabled}, nil
}
