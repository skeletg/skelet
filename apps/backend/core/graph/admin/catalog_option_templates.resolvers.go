package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CreateOptionTemplate(ctx context.Context, input model.OptionTemplateInput) (*model.OptionTemplate, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	t, err := r.CatalogSvc.CreateOptionTemplate(ctx, catalog.OptionTemplateInput{Name: input.Name, Description: ptr.Deref(input.Description), CurrencyCode: input.CurrencyCode})
	if err != nil {
		return nil, err
	}
	return r.reloadOptionTemplate(ctx, t.ID)
}

func (r *mutationResolver) UpdateOptionTemplate(ctx context.Context, id string, input model.OptionTemplateInput) (*model.OptionTemplate, error) {
	tid, err := parseID(id, "template")
	if err != nil {
		return nil, err
	}
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	if _, err := r.CatalogSvc.UpdateOptionTemplate(ctx, tid, catalog.OptionTemplateInput{Name: input.Name, Description: ptr.Deref(input.Description), CurrencyCode: input.CurrencyCode}); err != nil {
		return nil, err
	}
	return r.reloadOptionTemplate(ctx, tid)
}

func (r *mutationResolver) DeleteOptionTemplate(ctx context.Context, id string) (bool, error) {
	tid, err := parseID(id, "template")
	if err != nil {
		return false, err
	}
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return false, err
	}
	if err := r.CatalogSvc.DeleteOptionTemplate(ctx, tid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SetOptionTemplateRules(ctx context.Context, templateID string, rules []*model.ProductOptionRuleInput) (*model.OptionTemplate, error) {
	tid, err := parseID(templateID, "template")
	if err != nil {
		return nil, err
	}
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	inputs, err := optionRuleInputs(rules)
	if err != nil {
		return nil, err
	}
	if _, err := r.CatalogSvc.SetOptionTemplateRules(ctx, tid, inputs); err != nil {
		return nil, err
	}
	return r.reloadOptionTemplate(ctx, tid)
}

func (r *mutationResolver) SetProductOptionTemplates(ctx context.Context, productID string, templateIds []string) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	ids, err := parseTemplateIDs(templateIds)
	if err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.SetProductOptionTemplates(ctx, pid, ids); err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, pid)
}

func (r *mutationResolver) AssignOptionTemplate(ctx context.Context, templateID string, productIds []string) (*model.OptionTemplate, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return nil, err
	}
	tid, err := parseID(templateID, "template")
	if err != nil {
		return nil, err
	}
	pids, err := r.requireCatalogManageForProducts(ctx, productIds)
	if err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.AssignOptionTemplate(ctx, tid, pids); err != nil {
		return nil, err
	}
	return r.reloadOptionTemplate(ctx, tid)
}

func (r *mutationResolver) UnassignOptionTemplate(ctx context.Context, templateID string, productIds []string) (*model.OptionTemplate, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return nil, err
	}
	tid, err := parseID(templateID, "template")
	if err != nil {
		return nil, err
	}
	pids, err := r.requireCatalogManageForProducts(ctx, productIds)
	if err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.UnassignOptionTemplate(ctx, tid, pids); err != nil {
		return nil, err
	}
	return r.reloadOptionTemplate(ctx, tid)
}

func (r *mutationResolver) DetachOptionTemplate(ctx context.Context, productID string, templateID string) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	tid, err := parseID(templateID, "template")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.DetachOptionTemplate(ctx, pid, tid); err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, pid)
}

func (r *mutationResolver) SetProductConfiguratorLayout(ctx context.Context, productID string, layout model.ConfiguratorLayout) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.SetProductConfiguratorLayout(ctx, pid, catalog.ConfiguratorLayout(strings.ToLower(string(layout)))); err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, pid)
}

func (r *mutationResolver) SetProductTypeOptionTemplates(ctx context.Context, productTypeID string, templateIds []string) (*model.ProductType, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	ptID, err := parseID(productTypeID, "product type")
	if err != nil {
		return nil, err
	}
	ids, err := parseTemplateIDs(templateIds)
	if err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.SetProductTypeOptionTemplates(ctx, ptID, ids); err != nil {
		return nil, err
	}
	pt, err := r.CatalogSvc.GetProductType(ctx, ptID)
	if err != nil {
		return nil, err
	}
	return toProductTypeModel(pt), nil
}

func (r *optionTemplateResolver) ProductOptions(ctx context.Context, obj *model.OptionTemplate) ([]*model.ProductOption, error) {
	tid, err := parseID(obj.ID, "template")
	if err != nil {
		return nil, err
	}
	options, err := r.CatalogSvc.GetOptionTemplateOptions(ctx, tid, r.activeLocale(ctx))
	if err != nil {
		return nil, fmt.Errorf("load template options: %w", err)
	}
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "catalog.view")
	if err != nil {
		return nil, err
	}
	out := toProductOptionModels(options, perm)
	r.attachOptionImages(ctx, out, options)
	return out, nil
}

func (r *optionTemplateResolver) OptionRules(ctx context.Context, obj *model.OptionTemplate) ([]*model.ProductOptionRule, error) {
	tid, err := parseID(obj.ID, "template")
	if err != nil {
		return nil, err
	}
	rules, err := r.CatalogSvc.GetOptionTemplateRules(ctx, tid, r.activeLocale(ctx))
	if err != nil {
		return nil, fmt.Errorf("load template rules: %w", err)
	}
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "catalog.view")
	if err != nil {
		return nil, err
	}
	return toProductOptionRuleModels(rules, perm), nil
}

func (r *optionTemplateResolver) Products(ctx context.Context, obj *model.OptionTemplate, first *int32, offset *int32) ([]*model.Product, error) {
	tid, err := parseID(obj.ID, "template")
	if err != nil {
		return nil, err
	}
	limit := min(max(int32Or(first, 50), 1), 100)
	ids, _, err := r.CatalogSvc.ListProductIDsUsingOptionTemplate(ctx, tid, limit, max(int32Or(offset, 0), 0))
	if err != nil {
		return nil, err
	}
	products, err := r.CatalogSvc.ListProductsWithGraphByIDs(ctx, ids, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	return (&queryResolver{r.Resolver}).renderProductRows(ctx, products, nil)
}

func (r *optionTemplateResolver) ProductTypes(ctx context.Context, obj *model.OptionTemplate) ([]*model.ProductType, error) {
	tid, err := parseID(obj.ID, "template")
	if err != nil {
		return nil, err
	}
	ids, err := r.CatalogSvc.ListProductTypeIDsUsingOptionTemplate(ctx, tid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ProductType, 0, len(ids))
	for _, id := range ids {
		pt, err := r.CatalogSvc.GetProductType(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, toProductTypeModel(pt))
	}
	return out, nil
}

func (r *productResolver) OptionTemplates(ctx context.Context, obj *model.Product) ([]*model.OptionTemplate, error) {
	pid, err := parseID(obj.ID, "product")
	if err != nil {
		return nil, err
	}
	templates, err := r.CatalogSvc.GetProductOptionTemplates(ctx, pid)
	if err != nil {
		return nil, fmt.Errorf("load product templates: %w", err)
	}
	return toOptionTemplateModels(templates), nil
}

func (r *productResolver) TypeOptionTemplates(ctx context.Context, obj *model.Product) ([]*model.OptionTemplate, error) {
	pid, err := parseID(obj.ID, "product")
	if err != nil {
		return nil, err
	}
	templates, err := r.CatalogSvc.GetInheritedOptionTemplates(ctx, pid)
	if err != nil {
		return nil, fmt.Errorf("load product type templates: %w", err)
	}
	return toOptionTemplateModels(templates), nil
}

func (r *productResolver) ConfiguratorLayout(ctx context.Context, obj *model.Product) (model.ConfiguratorLayout, error) {
	pid, err := parseID(obj.ID, "product")
	if err != nil {
		return "", err
	}
	layout, err := r.CatalogSvc.GetProductConfiguratorLayout(ctx, pid)
	if err != nil {
		return "", err
	}
	return model.ConfiguratorLayout(strings.ToUpper(string(layout))), nil
}

func (r *productTypeResolver) OptionTemplates(ctx context.Context, obj *model.ProductType) ([]*model.OptionTemplate, error) {
	ptID, err := parseID(obj.ID, "product type")
	if err != nil {
		return nil, err
	}
	templates, err := r.CatalogSvc.GetProductTypeOptionTemplates(ctx, ptID)
	if err != nil {
		return nil, fmt.Errorf("load product type templates: %w", err)
	}
	return toOptionTemplateModels(templates), nil
}

func (r *queryResolver) OptionTemplates(ctx context.Context) ([]*model.OptionTemplate, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	templates, err := r.CatalogSvc.ListOptionTemplates(ctx)
	if err != nil {
		return nil, err
	}
	return toOptionTemplateModels(templates), nil
}

func (r *queryResolver) OptionTemplate(ctx context.Context, id string) (*model.OptionTemplate, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	tid, err := parseID(id, "template")
	if err != nil {
		return nil, err
	}
	t, err := r.reloadOptionTemplate(ctx, tid)
	if err != nil {
		if errors.Is(err, catalog.ErrOptionTemplateNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (r *Resolver) OptionTemplate() OptionTemplateResolver { return &optionTemplateResolver{r} }

type optionTemplateResolver struct{ *Resolver }
