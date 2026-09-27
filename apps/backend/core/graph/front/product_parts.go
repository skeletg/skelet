package front

import (
	"context"
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

// A variant keeps its parent's private axis attributes - the variant picker needs them
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
