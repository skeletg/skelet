package front

import (
	"sort"

	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/skeletg/skelet/apps/backend/core/internal/search"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/dataloaders"
	model1 "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/graph/modelconv"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
)

// Reads the request-scoped cache (primed by list resolvers) only when channelID matches the store it was primed against, else falls back to a single-product call
// The storefront resolves exactly one store per request, so an admin-style channelID override on inStock/salableQuantity correctly bypasses the cache
func (r *productResolver) salableFor(ctx context.Context, productID, channelID uuid.UUID) (float64, error) {
	if storeID := channelIDFromContext(ctx); storeID != nil && *storeID == channelID {
		if v, ok := r.cachedSalable(ctx, productID); ok {
			return v, nil
		}
		if r.primeGroupOnce(ctx, "stock", productID, func(ids []uuid.UUID) error { return r.primeStock(ctx, ids, channelID) }) {
			if v, ok := r.cachedSalable(ctx, productID); ok {
				return v, nil
			}
		}
	}
	return r.InventorySvc.Salable(ctx, productID, channelID)
}

// A wrapper carries no inventory_items row, so this aggregates children: configurable/grouped are in stock when ANY child is sellable, a bundle only when every option can meet its minimum from selections sellable and listed here
// No children means out of stock, and for configurable/grouped every child row counts even ones unpublished on this store - an over-approximation that errs toward showing the parent available
func (r *productResolver) wrapperInStock(ctx context.Context, kind model.ProductKind, pid, channelID uuid.UUID) (bool, error) {
	switch kind {
	case model.ProductKindConfigurable:
		ids, err := r.loadChildIDs(ctx, pid)
		if err != nil {
			return false, err
		}
		sellable, err := r.sellableByIDs(ctx, ids, channelID)
		if err != nil {
			return false, err
		}
		return anyTrue(sellable), nil

	case model.ProductKindGrouped:
		members, err := r.CatalogSvc.ListGroupedMembers(ctx, pid, r.activeLocale(ctx))
		if err != nil {
			return false, err
		}
		ids := make([]uuid.UUID, len(members))
		for i, m := range members {
			ids[i] = m.MemberID
		}
		sellable, err := r.sellableByIDs(ctx, ids, channelID)
		if err != nil {
			return false, err
		}
		return anyTrue(sellable), nil

	case model.ProductKindKit:
		// A kit ships every component, so ALL must be sellable - the any-of rule of the other wrappers would call a kit with one dead component in stock
		comps, err := r.CatalogSvc.KitComponentRefs(ctx, pid)
		if err != nil {
			return false, err
		}
		if len(comps) == 0 {
			// No components = nothing to gate, same as an untracked simple
			return true, nil
		}
		ids := make([]uuid.UUID, len(comps))
		for i, c := range comps {
			ids[i] = c.ComponentProductID
		}
		sellable, err := r.sellableByIDs(ctx, ids, channelID)
		if err != nil {
			return false, err
		}
		return allTrue(sellable, ids), nil

	case model.ProductKindBundle:
		opts, err := r.rawBundleOptions(ctx, pid, r.activeLocale(ctx))
		if err != nil {
			return false, err
		}
		// One batched lookup over every selection - a pick has to be both sellable and listed on this store, each option needs as many of those as add-to-cart demands, and the bundle as a whole needs at least one, since an empty bundle can't be added
		ids := make([]uuid.UUID, 0, len(opts))
		for _, opt := range opts {
			for _, sel := range opt.Selections {
				ids = append(ids, sel.ProductID)
			}
		}
		sellable, err := r.sellableByIDs(ctx, ids, channelID)
		if err != nil {
			return false, err
		}
		listed, err := r.CatalogSvc.FilterVisibleProductIDsForChannel(ctx, ids, channelID)
		if err != nil {
			return false, err
		}
		return bundleBuyable(opts, func(id uuid.UUID) bool {
			_, ok := listed[id]
			return ok && sellable[id]
		}), nil
	}
	return true, nil
}

// Whether a customer could add the bundle at all, given which selection products they could buy - add-to-cart's rules: each option needs its minimum (1 when required), and the bundle needs at least one pick overall
func bundleBuyable(opts []catalog.BundleOption, canBuy func(uuid.UUID) bool) bool {
	buyable := false
	for _, opt := range opts {
		need := opt.MinSelections
		if opt.IsRequired && need < 1 {
			need = 1
		}
		var have int32
		for _, sel := range opt.Selections {
			if canBuy(sel.ProductID) {
				have++
			}
		}
		if have < need {
			return false
		}
		buyable = buyable || have > 0
	}
	return buyable
}

// Mirrors the simple branch of InStock over a batched salable lookup - salable<0 (untracked sentinel) or >0 is sellable, and exactly zero falls back to the item's backorderable flag
func (r *productResolver) sellableByIDs(ctx context.Context, ids []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]bool, error) {
	out := make(map[uuid.UUID]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	bySalable, backorderable, err := r.InventorySvc.StockByProductIDs(ctx, ids, channelID)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if _, done := out[id]; done {
			continue // de-dupe repeated ids across bundle options
		}
		s, ok := bySalable[id]
		if !ok || s < 0 || s > 0 {
			out[id] = true // missing key (untracked sentinel), unbounded, or positive
			continue
		}
		out[id] = backorderable[id]
	}
	return out, nil
}

func anyTrue(m map[uuid.UUID]bool) bool {
	for _, v := range m {
		if v {
			return true
		}
	}
	return false
}

// Every id must be sellable - the kit gate, since one missing component blocks the whole kit
func allTrue(m map[uuid.UUID]bool, ids []uuid.UUID) bool {
	for _, id := range ids {
		if !m[id] {
			return false
		}
	}
	return true
}

// Loader-backed body of PromotionalDisplays - walks the in-process per-store rules cache and routes every per-rule lookup through a loader, so N sibling resolvers on a PLP cost one conditions batch, one i18n contents batch and one media batch total
// The non-loader fallback in catalog.resolvers.go ran roughly N queries per product instead
func (r *productResolver) promotionalDisplaysViaLoaders(ctx context.Context, loaders *dataloaders.Loaders, storeID *uuid.UUID, candidate promotion.ProductCandidate) ([]*model1.PromotionalDisplay, error) {
	rules, err := r.PromotionSvc.ActiveRulesForChannel(ctx, storeID)
	if err != nil || len(rules) == 0 {
		return nil, nil
	}
	out := make([]*model1.PromotionalDisplay, 0, len(rules))
	for i := range rules {
		rule := rules[i]
		// Drop rules with no creative - same filter as DisplaysForProduct
		if rule.LabelMediaID == nil && rule.BannerMediaID == nil && rule.PillText == "" {
			continue
		}
		conds, err := loaders.PromotionConditions.Load(ctx, rule.ID)()
		if err != nil {
			continue
		}
		rule.Conditions = conds
		if !promotion.MatchesProduct(rule, candidate) {
			continue
		}
		label := rule.PublicLabel
		if content, err := loaders.PromotionRuleContent.Load(ctx, rule.ID)(); err == nil && content.Label != "" {
			label = content.Label
		}
		out = append(out, &model1.PromotionalDisplay{
			RuleID:        rule.ID.String(),
			RuleSlug:      rule.Slug,
			PublicLabel:   label,
			LabelMedia:    r.loadPromotionMediaViaLoader(ctx, loaders, rule.LabelMediaID),
			BannerMedia:   r.loadPromotionMediaViaLoader(ctx, loaders, rule.BannerMediaID),
			PillText:      rule.PillText,
			PillTextColor: rule.PillTextColor,
			PillBgColor:   rule.PillBgColor,
		})
	}
	return out, nil
}

// nil on a missing id, asset-not-found or loader error, so the surrounding display still renders without media
func (r *productResolver) loadPromotionMediaViaLoader(ctx context.Context, loaders *dataloaders.Loaders, mediaID *uuid.UUID) *model.ProductMedia {
	if mediaID == nil {
		return nil
	}
	asset, err := loaders.MediaAsset.Load(ctx, *mediaID)()
	if err != nil || asset.ID == uuid.Nil {
		return nil
	}
	var width, height int32
	if asset.Width != nil {
		width = *asset.Width
	}
	if asset.Height != nil {
		height = *asset.Height
	}
	return &model.ProductMedia{
		ID:        asset.ID.String(),
		URL:       asset.URL,
		AltText:   asset.AltText,
		MimeType:  asset.MimeType,
		Width:     width,
		Height:    height,
		SizeBytes: int32(asset.SizeBytes),
		Status:    model.ProductMediaStatusReady,
		CreatedAt: asset.CreatedAt,
	}
}

// Projects a medialibrary asset into the ProductMedia shape the storefront already consumes - nil on any error so the caller still renders the rule's text
func (r *productResolver) loadPromotionMedia(ctx context.Context, mediaID *uuid.UUID) *model.ProductMedia {
	if mediaID == nil || r.MediaLibrarySvc == nil {
		return nil
	}
	asset, err := r.MediaLibrarySvc.GetAsset(ctx, *mediaID)
	if err != nil {
		return nil
	}
	var width, height int32
	if asset.Width != nil {
		width = *asset.Width
	}
	if asset.Height != nil {
		height = *asset.Height
	}
	return &model.ProductMedia{
		ID:        asset.ID.String(),
		URL:       asset.URL,
		AltText:   asset.AltText,
		MimeType:  asset.MimeType,
		Width:     width,
		Height:    height,
		SizeBytes: int32(asset.SizeBytes),
		Status:    model.ProductMediaStatusReady,
		CreatedAt: asset.CreatedAt,
	}
}

// Categories and attribute values are already hydrated on the model by the parent resolver, so no extra catalog round-trip
func productCandidateFromModel(p *model.Product) (promotion.ProductCandidate, error) {
	cats := make([]uuid.UUID, 0, len(p.Categories))
	for _, c := range p.Categories {
		if c == nil {
			continue
		}
		id, err := uuid.Parse(c.ID)
		if err != nil {
			continue
		}
		cats = append(cats, id)
	}
	attrs := attributeKVsFromModelValues(p.AttributeValues)
	return promotion.ProductCandidate{
		ProductSKU: p.Sku,
		// the GraphQL enum is SCREAMING but core.product_kind params only ever hold lowercase kinds and compare case-sensitively, so an unlowered kind matched nothing
		Kind:         strings.ToLower(string(p.Kind)),
		CategoryIDs:  cats,
		AttributeKVs: attrs,
	}, nil
}

func attributeKVsFromModelValues(values []*model.ProductAttributeValue) map[string]string {
	attrs := make(map[string]string, len(values))
	for _, av := range values {
		if av == nil || av.Attribute == nil {
			continue
		}
		slug := av.Attribute.Slug
		if slug == "" {
			continue
		}
		if len(av.SelectedValues) > 0 {
			parts := make([]string, 0, len(av.SelectedValues))
			for _, sv := range av.SelectedValues {
				if sv != nil {
					parts = append(parts, sv.Slug)
				}
			}
			if len(parts) > 0 {
				attrs[slug] = strings.Join(parts, ",")
			}
		} else if av.ScalarValue != nil {
			attrs[slug] = fmt.Sprintf("%v", av.ScalarValue)
		}
	}
	return attrs
}

// Private attributes are dropped from this storefront projection - a configurable axis still reaches the client through configurableOptions and keepPublicAttributeValues' exemption
func toProductTypeModel(pt catalog.ProductType) *model.ProductType {
	attrs := make([]*model.ProductTypeAttribute, 0, len(pt.Attributes))
	for _, a := range pt.Attributes {
		if !a.Attribute.IsPublic {
			continue
		}
		attrs = append(attrs, toProductTypeAttributeModel(a))
	}
	groups := make([]*model.AttributeGroup, len(pt.Groups))
	for i, g := range pt.Groups {
		groups[i] = toAttributeGroupModel(g)
	}
	return &model.ProductType{
		ID:                 pt.ID.String(),
		Slug:               pt.Slug,
		Name:               pt.Name,
		IsDigital:          pt.IsDigital,
		IsShippingRequired: pt.IsShippingRequired,
		URLTemplate:        pt.URLTemplate,
		TracksInventory:    pt.TracksInventory,
		Attributes:         attrs,
		Groups:             groups,
		Metadata:           toMetadataItems(pt.Metadata),
		PrivateMetadata:    toMetadataItems(pt.PrivateMetadata),
		CreatedAt:          pt.CreatedAt,
		UpdatedAt:          pt.UpdatedAt,
	}
}

func toProductTypeAttributeModel(pta catalog.ProductTypeAttribute) *model.ProductTypeAttribute {
	m := &model.ProductTypeAttribute{
		Attribute: toAttributeModel(pta.Attribute),
		SortOrder: pta.SortOrder,
	}
	if pta.GroupID != nil {
		id := pta.GroupID.String()
		m.GroupID = &id
	}
	return m
}

func toAttributeGroupModel(g catalog.AttributeGroup) *model.AttributeGroup {
	attrs := make([]*model.ProductTypeAttribute, 0, len(g.Attributes))
	for _, a := range g.Attributes {
		if !a.Attribute.IsPublic {
			continue
		}
		attrs = append(attrs, toProductTypeAttributeModel(a))
	}
	return &model.AttributeGroup{
		ID:         g.ID.String(),
		Name:       g.Name,
		SortOrder:  g.SortOrder,
		Attributes: attrs,
	}
}

func toAttributeModel(a catalog.Attribute) *model.Attribute {
	values := make([]*model.AttributeValue, len(a.Values))
	for i, v := range a.Values {
		values[i] = toAttributeValueModel(v)
	}
	return &model.Attribute{
		ID:                 a.ID.String(),
		Slug:               a.Slug,
		Name:               a.Name,
		InputType:          attributeInputTypeToGQL(a.InputType),
		RelationshipTarget: modelconv.ReferenceTargetToGQL(a.RelationshipTarget),
		Unit:               a.Unit,
		ValueRequired:      a.ValueRequired,
		Filterable:         a.Filterable,
		Searchable:         a.Searchable,
		IsPublic:           a.IsPublic,
		SynapseTrack:       a.SynapseTrack,
		SortOrder:          a.SortOrder,
		Values:             values,
		Metadata:           toMetadataItems(a.Metadata),
		PrivateMetadata:    toMetadataItems(a.PrivateMetadata),
		CreatedAt:          a.CreatedAt,
		UpdatedAt:          a.UpdatedAt,
	}
}

func toAttributeValueModel(v catalog.AttributeValue) *model.AttributeValue {
	return &model.AttributeValue{
		ID:        v.ID.String(),
		Slug:      v.Slug,
		Name:      v.Name,
		Value:     v.Value,
		SortOrder: v.SortOrder,
	}
}

// Scalar fields only - attribute rendering needs to load the product type, so it lives on renderProduct rather than this pure mapper
func toProductBaseModel(p catalog.Product) *model.Product {
	m := &model.Product{
		ID:                      p.ID.String(),
		ProductType:             &model.ProductType{ID: p.ProductTypeID.String()}, // carries the id for the lazy productType field
		Kind:                    model.ProductKind(strings.ToUpper(string(p.Kind))),
		VariantMode:             productVariantModeModel(p),
		DefaultPrice:            int32(p.DefaultPrice),
		DefaultCurrencyCode:     p.DefaultCurrencyCode,
		Sku:                     p.SKU,
		SeoNoindex:              p.SEONoindex,
		CanonicalURL:            p.CanonicalURL,
		URLAuto:                 p.URLAuto,
		URLTemplateOverride:     p.URLTemplateOverride,
		TracksInventoryOverride: p.TracksInventoryOverride,
		QuantityDecimals:        int32(p.QuantityDecimals),
		QuantityStep:            p.QuantityStep,
		MinOrderQty:             p.MinOrderQty,
		MaxOrderQty:             p.MaxOrderQty,
		Metadata:                toMetadataItems(p.Metadata),
		PrivateMetadata:         toMetadataItems(p.PrivateMetadata),
		CreatedAt:               p.CreatedAt,
		UpdatedAt:               p.UpdatedAt,
	}
	if p.Content != nil { // inline locale-resolved content fields, nil when that locale has none
		m.Name = &p.Content.Name
		if len(p.Content.ShortDescription) > 0 {
			var v any
			_ = json.Unmarshal(p.Content.ShortDescription, &v)
			m.ShortDescription = v
		}
		if len(p.Content.Description) > 0 {
			var v any
			_ = json.Unmarshal(p.Content.Description, &v)
			m.Description = v
			m.DescriptionRichText = v
		}
		m.MetaTitle = p.Content.MetaTitle
		m.MetaDescription = p.Content.MetaDescription
		m.MetaKeywords = p.Content.MetaKeywords
	}
	return m
}

// Not found when every variant is hidden, a required bundle option is empty or no grouped member is visible
func (r *Resolver) renderProduct(ctx context.Context, p catalog.Product) (*model.Product, error) {
	if p.Kind == catalog.KindConfigurable || p.Kind == catalog.KindGrouped {
		v, err := r.childVisibilityOf(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("filter visible children: %w", err)
		}
		if v.had && len(v.visible) == 0 {
			return nil, catalog.ErrProductNotFound
		}
		if p.Kind == catalog.KindConfigurable { // registered before any field resolves, so parent and variants share each fetch
			dataloaders.For(ctx).RegisterVariants(p.ID, v.visible)
		}
	}
	switch p.Kind {
	case catalog.KindBundle:
		if _, err := r.visibleBundleOptions(ctx, p.ID); err != nil {
			return nil, err
		}
	case catalog.KindGrouped:
		if _, err := r.visibleGroupedMembers(ctx, p.ID); err != nil {
			return nil, err
		}
	}
	dataloaders.For(ctx).PrimeProductRow(ctx, p)
	return toProductBaseModel(p), nil
}

// Reuses the input slice's backing array
// Shared by the single-level children and grouped-member filters; bundles filter per-option inside renderBundleOptions
func keepVisibleByChannel[T any](ctx context.Context, r *Resolver, items []T, id func(T) uuid.UUID) ([]T, error) {
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return items[:0], nil
	}
	if len(items) == 0 {
		return items, nil
	}
	ids := make([]uuid.UUID, len(items))
	for i, it := range items {
		ids[i] = id(it)
	}
	visible, err := r.CatalogSvc.FilterVisibleProductIDsForChannel(ctx, ids, *storeID)
	if err != nil {
		return nil, err
	}
	kept := items[:0]
	for _, it := range items {
		if _, ok := visible[id(it)]; ok {
			kept = append(kept, it)
		}
	}
	return kept, nil
}

// Children arrive already visibility-filtered - each unique child product type loads once and content + attribute selections batch-load, so query counts stay flat regardless of variant count
// axisAttrs is the parent's configurable-option exemption for the isPublic filter, since the variant picker reads axis values off each child
func (r *Resolver) renderChildren(ctx context.Context, children []catalog.Product, axisAttrs map[string]bool) ([]*model.Product, error) {
	out := make([]*model.Product, len(children))
	if len(children) == 0 {
		return out, nil
	}
	childTypeCache := make(map[uuid.UUID]catalog.ProductType)
	childIDs := make([]uuid.UUID, len(children))
	for i, child := range children {
		childIDs[i] = child.ID
		if _, ok := childTypeCache[child.ProductTypeID]; !ok {
			cpt, err := r.getProductType(ctx, child.ProductTypeID)
			if err != nil {
				return nil, err
			}
			childTypeCache[child.ProductTypeID] = cpt
		}
	}
	// One query each for locale-resolved content and selected attribute values - the per-child versions were an N+1 that scaled with variant count
	childContent, err := r.CatalogSvc.LoadProductsContent(ctx, childIDs, r.activeLocale(ctx))
	if err != nil {
		return nil, fmt.Errorf("load child content: %w", err)
	}
	childSelections, err := r.CatalogSvc.LoadProductsAttributeSelections(ctx, childIDs)
	if err != nil {
		return nil, fmt.Errorf("load child attribute selections: %w", err)
	}
	loaders := dataloaders.For(ctx)
	for i, child := range children {
		childPT := childTypeCache[child.ProductTypeID]
		if len(child.AttributeSelections) == 0 {
			child.AttributeSelections = childSelections[child.ID]
		}
		if c, ok := childContent[child.ID]; ok {
			cp := c
			child.Content = &cp
		}
		loaders.PrimeProductRow(ctx, child)
		childAttrs := buildProductAttributeValues(childPT, child.Attributes, child.AttributeSelections, r.hydrateProductReferences(ctx, child.AttributeReferences))
		r.stashPromotionAttrKVs(ctx, child.ID, childAttrs)
		childAttrs = keepPublicAttributeValues(childAttrs, axisAttrs)
		if childAttrs == nil {
			childAttrs = []*model.ProductAttributeValue{}
		}
		m := toProductBaseModel(child)
		m.AttributeValues = childAttrs
		m.ConfigurableOptions = []*model.Attribute{}
		out[i] = m
	}
	return out, nil
}

// A required option left with no published selection makes the bundle unsellable
func (r *Resolver) visibleBundleOptions(ctx context.Context, productID uuid.UUID) ([]catalog.BundleOption, error) {
	locale := r.activeLocale(ctx)
	c := getRenderCache(ctx)
	key := productID.String() + ":" + locale
	return renderMemo(c, "bundle:"+key,
		func() ([]catalog.BundleOption, bool) {
			c.mu.RLock()
			defer c.mu.RUnlock()
			o, ok := c.bundleOptions[key]
			return o, ok
		},
		func() ([]catalog.BundleOption, error) { return r.loadVisibleBundleOptions(ctx, productID, locale) },
		func(o []catalog.BundleOption) {
			c.mu.Lock()
			c.bundleOptions[key] = o
			c.mu.Unlock()
		})
}

// Hidden selections too, since the stock aggregate counts them
func (r *Resolver) rawBundleOptions(ctx context.Context, productID uuid.UUID, locale string) ([]catalog.BundleOption, error) {
	c := getRenderCache(ctx)
	key := productID.String() + ":" + locale
	return renderMemo(c, "bundle-raw:"+key,
		func() ([]catalog.BundleOption, bool) {
			c.mu.RLock()
			defer c.mu.RUnlock()
			o, ok := c.rawBundleOptions[key]
			return o, ok
		},
		func() ([]catalog.BundleOption, error) { return r.CatalogSvc.GetBundleOptions(ctx, productID, locale) },
		func(o []catalog.BundleOption) {
			c.mu.Lock()
			c.rawBundleOptions[key] = o
			c.mu.Unlock()
		})
}

// The linked source products of these options this channel actually shows - a hidden source renders as no product
func (r *Resolver) visibleOptionSources(ctx context.Context, opts []catalog.ProductOption) (map[uuid.UUID]struct{}, error) {
	var ids []uuid.UUID
	for _, o := range opts {
		for _, v := range o.Values {
			if v.SourceProductID != nil {
				ids = append(ids, *v.SourceProductID)
			}
		}
	}
	storeID := channelIDFromContext(ctx)
	if len(ids) == 0 || storeID == nil {
		return nil, nil
	}
	visible, err := r.CatalogSvc.FilterVisibleProductIDsForChannel(ctx, ids, *storeID)
	if err != nil {
		return nil, fmt.Errorf("filter option source products: %w", err)
	}
	return visible, nil
}

func hasKey(m map[uuid.UUID]struct{}, id uuid.UUID) bool {
	_, ok := m[id]
	return ok
}

func (r *Resolver) loadVisibleBundleOptions(ctx context.Context, productID uuid.UUID, locale string) ([]catalog.BundleOption, error) {
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return nil, catalog.ErrProductNotFound
	}
	raw, err := r.rawBundleOptions(ctx, productID, locale)
	if err != nil {
		return nil, fmt.Errorf("load bundle options: %w", err)
	}
	bundleOpts := make([]catalog.BundleOption, len(raw)) // the filter below rewrites Selections, so copy off the memo
	for i, o := range raw {
		o.Selections = append([]catalog.BundleSelection(nil), o.Selections...)
		bundleOpts[i] = o
	}
	{
		ids := make([]uuid.UUID, 0)
		for _, o := range bundleOpts {
			for _, s := range o.Selections {
				ids = append(ids, s.ProductID)
			}
		}
		visible, err := r.CatalogSvc.FilterVisibleProductIDsForChannel(ctx, ids, *storeID)
		if err != nil {
			return nil, fmt.Errorf("filter bundle selections: %w", err)
		}
		for i := range bundleOpts {
			kept := bundleOpts[i].Selections[:0]
			for _, s := range bundleOpts[i].Selections {
				if _, ok := visible[s.ProductID]; ok {
					kept = append(kept, s)
				}
			}
			bundleOpts[i].Selections = kept
			if bundleOpts[i].IsRequired && len(kept) == 0 {
				return nil, catalog.ErrProductNotFound
			}
		}
	}
	return bundleOpts, nil
}

func (r *Resolver) renderBundleOptions(ctx context.Context, productID uuid.UUID) ([]*model.BundleOption, error) {
	bundleOpts, err := r.visibleBundleOptions(ctx, productID)
	if err != nil {
		return nil, err
	}
	out := toBundleOptionModels(bundleOpts)
	var partIDs []uuid.UUID
	for _, o := range bundleOpts {
		for _, s := range o.Selections {
			partIDs = append(partIDs, s.ProductID)
		}
	}
	dataloaders.For(ctx).RegisterGroup(partIDs)
	return out, nil
}

// Members unpublished on the active store are dropped, and losing all of them makes the grouped product unsellable, so ErrProductNotFound
func (r *Resolver) visibleGroupedMembers(ctx context.Context, productID uuid.UUID) ([]catalog.GroupedMember, error) {
	locale := r.activeLocale(ctx)
	c := getRenderCache(ctx)
	key := productID.String() + ":" + locale
	return renderMemo(c, "grouped:"+key,
		func() ([]catalog.GroupedMember, bool) {
			c.mu.RLock()
			defer c.mu.RUnlock()
			m, ok := c.groupedMembers[key]
			return m, ok
		},
		func() ([]catalog.GroupedMember, error) {
			members, err := r.CatalogSvc.ListGroupedMembers(ctx, productID, locale)
			if err != nil {
				return nil, fmt.Errorf("load grouped members: %w", err)
			}
			hadMembers := len(members) > 0
			members, err = keepVisibleByChannel(ctx, r, members, func(m catalog.GroupedMember) uuid.UUID { return m.MemberID })
			if err != nil {
				return nil, fmt.Errorf("filter grouped members: %w", err)
			}
			if hadMembers && len(members) == 0 {
				return nil, catalog.ErrProductNotFound
			}
			return members, nil
		},
		func(m []catalog.GroupedMember) {
			c.mu.Lock()
			c.groupedMembers[key] = m
			c.mu.Unlock()
		})
}

func (r *Resolver) renderGroupedMembers(ctx context.Context, productID uuid.UUID) ([]*model.GroupedMember, error) {
	members, err := r.visibleGroupedMembers(ctx, productID)
	if err != nil {
		return nil, err
	}
	out := toGroupedMemberModels(members)
	partIDs := make([]uuid.UUID, 0, len(members))
	for _, m := range members {
		partIDs = append(partIDs, m.MemberID)
	}
	dataloaders.For(ctx).RegisterGroup(partIDs)
	return out, nil
}

// Mirror of the admin helper, duplicated rather than imported because package-private scoping keeps the two from sharing code
func toBundleOptionModels(opts []catalog.BundleOption) []*model.BundleOption {
	out := make([]*model.BundleOption, len(opts))
	for i, o := range opts {
		content := make([]*model.BundleOptionContent, len(o.AllContent))
		for j, c := range o.AllContent {
			content[j] = &model.BundleOptionContent{Locale: c.Locale, Title: c.Title}
		}
		sels := make([]*model.BundleSelection, len(o.Selections))
		for j, s := range o.Selections {
			prod := &model.Product{
				ID:                  s.ProductID.String(),
				URL:                 s.ProductURL,
				Sku:                 s.ProductSKU,
				Kind:                model.ProductKindSimple,
				DefaultPrice:        int32(s.ProductDefaultPrice),
				Price:               int32(s.ProductDefaultPrice),
				DefaultCurrencyCode: s.ProductCurrency,
			}
			if s.ProductName != "" {
				name := s.ProductName
				prod.Name = &name
			}
			sels[j] = &model.BundleSelection{
				ID:        s.ID.String(),
				Product:   prod,
				Quantity:  s.Quantity,
				IsDefault: s.IsDefault,
				SortOrder: s.SortOrder,
			}
		}
		out[i] = &model.BundleOption{
			ID:            o.ID.String(),
			InputType:     model.BundleInputType(strings.ToUpper(string(o.InputType))),
			IsRequired:    o.IsRequired,
			SortOrder:     o.SortOrder,
			MinSelections: o.MinSelections,
			MaxSelections: o.MaxSelections,
			Title:         o.Title,
			Content:       content,
			Selections:    sels,
			CreatedAt:     o.CreatedAt,
			UpdatedAt:     o.UpdatedAt,
		}
	}
	return out
}

// One entry per attribute assigned to the product type, scalar and selectable data merged into one uniform list
func buildProductAttributeValues(
	pt catalog.ProductType,
	scalars catalog.ProductScalarAttributes,
	selections []catalog.ProductAttributeSelection,
	refsByAttr map[string][]*model.ProductAttributeReference,
) []*model.ProductAttributeValue {
	selByAttr := make(map[string][]catalog.AttributeValue)
	for _, sel := range selections {
		k := sel.AttributeID.String()
		selByAttr[k] = append(selByAttr[k], sel.Value)
	}

	var out []*model.ProductAttributeValue
	for _, a := range pt.Attributes {
		entry := &model.ProductAttributeValue{
			Attribute:      toAttributeModel(a.Attribute),
			SelectedValues: []*model.AttributeValue{},
			References:     []*model.ProductAttributeReference{},
		}
		switch {
		case a.Attribute.InputType.IsReference():
			if refs := refsByAttr[a.Attribute.ID.String()]; len(refs) > 0 {
				entry.References = refs
			}
		case a.Attribute.InputType.AcceptsValues():
			vals := selByAttr[a.Attribute.ID.String()]
			entry.SelectedValues = make([]*model.AttributeValue, len(vals))
			for i, v := range vals {
				entry.SelectedValues[i] = toAttributeValueModel(v)
			}
		default:
			if v, ok := scalars[a.Attribute.Slug]; ok && v != nil {
				entry.ScalarValue = v
			}
		}
		out = append(out, entry)
	}
	if out == nil {
		out = []*model.ProductAttributeValue{}
	}
	return out
}

// isPublic enforcement for the storefront response, applied after the promotion-candidate stash - axisAttrs exempts configurable axes, whose values the variant picker needs even when an operator (mis)marks one private
func keepPublicAttributeValues(values []*model.ProductAttributeValue, axisAttrs map[string]bool) []*model.ProductAttributeValue {
	kept := values[:0]
	for _, av := range values {
		if av.Attribute != nil && !av.Attribute.IsPublic && !axisAttrs[av.Attribute.ID] {
			continue
		}
		kept = append(kept, av)
	}
	return kept
}

// The native engines gate facets and filters by IsPublic themselves, but an external engine tier answers before that gate - browse strips both at this seam
// ok=false means the attribute set couldn't be read, and callers fail closed by dropping every attribute filter and facet
func (r *Resolver) privateAttributeSlugs(ctx context.Context) (map[string]bool, bool) {
	attrs, err := r.getAttributesCached(ctx)
	if err != nil {
		log.Printf("[browse] attribute visibility unavailable, stripping attribute filters and facets: %v", err)
		return nil, false
	}
	out := map[string]bool{}
	for _, a := range attrs {
		if !a.IsPublic {
			out[a.Slug] = true
		}
	}
	return out, true
}

// Grouped by attribute id, with targets not visible on the active store silently skipped
func (r *Resolver) hydrateProductReferences(ctx context.Context, refs []catalog.ProductAttributeReference) map[string][]*model.ProductAttributeReference {
	out := make(map[string][]*model.ProductAttributeReference)
	if len(refs) == 0 {
		return out
	}
	locale := r.activeLocale(ctx)
	storeID := channelIDFromContext(ctx)
	idsByType := map[catalog.ReferenceTarget][]uuid.UUID{}
	for _, ref := range refs {
		idsByType[ref.TargetType] = append(idsByType[ref.TargetType], ref.TargetID)
	}
	labels := map[catalog.ReferenceTarget]map[uuid.UUID]string{}
	for tt, ids := range idsByType {
		labels[tt] = r.CatalogSvc.LinkLabelsByIDs(ctx, string(tt), ids, locale)
	}
	for _, ref := range refs {
		name := labels[ref.TargetType][ref.TargetID]
		if name == "" {
			continue
		}
		gqlTarget := modelconv.ReferenceTargetToGQL(ref.TargetType)
		if gqlTarget == nil {
			continue
		}
		path, ok := r.URLRewriteSvc.CanonicalPathForChannel(ctx, urlrewrite.EntityType(ref.TargetType), ref.TargetID, storeID)
		if !ok {
			continue // no resolvable storefront URL → omit
		}
		key := ref.AttributeID.String()
		out[key] = append(out[key], &model.ProductAttributeReference{
			TargetType: *gqlTarget,
			TargetID:   ref.TargetID.String(),
			Label:      name,
			URL:        "/" + path,
		})
	}
	return out
}

func toCategoryModel(c catalog.Category) *model.Category {
	ancestors := make([]*model.Category, len(c.Ancestors))
	for i, a := range c.Ancestors {
		ancestors[i] = toCategoryModel(a)
	}
	children := make([]*model.Category, len(c.Children))
	for i, ch := range c.Children {
		children[i] = toCategoryModel(ch)
	}
	content := make([]*model.CategoryContent, len(c.AllContent))
	for i, cc := range c.AllContent {
		content[i] = toCategoryContentModel(cc)
	}
	m := &model.Category{
		ID:              c.ID.String(),
		Slug:            c.Slug,
		URLPath:         c.URLPath,
		Position:        c.Position,
		IsPublished:     c.IsPublished,
		SeoNoindex:      c.SEONoindex,
		Ancestors:       ancestors,
		Children:        children,
		Level:           int32(len(ancestors)),
		Content:         content,
		Metadata:        toMetadataItems(c.Metadata),
		PrivateMetadata: toMetadataItems(c.PrivateMetadata),
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
	// Ancestors only arrive after Query.category's strict chain check, so the tail is a visible parent - and a child's parent is the node it hangs under
	for i := 1; i < len(ancestors); i++ {
		ancestors[i].Parent = ancestors[i-1]
	}
	if len(ancestors) > 0 {
		m.Parent = ancestors[len(ancestors)-1]
	}
	for _, ch := range children {
		ch.Parent = m
	}
	if c.CoverImage != nil {
		m.CoverImage = &model.CollectionCover{
			URL:      c.CoverImage.URL,
			AltText:  c.CoverImage.AltText,
			Width:    c.CoverImage.Width,
			Height:   c.CoverImage.Height,
			MimeType: c.CoverImage.MimeType,
			Status:   model.ProductMediaStatus(c.CoverImage.Status),
		}
	}
	if len(c.AllContent) > 0 {
		cc := c.AllContent[0]
		m.Name = &cc.Name
	}

	if c.Content != nil {
		m.Name = &c.Content.Name
		if len(c.Content.ShortDescription) > 0 {
			var v any
			if err := json.Unmarshal(c.Content.ShortDescription, &v); err == nil {
				m.ShortDescription = v
			}
		}
		if len(c.Content.Description) > 0 {
			var v any
			if err := json.Unmarshal(c.Content.Description, &v); err == nil {
				m.Description = v
			}
		}
		if c.Content.MetaTitle != nil {
			m.MetaTitle = c.Content.MetaTitle
		}
		if c.Content.MetaDescription != nil {
			m.MetaDescription = c.Content.MetaDescription
		}
	}
	return m
}

func toCollectionModel(c catalog.Collection) *model.Collection {
	m := &model.Collection{
		ID:              c.ID.String(),
		Slug:            c.Slug,
		IsPublished:     c.IsPublished,
		SeoNoindex:      c.SEONoindex,
		Content:         toCollectionContentModels(c.AllContent),
		CoverImage:      toCollectionCoverModel(c.CoverImage),
		Products:        []*model.Product{},
		Metadata:        toMetadataItems(c.Metadata),
		PrivateMetadata: toMetadataItems(c.PrivateMetadata),
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
	if c.Content != nil {
		m.Name = &c.Content.Name
		if len(c.Content.Description) > 0 {
			var v any
			_ = json.Unmarshal(c.Content.Description, &v)
			m.Description = v
		}
		m.MetaTitle = c.Content.MetaTitle
		m.MetaDescription = c.Content.MetaDescription
		m.MetaKeywords = c.Content.MetaKeywords
	}
	return m // a collection's products are loaded and set by resolvers, not in this mapper
}

func toCollectionModels(collections []catalog.Collection) []*model.Collection {
	out := make([]*model.Collection, len(collections))
	for i, c := range collections {
		out[i] = toCollectionModel(c)
	}
	return out
}

// An explicit channelId may only repeat the request's store
func (r *Resolver) resolveSalableChannel(ctx context.Context, channelID *string) (uuid.UUID, error) {
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil {
		return uuid.Nil, nil
	}
	if channelID != nil && *channelID != "" {
		if id, err := uuid.Parse(*channelID); err != nil || id != sv.ID {
			return uuid.Nil, invalidID("channel")
		}
	}
	return sv.ID, nil
}

// Empty or unknown values default to BEFORE, matching the dashboard's own fallback
func facetH1PositionToGQL(s string) model1.BrowseFacetH1Position {
	if catalog.H1Position(s) == catalog.H1PositionAfter {
		return model1.BrowseFacetH1PositionAfter
	}
	return model1.BrowseFacetH1PositionBefore
}

func int32PtrFrom64(p *int64) *int32 {
	if p == nil {
		return nil
	}
	v := int32(*p)
	return &v
}

// Rows scoped to another channel or customer group are dropped, so "Buy N+ for $X" renders without leaking other groups' pricing
func (r *Resolver) renderTierPrices(ctx context.Context, productID uuid.UUID) []*model1.TierPrice {
	channelID := channelIDFromContext(ctx)
	var groupID *uuid.UUID
	if cid := channel.OptionalCustomerID(ctx); cid != nil {
		// Through the Customer dataloader so a B2B grid of N cards resolves the same customer once, not once per card
		if loaders := dataloaders.For(ctx); loaders != nil {
			if c, cerr := loaders.Customer.Load(ctx, *cid)(); cerr == nil && c.CustomerGroupID != uuid.Nil {
				g := c.CustomerGroupID
				groupID = &g
			}
		} else if r.CustomerSvc != nil {
			if c, cerr := r.CustomerSvc.GetByID(ctx, *cid); cerr == nil && c.CustomerGroupID != uuid.Nil {
				g := c.CustomerGroupID
				groupID = &g
			}
		}
	}
	out := []*model1.TierPrice{}
	rows, err := r.CatalogSvc.ApplicableGroupPrices(ctx, productID, channelID, groupID)
	if err != nil {
		return out
	}
	for _, t := range rows {
		out = append(out, &model1.TierPrice{MinQty: t.MinQty, Price: int32(t.Price)})
	}
	return out
}

// For rails (linked, also-bought, recently-viewed) - channel-published filter, requested id order preserved, unrenderable entries silently dropped
// No store, or a visibility read that failed, shows nothing rather than the unfiltered catalog
func (r *Resolver) renderProductsByIDs(ctx context.Context, ids []uuid.UUID) []*model.Product {
	sid := channelIDFromContext(ctx)
	if len(ids) == 0 || sid == nil {
		return []*model.Product{}
	}
	visible, err := r.CatalogSvc.FilterVisibleProductIDsForChannel(ctx, ids, *sid)
	if err != nil {
		return []*model.Product{}
	}
	keep := make([]uuid.UUID, 0, len(ids)) // requested order kept, entries hidden on this store dropped
	for _, id := range ids {
		if _, ok := visible[id]; ok {
			keep = append(keep, id)
		}
	}
	if len(keep) == 0 {
		return []*model.Product{}
	}
	// Every per-card association batched up front (mirrors BrowseProducts) so the per-id render below is a cache read, not a listing/salable/backorderable/review/media fan-out per product
	var chID uuid.UUID
	if s := channelIDFromContext(ctx); s != nil {
		chID = *s
	}
	r.primeProductCards(ctx, keep, chID)
	products, err := r.CatalogSvc.ListProductHeadersForChannelByIDs(ctx, keep, chID, r.activeLocale(ctx))
	if err != nil {
		return []*model.Product{}
	}
	if err := r.primeChildVisibility(ctx, products); err != nil {
		return []*model.Product{}
	}
	out := make([]*model.Product, 0, len(products))
	for _, p := range products {
		m, rerr := r.renderProduct(ctx, p)
		if rerr != nil {
			continue
		}
		out = append(out, m)
	}
	return out
}

// Pins only reorder within the returned page - they never inject products the query didn't match
func sortHitsByPin(hits []search.ProductHit, rank map[string]int) {
	sort.SliceStable(hits, func(i, j int) bool {
		ri, iok := rank[hits[i].ProductID.String()]
		rj, jok := rank[hits[j].ProductID.String()]
		switch {
		case iok && jok:
			return ri < rj
		case iok:
			return true
		default:
			return false
		}
	})
}

// Round-trips through JSON so the JSON scalar emits plain maps/arrays, snake_case keys per the catalog struct tags
func visibleIfToAny(g *catalog.OptionConditionGroup) any {
	raw, err := json.Marshal(g)
	if err != nil {
		return nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func publishedCollections(cols []catalog.Collection) []catalog.Collection {
	out := make([]catalog.Collection, 0, len(cols))
	for _, c := range cols {
		if c.IsPublished {
			out = append(out, c)
		}
	}
	return out
}

// Grids page through browseProducts, so this list only feeds small merchandising blocks - fetched 4x over so channel filtering rarely leaves it short
const (
	membershipProductsCap int32 = 250
	membershipFetch             = 4 * membershipProductsCap
)

// Only category(id) and collection(id) resolve their members - nested under a list they stay empty, or one anonymous query could fan out across every category
func nestedMembership(ctx context.Context) bool {
	return len(graphql.GetFieldContext(ctx).Path()) > 2
}

// Membership ignores channels, so products off this store are dropped here
func (r *Resolver) renderMemberProducts(ctx context.Context, products []catalog.Product, storeID uuid.UUID) ([]*model.Product, error) {
	if len(products) == 0 {
		return []*model.Product{}, nil
	}
	ids := make([]uuid.UUID, len(products))
	for i, p := range products {
		ids[i] = p.ID
	}
	visible, err := r.CatalogSvc.FilterVisibleProductIDsForChannel(ctx, ids, storeID)
	if err != nil {
		return nil, err
	}
	kept := products[:0]
	for _, p := range products {
		if _, ok := visible[p.ID]; ok {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return []*model.Product{}, nil
	}
	if len(kept) > int(membershipProductsCap) {
		kept = kept[:membershipProductsCap]
	}
	ids = ids[:0]
	for _, p := range kept {
		ids = append(ids, p.ID)
	}
	r.primeProductCards(ctx, ids, storeID)
	out := make([]*model.Product, len(kept))
	for i, p := range kept {
		out[i] = toProductBaseModel(p)
	}
	return out, nil
}

func optionPricingChannel(ctx context.Context) (uuid.UUID, string, bool) {
	ch, ok := channel.FromContext(ctx)
	if !ok || ch == nil {
		return uuid.Nil, "", false
	}
	return ch.ID, ch.CurrencyCode, true
}

// Rules as the cart applies them on this channel - a price action carries the amount this channel charges, or flags
// that there's none to charge in its currency
func toCustomOptionRuleModels(ctx context.Context, rules []catalog.ProductOptionRule) []*model1.ProductCustomOptionRule {
	chID, chCur, perChannel := optionPricingChannel(ctx)
	out := make([]*model1.ProductCustomOptionRule, len(rules))
	for i, r := range rules {
		actions := make([]*model1.ProductCustomOptionRuleAction, len(r.Actions))
		for j, a := range r.Actions {
			m := &model1.ProductCustomOptionRuleAction{Type: string(a.Type), Quantity: a.Quantity}
			if a.OptionID != nil {
				id := a.OptionID.String()
				m.OptionID = &id
			}
			if a.Type == catalog.RuleActionAdjustPrice {
				pt := string(a.PriceType)
				pv := a.PriceValue
				if perChannel {
					v, ok := a.ChannelPriceValue(chID, chCur, r.Currency)
					pv, m.PriceUnavailable = v, !ok
				}
				m.PriceType, m.PriceValue = &pt, &pv
			}
			actions[j] = m
		}
		out[i] = &model1.ProductCustomOptionRule{
			ID:        r.ID.String(),
			Label:     r.Label,
			Condition: visibleIfToAny(&r.Condition),
			Actions:   actions,
		}
	}
	return out
}
