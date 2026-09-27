package front

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/dataloaders"
	model1 "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/graphqlx"
	"github.com/skeletg/skelet/apps/backend/core/internal/media"
	"github.com/skeletg/skelet/apps/backend/core/internal/search"
)

func (r *categoryResolver) Products(ctx context.Context, obj *model.Category) ([]*model.Product, error) {
	cid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, invalidID("category")
	}
	storeID := channelIDFromContext(ctx)
	if storeID == nil || nestedMembership(ctx) {
		return []*model.Product{}, nil
	}
	products, err := r.CatalogSvc.ListProductsByCategory(ctx, cid, r.activeLocale(ctx), membershipFetch)
	if err != nil {
		return nil, err
	}
	return r.renderMemberProducts(ctx, products, *storeID)
}

func (r *collectionResolver) Products(ctx context.Context, obj *model.Collection) ([]*model.Product, error) {
	cid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, invalidID("collection")
	}
	storeID := channelIDFromContext(ctx)
	if storeID == nil || nestedMembership(ctx) {
		return []*model.Product{}, nil
	}
	products, err := r.CatalogSvc.ListProductsByCollection(ctx, cid, r.activeLocale(ctx), membershipFetch)
	if err != nil {
		return nil, err
	}
	return r.renderMemberProducts(ctx, products, *storeID)
}

// Sellable means salable > 0 OR backorderable - the second branch unblocks "tracked, threshold set, no physical stock yet, backorderable=true" from sitting at inStock=false forever, which would disable Add to cart and keep the customer from ever reaching checkout, where Reserve does allow the backorder
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
		return true, nil
	}
	// Wrappers own no inventory_items row, so stock has to aggregate from children - reading the wrapper's own salable returns the "always in stock" sentinel and reports every configurable/bundle/grouped product in stock even when nothing behind it is buyable
	switch obj.Kind {
	case model.ProductKindConfigurable, model.ProductKindGrouped, model.ProductKindBundle, model.ProductKindKit:
		return r.wrapperInStock(ctx, obj.Kind, pid, svID)
	}
	salable, err := r.salableFor(ctx, pid, svID)
	if err != nil {
		return false, err
	}
	if salable < 0 {
		return true, nil // unbounded "always in stock" sentinel
	}
	if salable > 0 {
		return true, nil
	}
	// Only zero-stock tracked items reach here, and on list surfaces primeBackorderable has already pinned the answer, so no per-card lookup
	if v, ok := r.cachedBackorderable(ctx, pid); ok {
		return v, nil
	}
	item, err := r.InventorySvc.GetItemByProduct(ctx, pid)
	if err != nil {
		// A missing item row should be impossible (salable=0 implies tracked), but the unexpected case reads as out of stock rather than crashing the storefront
		return false, nil
	}
	return item.Backorderable, nil
}

// nil for unbounded "always in stock" products
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
		return nil, nil
	}
	v := salable
	return &v, nil
}

func (r *productResolver) TierPrices(ctx context.Context, obj *model.Product) ([]*model1.TierPrice, error) {
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return []*model1.TierPrice{}, nil
	}
	return r.renderTierPrices(ctx, pid), nil
}

func (r *productResolver) LinkedProducts(ctx context.Context, obj *model.Product, kind model.ProductLinkKind) ([]*model.Product, error) {
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return []*model.Product{}, nil
	}
	ids, err := r.CatalogSvc.ListProductLinks(ctx, pid, catalog.LinkKind(strings.ToLower(string(kind))))
	if err != nil {
		return nil, err
	}
	return r.renderProductsByIDs(ctx, ids), nil
}

func (r *productResolver) AlsoBought(ctx context.Context, obj *model.Product, limit *int32) ([]*model.Product, error) {
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return []*model.Product{}, nil
	}
	lim := int32(8)
	if limit != nil {
		lim = *limit
	}
	if lim < 1 { // clamped (unbounded before) so a deep nested query can't amplify each node's fan-out
		lim = 1
	}
	if lim > 24 {
		lim = 24
	}
	ids, err := r.CatalogSvc.ListAlsoBought(ctx, pid, lim)
	if err != nil {
		return nil, err
	}
	return r.renderProductsByIDs(ctx, ids), nil
}

func (r *productResolver) Backorderable(ctx context.Context, obj *model.Product) (bool, error) {
	pid, err := uuid.Parse(obj.ID) // a missing item row (untracked product) is simply not backorderable, and never errors the product query
	if err != nil {
		return false, nil
	}
	// List surfaces prime this so cards don't each fire an inventory lookup - an unprimed PDP falls through to one call
	if v, ok := r.cachedBackorderable(ctx, pid); ok {
		return v, nil
	}
	item, err := r.InventorySvc.GetItemByProduct(ctx, pid)
	if err != nil {
		return false, nil
	}
	return item.Backorderable, nil
}

func (r *productResolver) Options(ctx context.Context, obj *model.Product) ([]*model1.ProductCustomOption, error) {
	if r.CatalogSvc == nil || obj == nil {
		return []*model1.ProductCustomOption{}, nil
	}
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, err
	}
	opts, err := r.CatalogSvc.GetProductOptions(ctx, pid, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	// A variant carries its configurable parent's product-wide options too - the cart resolution merges and ENFORCES them (adapters.ResolveProductOptions), so a variant-own URL that didn't render them could never satisfy a required parent option - fail-open, a hiccup here degrades to the variant-only set rather than killing the PDP
	if obj.Kind == model.ProductKindSimple {
		if parents, perr := r.CatalogSvc.ListConfigurableParentByChildIDs(ctx, []uuid.UUID{pid}); perr == nil {
			if link, ok := parents[pid]; ok && link.ParentID != uuid.Nil {
				if parentOpts, poerr := r.CatalogSvc.GetProductOptions(ctx, link.ParentID, r.activeLocale(ctx)); poerr == nil {
					opts = append(opts, parentOpts...)
					sort.SliceStable(opts, func(i, j int) bool { return opts[i].SortOrder < opts[j].SortOrder })
				}
			}
		}
	}
	out := make([]*model1.ProductCustomOption, 0, len(opts))
	for _, o := range opts {
		mo := &model1.ProductCustomOption{
			ID:         o.ID.String(),
			InputType:  string(o.InputType),
			IsRequired: o.IsRequired,
			SortOrder:  o.SortOrder,
			Title:      o.Title,
			HelpText:   o.HelpText,
			MaxChars:   o.MaxChars,
			PriceValue: o.PriceValue,
		}
		if o.MaxFileBytes != nil {
			v := int32(*o.MaxFileBytes)
			mo.MaxFileBytes = &v
		}
		if o.FileExtensions != "" {
			s := o.FileExtensions
			mo.FileExtensions = &s
		}
		if o.PriceType != nil {
			mo.PriceType = string(*o.PriceType)
		}
		if o.VisibleIf != nil {
			mo.VisibleIf = visibleIfToAny(o.VisibleIf)
		}
		for _, v := range o.Values {
			mv := &model1.ProductCustomOptionValue{
				ID:         v.ID.String(),
				Title:      v.Title,
				PriceValue: v.PriceValue,
				PriceType:  string(v.PriceType),
				Quantity:   v.Quantity,
				IsDefault:  v.IsDefault,
			}
			if v.ForcedLineQty != nil {
				q := *v.ForcedLineQty
				mv.ForcedLineQty = &q
			}
			// Slim stand-in built from what GetProductOptions already hydrated - no extra fetch, mirroring the admin toProductOptionValueModel mapping
			if v.SourceProductID != nil {
				prod := &model.Product{
					ID:                  v.SourceProductID.String(),
					URL:                 v.SourceProductURL,
					Sku:                 v.SourceProductSKU,
					Kind:                model.ProductKindSimple,
					DefaultPrice:        int32(v.SourceProductDefaultPrice),
					Price:               int32(v.SourceProductDefaultPrice),
					DefaultCurrencyCode: v.SourceProductCurrency,
				}
				if v.SourceProductName != "" {
					name := v.SourceProductName
					prod.Name = &name
				}
				mv.SourceProduct = prod
			}
			mo.Values = append(mo.Values, mv)
		}
		out = append(out, mo)
	}
	return out, nil
}

// null for non-SUBSCRIPTION kinds and for subscriptions with no terms configured
func (r *productResolver) SubscriptionTerms(ctx context.Context, obj *model.Product) (*model.SubscriptionTerms, error) {
	if r.CatalogSvc == nil || obj == nil {
		return nil, nil
	}
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, err
	}
	terms, err := r.CatalogSvc.GetSubscriptionTerms(ctx, pid)
	if err != nil {
		if errors.Is(err, catalog.ErrSubscriptionTermsNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toFrontSubscriptionTermsModel(terms), nil
}

// Walks active rules whose PRODUCT-level conditions could match, sorted by priority asc - cart-level conditions are ignored on purpose, see promotion.MatchesProduct - and the locale-resolved label falls back to the rule's English-default `public_label` when promotion_rule_contents has no per-locale row
func (r *productResolver) PromotionalDisplays(ctx context.Context, obj *model.Product) ([]*model1.PromotionalDisplay, error) {
	if r.PromotionSvc == nil || obj == nil {
		return nil, nil
	}
	candidate, err := productCandidateFromModel(obj)
	if err != nil {
		return nil, nil
	}
	// The model's values are isPublic-filtered, so match on the pre-filter KVs renderProduct stashed - a rule conditioned on a private attribute still badges
	if pid, perr := uuid.Parse(obj.ID); perr == nil {
		if kvs, ok := r.cachedPromotionAttrKVs(ctx, pid); ok {
			candidate.AttributeKVs = kvs
		}
	}
	store, _ := channel.FromContext(ctx)
	var storeID *uuid.UUID
	if store != nil {
		storeID = &store.ID
	}
	// With loaders mounted, a PLP with N cards collapses every per-rule lookup into one roundtrip each for conditions, content and asset
	if loaders := dataloaders.For(ctx); loaders != nil {
		return r.promotionalDisplaysViaLoaders(ctx, loaders, storeID, candidate)
	}
	rules, err := r.PromotionSvc.DisplaysForProduct(ctx, storeID, candidate)
	if err != nil {
		return nil, nil
	}
	locale := r.activeLocale(ctx)
	out := make([]*model1.PromotionalDisplay, 0, len(rules))
	for _, rule := range rules {
		label := rule.PublicLabel
		if locale != "" {
			label = r.PromotionSvc.ResolvePublicLabel(ctx, rule.ID, locale, rule.PublicLabel)
		}
		out = append(out, &model1.PromotionalDisplay{
			RuleID:        rule.ID.String(),
			RuleSlug:      rule.Slug,
			PublicLabel:   label,
			LabelMedia:    r.loadPromotionMedia(ctx, rule.LabelMediaID),
			BannerMedia:   r.loadPromotionMedia(ctx, rule.BannerMediaID),
			PillText:      rule.PillText,
			PillTextColor: rule.PillTextColor,
			PillBgColor:   rule.PillBgColor,
		})
	}
	return out, nil
}

func (r *productMediaResolver) TransformURL(ctx context.Context, obj *model.ProductMedia, width int32, height *int32) (*string, error) {
	if obj == nil || r.MediaPublicBase == "" || !strings.HasPrefix(obj.MimeType, "image/") {
		return nil, nil
	}
	key, ok := strings.CutPrefix(obj.URL, r.MediaPublicBase)
	if !ok || key == "" {
		return nil, nil // externally-hosted URL, outside our media base - no derivative
	}
	w := media.SnapWidth(int(width))
	spec := fmt.Sprintf("w%d", w)
	if height != nil && *height > 0 {
		spec = fmt.Sprintf("w%dh%d", w, media.SnapWidth(int(*height)))
	}
	u := "/media/t/" + spec + "/" + key
	return &u, nil
}

func (r *queryResolver) ProductsByIds(ctx context.Context, ids []string) ([]*model.Product, error) {
	if len(ids) > 24 {
		ids = ids[:24]
	}
	parsed := make([]uuid.UUID, 0, len(ids))
	for _, raw := range ids {
		if id, err := uuid.Parse(raw); err == nil {
			parsed = append(parsed, id)
		}
	}
	return r.renderProductsByIDs(ctx, parsed), nil
}

func (r *queryResolver) ProductsBySkus(ctx context.Context, skus []string) ([]*model.Product, error) {
	if len(skus) > 24 {
		skus = skus[:24]
	}
	bySKU, err := r.CatalogSvc.GetProductsBySKUs(ctx, skus)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(skus))
	for _, sku := range skus {
		if p, ok := bySKU[sku]; ok {
			ids = append(ids, p.ID)
		}
	}
	return r.renderProductsByIDs(ctx, ids), nil
}

// Display names come back in the active locale where a translation exists - private attributes stay off this storefront surface entirely
func (r *queryResolver) Attributes(ctx context.Context) ([]*model.Attribute, error) {
	attrs, err := r.CatalogSvc.ListAttributes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Attribute, 0, len(attrs))
	for _, a := range attrs {
		if !a.IsPublic {
			continue
		}
		out = append(out, toAttributeModel(a))
	}
	r.localizeAttributeModels(ctx, out)
	return out, nil
}

func (r *queryResolver) Attribute(ctx context.Context, id string) (*model.Attribute, error) {
	attrID, err := uuid.Parse(id)
	if err != nil {
		return nil, invalidID("attribute")
	}
	attr, err := r.CatalogSvc.GetAttribute(ctx, attrID)
	if err != nil {
		return nil, err
	}
	if !attr.IsPublic {
		// The same answer an unknown id gets, so the response doesn't confirm the attribute exists
		return nil, catalog.ErrAttributeNotFound
	}
	out := toAttributeModel(attr)
	r.localizeAttributeModels(ctx, []*model.Attribute{out})
	return out, nil
}

// Same store-scoping rule as Products - the row is ErrProductNotFound when no published listing exists here, and with no resolved store every direct fetch is not-found, which is what defends against ID enumeration on a misconfigured deployment
func (r *queryResolver) Product(ctx context.Context, id string) (*model.Product, error) {
	if err := r.checkCatalogRate(ctx); err != nil {
		return nil, err
	}
	pid, err := uuid.Parse(id)
	if err != nil {
		return nil, invalidID("product")
	}
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return nil, catalog.ErrProductNotFound
	}
	p, err := r.CatalogSvc.GetProductForChannel(ctx, pid, *storeID, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	// One batched prime so the per-variant inStock / salableQuantity resolvers read the request cache - without it a configurable with many options is one Salable query per variant
	primeIDs := make([]uuid.UUID, 0, len(p.Children)+1)
	primeIDs = append(primeIDs, pid)
	for _, c := range p.Children {
		primeIDs = append(primeIDs, c.ID)
	}
	if err := r.primeSalable(ctx, primeIDs, *storeID); err != nil {
		return nil, err
	}
	cols, err := r.CatalogSvc.ListCollectionsByProductIDs(ctx, []uuid.UUID{pid}, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	p.Collections = publishedCollections(cols[pid])
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
	// Same affiliation gate as Products - strip unpublished categories and null the primary when it isn't visible on this store
	{
		oneCats := map[uuid.UUID][]catalog.Category{pid: p.Categories}
		onePrimary := map[uuid.UUID]*catalog.Category{pid: p.PrimaryCategory}
		if catIDs := collectCategoryIDs(oneCats, onePrimary); len(catIDs) > 0 {
			visible, err := r.CatalogSvc.FilterVisibleCategoryIDsForChannel(ctx, catIDs, *storeID)
			if err != nil {
				return nil, err
			}
			kept := p.Categories[:0]
			for _, c := range p.Categories {
				if _, ok := visible[c.ID]; ok {
					kept = append(kept, c)
				}
			}
			p.Categories = kept
			if p.PrimaryCategory != nil {
				if _, ok := visible[p.PrimaryCategory.ID]; !ok {
					p.PrimaryCategory = nil
				}
			}
		}
	}

	// The fetch above is locale-agnostic and skips ancestors, so the breadcrumb name comes back unresolved - re-resolve it the way the Category page does, falling back to the un-hydrated primary category on error rather than sinking the PDP
	if p.PrimaryCategory != nil {
		locale := r.activeLocale(ctx)
		if hydrated, err := r.CatalogSvc.GetPublishedCategoryForChannel(ctx, *storeID, p.PrimaryCategory.ID, locale); err == nil {
			ancestors, err := r.CatalogSvc.GetCategoryAncestors(ctx, p.PrimaryCategory.ID, locale)
			if err != nil {
				return nil, err
			}
			hydrated.Ancestors = ancestors
			p.PrimaryCategory = &hydrated
		}
	}

	return r.renderProduct(ctx, p)
}

func (r *queryResolver) Collections(ctx context.Context) ([]*model.Collection, error) {
	if channelIDFromContext(ctx) == nil {
		return []*model.Collection{}, nil
	}
	collections, err := r.CatalogSvc.ListCollections(ctx, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	collections = publishedCollections(collections)
	out := make([]*model.Collection, len(collections))
	for i, c := range collections {
		out[i] = toCollectionModel(c)
	}
	return out, nil
}

// Linked products are rendered with the main-image role so the storefront draws cards without a follow-up round-trip
func (r *queryResolver) Collection(ctx context.Context, id string) (*model.Collection, error) {
	cid, err := uuid.Parse(id)
	if err != nil {
		return nil, invalidID("collection")
	}
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return nil, catalog.ErrCollectionNotFound
	}
	locale := r.activeLocale(ctx)
	c, err := r.CatalogSvc.GetCollection(ctx, cid, locale)
	if err != nil {
		return nil, err
	}
	if !c.IsPublished {
		return nil, catalog.ErrCollectionNotFound
	}
	return toCollectionModel(c), nil
}

func (r *queryResolver) RootCategories(ctx context.Context) ([]*model.Category, error) {
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return []*model.Category{}, nil
	}
	cats, err := r.CatalogSvc.ListPublishedRootCategoriesForChannel(ctx, *storeID, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]*model.Category, len(cats))
	for i, c := range cats {
		out[i] = toCategoryModel(c)
	}
	return out, nil
}

// Strict ancestor rule with a store in scope: the parent's own row AND every link in its chain to root must be listed-and-published here before any child is exposed
// Otherwise an unpublished parentId enumerates children that are individually published on this store but whose breadcrumb is broken
func (r *queryResolver) CategoryChildren(ctx context.Context, parentID string) ([]*model.Category, error) {
	pid, err := uuid.Parse(parentID)
	if err != nil {
		return nil, invalidID("category")
	}
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return []*model.Category{}, nil
	}
	visible, err := r.CatalogSvc.FilterFullyVisibleCategoryIDsForChannel(ctx, []uuid.UUID{pid}, *storeID)
	if err != nil {
		return nil, err
	}
	if _, ok := visible[pid]; !ok {
		return []*model.Category{}, nil
	}
	cats, err := r.CatalogSvc.ListPublishedCategoryChildrenForChannel(ctx, *storeID, pid, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]*model.Category, len(cats))
	for i, c := range cats {
		out[i] = toCategoryModel(c)
	}
	return out, nil
}

// Gated on the (category, store) listing when a channel is in scope - a non-listed category returns ErrCategoryNotFound so its existence stays private
// Strict ancestor rule: the whole chain to root must be listed-and-published here too, so a broken link reads as not-visible and a category page never renders with ancestors gapped out of the trail
func (r *queryResolver) Category(ctx context.Context, id string) (*model.Category, error) {
	cid, err := uuid.Parse(id)
	if err != nil {
		return nil, invalidID("category")
	}
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return nil, catalog.ErrCategoryNotFound
	}
	c, err := r.CatalogSvc.GetPublishedCategoryForChannel(ctx, *storeID, cid, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	ancestors, err := r.CatalogSvc.GetCategoryAncestors(ctx, cid, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	// {cid} alone would do - the CTE re-walks parent_id from cid itself - but ancestors ride along in case the ancestor service ever back-fills a synthetic chain, and the query already deduplicates by origin
	{
		chainIDs := make([]uuid.UUID, 0, 1+len(ancestors))
		chainIDs = append(chainIDs, cid)
		for _, a := range ancestors {
			chainIDs = append(chainIDs, a.ID)
		}
		visible, err := r.CatalogSvc.FilterFullyVisibleCategoryIDsForChannel(ctx, chainIDs, *storeID)
		if err != nil {
			return nil, err
		}
		if _, ok := visible[cid]; !ok {
			return nil, catalog.ErrCategoryNotFound
		}
	}
	children, err := r.CatalogSvc.ListPublishedCategoryChildrenForChannel(ctx, *storeID, cid, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	c.Ancestors = ancestors
	c.Children = children
	return toCategoryModel(c), nil
}

// Empty result whenever the index isn't available - search is a soft dependency by design, so a downed OpenSearch never takes the storefront with it
func (r *queryResolver) SearchProducts(ctx context.Context, query string, limit *int32, offset *int32) (*model1.ProductSearchResult, error) {
	if err := r.checkCatalogRate(ctx); err != nil {
		return nil, err
	}
	empty := &model1.ProductSearchResult{Items: []*model1.ProductSearchHit{}, TotalCount: 0}
	if r.ProductSearch == nil {
		return empty, nil
	}
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return empty, nil
	}
	lim, off := 0, 0 // limit 0 means unspecified, so the service applies search/general/result_limit_default and clamps - a default here would shadow the admin setting
	if limit != nil {
		lim = int(*limit)
	}
	if offset != nil {
		off = int(*offset)
	}
	// Merchandising: a configured redirect short-circuits results, synonym groups expand the query, pins reorder the returned page
	originalQuery := query
	if r.SearchMerchSvc != nil {
		if target, ok := r.SearchMerchSvc.RedirectFor(ctx, *storeID, query); ok {
			return &model1.ProductSearchResult{Items: []*model1.ProductSearchHit{}, TotalCount: 0, RedirectURL: &target}, nil
		}
		query = r.SearchMerchSvc.ExpandQuery(ctx, *storeID, query)
	}
	var merchBoosts map[uuid.UUID]float64
	if r.SearchMerchSvc != nil {
		merchBoosts = r.SearchMerchSvc.BoostsFor(ctx, *storeID, originalQuery)
	}
	hits, total, err := r.ProductSearch.SearchProducts(ctx, query, *storeID, r.activeLocale(ctx), lim, off, merchBoosts)
	if err != nil {
		// Every tier failed - an empty result is indistinguishable from "no matches", so say so loudly
		log.Printf("[search:error] product search failed on every tier for store %s: %v", *storeID, err)
		graphqlx.MarkUncacheable(ctx)
		return empty, nil
	}
	if r.SearchMerchSvc != nil && off == 0 {
		if pinned := r.SearchMerchSvc.PinsFor(ctx, *storeID, originalQuery); len(pinned) > 0 { // pins key on the ORIGINAL query, not the synonym-expanded one
			rank := make(map[string]int, len(pinned))
			for i, id := range pinned {
				rank[id.String()] = i + 1
			}
			sortHitsByPin(hits, rank)
		}
	}
	items := make([]*model1.ProductSearchHit, len(hits))
	for i, h := range hits {
		var name *string
		if h.Name != "" {
			n := h.Name
			name = &n
		}
		items[i] = &model1.ProductSearchHit{
			ID:   h.ProductID.String(),
			URL:  h.URL,
			Sku:  h.SKU,
			Name: name,
		}
	}
	return &model1.ProductSearchResult{Items: items, TotalCount: int32(total)}, nil
}

// Same degradation contract as searchProducts - no resolved store or an unavailable index gives an empty result, never an error response
func (r *queryResolver) BrowseProducts(ctx context.Context, query *string, filter *model1.BrowseProductsFilter, sort *string, limit *int32, offset *int32) (*model1.BrowseProductsResult, error) {
	if err := r.checkCatalogRate(ctx); err != nil {
		return nil, err
	}
	empty := &model1.BrowseProductsResult{
		Items:      []*model.Product{},
		TotalCount: 0,
		Facets:     []*model1.BrowseFacet{},
	}
	// No early return on a nil index - browse is the only listing path, so it falls through to the Postgres fallback below rather than blanking the storefront
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return empty, nil
	}
	locale := r.activeLocale(ctx)

	lim, off := 0, 0 // limit 0 means unspecified, so the service applies search/browse/page_size_default and clamps - a default here would shadow the admin setting
	if limit != nil {
		lim = int(*limit)
	}
	if offset != nil {
		off = int(*offset)
	}

	indexerFilter := search.BrowseFilter{}
	if filter != nil {
		if filter.CategoryID != nil {
			if id, err := uuid.Parse(*filter.CategoryID); err == nil {
				indexerFilter.CategoryID = &id
			}
		}
		if filter.CollectionID != nil {
			if id, err := uuid.Parse(*filter.CollectionID); err == nil {
				indexerFilter.CollectionID = &id
			}
		}
		if filter.PriceMinCents != nil {
			v := int64(*filter.PriceMinCents)
			indexerFilter.PriceMinCents = &v
		}
		if filter.PriceMaxCents != nil {
			v := int64(*filter.PriceMaxCents)
			indexerFilter.PriceMaxCents = &v
		}
		for _, a := range filter.Attributes {
			if a == nil || a.Slug == "" || len(a.Values) == 0 {
				continue
			}
			indexerFilter.AttributeIn = append(indexerFilter.AttributeIn, search.AttributeIn{
				Slug:   a.Slug,
				Values: a.Values,
			})
		}
		// Private attributes aren't probeable through filters - the native engines gate this themselves, but an external engine tier answers before their gate, so enforce at the seam
		if len(indexerFilter.AttributeIn) > 0 {
			private := r.privateAttributeSlugs(ctx)
			kept := indexerFilter.AttributeIn[:0]
			for _, af := range indexerFilter.AttributeIn {
				if !private[af.Slug] {
					kept = append(kept, af)
				}
			}
			indexerFilter.AttributeIn = kept
		}
		// Incoming value slugs may be this channel's own URL vocabulary, so translate back to global slugs before the search layer resolves them to display names
		if len(indexerFilter.AttributeIn) > 0 {
			indexerFilter.AttributeIn = r.resolveChannelFilterSlugs(ctx, *storeID, indexerFilter.AttributeIn)
		}
		if len(filter.Kinds) > 0 {
			kinds := make([]string, len(filter.Kinds))
			for i, k := range filter.Kinds {
				kinds[i] = string(k)
			}
			indexerFilter.Kinds = kinds
		}
	}

	sortSlug := ""
	if sort != nil {
		sortSlug = *sort
	}

	queryStr := ""
	if query != nil {
		queryStr = *query
	}

	// Same merchandising the search box applies (synonym expansion widens the query, boosts reorder) - without it the /search page ranked differently from the typeahead the shopper just clicked out of
	var browseBoosts map[uuid.UUID]float64
	if r.SearchMerchSvc != nil && queryStr != "" {
		browseBoosts = r.SearchMerchSvc.BoostsFor(ctx, *storeID, queryStr)
		queryStr = r.SearchMerchSvc.ExpandQuery(ctx, *storeID, queryStr)
	}
	// The tier handles primary-then-backup and reports degradation itself, so an error here means BOTH engines failed
	res, err := r.ProductSearch.BrowseProducts(ctx, *storeID, locale, queryStr, indexerFilter, sortSlug, lim, off, browseBoosts)
	if err != nil {
		log.Printf("[search:error] browse failed on every tier for store %s: %v", *storeID, err)
		return empty, nil
	}
	// Same seam rule as the filter input above - a facet on a private attribute never renders, whichever tier built it
	if private := r.privateAttributeSlugs(ctx); len(private) > 0 {
		kept := res.Facets[:0]
		for _, fg := range res.Facets {
			if !private[fg.Slug] {
				kept = append(kept, fg)
			}
		}
		res.Facets = kept
	}
	// Category/collection pages honor that page's filter presentation (admin-pinned order, hidden facets) - applied here so every engine tier gets the same treatment. Soft-fail: a lookup error leaves the default facet order rather than blanking the page
	var facetOverrides []catalog.FacetOverride
	if indexerFilter.CategoryID != nil {
		facetOverrides, _ = r.CatalogSvc.ListCategoryFacetOverrides(ctx, *indexerFilter.CategoryID)
	} else if indexerFilter.CollectionID != nil {
		facetOverrides, _ = r.CatalogSvc.ListCollectionFacetOverrides(ctx, *indexerFilter.CollectionID)
	}
	if len(facetOverrides) > 0 {
		order := make(map[string]int32, len(facetOverrides))
		hidden := make(map[string]bool, len(facetOverrides))
		for _, o := range facetOverrides {
			order[o.AttributeSlug] = o.SortOrder
			if o.Hidden {
				hidden[o.AttributeSlug] = true
			}
		}
		res.Facets = search.ApplyFacetOverrides(res.Facets, order, hidden)
	}

	// Hydrated through the standard storefront loader so the response carries the same Product shape as Products / Product(id), dropping any ID the channel filter rejects - the index can briefly lag a depublish
	// Every per-product association is batched up-front so the per-card loop is a lookup, but the store-scoped publication gate stays per-id because that is what drops the stale index entries
	hydratedIDs := res.IDs
	if len(hydratedIDs) > 0 {
		if err := r.primeListingsAndChannels(ctx, hydratedIDs); err != nil {
			return empty, nil
		}
		if err := r.primeSalable(ctx, hydratedIDs, *storeID); err != nil {
			return empty, nil
		}
		_ = r.primeBackorderable(ctx, hydratedIDs)
		if err := r.primeReviewAggregates(ctx, hydratedIDs); err != nil {
			return empty, nil
		}
		r.primeProductAttributeLocaleValues(ctx, hydratedIDs)
	}
	collectionsByProduct, err := r.CatalogSvc.ListCollectionsByProductIDs(ctx, hydratedIDs, locale)
	if err != nil {
		return empty, nil
	}
	categoriesByProduct, err := r.CatalogSvc.ListCategoriesByProductIDs(ctx, hydratedIDs, locale)
	if err != nil {
		return empty, nil
	}
	primaryCategoryByProduct, err := r.CatalogSvc.ListPrimaryCategoryByProductIDs(ctx, hydratedIDs)
	if err != nil {
		return empty, nil
	}

	// One publication-gated load for the whole page (attribute graph + locale content) plus one media batch, instead of ~9 queries per product - renderProduct takes the pre-fetched media and skips its own
	products, err := r.CatalogSvc.ListProductsForChannelByIDs(ctx, res.IDs, *storeID, locale)
	if err != nil {
		return empty, nil
	}
	mediaByProduct, err := r.CatalogSvc.ListProductMediaByProductIDs(ctx, hydratedIDs)
	if err != nil {
		return empty, nil
	}
	items := make([]*model.Product, 0, len(products))
	for _, p := range products {
		p.Collections = publishedCollections(collectionsByProduct[p.ID])
		p.Categories = categoriesByProduct[p.ID]
		p.PrimaryCategory = primaryCategoryByProduct[p.ID]
		m, err := r.renderProduct(ctx, p, mediaByProduct[p.ID])
		if err != nil {
			continue
		}
		items = append(items, m)
	}

	// Three bulk queries for facet/bucket names, descriptions and this channel's slug vocabulary, so browse stays O(facets) regardless of bucket count
	descs := r.loadFacetDescriptions(ctx, locale, storeID, res.Facets)

	facets := make([]*model1.BrowseFacet, 0, len(res.Facets))
	for _, fg := range res.Facets {
		buckets := make([]*model1.BrowseFacetBucket, 0, len(fg.Buckets))
		for _, b := range fg.Buckets {
			desc := ""
			if m, ok := descs.valueDescByAttrAndDisplay[fg.Slug]; ok {
				desc = m[b.Value]
			}
			display := b.Value
			if m, ok := descs.valueNameByAttrAndDisplay[fg.Slug]; ok {
				if n := m[b.Value]; n != "" {
					display = n
				}
			}
			slug := b.Slug
			if m, ok := descs.valueSlugByAttrAndDisplay[fg.Slug]; ok {
				if s := m[b.Value]; s != "" {
					slug = s
				}
			}
			buckets = append(buckets, &model1.BrowseFacetBucket{
				Value:       display,
				Slug:        slug,
				IsIndexable: b.IsIndexable,
				Description: desc,
				Count:       int32(b.Count),
			})
		}
		name := fg.Name
		if n := descs.attrNameBySlug[fg.Slug]; n != "" {
			name = n
		}
		facets = append(facets, &model1.BrowseFacet{
			Slug:             fg.Slug,
			Name:             name,
			DefaultCollapsed: fg.DefaultCollapsed,
			SortOrder:        fg.SortOrder,
			H1Position:       facetH1PositionToGQL(fg.H1Position),
			Description:      descs.attrDescBySlug[fg.Slug],
			Buckets:          buckets,
		})
	}

	return &model1.BrowseProductsResult{
		Items:      items,
		TotalCount: int32(res.Total),
		Facets:     facets,
	}, nil
}

// The returned order is the dropdown order and the first entry is the default - falls back to hardcoded defaults when the search service isn't wired, like the other search resolvers
func (r *queryResolver) ProductSortOptions(ctx context.Context) ([]*model1.ProductSortOption, error) {
	if r.ProductSearch == nil {
		return []*model1.ProductSortOption{}, nil
	}
	opts := r.ProductSearch.ListSortOptions(ctx)
	out := make([]*model1.ProductSortOption, len(opts))
	for i, o := range opts {
		out[i] = &model1.ProductSortOption{
			Slug:        o.Slug,
			Label:       o.Label,
			IsIndexable: o.IsIndexable,
		}
	}
	return out, nil
}

func (r *Resolver) Category() CategoryResolver { return &categoryResolver{r} }

func (r *Resolver) Collection() CollectionResolver { return &collectionResolver{r} }

func (r *Resolver) Product() ProductResolver { return &productResolver{r} }

func (r *Resolver) ProductMedia() ProductMediaResolver { return &productMediaResolver{r} }

type (
	categoryResolver     struct{ *Resolver }
	collectionResolver   struct{ *Resolver }
	productResolver      struct{ *Resolver }
	productMediaResolver struct{ *Resolver }
)
