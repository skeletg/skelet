package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
)

func (r *productResolver) PartOf(ctx context.Context, obj *model.Product) (*model.ProductMemberships, error) {
	// cheap shell - the sub-fields do the lookups lazily
	return &model.ProductMemberships{ProductID: obj.ID}, nil
}

func (r *productMembershipsResolver) Configurable(ctx context.Context, obj *model.ProductMemberships) (*model.ConfigurableMembership, error) {
	return r.buildConfigurableMembership(ctx, obj.ProductID)
}

func (r *productMembershipsResolver) Bundles(ctx context.Context, obj *model.ProductMemberships) ([]*model.BundleMembership, error) {
	return r.buildBundleMemberships(ctx, obj.ProductID)
}

func (r *productMembershipsResolver) GroupedProducts(ctx context.Context, obj *model.ProductMemberships) ([]*model.GroupedMembership, error) {
	return r.buildGroupedMemberships(ctx, obj.ProductID)
}

func (r *Resolver) ProductMemberships() ProductMembershipsResolver {
	return &productMembershipsResolver{r}
}

type productMembershipsResolver struct{ *Resolver }
