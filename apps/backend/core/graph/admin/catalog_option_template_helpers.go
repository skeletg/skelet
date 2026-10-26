package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

// An option write owes the dashboard whichever owner the option belongs to, with its refreshed options
func (r *Resolver) reloadOwner(ctx context.Context, owner catalog.OptionOwner) (model.ProductOptionOwner, error) {
	if owner.IsTemplate() {
		return r.reloadOptionTemplate(ctx, owner.TemplateID)
	}
	return r.reloadProduct(ctx, owner.ProductID)
}

func (r *Resolver) reloadOptionTemplate(ctx context.Context, id uuid.UUID) (*model.OptionTemplate, error) {
	t, err := r.CatalogSvc.GetOptionTemplate(ctx, id)
	if err != nil {
		return nil, err
	}
	return toOptionTemplateModel(t), nil
}

func toOptionTemplateModel(t catalog.OptionTemplate) *model.OptionTemplate {
	return &model.OptionTemplate{
		ID:               t.ID.String(),
		Name:             t.Name,
		Description:      t.Description,
		CurrencyCode:     t.CurrencyCode,
		OptionCount:      int32(t.OptionCount),
		RuleCount:        int32(t.RuleCount),
		ProductCount:     int32(t.ProductCount),
		ProductTypeCount: int32(t.ProductTypeCount),
		CreatedAt:        t.CreatedAt,
		UpdatedAt:        t.UpdatedAt,
	}
}

func parseTemplateIDs(raw []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, len(raw))
	for i, s := range raw {
		id, err := parseID(s, "template")
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return ids, nil
}

func toOptionTemplateModels(ts []catalog.OptionTemplate) []*model.OptionTemplate {
	out := make([]*model.OptionTemplate, len(ts))
	for i, t := range ts {
		out[i] = toOptionTemplateModel(t)
	}
	return out
}

// A new option goes on exactly one product or template - a product's is gated like its other product-wide rows, a
// template's needs catalog.manage everywhere since it reaches products on every store
func (r *mutationResolver) requireOptionOwnerManage(ctx context.Context, productID, templateID *string) (catalog.OptionOwner, error) {
	if (productID == nil || *productID == "") == (templateID == nil || *templateID == "") {
		return catalog.OptionOwner{}, fmt.Errorf("%w: give a productId or a templateId", catalog.ErrOptionTemplateInvalid)
	}
	if templateID != nil && *templateID != "" {
		id, err := parseID(*templateID, "template")
		if err != nil {
			return catalog.OptionOwner{}, err
		}
		return catalog.OptionOwner{TemplateID: id}, r.requireGlobal(ctx, "catalog.manage")
	}
	id, err := parseID(*productID, "product")
	if err != nil {
		return catalog.OptionOwner{}, err
	}
	return catalog.OptionOwner{ProductID: id}, r.requireCatalogManageOnAllListingChannels(ctx, id)
}

func (r *mutationResolver) requireCatalogManageForProducts(ctx context.Context, raw []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, len(raw))
	for i, s := range raw {
		id, err := parseID(s, "product")
		if err != nil {
			return nil, err
		}
		if err := r.requireCatalogManageOnAllListingChannels(ctx, id); err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return ids, nil
}
