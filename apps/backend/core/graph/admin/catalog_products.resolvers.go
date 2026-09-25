package admin

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CreateProduct(ctx context.Context, input model.CreateProductInput) (*model.Product, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return nil, err
	}
	productTypeID, err := parseID(input.ProductTypeID, "product type")
	if err != nil {
		return nil, err
	}
	p, err := r.CatalogSvc.CreateProduct(ctx, catalog.CreateProductInput{
		ProductTypeID:           productTypeID,
		Kind:                    catalog.ProductKind(strings.ToLower(string(input.Kind))),
		DefaultPrice:            int64(int32Or(input.DefaultPrice, 0)),
		DefaultCurrencyCode:     stringOr(input.DefaultCurrencyCode, ""),
		SKU:                     input.Sku,
		SEONoindex:              boolOrFalse(input.SeoNoindex),
		CanonicalURL:            stringOr(input.CanonicalURL, ""),
		URL:                     input.URL,
		URLAuto:                 input.URLAuto,
		URLTemplateOverride:     stringOr(input.URLTemplateOverride, ""),
		TracksInventoryOverride: input.TracksInventoryOverride,
		QuantityDecimals:        int16(int32Or(input.QuantityDecimals, 0)),
		QuantityStep:            float64Or(input.QuantityStep, 0), // 0 → repo defaults to 1
		Metadata:                metadataFromInput(input.Metadata),
		PrivateMetadata:         metadataFromInput(input.PrivateMetadata),
		Locale:                  input.Locale,
		Name:                    input.Name,
		ShortDescription:        marshalJSON(input.ShortDescription),
		Description:             marshalJSON(input.DescriptionRichText),
		MetaTitle:               stringOr(input.MetaTitle, ""),
		MetaDescription:         stringOr(input.MetaDescription, ""),
		MetaKeywords:            stringOr(input.MetaKeywords, ""),
	})
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}

func (r *mutationResolver) UpdateProduct(ctx context.Context, input model.UpdateProductInput) (*model.Product, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return nil, err
	}
	id, err := parseID(input.ID, "product")
	if err != nil {
		return nil, err
	}
	domainInput := catalog.UpdateProductInput{
		ID:                           id,
		DefaultCurrencyCode:          input.DefaultCurrencyCode,
		SKU:                          input.Sku,
		SEONoindex:                   input.SeoNoindex,
		CanonicalURL:                 input.CanonicalURL,
		URL:                          input.URL,
		URLAuto:                      input.URLAuto,
		URLTemplateOverride:          input.URLTemplateOverride,
		TracksInventoryOverride:      input.TracksInventoryOverride,
		ClearTracksInventoryOverride: input.ClearTracksInventoryOverride != nil && *input.ClearTracksInventoryOverride,
	}
	if input.DefaultPrice != nil {
		v := int64(*input.DefaultPrice)
		domainInput.DefaultPrice = &v
	}
	if input.CompareAtPrice != nil {
		v := int64(*input.CompareAtPrice)
		domainInput.CompareAtPrice = &v
	}
	domainInput.ClearCompareAtPrice = input.ClearCompareAtPrice != nil && *input.ClearCompareAtPrice
	if input.SpecialPrice != nil {
		v := int64(*input.SpecialPrice)
		domainInput.SpecialPrice = &v
		domainInput.SpecialFrom = input.SpecialFrom
		domainInput.SpecialTo = input.SpecialTo
	}
	domainInput.ClearSpecialPrice = input.ClearSpecialPrice != nil && *input.ClearSpecialPrice
	domainInput.WeightGrams = input.WeightGrams
	domainInput.LengthMm = input.LengthMm
	domainInput.WidthMm = input.WidthMm
	domainInput.HeightMm = input.HeightMm
	domainInput.MinOrderQty = input.MinOrderQty
	domainInput.MaxOrderQty = input.MaxOrderQty
	if input.CostCents != nil {
		v := int64(*input.CostCents)
		domainInput.CostCents = &v
	}
	if input.Metadata != nil {
		m := metadataFromInput(input.Metadata)
		domainInput.Metadata = &m
	}
	if input.PrivateMetadata != nil {
		m := metadataFromInput(input.PrivateMetadata)
		domainInput.PrivateMetadata = &m
	}
	if input.QuantityDecimals != nil {
		v := int16(*input.QuantityDecimals)
		domainInput.QuantityDecimals = &v
	}
	if input.QuantityStep != nil {
		domainInput.QuantityStep = input.QuantityStep
	}
	p, err := r.CatalogSvc.UpdateProduct(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}

func (r *mutationResolver) DeleteProduct(ctx context.Context, id string) (bool, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return false, err
	}
	pid, err := parseID(id, "product")
	if err != nil {
		return false, err
	}
	if err := r.CatalogSvc.DeleteProduct(ctx, pid); err != nil {
		return false, err
	}
	return true, nil
}

// UpsertProductContent creates or replaces translatable content for one locale. The translation row is shared by every channel that lists the product, so the gate requires the admin to have catalog.manage on *every* channel currently listing this product at the requested locale (or a global grant). Otherwise an (EU, de) admin could edit a product listed on EU+US and the change would leak to US.
func (r *mutationResolver) UpsertProductContent(ctx context.Context, productID string, locale *string, input model.UpsertProductContentInput) (*model.Product, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return nil, err
	}
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if locale == nil || *locale == "" {
		return nil, fmt.Errorf("locale is required")
	}
	listings, err := r.CatalogSvc.ListProductChannelListings(ctx, pid)
	if err != nil {
		return nil, err
	}
	channelIDs := make([]uuid.UUID, len(listings))
	for i, l := range listings {
		channelIDs[i] = l.ChannelID
	}
	if err := auth.RequireAllListedChannelsPermission(ctx, r.AdminSvc, "catalog.manage", channelIDs, *locale); err != nil {
		return nil, err
	}
	p, err := r.CatalogSvc.UpsertProductContent(ctx, pid, catalog.ProductContentInput{
		Locale:           *locale,
		Name:             input.Name,
		ShortDescription: marshalJSON(input.ShortDescription),
		Description:      marshalJSON(input.DescriptionRichText),
		MetaTitle:        stringOr(input.MetaTitle, ""),
		MetaDescription:  stringOr(input.MetaDescription, ""),
		MetaKeywords:     stringOr(input.MetaKeywords, ""),
	})
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}

// DeleteProductContent removes content for one locale. Same all- listed-channels gate as UpsertProductContent - the
// translation row is shared across every channel listing the product
func (r *mutationResolver) DeleteProductContent(ctx context.Context, productID string, locale *string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return false, err
	}
	pid, err := parseID(productID, "product")
	if err != nil {
		return false, err
	}
	if locale == nil || *locale == "" {
		return false, fmt.Errorf("locale is required")
	}
	listings, err := r.CatalogSvc.ListProductChannelListings(ctx, pid)
	if err != nil {
		return false, err
	}
	channelIDs := make([]uuid.UUID, len(listings))
	for i, l := range listings {
		channelIDs[i] = l.ChannelID
	}
	if err := auth.RequireAllListedChannelsPermission(ctx, r.AdminSvc, "catalog.manage", channelIDs, *locale); err != nil {
		return false, err
	}
	if err := r.CatalogSvc.DeleteProductContent(ctx, pid, *locale); err != nil {
		return false, err
	}
	return true, nil
}

// SetProductAttributes writes attribute values that render on every channel listing the product, so it carries the same all-listing-channels gate as the shared content rows
func (r *mutationResolver) SetProductAttributes(ctx context.Context, productID string, assignments []*model.ProductAttributeAssignmentInput) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	domainAssignments := make([]catalog.AttributeAssignment, 0, len(assignments))
	for _, a := range assignments {
		if a == nil {
			continue
		}
		da, err := productAttributeAssignmentFromGQL(a)
		if err != nil {
			return nil, err
		}
		domainAssignments = append(domainAssignments, da)
	}
	p, err := r.CatalogSvc.SetProductAttributes(ctx, pid, domainAssignments)
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}

func (r *mutationResolver) ClearProductAttribute(ctx context.Context, productID string, attributeID string) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	aid, err := parseID(attributeID, "attribute")
	if err != nil {
		return nil, err
	}
	p, err := r.CatalogSvc.ClearProductAttribute(ctx, pid, aid)
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}

func (r *mutationResolver) SetProductAttributeReferences(ctx context.Context, productID string, attributeID string, refs []*model.AttributeReferenceRefInput) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	aid, err := parseID(attributeID, "attribute")
	if err != nil {
		return nil, err
	}
	domainRefs := make([]catalog.ProductAttributeReference, 0, len(refs))
	for _, ref := range refs {
		if ref == nil {
			continue
		}
		tid, err := parseID(ref.TargetID, "target")
		if err != nil {
			return nil, err
		}
		domainRefs = append(domainRefs, catalog.ProductAttributeReference{
			AttributeID: aid,
			TargetType:  referenceTargetFromGQL(&ref.TargetType),
			TargetID:    tid,
		})
	}
	p, err := r.CatalogSvc.SetProductAttributeReferences(ctx, pid, aid, domainRefs)
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}

// SetProductTaxClass sets the product's tax class (empty → "standard").
func (r *mutationResolver) SetProductTaxClass(ctx context.Context, productID string, taxClass string) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	p, err := r.CatalogSvc.SetProductTaxClass(ctx, pid, taxClass)
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}

// SetConfigurableOptions defines which selectable attributes differentiate
// children of a configurable product. Replaces any previous options.
func (r *mutationResolver) SetConfigurableOptions(ctx context.Context, productID string, attributeIds []string) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	attrIDs := make([]uuid.UUID, 0, len(attributeIds))
	for _, s := range attributeIds {
		id, err := parseID(s, "attribute")
		if err != nil {
			return nil, err
		}
		attrIDs = append(attrIDs, id)
	}
	p, err := r.CatalogSvc.SetConfigurableOptions(ctx, pid, attrIDs)
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}

// AddProductChild links a child product under a configurable parent.
func (r *mutationResolver) AddProductChild(ctx context.Context, parentID string, childID string, sortOrder *int32) (*model.Product, error) {
	pid, err := parseID(parentID, "parent product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	cid, err := parseID(childID, "child product")
	if err != nil {
		return nil, err
	}
	p, err := r.CatalogSvc.AddProductChild(ctx, pid, cid, int32Or(sortOrder, 0))
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}

// RemoveProductChild unlinks a child from its configurable parent.
func (r *mutationResolver) RemoveProductChild(ctx context.Context, parentID string, childID string) (*model.Product, error) {
	pid, err := parseID(parentID, "parent product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	cid, err := parseID(childID, "child product")
	if err != nil {
		return nil, err
	}
	p, err := r.CatalogSvc.RemoveProductChild(ctx, pid, cid)
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, p)
}

func (r *mutationResolver) GenerateConfigurableChildren(ctx context.Context, parentID string, variants []*model.GenerateConfigurableChildInput) (*model.Product, error) {
	pid, err := parseID(parentID, "parent product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	inputs := make([]catalog.GenerateConfigurableChildInput, 0, len(variants))
	for i, v := range variants {
		if v == nil {
			return nil, fmt.Errorf("variant %d: input is required", i)
		}
		valueIDs := make([]uuid.UUID, 0, len(v.AttributeValueIds))
		for _, s := range v.AttributeValueIds {
			id, err := uuid.Parse(s)
			if err != nil {
				return nil, fmt.Errorf("variant %d: invalid attribute value id %q", i, s)
			}
			valueIDs = append(valueIDs, id)
		}
		inputs = append(inputs, catalog.GenerateConfigurableChildInput{
			AttributeValueIDs:   valueIDs,
			Name:                v.Name,
			SKU:                 ptr.Deref(v.Sku),
			DefaultPrice:        int64(int32Or(v.DefaultPrice, 0)),
			DefaultCurrencyCode: ptr.Deref(v.DefaultCurrencyCode),
			Locale:              ptr.Deref(v.Locale),
			SortOrder:           int32Or(v.SortOrder, 0),
		})
	}
	if _, err := r.CatalogSvc.GenerateConfigurableChildren(ctx, pid, inputs); err != nil {
		return nil, err
	}
	parent, err := r.CatalogSvc.GetProduct(ctx, pid, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, parent)
}

// AddProductDownload attaches a downloadable file to a product.
func (r *mutationResolver) AddProductDownload(ctx context.Context, input model.AddProductDownloadInput) (*model.Product, error) {
	productID, err := parseID(input.ProductID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, productID); err != nil {
		return nil, err
	}
	d, err := r.CatalogSvc.AddProductDownload(ctx, catalog.AddProductDownloadInput{
		ProductID:    productID,
		Title:        input.Title,
		URL:          input.URL,
		MaxDownloads: input.MaxDownloads,
		SortOrder:    int32Or(input.SortOrder, 0),
	})
	if err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, d.ProductID)
}

// UpdateProductDownload applies pointer-style updates to a product download.
func (r *mutationResolver) UpdateProductDownload(ctx context.Context, input model.UpdateProductDownloadInput) (*model.Product, error) {
	id, err := parseID(input.ID, "product download")
	if err != nil {
		return nil, err
	}
	// Load-then-gate: the download row is the only path back to the product whose listings scope this edit
	existing, err := r.CatalogSvc.GetProductDownload(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, existing.ProductID); err != nil {
		return nil, err
	}
	d, err := r.CatalogSvc.UpdateProductDownload(ctx, catalog.UpdateProductDownloadInput{
		ID:           id,
		Title:        input.Title,
		URL:          input.URL,
		MaxDownloads: input.MaxDownloads,
		SortOrder:    input.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, d.ProductID)
}

// RemoveProductDownload detaches a downloadable file from its product.
func (r *mutationResolver) RemoveProductDownload(ctx context.Context, id string) (*model.Product, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return nil, err
	}
	dlID, err := parseID(id, "product download")
	if err != nil {
		return nil, err
	}
	d, err := r.CatalogSvc.GetProductDownload(ctx, dlID)
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, d.ProductID); err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.DeleteProductDownload(ctx, dlID); err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, d.ProductID)
}

// SetSubscriptionTerms upserts a subscription product's recurring billing terms.
func (r *mutationResolver) SetSubscriptionTerms(ctx context.Context, input model.SetSubscriptionTermsInput) (*model.Product, error) {
	productID, err := parseID(input.ProductID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, productID); err != nil {
		return nil, err
	}
	trialDays := int32(0)
	if input.TrialDays != nil {
		trialDays = *input.TrialDays
	}
	if _, err := r.CatalogSvc.SetSubscriptionTerms(ctx, catalog.SetSubscriptionTermsInput{
		ProductID:     productID,
		IntervalUnit:  strings.ToLower(string(input.IntervalUnit)),
		IntervalCount: input.IntervalCount,
		TrialDays:     trialDays,
	}); err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, productID)
}

// ClearSubscriptionTerms removes a subscription product's recurring terms.
func (r *mutationResolver) ClearSubscriptionTerms(ctx context.Context, productID string) (*model.Product, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	if err := r.requireCatalogManageOnAllListingChannels(ctx, pid); err != nil {
		return nil, err
	}
	if err := r.CatalogSvc.ClearSubscriptionTerms(ctx, pid); err != nil {
		return nil, err
	}
	return r.reloadProduct(ctx, pid)
}

func (r *mutationResolver) SetProductListing(ctx context.Context, input model.SetProductListingInput) (*model.Product, error) {
	productID, err := parseID(input.ProductID, "product")
	if err != nil {
		return nil, err
	}
	storeID, err := parseID(input.ChannelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "catalog.manage", storeID, nil); err != nil {
		return nil, err
	}
	domainInput := catalog.ProductChannelListingInput{
		ProductID:   productID,
		ChannelID:   storeID,
		IsPublished: input.IsPublished,
		URL:         input.URL,
	}
	if input.Price != nil {
		v := int64(*input.Price)
		domainInput.Price = &v
	}
	if input.CompareAtPrice != nil {
		v := int64(*input.CompareAtPrice)
		domainInput.CompareAtPrice = &v
	}
	if input.SpecialPrice != nil {
		v := int64(*input.SpecialPrice)
		domainInput.SpecialPrice = &v
		domainInput.SpecialFrom = input.SpecialFrom
		domainInput.SpecialTo = input.SpecialTo
	}
	if _, err := r.CatalogSvc.SetProductListing(ctx, domainInput); err != nil {
		return nil, err
	}
	product, err := r.CatalogSvc.GetProduct(ctx, productID, "")
	if err != nil {
		return nil, err
	}
	return r.renderProduct(ctx, product)
}

func (r *mutationResolver) ClearProductListing(ctx context.Context, productID string, channelID string) (bool, error) {
	pid, err := parseID(productID, "product")
	if err != nil {
		return false, err
	}
	sid, err := parseID(channelID, "channel")
	if err != nil {
		return false, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "catalog.manage", sid, nil); err != nil {
		return false, err
	}
	if err := r.CatalogSvc.ClearProductListing(ctx, pid, sid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SetProductGroupPrices(ctx context.Context, productID string, prices []*model.ProductGroupPriceInput) ([]*model.ProductGroupPrice, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return nil, err
	}
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	inputs := make([]catalog.GroupPriceInput, 0, len(prices))
	for _, in := range prices {
		if in == nil {
			continue
		}
		gp := catalog.GroupPriceInput{MinQty: in.MinQty, Price: int64(in.Price)}
		if in.ChannelID != nil && *in.ChannelID != "" {
			id, perr := parseID(*in.ChannelID, "channel")
			if perr != nil {
				return nil, perr
			}
			gp.ChannelID = &id
		}
		if in.CustomerGroupID != nil && *in.CustomerGroupID != "" {
			id, perr := parseID(*in.CustomerGroupID, "customer group")
			if perr != nil {
				return nil, perr
			}
			gp.CustomerGroupID = &id
		}
		inputs = append(inputs, gp)
	}
	rows, err := r.CatalogSvc.SetGroupPrices(ctx, pid, inputs)
	if err != nil {
		return nil, err
	}
	return toGroupPriceModels(rows), nil
}

func (r *mutationResolver) SetProductLinks(ctx context.Context, productID string, kind model.ProductLinkKind, linkedProductIds []string) (bool, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "catalog.manage"); err != nil {
		return false, err
	}
	pid, err := parseID(productID, "product")
	if err != nil {
		return false, err
	}
	linked := make([]uuid.UUID, 0, len(linkedProductIds))
	for _, raw := range linkedProductIds {
		id, perr := parseID(raw, "product")
		if perr != nil {
			return false, perr
		}
		linked = append(linked, id)
	}
	if err := r.CatalogSvc.SetProductLinks(ctx, pid, catalog.LinkKind(strings.ToLower(string(kind))), linked); err != nil {
		return false, err
	}
	return true, nil
}

func (r *productResolver) SalableQuantity(ctx context.Context, obj *model.Product, channelID *string) (*float64, error) {
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, err
	}
	svID, err := r.resolveSalableChannel(ctx, channelID)
	if err != nil || svID == uuid.Nil {
		return nil, err
	}
	if r.InventorySvc == nil {
		return nil, nil
	}
	salable, err := r.salableFor(ctx, pid, svID)
	if err != nil {
		return nil, err
	}
	if salable < 0 {
		return nil, nil // unbounded "always in stock" sentinel
	}
	v := salable
	return &v, nil
}

// InStock is the resolver for the inStock field. Mirrors the front resolver: salable > 0 OR backorderable. See the
// front version's commentary for the backorder rationale
func (r *productResolver) InStock(ctx context.Context, obj *model.Product, channelID *string) (bool, error) {
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return false, err
	}
	svID, err := r.resolveSalableChannel(ctx, channelID)
	if err != nil {
		return false, err
	}
	if svID == uuid.Nil || r.InventorySvc == nil {
		return true, nil // ungated when no channel is in scope
	}
	salable, err := r.salableFor(ctx, pid, svID)
	if err != nil {
		return false, err
	}
	if salable < 0 {
		return true, nil
	}
	if salable > 0 {
		return true, nil
	}
	item, err := r.InventorySvc.GetItemByProduct(ctx, pid)
	if err != nil {
		return false, nil
	}
	return item.Backorderable, nil
}

// Downloads is the resolver for the downloads field. Lazy - only queries that select downloads (the product detail
// editor) pay the per-product load; the products list doesn't
func (r *productResolver) Downloads(ctx context.Context, obj *model.Product) ([]*model.ProductDownload, error) {
	pid, err := parseID(obj.ID, "product")
	if err != nil {
		return nil, err
	}
	downloads, err := r.CatalogSvc.ListProductDownloads(ctx, pid)
	if err != nil {
		return nil, fmt.Errorf("load product downloads: %w", err)
	}
	return toProductDownloadModels(downloads), nil
}

// ProductOptions is the resolver for the productOptions field. Lazy
// for the same reason as Downloads.
func (r *productResolver) ProductOptions(ctx context.Context, obj *model.Product) ([]*model.ProductOption, error) {
	pid, err := parseID(obj.ID, "product")
	if err != nil {
		return nil, err
	}
	options, err := r.CatalogSvc.GetProductOptions(ctx, pid, r.activeLocale(ctx))
	if err != nil {
		return nil, fmt.Errorf("load product options: %w", err)
	}
	return toProductOptionModels(options), nil
}

// Products is the resolver for the products field. When channelId is set, the result is restricted to products with a
// listing on that store (header-driven admin uses this in CHANNEL mode)
func (r *queryResolver) Products(ctx context.Context, channelID *string) ([]*model.Product, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	products, err := r.CatalogSvc.ListProductsWithGraph(ctx, r.activeLocale(ctx), catalog.MaxUnpagedRows)
	if err != nil {
		return nil, err
	}
	chID, err := r.scopedChannelID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if chID != nil {
		listings, err := r.CatalogSvc.ListProductChannelListingsByChannel(ctx, *chID)
		if err != nil {
			return nil, err
		}
		listed := make(map[uuid.UUID]struct{}, len(listings))
		for _, l := range listings {
			listed[l.ProductID] = struct{}{}
		}
		filtered := products[:0]
		for _, p := range products {
			if _, ok := listed[p.ID]; ok {
				filtered = append(filtered, p)
			}
		}
		products = filtered
	}
	return r.renderProductRows(ctx, products, channelID)
}

func (r *queryResolver) ProductsPaged(ctx context.Context, channelID *string, search *string, kinds []model.ProductKind, productTypeSlugs []string, published *bool, attributeValueIds []string, sortKey *string, sortDesc *bool, limit *int32, offset *int32) (*model.ProductPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	// Resolved before the page so LIMIT applies to rows the caller may actually
	// see - filtering after the fact would return short pages with holes
	chID, err := r.scopedChannelID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	valueIDs := make([]uuid.UUID, 0, len(attributeValueIds))
	for _, raw := range attributeValueIds {
		id, perr := uuid.Parse(raw)
		if perr != nil {
			return nil, fmt.Errorf("invalid attribute value id %q", raw)
		}
		valueIDs = append(valueIDs, id)
	}
	// The enum is upper-case over the wire and lower-case in the column, the same mapping createProduct does - passing the
	// wire form straight through matched no rows at all rather than failing
	kindValues := make([]string, 0, len(kinds))
	for _, k := range kinds {
		kindValues = append(kindValues, strings.ToLower(string(k)))
	}
	f := catalog.ProductPageFilter{
		Locale:            r.activeLocale(ctx),
		ChannelID:         chID,
		Kinds:             kindValues,
		TypeSlugs:         productTypeSlugs,
		Published:         published,
		AttributeValueIDs: valueIDs,
		SortDesc:          sortDesc != nil && *sortDesc,
		Limit:             intDeref(limit, 25),
		Offset:            intDeref(offset, 0),
	}
	if search != nil {
		f.Search = strings.TrimSpace(*search)
	}
	if sortKey != nil {
		f.SortKey = *sortKey
	}
	// An unbounded limit would reintroduce exactly the download this query exists
	// to avoid, so it is clamped rather than trusted
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 25
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	products, total, err := r.CatalogSvc.ListPageWithGraph(ctx, f)
	if err != nil {
		return nil, err
	}
	items, err := r.renderProductRows(ctx, products, channelID)
	if err != nil {
		return nil, err
	}
	return &model.ProductPage{TotalCount: int32(total), Items: items}, nil
}

func (r *queryResolver) Product(ctx context.Context, id string) (*model.Product, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	pid, err := parseID(id, "product")
	if err != nil {
		return nil, err
	}
	p, err := r.CatalogSvc.GetProduct(ctx, pid, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	cols, err := r.CatalogSvc.ListCollectionsByProductIDs(ctx, []uuid.UUID{pid}, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	p.Collections = cols[pid]
	cats, err := r.CatalogSvc.ListCategoriesByProductIDs(ctx, []uuid.UUID{pid}, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	p.Categories = cats[pid]
	primaryCats, err := r.CatalogSvc.ListPrimaryCategoryByProductIDs(ctx, []uuid.UUID{pid})
	if err != nil {
		return nil, err
	}
	p.PrimaryCategory = primaryCats[pid]
	return r.renderProduct(ctx, p)
}

func (r *queryResolver) ProductsByIds(ctx context.Context, ids []string) ([]*model.Product, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	if len(ids) > 100 {
		ids = ids[:100]
	}
	parsed := make([]uuid.UUID, 0, len(ids))
	for _, raw := range ids {
		id, err := parseID(raw, "product")
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, id)
	}
	products, err := r.CatalogSvc.ListProductsWithAttributesByIDs(ctx, parsed)
	if err != nil {
		return nil, err
	}
	return r.renderProductRows(ctx, products, nil)
}

func (r *queryResolver) ProductLinks(ctx context.Context, productID string, kind model.ProductLinkKind) ([]string, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "catalog.view"); err != nil {
		return nil, err
	}
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	ids, err := r.CatalogSvc.ListProductLinks(ctx, pid, catalog.LinkKind(strings.ToLower(string(kind))))
	if err != nil {
		return nil, err
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out, nil
}

func (r *Resolver) Product() ProductResolver { return &productResolver{r} }

type productResolver struct{ *Resolver }
