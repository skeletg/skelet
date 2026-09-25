package front

// Product.partOf helpers, kept OUT of *.resolvers.go because gqlgen relocates or comments out free functions in resolver files on regen
// The inverse LINK lookup batches through loadXLinks, then the parent goes through the normal channel-aware fetch + renderProduct - siblings, option and optionValues all come off the ALREADY-rendered parent, so they cost no extra queries and inherit its channel visibility filtering for free

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/dataloaders"
	model1 "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

// (nil, nil) means the parent isn't published on the active store - the caller renders the relationship as absent instead of erroring
func (r *Resolver) fetchMembershipProduct(ctx context.Context, id uuid.UUID) (*catalog.Product, error) {
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return nil, nil
	}
	p, err := r.CatalogSvc.GetProductForChannel(ctx, id, *storeID, r.activeLocale(ctx))
	if err != nil {
		if errors.Is(err, catalog.ErrProductNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *Resolver) loadConfigurableParentLink(ctx context.Context, childID uuid.UUID) (catalog.ConfigurableParentLink, error) {
	if l := dataloaders.For(ctx); l != nil {
		return l.ConfigurableParent.Load(ctx, childID)()
	}
	m, err := r.CatalogSvc.ListConfigurableParentByChildIDs(ctx, []uuid.UUID{childID})
	if err != nil {
		return catalog.ConfigurableParentLink{}, err
	}
	return m[childID], nil
}

func (r *Resolver) loadBundleLinks(ctx context.Context, productID uuid.UUID) ([]catalog.BundleSelectionLink, error) {
	if l := dataloaders.For(ctx); l != nil {
		return l.BundleMemberships.Load(ctx, productID)()
	}
	m, err := r.CatalogSvc.ListBundleSelectionLinksByProductIDs(ctx, []uuid.UUID{productID})
	if err != nil {
		return nil, err
	}
	return m[productID], nil
}

func (r *Resolver) loadGroupedLinks(ctx context.Context, memberID uuid.UUID) ([]catalog.GroupedMemberLink, error) {
	if l := dataloaders.For(ctx); l != nil {
		return l.GroupedMemberships.Load(ctx, memberID)()
	}
	m, err := r.CatalogSvc.ListGroupedMemberLinksByMemberIDs(ctx, []uuid.UUID{memberID})
	if err != nil {
		return nil, err
	}
	return m[memberID], nil
}

func (r *Resolver) buildConfigurableMembership(ctx context.Context, productIDStr string) (*model1.ConfigurableMembership, error) {
	productID, err := uuid.Parse(productIDStr)
	if err != nil {
		return nil, nil
	}
	link, err := r.loadConfigurableParentLink(ctx, productID)
	if err != nil {
		return nil, err
	}
	if link.ParentID == uuid.Nil {
		return nil, nil // not a variant
	}
	parent, err := r.fetchMembershipProduct(ctx, link.ParentID)
	if err != nil {
		return nil, err
	}
	if parent == nil {
		return nil, nil // parent hidden on this store
	}
	rendered, err := r.renderProduct(ctx, *parent)
	if err != nil {
		return nil, err
	}
	optIDs := make(map[string]struct{}, len(rendered.ConfigurableOptions)) // optionValues (this variant's super-attribute values) and siblings both come off the parent's already-built, visibility-filtered children
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
	return &model1.ConfigurableMembership{
		Parent:       rendered,
		VariantMode:  rendered.VariantMode,
		OptionValues: optionValues,
		SortOrder:    link.VariantSortOrder,
		Siblings:     siblings,
	}, nil
}

func (r *Resolver) buildBundleMemberships(ctx context.Context, productIDStr string) ([]*model1.BundleMembership, error) {
	out := []*model1.BundleMembership{}
	productID, err := uuid.Parse(productIDStr)
	if err != nil {
		return out, nil
	}
	links, err := r.loadBundleLinks(ctx, productID)
	if err != nil {
		return nil, err
	}
	for _, link := range links {
		bundle, err := r.fetchMembershipProduct(ctx, link.BundleID)
		if err != nil {
			return nil, err
		}
		if bundle == nil {
			continue // bundle hidden on this store
		}
		rendered, err := r.renderProduct(ctx, *bundle)
		if err != nil {
			return nil, err
		}
		optID := link.OptionID.String() // find this product's option on the rendered bundle - siblings are that option's other visible selections
		var optModel *model.BundleOption
		for _, o := range rendered.BundleOptions {
			if o.ID == optID {
				optModel = o
				break
			}
		}
		if optModel == nil {
			continue // option not visible / dropped by required-option gate
		}
		siblings := make([]*model.BundleSelection, 0, len(optModel.Selections))
		for _, s := range optModel.Selections {
			if s.Product != nil && s.Product.ID == productIDStr {
				continue
			}
			siblings = append(siblings, s)
		}
		out = append(out, &model1.BundleMembership{
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

func (r *Resolver) buildGroupedMemberships(ctx context.Context, productIDStr string) ([]*model1.GroupedMembership, error) {
	out := []*model1.GroupedMembership{}
	productID, err := uuid.Parse(productIDStr)
	if err != nil {
		return out, nil
	}
	links, err := r.loadGroupedLinks(ctx, productID)
	if err != nil {
		return nil, err
	}
	for _, link := range links {
		parent, err := r.fetchMembershipProduct(ctx, link.ParentID)
		if err != nil {
			return nil, err
		}
		if parent == nil {
			continue // grouped parent hidden on this store
		}
		rendered, err := r.renderProduct(ctx, *parent)
		if err != nil {
			return nil, err
		}
		siblings := make([]*model.GroupedMember, 0, len(rendered.GroupedMembers)) // the grouped parent's other members, already rendered
		for _, m := range rendered.GroupedMembers {
			if m.Member != nil && m.Member.ID == productIDStr {
				continue
			}
			siblings = append(siblings, m)
		}
		out = append(out, &model1.GroupedMembership{
			GroupedProduct:  rendered,
			DefaultQuantity: link.DefaultQuantity,
			SortOrder:       link.SortOrder,
			Siblings:        siblings,
		})
	}
	return out, nil
}
