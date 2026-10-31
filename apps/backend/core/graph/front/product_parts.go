package front

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/dataloaders"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

func (r *Resolver) loadVisibleChildren(ctx context.Context, parent *model.Product, parentID uuid.UUID) ([]catalog.Product, error) {
	v, err := r.childVisibilityOf(ctx, catalog.Product{ID: parentID, Kind: catalog.ProductKind(strings.ToLower(string(parent.Kind)))})
	if err != nil {
		return nil, err
	}
	if len(v.visible) == 0 {
		return nil, nil
	}
	var rows []catalog.Product
	if l := dataloaders.For(ctx); l != nil && l.ChildRows != nil {
		rows, err = l.ChildRows.Load(ctx, parentID)()
	} else {
		rows, err = r.CatalogSvc.ListProductChildren(ctx, parentID)
	}
	if err != nil {
		return nil, err
	}
	visible := make(map[uuid.UUID]struct{}, len(v.visible))
	for _, id := range v.visible {
		visible[id] = struct{}{}
	}
	kept := make([]catalog.Product, 0, len(v.visible))
	for _, row := range rows {
		if _, ok := visible[row.ID]; ok {
			kept = append(kept, row)
		}
	}
	return kept, nil
}

// The variant picker needs the parent's private axis attributes
func (r *Resolver) productChildren(ctx context.Context, obj *model.Product) ([]*model.Product, error) {
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, err
	}
	kids, err := r.loadVisibleChildren(ctx, obj, pid)
	if err != nil {
		return nil, err
	}
	if len(kids) == 0 {
		return []*model.Product{}, nil
	}
	ids := make([]uuid.UUID, len(kids))
	for i, k := range kids {
		ids[i] = k.ID
	}
	if obj.Kind == model.ProductKindConfigurable {
		dataloaders.For(ctx).RegisterVariants(pid, ids)
	}
	opts, err := r.loadConfigurableOptions(ctx, pid)
	if err != nil {
		return nil, err
	}
	axis := make(map[string]bool, len(opts))
	for _, a := range opts {
		axis[a.ID.String()] = true
	}
	out, err := r.renderChildren(ctx, kids, axis)
	if err != nil {
		return nil, err
	}
	r.localizeChildren(ctx, out, ids)
	return out, nil
}

func (r *Resolver) localizeChildren(ctx context.Context, children []*model.Product, ids []uuid.UUID) {
	locale := r.activeLocale(ctx)
	attrNames, valueNames := r.getAttributeNameMaps(ctx, locale)
	if len(attrNames) > 0 || len(valueNames) > 0 {
		for _, c := range children {
			applyAttributeModelNames(c, attrNames, valueNames)
		}
	}
	r.primeProductAttributeLocaleValues(ctx, ids)
	for _, c := range children {
		cid, err := uuid.Parse(c.ID)
		if err != nil {
			continue
		}
		overrides := map[string]string{}
		for _, row := range r.cachedProductLocaleValues(ctx, cid) {
			if row.Locale == locale {
				overrides[row.AttributeID.String()] = row.Value
			}
		}
		applyScalarLocaleValues(c.AttributeValues, overrides)
	}
}

func (r *Resolver) productBundleOptions(ctx context.Context, obj *model.Product) ([]*model.BundleOption, error) {
	if obj.Kind != model.ProductKindBundle {
		return []*model.BundleOption{}, nil
	}
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, err
	}
	return r.renderBundleOptions(ctx, pid)
}

func (r *Resolver) productGroupedMembers(ctx context.Context, obj *model.Product) ([]*model.GroupedMember, error) {
	if obj.Kind != model.ProductKindGrouped {
		return []*model.GroupedMember{}, nil
	}
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, err
	}
	return r.renderGroupedMembers(ctx, pid)
}

// A kit's whole contents, so the box is described truthfully - a component this store doesn't sell keeps its name but
// loses its url, since its page would 404 here
func (r *Resolver) productKitComponents(ctx context.Context, obj *model.Product) ([]*model.KitComponent, error) {
	if obj.Kind != model.ProductKindKit {
		return []*model.KitComponent{}, nil
	}
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, err
	}
	comps, err := r.CatalogSvc.ListKitComponents(ctx, pid, r.activeLocale(ctx))
	if err != nil {
		return nil, fmt.Errorf("load kit components: %w", err)
	}
	ids := make([]uuid.UUID, len(comps))
	for i, c := range comps {
		ids[i] = c.ComponentProductID
	}
	sold, err := keepVisibleByChannel(ctx, r, slices.Clone(ids), func(id uuid.UUID) uuid.UUID { return id })
	if err != nil {
		return nil, fmt.Errorf("filter kit components: %w", err)
	}
	out := make([]*model.KitComponent, len(comps))
	for i, c := range comps {
		if !slices.Contains(sold, c.ComponentProductID) {
			c.ComponentURL = ""
		}
		prod := &model.Product{
			ID:                  c.ComponentProductID.String(),
			URL:                 c.ComponentURL,
			Sku:                 c.ComponentSKU,
			Kind:                model.ProductKindSimple,
			DefaultPrice:        int32(c.ComponentDefaultPrice),
			Price:               int32(c.ComponentDefaultPrice),
			DefaultCurrencyCode: c.ComponentCurrency,
		}
		if c.ComponentName != "" {
			name := c.ComponentName
			prod.Name = &name
		}
		out[i] = &model.KitComponent{ID: c.ID.String(), Component: prod, Quantity: c.Quantity, SortOrder: c.SortOrder}
	}
	return out, nil
}
