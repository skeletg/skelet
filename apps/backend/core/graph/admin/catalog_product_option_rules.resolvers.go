package admin

import (
	"context"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) SetProductOptionRules(ctx context.Context, productID string, rules []*model.ProductOptionRuleInput) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	inputs, err := optionRuleInputs(rules)
	if err != nil {
		return nil, err
	}
	if _, err := r.CatalogSvc.SetProductOptionRules(ctx, pid, inputs); err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, pid)
}

func (r *productResolver) OptionRules(ctx context.Context, obj *model.Product) ([]*model.ProductOptionRule, error) {
	pid, err := parseID(obj.ID, "product")
	if err != nil {
		return nil, err
	}
	rules, err := r.CatalogSvc.GetProductOptionRules(ctx, pid, r.activeLocale(ctx))
	if err != nil {
		return nil, fmt.Errorf("load product option rules: %w", err)
	}
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "catalog.view")
	if err != nil {
		return nil, err
	}
	return toProductOptionRuleModels(rules, perm), nil
}
