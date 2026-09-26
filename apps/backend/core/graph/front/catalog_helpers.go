package front

import (
	"sort"

	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/skeletg/skelet/apps/backend/core/internal/search"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/dataloaders"
	model1 "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/graph/modelconv"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/pricing"
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
	}
	return r.InventorySvc.Salable(ctx, productID, channelID)
}

// A wrapper carries no inventory_items row, so this aggregates children: configurable/grouped are in stock when ANY child is sellable, a bundle only when EVERY required option has a sellable selection (optional options don't gate)
// No children means out of stock, and every child row counts even ones unpublished on this store - an over-approximation that errs toward showing the parent available
func (r *productResolver) wrapperInStock(ctx context.Context, kind model.ProductKind, pid, channelID uuid.UUID) (bool, error) {
	switch kind {
	case model.ProductKindConfigurable:
		children, err := r.CatalogSvc.ListProductChildren(ctx, pid)
		if err != nil {
			return false, err
		}
		ids := make([]uuid.UUID, len(children))
		for i, c := range children {
			ids[i] = c.ID
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
		comps, err := r.CatalogSvc.ListKitComponents(ctx, pid)
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
		opts, err := r.CatalogSvc.GetBundleOptions(ctx, pid, r.activeLocale(ctx))
		if err != nil {
			return false, err
		}
		// One batched lookup over every selection, then each required option has to hold a sellable one
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
		for _, opt := range opts {
			if !opt.IsRequired {
				continue
			}
			ok := false
			for _, sel := range opt.Selections {
				if sellable[sel.ProductID] {
					ok = true
					break
				}
			}
			if !ok {
				return false, nil
			}
		}
		return true, nil
	}
	return true, nil
}

// Mirrors the simple branch of InStock over a batched salable lookup - salable<0 (untracked sentinel) or >0 is sellable, and exactly zero falls back to the item's backorderable flag
func (r *productResolver) sellableByIDs(ctx context.Context, ids []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]bool, error) {
	out := make(map[uuid.UUID]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	bySalable, err := r.InventorySvc.SalableByProductIDs(ctx, ids, channelID)
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
		item, err := r.InventorySvc.GetItemByProduct(ctx, id) // s == 0, so sellable only if the item is backorderable
		if err != nil {
			out[id] = false
			continue
		}
		out[id] = item.Backorderable
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
	asset, err := loaders.PromotionAsset.Load(ctx, *mediaID)()
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
func toProductBaseModel(p catalog.Product, productType *model.ProductType, attributeValues []*model.ProductAttributeValue, children []*model.Product, mediaList []*model.ProductMedia, configurableOptions []*model.Attribute, listings []*model.ProductChannelListing, collections []*model.Collection, chListing *catalog.ProductChannelListing) *model.Product {
	// Listing override folded with any active scheduled special - the same resolution the cart charges through ResolveSimplePricing
	resolved := pricing.Resolve(catalog.EffectivePricingInputs(p, chListing), time.Now())
	categories := make([]*model.Category, len(p.Categories))
	for i, c := range p.Categories {
		categories[i] = toCategoryModel(c)
	}
	var primaryCategory *model.Category
	if p.PrimaryCategory != nil {
		primaryCategory = toCategoryModel(*p.PrimaryCategory)
	}
	// Any image beats an empty box: only an explicit setProductMediaRole call assigns MAIN, so a product whose images arrived through import, a plugin or the API renders as a blank card, blank variant row and blank part stub without this. Same rule enrichPartProducts applies to bundle and grouped parts - thumbnail and og_image deliberately DON'T fall back, since a hero standing in for a thumbnail is the wrong size and a missing og image should let the page's own metadata fallback decide
	mainImage := resolveMediaRole(p.MediaRoles, catalog.MediaRoleMain, mediaList)
	if mainImage == nil && len(mediaList) > 0 {
		mainImage = mediaList[0]
	}
	var compareAtModel *int32
	if resolved.CompareAt != nil {
		v := int32(*resolved.CompareAt)
		compareAtModel = &v
	}
	m := &model.Product{
		ID:                      p.ID.String(),
		ProductType:             productType,
		Kind:                    model.ProductKind(strings.ToUpper(string(p.Kind))),
		VariantMode:             productVariantModeModel(p),
		DefaultPrice:            int32(p.DefaultPrice),
		DefaultCurrencyCode:     p.DefaultCurrencyCode,
		Price:                   int32(resolved.Price),
		CompareAtPrice:          compareAtModel,
		OnSale:                  resolved.OnSale,
		Listings:                listings,
		Sku:                     p.SKU,
		SeoNoindex:              p.SEONoindex,
		CanonicalURL:            p.CanonicalURL,
		URL:                     p.URL,
		URLAuto:                 p.URLAuto,
		URLTemplateOverride:     p.URLTemplateOverride,
		TracksInventoryOverride: p.TracksInventoryOverride,
		TracksInventory:         effectiveTracksInventory(p, productType), // the effective flag - override, then product type
		QuantityDecimals:        int32(p.QuantityDecimals),
		QuantityStep:            p.QuantityStep,
		MinOrderQty:             p.MinOrderQty,
		MaxOrderQty:             p.MaxOrderQty,
		AttributeValues:         attributeValues,
		Children:                children,
		ConfigurableOptions:     configurableOptions,
		Categories:              categories,
		PrimaryCategory:         primaryCategory,
		Collections:             collections,
		Media:                   mediaList,
		MainImage:               mainImage,
		Thumbnail:               resolveMediaRole(p.MediaRoles, catalog.MediaRoleThumbnail, mediaList),
		OgImage:                 resolveMediaRole(p.MediaRoles, catalog.MediaRoleOGImage, mediaList),
		Metadata:                toMetadataItems(p.Metadata),
		PrivateMetadata:         toMetadataItems(p.PrivateMetadata),
		Content:                 toProductContentModels(p.AllContent),
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

// mediaOverride replaces the per-product media fetch - list resolvers pass a slice from ListProductMediaByProductIDs to dodge the N+1
func (r *Resolver) renderProduct(ctx context.Context, p catalog.Product, mediaOverride ...[]catalog.ProductMedia) (*model.Product, error) {
	pt, err := r.getProductType(ctx, p.ProductTypeID) // hydrated with attributes, which buildProductAttributeValues needs to project attributeValues
	if err != nil {
		return nil, err
	}
	attrValues := buildProductAttributeValues(pt, p.Attributes, p.AttributeSelections, r.hydrateProductReferences(ctx, p.AttributeReferences))
	axisAttrs := make(map[string]bool, len(p.ConfigurableOptions))
	for _, a := range p.ConfigurableOptions {
		axisAttrs[a.ID.String()] = true
	}
	// The pre-filter values are stashed so promotion display rules keep matching private attributes - the response only ever carries the rule's own label
	r.stashPromotionAttrKVs(ctx, p.ID, attrValues)
	attrValues = keepPublicAttributeValues(attrValues, axisAttrs)

	// Filtered up front so the per-child loads below only run for variants that get emitted, and a configurable/grouped parent whose children all go hidden on this store is unsellable, so not visible
	hadChildren := len(p.Children) > 0
	p.Children, err = keepVisibleByChannel(ctx, r, p.Children, func(c catalog.Product) uuid.UUID { return c.ID })
	if err != nil {
		return nil, fmt.Errorf("filter visible children: %w", err)
	}
	if hadChildren && len(p.Children) == 0 && (p.Kind == catalog.KindConfigurable || p.Kind == catalog.KindGrouped) {
		return nil, catalog.ErrProductNotFound
	}

	children, err := r.renderChildren(ctx, p.Children, axisAttrs)
	if err != nil {
		return nil, err
	}

	configurableOptions := make([]*model.Attribute, len(p.ConfigurableOptions)) // already loaded by GetProductWithAttributes
	for i, a := range p.ConfigurableOptions {
		configurableOptions[i] = toAttributeModel(a)
	}

	var mediaItems []catalog.ProductMedia // the pre-fetched slice on the batch path, otherwise fetched here
	if len(mediaOverride) > 0 {
		mediaItems = mediaOverride[0]
	} else {
		mediaItems, err = r.CatalogSvc.ListProductMedia(ctx, p.ID)
		if err != nil {
			return nil, err
		}
	}
	mediaList := make([]*model.ProductMedia, len(mediaItems))
	for i, m := range mediaItems {
		mediaList[i] = toProductMediaModel(m)
	}

	listings, err := r.renderProductListings(ctx, p) // per-store listings, with effective price/currency resolved under the inheritance rule
	if err != nil {
		return nil, err
	}

	collections := toCollectionModels(p.Collections)
	// Raw listing for the active channel drives effective-price resolution - the loader dedupes it against renderProductListings' fetch above
	var chListing *catalog.ProductChannelListing
	if sid := channelIDFromContext(ctx); sid != nil {
		if rows, lerr := r.getProductChannelListings(ctx, p.ID); lerr == nil {
			for i := range rows {
				if rows[i].ChannelID == *sid {
					chListing = &rows[i]
					break
				}
			}
		}
	}
	out := toProductBaseModel(p, toProductTypeModel(pt), attrValues, children, mediaList, configurableOptions, listings, collections, chListing)

	switch p.Kind {
	case catalog.KindBundle:
		if out.BundleOptions, err = r.renderBundleOptions(ctx, p.ID); err != nil {
			return nil, err
		}
	case catalog.KindGrouped:
		if out.GroupedMembers, err = r.renderGroupedMembers(ctx, p.ID); err != nil {
			return nil, err
		}
	}

	r.localizeProductModel(ctx, p, out) // translated attribute/value names, plain_text scalar translations, channel slug/url_path overrides
	return out, nil
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
	// A hydrating load, with attributes, so each child's attributeValues - what the storefront variant picker reads - is actually populated
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
	// Media + roles power per-variant galleries, and the active channel's listing drives effective-price resolution - without it every child rendered at its base default price, hiding listing overrides and specials
	childMedia, err := r.CatalogSvc.ListProductMediaByProductIDs(ctx, childIDs)
	if err != nil {
		return nil, fmt.Errorf("load child media: %w", err)
	}
	childRoles, err := r.CatalogSvc.ListProductMediaRolesByProductIDs(ctx, childIDs)
	if err != nil {
		return nil, fmt.Errorf("load child media roles: %w", err)
	}
	sid := channelIDFromContext(ctx)
	var childListings map[uuid.UUID][]catalog.ProductChannelListing
	if sid != nil {
		if childListings, err = r.CatalogSvc.ListProductChannelListingsByProductIDsForChannel(ctx, childIDs, *sid); err != nil {
			return nil, fmt.Errorf("load child listings: %w", err)
		}
	}
	for i, child := range children {
		childPT := childTypeCache[child.ProductTypeID]
		if len(child.AttributeSelections) == 0 {
			child.AttributeSelections = childSelections[child.ID]
		}
		if c, ok := childContent[child.ID]; ok {
			cp := c
			child.Content = &cp
		}
		if len(child.MediaRoles) == 0 {
			child.MediaRoles = childRoles[child.ID]
		}
		mediaModels := make([]*model.ProductMedia, len(childMedia[child.ID]))
		for j, m := range childMedia[child.ID] {
			mediaModels[j] = toProductMediaModel(m)
		}
		var chListing *catalog.ProductChannelListing
		if sid != nil {
			rows := childListings[child.ID]
			for j := range rows {
				if rows[j].ChannelID == *sid {
					chListing = &rows[j]
					break
				}
			}
		}
		childAttrs := buildProductAttributeValues(childPT, child.Attributes, child.AttributeSelections, r.hydrateProductReferences(ctx, child.AttributeReferences))
		r.stashPromotionAttrKVs(ctx, child.ID, childAttrs)
		childAttrs = keepPublicAttributeValues(childAttrs, axisAttrs)
		out[i] = toProductBaseModel(child, toProductTypeModel(childPT), childAttrs, []*model.Product{}, mediaModels, []*model.Attribute{}, nil, []*model.Collection{}, chListing)
	}
	return out, nil
}

// The pure mappers build bundle-selection and grouped-member stubs from denormalized row fields, which left price at the catalog default (listing overrides and specials invisible) and mainImage always empty
// Batched to a handful of lookups regardless of part count, and fail-open per part - a lookup miss keeps the stub's defaults rather than dropping the row
func (r *Resolver) enrichPartProducts(ctx context.Context, parts []*model.Product) {
	if len(parts) == 0 {
		return
	}
	ids := make([]uuid.UUID, 0, len(parts))
	byID := make(map[uuid.UUID][]*model.Product, len(parts))
	for _, p := range parts {
		id, err := uuid.Parse(p.ID)
		if err != nil {
			continue
		}
		if len(byID[id]) == 0 {
			ids = append(ids, id)
		}
		byID[id] = append(byID[id], p)
	}
	sid := channelIDFromContext(ctx)
	var listings map[uuid.UUID][]catalog.ProductChannelListing
	rowByID := make(map[uuid.UUID]catalog.Product, len(ids))
	if sid != nil {
		listings, _ = r.CatalogSvc.ListProductChannelListingsByProductIDsForChannel(ctx, ids, *sid)
		// The full rows carry the product-level special-price fields the stubs lack, and the parts were already visibility-filtered so the channel gate inside is a no-op
		if rows, err := r.CatalogSvc.ListProductsForChannelByIDs(ctx, ids, *sid, r.activeLocale(ctx)); err == nil {
			for _, row := range rows {
				rowByID[row.ID] = row
			}
		}
	}
	roles, _ := r.CatalogSvc.ListProductMediaRolesByProductIDs(ctx, ids)
	media, _ := r.CatalogSvc.ListProductMediaByProductIDs(ctx, ids)
	for id, models := range byID {
		mediaModels := make([]*model.ProductMedia, len(media[id]))
		for i, m := range media[id] {
			mediaModels[i] = toProductMediaModel(m)
		}
		mainImage := resolveMediaRole(roles[id], catalog.MediaRoleMain, mediaModels)
		if mainImage == nil && len(mediaModels) > 0 {
			// Parts render a single thumbnail, so any image beats an empty box
			mainImage = mediaModels[0]
		}
		var chListing *catalog.ProductChannelListing
		if sid != nil {
			ls := listings[id]
			for i := range ls {
				if ls[i].ChannelID == *sid {
					chListing = &ls[i]
					break
				}
			}
		}
		for _, m := range models {
			m.Media = mediaModels
			m.MainImage = mainImage
			if row, ok := rowByID[id]; ok {
				resolved := pricing.Resolve(catalog.EffectivePricingInputs(row, chListing), time.Now())
				m.Price = int32(resolved.Price)
				m.OnSale = resolved.OnSale
				if resolved.CompareAt != nil {
					v := int32(*resolved.CompareAt)
					m.CompareAtPrice = &v
				}
			}
		}
	}
}

// Drops selections unpublished on the active store in one batched query across all options - a REQUIRED option left with zero selections makes the bundle unsellable, so ErrProductNotFound, while optional ones may render empty
func (r *Resolver) renderBundleOptions(ctx context.Context, productID uuid.UUID) ([]*model.BundleOption, error) {
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return nil, catalog.ErrProductNotFound
	}
	bundleOpts, err := r.CatalogSvc.GetBundleOptions(ctx, productID, r.activeLocale(ctx))
	if err != nil {
		return nil, fmt.Errorf("load bundle options: %w", err)
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
	out := toBundleOptionModels(bundleOpts)
	parts := make([]*model.Product, 0)
	for _, o := range out {
		for _, s := range o.Selections {
			parts = append(parts, s.Product)
		}
	}
	r.enrichPartProducts(ctx, parts)
	return out, nil
}

// Members unpublished on the active store are dropped, and losing all of them makes the grouped product unsellable, so ErrProductNotFound
func (r *Resolver) renderGroupedMembers(ctx context.Context, productID uuid.UUID) ([]*model.GroupedMember, error) {
	members, err := r.CatalogSvc.ListGroupedMembers(ctx, productID, r.activeLocale(ctx))
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
	out := toGroupedMemberModels(members)
	parts := make([]*model.Product, 0, len(out))
	for _, m := range out {
		parts = append(parts, m.Member)
	}
	r.enrichPartProducts(ctx, parts)
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
			ID:         o.ID.String(),
			InputType:  model.BundleInputType(strings.ToUpper(string(o.InputType))),
			IsRequired: o.IsRequired,
			SortOrder:  o.SortOrder,
			Title:      o.Title,
			Content:    content,
			Selections: sels,
			CreatedAt:  o.CreatedAt,
			UpdatedAt:  o.UpdatedAt,
		}
	}
	return out
}

// Each row's effective price/currency resolves under the inheritance rule - a NULL listing.price takes the product default only if the currencies match
func (r *Resolver) renderProductListings(ctx context.Context, p catalog.Product) ([]*model.ProductChannelListing, error) {
	rows, err := r.getProductChannelListings(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []*model.ProductChannelListing{}, nil
	}
	out := make([]*model.ProductChannelListing, 0, len(rows))
	for _, row := range rows {
		sv, err := r.getChannel(ctx, row.ChannelID)
		if err != nil {
			return nil, err
		}
		var priceField *int32
		if row.Price != nil {
			v := int32(*row.Price)
			priceField = &v
		}
		// Explicit listing price wins, otherwise inherit the product default when currencies match, else null
		var effective *int32
		effectiveCurrency := sv.CurrencyCode
		if row.Price != nil || sv.CurrencyCode == p.DefaultCurrencyCode {
			res := pricing.Resolve(catalog.EffectivePricingInputs(p, &row), time.Now())
			v := int32(res.Price)
			effective = &v
		}
		out = append(out, &model.ProductChannelListing{
			Channel:           toChannelModel(sv),
			Price:             priceField,
			IsPublished:       row.IsPublished,
			EffectivePrice:    effective,
			EffectiveCurrency: effectiveCurrency,
			CompareAtPrice:    int32PtrFrom64(row.CompareAtPrice),
			SpecialPrice:      int32PtrFrom64(row.SpecialPrice),
			SpecialFrom:       row.SpecialFrom,
			SpecialTo:         row.SpecialTo,
		})
	}
	return out, nil
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
func (r *Resolver) privateAttributeSlugs(ctx context.Context) map[string]bool {
	attrs, err := r.getAttributesCached(ctx)
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, a := range attrs {
		if !a.IsPublic {
			out[a.Slug] = true
		}
	}
	return out
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

// Flattens both the per-product categories map and the primary-category map so Products/Product can run one batched visibility check before applying the result back to each
func collectCategoryIDs(byProduct map[uuid.UUID][]catalog.Category, primaryByProduct map[uuid.UUID]*catalog.Category) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{})
	out := make([]uuid.UUID, 0)
	for _, cats := range byProduct {
		for _, c := range cats {
			if _, ok := seen[c.ID]; ok {
				continue
			}
			seen[c.ID] = struct{}{}
			out = append(out, c.ID)
		}
	}
	for _, primary := range primaryByProduct {
		if primary == nil {
			continue
		}
		if _, ok := seen[primary.ID]; ok {
			continue
		}
		seen[primary.ID] = struct{}{}
		out = append(out, primary.ID)
	}
	return out
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
	rows, err := r.CatalogSvc.ListGroupPrices(ctx, productID)
	if err != nil || len(rows) == 0 {
		return []*model1.TierPrice{}
	}
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
	for _, t := range rows {
		if t.ChannelID != nil && (channelID == nil || *t.ChannelID != *channelID) {
			continue
		}
		if t.CustomerGroupID != nil {
			if groupID == nil || *t.CustomerGroupID != *groupID {
				continue
			}
		}
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
	mediaByID, err := r.CatalogSvc.ListProductMediaByProductIDs(ctx, keep)
	if err != nil {
		mediaByID = nil // degrade to per-card media rather than dropping the rail
	}
	locale := r.activeLocale(ctx)
	out := make([]*model.Product, 0, len(keep))
	for _, id := range keep {
		p, gerr := r.CatalogSvc.GetProduct(ctx, id, locale)
		if gerr != nil {
			continue
		}
		var (
			m    *model.Product
			rerr error
		)
		if mediaByID != nil {
			m, rerr = r.renderProduct(ctx, p, mediaByID[id]) // batched media (nil = none)
		} else {
			m, rerr = r.renderProduct(ctx, p) // batch failed: fall back to per-card media
		}
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
	mediaByID, err := r.CatalogSvc.ListProductMediaByProductIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	rolesByID, err := r.CatalogSvc.ListProductMediaRolesByProductIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Product, len(kept))
	for i, p := range kept {
		p.MediaRoles = rolesByID[p.ID]
		mediaItems := mediaByID[p.ID]
		mediaList := make([]*model.ProductMedia, len(mediaItems))
		for j, m := range mediaItems {
			mediaList[j] = toProductMediaModel(m)
		}
		out[i] = toProductBaseModel(p, nil, nil, nil, mediaList, nil, nil, nil, nil)
	}
	return out, nil
}
