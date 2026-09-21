package front

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	model1 "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
)

func (r *productResolver) PartOf(ctx context.Context, obj *model.Product) (*model1.ProductMemberships, error) {
	// cheap shell - no lookup fires until a sub-field is selected
	return &model1.ProductMemberships{ProductID: obj.ID}, nil
}

func (r *productMembershipsResolver) Configurable(ctx context.Context, obj *model1.ProductMemberships) (*model1.ConfigurableMembership, error) {
	return r.buildConfigurableMembership(ctx, obj.ProductID)
}

func (r *productMembershipsResolver) Bundles(ctx context.Context, obj *model1.ProductMemberships) ([]*model1.BundleMembership, error) {
	return r.buildBundleMemberships(ctx, obj.ProductID)
}

func (r *productMembershipsResolver) GroupedProducts(ctx context.Context, obj *model1.ProductMemberships) ([]*model1.GroupedMembership, error) {
	return r.buildGroupedMemberships(ctx, obj.ProductID)
}

func (r *Resolver) ProductMemberships() ProductMembershipsResolver {
	return &productMembershipsResolver{r}
}

type productMembershipsResolver struct{ *Resolver }
