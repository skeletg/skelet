package admin

import (
	"context"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
)

func (r *mutationResolver) SetProductVariantMode(ctx context.Context, productID string, mode model.VariantMode) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	var dm catalog.VariantMode
	switch mode {
	case model.VariantModeAttribute:
		dm = catalog.VariantModeAttribute
	case model.VariantModeList:
		dm = catalog.VariantModeList
	default:
		return nil, fmt.Errorf("invalid variant mode: %q", mode)
	}
	p, err := r.CatalogSvc.SetProductVariantMode(ctx, pid, dm)
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}
