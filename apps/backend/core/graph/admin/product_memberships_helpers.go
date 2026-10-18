package admin

// Lives here rather than *.resolvers.go - gqlgen relocates or comments out free functions in resolver files on regen Admin has no DataLoader layer and no channel-visibility gate, so every membership is a per-call service lookup over all relationships, published or not - fine on a product detail page, an N+1 if you select partOf on a LIST Siblings, option and optionValues come off the already-rendered parent's collections, so they cost no extra queries

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

// Renders a membership parent exactly as the Product(id) query does, and answers (nil, nil) when the product is gone
func (r *Resolver) renderMembershipProduct(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	p, err := r.CatalogSvc.GetProduct(ctx, id, r.activeLocale(ctx))
	if err != nil {
		if errors.Is(err, catalog.ErrProductNotFound) {
			return nil, nil
		}
		return nil, err
	}
	cols, err := r.CatalogSvc.ListCollectionsByProductIDs(ctx, []uuid.UUID{id}, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	p.Collections = cols[id]
	cats, err := r.CatalogSvc.ListCategoriesByProductIDs(ctx, []uuid.UUID{id}, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	p.Categories = cats[id]
	return r.renderProduct(ctx, p)
}

func (r *Resolver) buildConfigurableMembership(ctx context.Context, productIDStr string) (*model.ConfigurableMembership, error) {
	productID, err := uuid.Parse(productIDStr)
	if err != nil {
		return nil, nil
	}
	links, err := r.CatalogSvc.ListConfigurableParentByChildIDs(ctx, []uuid.UUID{productID})
	if err != nil {
		return nil, err
	}
	link := links[productID]
	if link.ParentID == uuid.Nil {
		return nil, nil
	}
	rendered, err := r.renderMembershipProduct(ctx, link.ParentID)
	if err != nil {
		return nil, err
	}
	if rendered == nil {
		return nil, nil
	}
	optIDs := make(map[string]struct{}, len(rendered.ConfigurableOptions))
	for _, o := range rendered.ConfigurableOptions {
		optIDs[o.ID] = struct{}{}
	}
	optionValues := []*model.ProductAttributeValue{}
	siblings := make([]*model.Product, 0, len(rendered.Children))
	for _, c := range rendered.Children {
		if c.ID == productIDStr {
			for _, av := range c.AttributeValues {
				if av.Attribute == nil {
					continue
				}
				if _, ok := optIDs[av.Attribute.ID]; ok {
					optionValues = append(optionValues, av)
				}
			}
			continue
		}
		siblings = append(siblings, c)
	}
	return &model.ConfigurableMembership{
		Parent:       rendered,
		VariantMode:  rendered.VariantMode,
		OptionValues: optionValues,
		SortOrder:    link.VariantSortOrder,
		Siblings:     siblings,
	}, nil
}

func (r *Resolver) buildBundleMemberships(ctx context.Context, productIDStr string) ([]*model.BundleMembership, error) {
	out := []*model.BundleMembership{}
	productID, err := uuid.Parse(productIDStr)
	if err != nil {
		return out, nil
	}
	links, err := r.CatalogSvc.ListBundleSelectionLinksByProductIDs(ctx, []uuid.UUID{productID})
	if err != nil {
		return nil, err
	}
	for _, link := range links[productID] {
		rendered, err := r.renderMembershipProduct(ctx, link.BundleID)
		if err != nil {
			return nil, err
		}
		if rendered == nil {
			continue
		}
		optID := link.OptionID.String()
		var optModel *model.BundleOption
		for _, o := range rendered.BundleOptions {
			if o.ID == optID {
				optModel = o
				break
			}
		}
		if optModel == nil {
			continue
		}
		siblings := make([]*model.BundleSelection, 0, len(optModel.Selections))
		for _, s := range optModel.Selections {
			if s.Product != nil && s.Product.ID == productIDStr {
				continue
			}
			siblings = append(siblings, s)
		}
		out = append(out, &model.BundleMembership{
			Bundle:    rendered,
			Option:    optModel,
			Quantity:  link.Quantity,
			IsDefault: link.IsDefault,
			SortOrder: link.SortOrder,
			Siblings:  siblings,
		})
	}
	return out, nil
}

func (r *Resolver) buildGroupedMemberships(ctx context.Context, productIDStr string) ([]*model.GroupedMembership, error) {
	out := []*model.GroupedMembership{}
	productID, err := uuid.Parse(productIDStr)
	if err != nil {
		return out, nil
	}
	links, err := r.CatalogSvc.ListGroupedMemberLinksByMemberIDs(ctx, []uuid.UUID{productID})
	if err != nil {
		return nil, err
	}
	for _, link := range links[productID] {
		rendered, err := r.renderMembershipProduct(ctx, link.ParentID)
		if err != nil {
			return nil, err
		}
		if rendered == nil {
			continue
		}
		siblings := make([]*model.GroupedMember, 0, len(rendered.GroupedMembers))
		for _, gm := range rendered.GroupedMembers {
			if gm.Member != nil && gm.Member.ID == productIDStr {
				continue
			}
			siblings = append(siblings, gm)
		}
		out = append(out, &model.GroupedMembership{
			GroupedProduct:  rendered,
			DefaultQuantity: link.DefaultQuantity,
			SortOrder:       link.SortOrder,
			Siblings:        siblings,
		})
	}
	return out, nil
}
