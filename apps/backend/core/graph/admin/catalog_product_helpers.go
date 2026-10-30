package admin // renderProduct and friends live in a helpers file because gqlgen strips unrecognized funcs out of .resolvers.go on regeneration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/pricing"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// The render cache is primed against one channel only, so a caller asking about a different store falls through to a fresh fetch rather than reading another store's stock
func (r *productResolver) salableFor(ctx context.Context, productID, channelID uuid.UUID) (float64, error) {
	if v, ok := r.cachedSalable(ctx, productID); ok {
		return v, nil
	}
	return r.InventorySvc.Salable(ctx, productID, channelID)
}

// A mutation against a sub-aggregate (a bundle option, say) still owes the dashboard the parent product with its refreshed nesting
func (r *Resolver) reloadProduct(ctx context.Context, productID uuid.UUID) (*model.Product, error) {
	p, err := r.CatalogSvc.GetProduct(ctx, productID, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}

func toProductDownloadModels(ds []catalog.ProductDownload) []*model.ProductDownload {
	out := make([]*model.ProductDownload, len(ds))
	for i, d := range ds {
		out[i] = toProductDownloadModel(d)
	}
	return out
}

func toProductDownloadModel(d catalog.ProductDownload) *model.ProductDownload {
	return &model.ProductDownload{
		ID:           d.ID.String(),
		Title:        d.Title,
		URL:          d.URL,
		MaxDownloads: d.MaxDownloads,
		SortOrder:    d.SortOrder,
		CreatedAt:    d.CreatedAt,
		UpdatedAt:    d.UpdatedAt,
	}
}

// nil map and empty map are different answers downstream - null input means no change, [] means clear
func metadataFromInput(in []*model.MetadataInput) catalog.Metadata {
	if in == nil {
		return nil
	}
	out := make(catalog.Metadata, len(in))
	for _, item := range in {
		if item == nil {
			continue
		}
		out[item.Key] = item.Value
	}
	return out
}

// Scalars only - attribute rendering needs the product type loaded, so it stays on renderProduct and this mapper stays pure
func toProductBaseModel(p catalog.Product, productType *model.ProductType, attributeValues []*model.ProductAttributeValue, children []*model.Product, mediaList []*model.ProductMedia, configurableOptions []*model.Attribute, listings []*model.ProductChannelListing, collections []*model.Collection) *model.Product {
	categories := make([]*model.Category, len(p.Categories))
	for i, c := range p.Categories {
		categories[i] = toCategoryModel(c)
	}
	var primaryCategory *model.Category
	if p.PrimaryCategory != nil {
		primaryCategory = toCategoryModel(*p.PrimaryCategory)
	}
	resolvedPrice := pricing.Resolve(catalog.EffectivePricingInputs(p, nil), time.Now()) // no listing folded in - the per-channel number lives on each listing's effectivePrice
	// The role fields stay literal - the media-role editor treats them as the assignment itself and as its staged-save baseline, so a stand-in would make "set this as main" read as no change. displayImage is the one that falls back, for the tiles and rows that just need a picture
	mainImage := resolveMediaRole(p.MediaRoles, catalog.MediaRoleMain, mediaList)
	thumbnail := resolveMediaRole(p.MediaRoles, catalog.MediaRoleThumbnail, mediaList)
	displayImage := mainImage
	if displayImage == nil {
		displayImage = thumbnail
	}
	if displayImage == nil && len(mediaList) > 0 {
		displayImage = mediaList[0]
	}
	m := &model.Product{
		ID:                      p.ID.String(),
		ProductType:             productType,
		Kind:                    model.ProductKind(strings.ToUpper(string(p.Kind))),
		VariantMode:             productVariantModeModel(p),
		DefaultPrice:            int32(p.DefaultPrice),
		DefaultCurrencyCode:     p.DefaultCurrencyCode,
		CompareAtPrice:          int32PtrFrom64(p.CompareAtPrice),
		SpecialPrice:            int32PtrFrom64(p.SpecialPrice),
		SpecialFrom:             p.SpecialFrom,
		SpecialTo:               p.SpecialTo,
		Price:                   int32(resolvedPrice.Price),
		CompareAtPriceResolved:  int32PtrFrom64(resolvedPrice.CompareAt),
		OnSale:                  resolvedPrice.OnSale,
		WeightGrams:             p.WeightGrams,
		LengthMm:                p.LengthMm,
		WidthMm:                 p.WidthMm,
		HeightMm:                p.HeightMm,
		MinOrderQty:             p.MinOrderQty,
		MaxOrderQty:             p.MaxOrderQty,
		CostCents:               int32PtrFrom64(p.CostCents),
		Listings:                listings,
		Sku:                     p.SKU,
		TaxClass:                p.TaxClass,
		SeoNoindex:              p.SEONoindex,
		CanonicalURL:            p.CanonicalURL,
		URL:                     p.URL,
		URLAuto:                 p.URLAuto,
		URLTemplateOverride:     p.URLTemplateOverride,
		TracksInventoryOverride: p.TracksInventoryOverride,
		TracksInventory:         effectiveTracksInventory(p, productType), // the effective flag, product override falling back to the type's
		QuantityDecimals:        int32(p.QuantityDecimals),
		QuantityStep:            p.QuantityStep,
		AttributeValues:         attributeValues,
		Children:                children,
		ConfigurableOptions:     configurableOptions,
		Categories:              categories,
		PrimaryCategory:         primaryCategory,
		Collections:             collections,
		Media:                   mediaList,
		MainImage:               mainImage,
		Thumbnail:               thumbnail,
		OgImage:                 resolveMediaRole(p.MediaRoles, catalog.MediaRoleOGImage, mediaList),
		DisplayImage:            displayImage,
		Metadata:                toMetadataItems(p.Metadata),
		PrivateMetadata:         toMetadataItems(p.PrivateMetadata),
		Content:                 allProductContent(p),
		CreatedAt:               p.CreatedAt,
		UpdatedAt:               p.UpdatedAt,
	}
	if p.Content != nil { // inline locale-resolved content, left nil when this locale has none
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
		locale := p.Content.Locale
		m.ContentLocale = &locale
	}
	return m
}

// mediaOverride takes a pre-fetched slice from ListProductMediaByProductIDs - list resolvers pass one to skip the per-product media fetch
func (r *Resolver) renderProduct(ctx context.Context, p catalog.Product, mediaOverride ...[]catalog.ProductMedia) (*model.Product, error) {
	pt, err := r.getProductType(ctx, p.ProductTypeID)
	if err != nil {
		return nil, err
	}

	attrValues := buildProductAttributeValues(pt, p.Attributes, p.AttributeSelections, r.hydrateProductReferences(ctx, p.AttributeReferences))

	childTypeCache := make(map[uuid.UUID]catalog.ProductType) // one fetch per distinct child type instead of one per child
	for _, child := range p.Children {
		if _, ok := childTypeCache[child.ProductTypeID]; !ok {
			cpt, err := r.getProductType(ctx, child.ProductTypeID)
			if err != nil {
				return nil, err
			}
			childTypeCache[child.ProductTypeID] = cpt
		}
	}
	children := make([]*model.Product, len(p.Children))
	for i, child := range p.Children {
		childPT := childTypeCache[child.ProductTypeID]
		childAttrs := buildProductAttributeValues(childPT, child.Attributes, child.AttributeSelections, r.hydrateProductReferences(ctx, child.AttributeReferences))
		children[i] = toProductBaseModel(child, toProductTypeModel(childPT), childAttrs, []*model.Product{}, []*model.ProductMedia{}, []*model.Attribute{}, nil, []*model.Collection{})
	}

	configurableOptions := make([]*model.Attribute, len(p.ConfigurableOptions)) // already loaded by GetProductWithAttributes
	for i, a := range p.ConfigurableOptions {
		configurableOptions[i] = toAttributeModel(a)
	}

	var mediaItems []catalog.ProductMedia
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

	listings, err := r.renderProductListings(ctx, p)
	if err != nil {
		return nil, err
	}

	collections := toCollectionModels(p.Collections)
	out := toProductBaseModel(p, toProductTypeModel(pt), attrValues, children, mediaList, configurableOptions, listings, collections)
	if gps, gerr := r.CatalogSvc.ListGroupPrices(ctx, p.ID); gerr == nil {
		out.GroupPrices = toGroupPriceModels(gps)
	} else {
		out.GroupPrices = []*model.ProductGroupPrice{}
	}

	// Every kind-specific field below stays nil for other kinds, and the schema's non-null list constraint renders that as an empty array
	if p.Kind == catalog.KindBundle {
		bundleOpts, err := r.CatalogSvc.GetBundleOptions(ctx, p.ID, r.activeLocale(ctx))
		if err != nil {
			return nil, fmt.Errorf("load bundle options: %w", err)
		}
		out.BundleOptions = toBundleOptionModels(bundleOpts)
	}

	if p.Kind == catalog.KindGrouped {
		members, err := r.CatalogSvc.ListGroupedMembers(ctx, p.ID, r.activeLocale(ctx))
		if err != nil {
			return nil, fmt.Errorf("load grouped members: %w", err)
		}
		out.GroupedMembers = toGroupedMemberModels(members)
	}

	if p.Kind == catalog.KindKit {
		comps, err := r.CatalogSvc.ListKitComponents(ctx, p.ID, r.activeLocale(ctx))
		if err != nil {
			return nil, fmt.Errorf("load kit components: %w", err)
		}
		out.KitComponents = toKitComponentModels(comps)
	}

	// Custom options and downloadable files are deliberately absent here - they're field resolvers in gqlgen.admin.yml, and hydrating them eagerly cost two queries per row on a list nobody renders them in
	if p.Kind == catalog.KindSubscription {
		terms, err := r.CatalogSvc.GetSubscriptionTerms(ctx, p.ID)
		if err == nil {
			out.SubscriptionTerms = toSubscriptionTermsModel(terms)
		} else if !errors.Is(err, catalog.ErrSubscriptionTermsNotFound) {
			return nil, fmt.Errorf("load subscription terms: %w", err)
		}
	}

	return out, nil
}

// Effective price follows the inheritance rule - a NULL listing price falls back to the product default only when the currencies match
func (r *Resolver) renderProductListings(ctx context.Context, p catalog.Product) ([]*model.ProductChannelListing, error) {
	rows, err := r.getProductChannelListings(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []*model.ProductChannelListing{}, nil
	}
	// A listing row leaks a channel the same way a per-channel list query would, so an admin scoped to channel A must not see channel B's listing riding along on a product
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "catalog.view")
	if err != nil {
		return nil, err
	}
	out := make([]*model.ProductChannelListing, 0, len(rows))
	for _, row := range rows {
		if !perm.Allows(row.ChannelID) {
			continue
		}
		sv, err := r.getChannel(ctx, row.ChannelID)
		if err != nil {
			return nil, err
		}
		var priceField *int32
		if row.Price != nil {
			v := int32(*row.Price)
			priceField = &v
		}
		var effective *int32 // explicit listing price wins, then the product default when currencies match, else null - internal/pricing folds in any active scheduled special
		effectiveCurrency := sv.CurrencyCode
		if row.Price != nil || sv.CurrencyCode == p.DefaultCurrencyCode {
			res := pricing.Resolve(catalog.EffectivePricingInputs(p, &row), time.Now())
			v := int32(res.Price)
			effective = &v
		}
		out = append(out, &model.ProductChannelListing{
			ProductID:         p.ID.String(),
			ChannelID:         row.ChannelID.String(),
			Channel:           toChannelModel(sv),
			Price:             priceField,
			IsPublished:       row.IsPublished,
			URL:               row.URL,
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

func toGroupPriceModels(rows []catalog.GroupPrice) []*model.ProductGroupPrice {
	out := make([]*model.ProductGroupPrice, len(rows))
	for i, gp := range rows {
		m := &model.ProductGroupPrice{
			ID:     gp.ID.String(),
			MinQty: gp.MinQty,
			Price:  int32(gp.Price),
		}
		if gp.ChannelID != nil {
			v := gp.ChannelID.String()
			m.ChannelID = &v
		}
		if gp.CustomerGroupID != nil {
			v := gp.CustomerGroupID.String()
			m.CustomerGroupID = &v
		}
		out[i] = m
	}
	return out
}

func int32PtrFrom64(p *int64) *int32 {
	if p == nil {
		return nil
	}
	v := int32(*p)
	return &v
}

// One entry per attribute on the product type, scalar and selectable merged into a uniform list the admin UI can iterate
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

// A target that no longer resolves is skipped rather than erroring, and admin reads the global path with channelID nil
func (r *Resolver) hydrateProductReferences(ctx context.Context, refs []catalog.ProductAttributeReference) map[string][]*model.ProductAttributeReference {
	out := make(map[string][]*model.ProductAttributeReference)
	if len(refs) == 0 {
		return out
	}
	locale := r.activeLocale(ctx)
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
			continue // target gone / not visible → omit
		}
		gqlTarget := referenceTargetToGQL(ref.TargetType)
		if gqlTarget == nil {
			continue
		}
		path, _ := r.UrlRewriteSvc.CanonicalPathForChannel(ctx, urlrewrite.EntityType(ref.TargetType), ref.TargetID, nil)
		key := ref.AttributeID.String()
		out[key] = append(out[key], &model.ProductAttributeReference{
			TargetType: *gqlTarget,
			TargetID:   ref.TargetID.String(),
			Label:      name,
			URL:        path,
		})
	}
	return out
}

// Field presence picks the path: valueIds present even as [] means selectable (set or clear), absent means scalar (scalarValue sets, null clears) - the service rejects a path that doesn't match the attribute's inputType
func productAttributeAssignmentFromGQL(in *model.ProductAttributeAssignmentInput) (catalog.AttributeAssignment, error) {
	attrID, err := uuid.Parse(in.AttributeID)
	if err != nil {
		return catalog.AttributeAssignment{}, errInvalidID("attribute id", nil)
	}
	a := catalog.AttributeAssignment{AttributeID: attrID}
	if in.ValueIds != nil {
		a.ValueIDs = make([]uuid.UUID, 0, len(in.ValueIds))
		for _, sid := range in.ValueIds {
			id, err := uuid.Parse(sid)
			if err != nil {
				return catalog.AttributeAssignment{}, fmt.Errorf("invalid attribute value id: %s", sid)
			}
			a.ValueIDs = append(a.ValueIDs, id)
		}
		return a, nil
	}
	a.HasScalar = true // scalar intent, so even a nil ScalarValue reads as "clear" rather than "no change"
	a.ScalarValue = in.ScalarValue
	return a, nil
}

// The explicit channelId arg, else the admin's ambient scope - uuid.Nil when neither is present, which callers read as ungated
func (r *Resolver) resolveSalableChannel(ctx context.Context, channelID *string) (uuid.UUID, error) {
	id, err := r.scopedChannelID(ctx, channelID)
	if err != nil {
		return uuid.Nil, err
	}
	if id == nil {
		return uuid.Nil, nil
	}
	return *id, nil
}

// Batch-loads what renderProduct and the field resolvers would otherwise fetch per row - shared by `products` and `productsPaged` so the two can't drift in what a row contains
func (r *queryResolver) renderProductRows(ctx context.Context, products []catalog.Product, channelID *string) ([]*model.Product, error) {
	ids := make([]uuid.UUID, len(products))
	for i, p := range products {
		ids[i] = p.ID
	}
	// Cache is attached by RenderCacheMiddleware
	if err := r.primeListingsAndChannels(ctx, ids); err != nil {
		return nil, err
	}
	if err := r.primeProductContent(ctx, ids); err != nil {
		return nil, err
	}
	if channelID != nil && *channelID != "" {
		if svID, err := uuid.Parse(*channelID); err == nil {
			if err := r.primeSalable(ctx, ids, svID); err != nil {
				return nil, err
			}
		}
	}
	mediaByProduct, err := r.CatalogSvc.ListProductMediaByProductIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	rolesByProduct, err := r.CatalogSvc.ListProductMediaRolesByProductIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	collectionsByProduct, err := r.CatalogSvc.ListCollectionsByProductIDs(ctx, ids, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	categoriesByProduct, err := r.CatalogSvc.ListCategoriesByProductIDs(ctx, ids, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	primaryCategoryByProduct, err := r.CatalogSvc.ListPrimaryCategoryByProductIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Product, len(products))
	for i, p := range products {
		p.MediaRoles = rolesByProduct[p.ID]
		p.Collections = collectionsByProduct[p.ID]
		p.Categories = categoriesByProduct[p.ID]
		p.PrimaryCategory = primaryCategoryByProduct[p.ID]
		m, err := r.renderProduct(ctx, p, mediaByProduct[p.ID])
		if err != nil {
			return nil, err
		}
		out[i] = m
	}
	return out, nil
}

// Nil unless this path loaded every locale's row - the content resolver then reads them itself, rather than
// answering [] for a product that has translations
func allProductContent(p catalog.Product) []*model.ProductContent {
	if p.AllContent == nil {
		return nil
	}
	return toProductContentModels(p.AllContent)
}

var priceBoundPattern = regexp.MustCompile(`^\d{1,12}(\.\d{1,6})?$`)

// The bound reaches SQL as numeric text, so anything but a plain non-negative amount stops here
func priceBound(field string, v *string) (string, error) {
	if v == nil {
		return "", nil
	}
	s := strings.TrimSpace(*v)
	if s == "" || priceBoundPattern.MatchString(s) {
		return s, nil
	}
	return "", errInvalidInput(field, fmt.Errorf("want an amount like 12.50, got %q", s))
}
