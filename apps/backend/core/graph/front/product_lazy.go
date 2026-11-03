package front

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/dataloaders"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/pricing"
)

func (r *Resolver) productRowOf(ctx context.Context, obj *model.Product) (catalog.Product, error) {
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return catalog.Product{}, err
	}
	var row catalog.Product
	if l := dataloaders.For(ctx); l != nil && l.ProductRow != nil {
		row, err = l.LoadProductRow(ctx, pid)
	} else {
		var byID map[uuid.UUID]catalog.Product
		byID, err = r.CatalogSvc.ListProductRowsByIDs(ctx, []uuid.UUID{pid})
		row = byID[pid]
	}
	if err != nil {
		return catalog.Product{}, err
	}
	if row.ID == uuid.Nil {
		return catalog.Product{}, fmt.Errorf("product %s: %w", pid, catalog.ErrProductNotFound)
	}
	return row, nil
}

func (r *Resolver) channelListingOf(ctx context.Context, productID uuid.UUID) *catalog.ProductChannelListing {
	sid := channelIDFromContext(ctx)
	if sid == nil {
		return nil
	}
	rows, err := r.getProductChannelListings(ctx, productID)
	if err != nil {
		return nil
	}
	for i := range rows {
		if rows[i].ChannelID == *sid {
			return &rows[i]
		}
	}
	return nil
}

// Resolved like the cart's ResolveSimplePricing, so the shelf price is the charged price
func (r *Resolver) productPricing(ctx context.Context, obj *model.Product) (pricing.Resolved, error) {
	row, err := r.productRowOf(ctx, obj)
	if err != nil {
		return pricing.Resolved{}, err
	}
	return pricing.Resolve(catalog.EffectivePricingInputs(row, r.channelListingOf(ctx, row.ID)), time.Now()), nil
}

// The request store's listing for a bundle - nil for any other kind, which prices as a plain sum of nothing
func (r *Resolver) bundleListingOf(ctx context.Context, obj *model.Product) (*catalog.ProductChannelListing, error) {
	row, err := r.productRowOf(ctx, obj)
	if err != nil || row.Kind != catalog.KindBundle {
		return nil, err
	}
	return r.channelListingOf(ctx, row.ID), nil
}

func (r *Resolver) productURL(ctx context.Context, obj *model.Product) (string, error) {
	row, err := r.productRowOf(ctx, obj)
	if err != nil {
		return "", err
	}
	if lst := r.channelListingOf(ctx, row.ID); lst != nil && lst.URL != nil {
		return *lst.URL, nil
	}
	return row.URL, nil
}

func (r *Resolver) loadConfigurableOptions(ctx context.Context, pid uuid.UUID) ([]catalog.Attribute, error) {
	if l := dataloaders.For(ctx); l != nil && l.ConfigurableOptions != nil {
		return l.LoadConfigurableOptions(ctx, pid)
	}
	byID, err := r.CatalogSvc.ListConfigurableOptionsByProductIDs(ctx, []uuid.UUID{pid})
	return byID[pid], err
}

// Variant builders set these, since they depend on the parent
func (r *Resolver) productConfigurableOptions(ctx context.Context, obj *model.Product) ([]*model.Attribute, error) {
	if obj.ConfigurableOptions != nil {
		return obj.ConfigurableOptions, nil
	}
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, err
	}
	opts, err := r.loadConfigurableOptions(ctx, pid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Attribute, len(opts))
	for i, a := range opts {
		out[i] = toAttributeModel(a)
	}
	attrNames, valueNames := r.getAttributeNameMaps(ctx, r.activeLocale(ctx))
	applyAttributeModelNames(&model.Product{ConfigurableOptions: out}, attrNames, valueNames)
	return out, nil
}

func (r *Resolver) productAttributeValues(ctx context.Context, obj *model.Product) ([]*model.ProductAttributeValue, error) {
	if obj.AttributeValues != nil {
		return obj.AttributeValues, nil
	}
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, err
	}
	c := getRenderCache(ctx)
	key := pid.String() + ":" + r.activeLocale(ctx)
	return renderMemo(c, "attrvalues:"+key,
		func() ([]*model.ProductAttributeValue, bool) {
			c.mu.RLock()
			defer c.mu.RUnlock()
			v, ok := c.attributeValues[key]
			return v, ok
		},
		func() ([]*model.ProductAttributeValue, error) { return r.buildAttributeValues(ctx, obj, pid) },
		func(v []*model.ProductAttributeValue) {
			c.mu.Lock()
			c.attributeValues[key] = v
			c.mu.Unlock()
		})
}

func (r *Resolver) buildAttributeValues(ctx context.Context, obj *model.Product, pid uuid.UUID) ([]*model.ProductAttributeValue, error) {
	row, err := r.productRowOf(ctx, obj)
	if err != nil {
		return nil, err
	}
	pt, err := r.getProductType(ctx, row.ProductTypeID)
	if err != nil {
		return nil, err
	}
	var sels []catalog.ProductAttributeSelection
	var refs []catalog.ProductAttributeReference
	if l := dataloaders.For(ctx); l != nil && l.ProductSelections != nil {
		if sels, err = l.LoadProductSelections(ctx, pid); err != nil {
			return nil, err
		}
		if refs, err = l.LoadProductReferences(ctx, pid); err != nil {
			return nil, err
		}
	} else {
		selsBy, err := r.CatalogSvc.LoadProductsAttributeSelections(ctx, []uuid.UUID{pid})
		if err != nil {
			return nil, err
		}
		refsBy, err := r.CatalogSvc.LoadProductsAttributeReferences(ctx, []uuid.UUID{pid})
		if err != nil {
			return nil, err
		}
		sels, refs = selsBy[pid], refsBy[pid]
	}
	values := buildProductAttributeValues(pt, row.Attributes, sels, r.hydrateProductReferences(ctx, refs))
	// The pre-filter values are stashed so promotion display rules keep matching private attributes
	r.stashPromotionAttrKVs(ctx, pid, values)
	opts, err := r.loadConfigurableOptions(ctx, pid)
	if err != nil {
		return nil, err
	}
	axis := make(map[string]bool, len(opts))
	for _, a := range opts {
		axis[a.ID.String()] = true
	}
	values = keepPublicAttributeValues(values, axis)
	if values == nil {
		values = []*model.ProductAttributeValue{}
	}
	locale := r.activeLocale(ctx)
	attrNames, valueNames := r.getAttributeNameMaps(ctx, locale)
	applyAttributeModelNames(&model.Product{AttributeValues: values}, attrNames, valueNames)
	if !r.primeGroupOnce(ctx, "locale-values", pid, func(ids []uuid.UUID) error { r.primeProductAttributeLocaleValues(ctx, ids); return nil }) {
		r.primeProductAttributeLocaleValues(ctx, []uuid.UUID{pid})
	}
	overrides := map[string]string{}
	for _, lv := range r.cachedProductLocaleValues(ctx, pid) {
		if lv.Locale == locale {
			overrides[lv.AttributeID.String()] = lv.Value
		}
	}
	applyScalarLocaleValues(values, overrides)
	return values, nil
}

func (r *Resolver) promotionAttrKVs(ctx context.Context, obj *model.Product, pid uuid.UUID) (map[string]string, bool) {
	if kvs, ok := r.cachedPromotionAttrKVs(ctx, pid); ok {
		return kvs, true
	}
	if _, err := r.productAttributeValues(ctx, obj); err != nil {
		return nil, false
	}
	return r.cachedPromotionAttrKVs(ctx, pid)
}

type childVisibility struct {
	had     bool
	visible []uuid.UUID
}

func (r *Resolver) computeChildVisibility(ctx context.Context, parents []uuid.UUID) (map[uuid.UUID]childVisibility, error) {
	out := make(map[uuid.UUID]childVisibility, len(parents))
	if len(parents) == 0 {
		return out, nil
	}
	byParent, err := r.CatalogSvc.ListProductChildIDsByParentIDs(ctx, parents)
	if err != nil {
		return nil, err
	}
	if l := dataloaders.For(ctx); l != nil && l.ChildIDs != nil { // the wrapper inStock aggregate counts every child id, published or not
		for _, pid := range parents {
			l.ChildIDs.Prime(ctx, pid, byParent[pid])
		}
	}
	var all []uuid.UUID
	for _, ids := range byParent {
		all = append(all, ids...)
	}
	visible := map[uuid.UUID]struct{}{}
	if sid := channelIDFromContext(ctx); sid != nil && len(all) > 0 {
		if visible, err = r.CatalogSvc.FilterVisibleProductIDsForChannel(ctx, all, *sid); err != nil {
			return nil, err
		}
	}
	for _, pid := range parents {
		v := childVisibility{had: len(byParent[pid]) > 0, visible: []uuid.UUID{}}
		for _, id := range byParent[pid] {
			if _, ok := visible[id]; ok {
				v.visible = append(v.visible, id)
			}
		}
		out[pid] = v
	}
	return out, nil
}

func (r *Resolver) primeChildVisibility(ctx context.Context, products []catalog.Product) error {
	c := getRenderCache(ctx)
	if c == nil {
		return nil
	}
	var parents []uuid.UUID
	c.mu.RLock()
	for _, p := range products {
		if p.Kind != catalog.KindConfigurable && p.Kind != catalog.KindGrouped {
			continue
		}
		if _, ok := c.childVisibility[p.ID]; !ok {
			parents = append(parents, p.ID)
		}
	}
	c.mu.RUnlock()
	byParent, err := r.computeChildVisibility(ctx, parents)
	if err != nil {
		return err
	}
	c.mu.Lock()
	maps.Copy(c.childVisibility, byParent)
	c.mu.Unlock()
	return nil
}

func (r *Resolver) childVisibilityOf(ctx context.Context, p catalog.Product) (childVisibility, error) {
	c := getRenderCache(ctx)
	if c != nil {
		c.mu.RLock()
		v, ok := c.childVisibility[p.ID]
		c.mu.RUnlock()
		if ok {
			return v, nil
		}
	}
	byParent, err := r.computeChildVisibility(ctx, []uuid.UUID{p.ID})
	if err != nil {
		return childVisibility{}, err
	}
	if c != nil {
		c.mu.Lock()
		c.childVisibility[p.ID] = byParent[p.ID]
		c.mu.Unlock()
	}
	return byParent[p.ID], nil
}
