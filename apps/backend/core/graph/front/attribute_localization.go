package front

// Locale + channel post-processing for built Product models - the catalog stores attribute names, value names, plain_text scalars and slugs/url_paths globally, the storefront view localizes them per request Attribute.name / AttributeValue.name <- catalog_attribute_content / catalog_attribute_value_content for the active locale (an empty name means untranslated, keep the global one) plain_text scalarValue <- catalog_product_attribute_locale_values, Product.slug / urlPath <- the active channel's listing overrides, and it is all applied at the model edge (after toProductBaseModel) so the domain structs and every admin path keep returning canonical data Everything rides the request-scoped render cache - names built once per (request, locale), scalars batch-primed per list, listings already primed - so list pages pay O(1) extra queries, not O(products)

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

// Two batch queries over the full content tables, memoized per (request, locale) - attribute metadata is small and every rendered product on the page shares the result
func (r *Resolver) getAttributeNameMaps(ctx context.Context, locale string) (map[string]string, map[string]string) {
	c := getRenderCache(ctx)
	if c != nil {
		c.mu.RLock()
		m, ok := c.attrNameMaps[locale]
		c.mu.RUnlock()
		if ok {
			return m.attrNames, m.valueNames
		}
	}

	attrs, err := r.getAttributesCached(ctx)
	if err != nil {
		return nil, nil
	}
	var attrIDs, valueIDs []uuid.UUID
	for _, a := range attrs {
		attrIDs = append(attrIDs, a.ID)
		for _, v := range a.Values {
			valueIDs = append(valueIDs, v.ID)
		}
	}

	attrNames := map[string]string{}
	if len(attrIDs) > 0 {
		if rows, err := r.CatalogSvc.ListAttributeContentByAttributeIDs(ctx, attrIDs); err == nil {
			for _, row := range rows {
				if row.Locale == locale && row.Name != "" {
					attrNames[row.AttributeID.String()] = row.Name
				}
			}
		}
	}
	valueNames := map[string]string{}
	if len(valueIDs) > 0 {
		if rows, err := r.CatalogSvc.ListAttributeValueContentByValueIDs(ctx, valueIDs); err == nil {
			for _, row := range rows {
				if row.Locale == locale && row.Name != "" {
					valueNames[row.ValueID.String()] = row.Name
				}
			}
		}
	}

	if c != nil {
		c.mu.Lock()
		c.attrNameMaps[locale] = attrNameMaps{attrNames: attrNames, valueNames: valueNames}
		c.mu.Unlock()
	}
	return attrNames, valueNames
}

// Rewrites in place, children included - call after toProductBaseModel, and pass the domain product because the UUID is needed
func (r *Resolver) localizeProductModel(ctx context.Context, p catalog.Product, m *model.Product) {
	locale := r.activeLocale(ctx)

	attrNames, valueNames := r.getAttributeNameMaps(ctx, locale)
	if len(attrNames) > 0 || len(valueNames) > 0 {
		applyAttributeModelNames(m, attrNames, valueNames)
	}

	ids := make([]uuid.UUID, 0, 1+len(p.Children))
	ids = append(ids, p.ID)
	for _, c := range p.Children {
		ids = append(ids, c.ID)
	}
	r.primeProductAttributeLocaleValues(ctx, ids)
	byProduct := map[string]map[string]string{}
	for _, id := range ids {
		for _, row := range r.cachedProductLocaleValues(ctx, id) {
			if row.Locale != locale {
				continue
			}
			pid := row.ProductID.String()
			if byProduct[pid] == nil {
				byProduct[pid] = map[string]string{}
			}
			byProduct[pid][row.AttributeID.String()] = row.Value
		}
	}
	if len(byProduct) > 0 {
		applyScalarLocaleValues(m.AttributeValues, byProduct[m.ID])
		for _, child := range m.Children {
			applyScalarLocaleValues(child.AttributeValues, byProduct[child.ID])
		}
	}

	if storeID := channelIDFromContext(ctx); storeID != nil { // channel slug/url_path overrides ride the render cache (primed in list resolvers, loader-backed otherwise) - no per-product PK query here
		if rows, err := r.getProductChannelListings(ctx, p.ID); err == nil {
			for _, lst := range rows {
				if lst.ChannelID != *storeID {
					continue
				}
				if lst.URL != nil {
					m.URL = *lst.URL
				}
				break
			}
		}
	}
}

func (r *Resolver) localizeAttributeModels(ctx context.Context, attrs []*model.Attribute) {
	locale := r.activeLocale(ctx)
	attrNames, valueNames := r.getAttributeNameMaps(ctx, locale)
	if len(attrNames) == 0 && len(valueNames) == 0 {
		return
	}
	for _, a := range attrs {
		if a == nil {
			continue
		}
		if n, ok := attrNames[a.ID]; ok {
			a.Name = n
		}
		for _, v := range a.Values {
			if v == nil {
				continue
			}
			if n, ok := valueNames[v.ID]; ok {
				v.Name = n
			}
		}
	}
}

// Model pointers are shared between the parent's option list and child entries, so double-application is harmless - the lookup is an idempotent map hit by ID
func applyAttributeModelNames(m *model.Product, attrNames, valueNames map[string]string) {
	renameAttr := func(a *model.Attribute) {
		if a == nil {
			return
		}
		if n, ok := attrNames[a.ID]; ok {
			a.Name = n
		}
		for _, v := range a.Values {
			if v == nil {
				continue
			}
			if n, ok := valueNames[v.ID]; ok {
				v.Name = n
			}
		}
	}
	walk := func(prod *model.Product) {
		if prod == nil {
			return
		}
		for _, av := range prod.AttributeValues {
			if av == nil {
				continue
			}
			renameAttr(av.Attribute)
			for _, sv := range av.SelectedValues {
				if sv == nil {
					continue
				}
				if n, ok := valueNames[sv.ID]; ok {
					sv.Name = n
				}
			}
		}
		for _, opt := range prod.ConfigurableOptions {
			renameAttr(opt)
		}
	}
	walk(m)
	for _, child := range m.Children {
		walk(child)
	}
}

// overrides is keyed by attribute UUID string - nil or missing keys leave the canonical value in place
func applyScalarLocaleValues(entries []*model.ProductAttributeValue, overrides map[string]string) {
	if len(overrides) == 0 {
		return
	}
	for _, e := range entries {
		if e == nil || e.Attribute == nil || e.ScalarValue == nil {
			continue
		}
		if e.Attribute.InputType != model.AttributeInputTypePlainText {
			continue
		}
		if v, ok := overrides[e.Attribute.ID]; ok {
			e.ScalarValue = v
		}
	}
}
