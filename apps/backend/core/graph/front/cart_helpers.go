package front

// Helpers live out of cart.resolvers.go so gqlgen's regen pass can't bury them in a comment block at the bottom of the resolver file - same reason blog_helpers.go and url_rewrite_helpers.go exist

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	frontmodel "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/media"
	"github.com/skeletg/skelet/apps/backend/core/internal/money"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// Enforces "a cart belongs to exactly one store" at the front mutation boundary, right after parsing cartID - a mismatch (stale cookie from another store, hand-crafted cartId) is rejected without touching the cart row
func (r *Resolver) assertCartOnCurrentStore(ctx context.Context, cid uuid.UUID) error {
	c, err := r.CartSvc.GetCart(ctx, cid, channel.OptionalCustomerID(ctx))
	if err != nil {
		return cart.ErrCartStoreMismatch
	}
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil || c.ChannelID != sv.ID {
		return cart.ErrCartStoreMismatch
	}
	return nil
}

// The single hydration path for EVERY cart-returning resolver - localized line names + images, applied promotions and save-for-later items, so any operation that touches a cart returns a complete one and needs no follow-up query
func (r *Resolver) fullCartModel(ctx context.Context, c cart.Cart) *frontmodel.Cart {
	m := toCartModel(c)
	r.HydrateCartLineNames(ctx, m)
	r.HydrateCartPromotions(ctx, m, c, "")
	r.HydrateCartSavedItems(ctx, m, channel.OptionalCustomerID(ctx))
	r.HydrateCartLineIssues(ctx, m, channel.OptionalCustomerID(ctx))
	return m
}

// Drift since add-time: availableQuantity only for tracked non-backorderable products, currentUnitPrice only when it differs from the snapshot, and any lookup miss just leaves them unset
func (r *Resolver) HydrateCartLineIssues(ctx context.Context, m *frontmodel.Cart, callerID *uuid.UUID) {
	if m == nil || len(m.Lines) == 0 {
		return
	}
	cid, err := uuid.Parse(m.ID)
	if err != nil {
		return
	}

	if r.CartSvc != nil { // price drift on simple lines, via the cart's own pricing path
		if prices, err := r.CartSvc.CurrentSimpleLinePrices(ctx, cid, callerID); err == nil {
			for _, line := range m.Lines {
				lid, perr := uuid.Parse(line.ID)
				if perr != nil {
					continue
				}
				if cur, ok := prices[lid]; ok && cur != int64(line.UnitPrice) {
					v := int32(cur)
					line.CurrentUnitPrice = &v
				}
			}
		}
	}

	// Stock drift via inventory, with warnings suppressed on backorderable lines
	if r.CatalogSvc == nil || r.InventorySvc == nil {
		return
	}
	store, ok := channel.FromContext(ctx)
	if !ok || store == nil {
		return
	}
	skus := make([]string, 0, len(m.Lines))
	for _, line := range m.Lines {
		skus = append(skus, line.Sku)
	}
	bySKU, err := r.CatalogSvc.GetProductsBySKUs(ctx, skus)
	if err != nil || len(bySKU) == 0 {
		return
	}
	ids := make([]uuid.UUID, 0, len(bySKU))
	for _, p := range bySKU {
		ids = append(ids, p.ID)
	}
	salable, err := r.InventorySvc.SalableByProductIDs(ctx, ids, store.ID)
	if err != nil {
		return
	}
	backorderable, _ := r.InventorySvc.BackorderableByProductIDs(ctx, ids)
	for _, line := range m.Lines {
		p, ok := bySKU[line.Sku]
		if !ok {
			continue
		}
		// A missing key or the -1 sentinel means untracked or always in stock, and backorderable means orderable past zero - neither earns a low-stock warning
		if q, ok := salable[p.ID]; ok && q >= 0 && !backorderable[p.ID] {
			v := q
			line.AvailableQuantity = &v
		}
	}
}

func toCartModel(c cart.Cart) *frontmodel.Cart {
	lines := make([]*frontmodel.CartLine, len(c.Lines))
	for i, l := range c.Lines {
		lines[i] = toCartLineModel(l)
	}
	var subtotal int64
	for _, l := range c.Lines {
		subtotal += money.LineTotal(l.UnitPrice, l.Quantity)
	}
	return &frontmodel.Cart{
		ID:       c.ID.String(),
		Note:     c.Note,
		Lines:    lines,
		Subtotal: int32(subtotal),
	}
}

// The add-time snapshot stores the product's slug because the cart adapter has no locale in scope - this render-time pass swaps in the live localized name so customers don't see slug-shaped strings Empty names stay empty on purpose - the storefront's displayName() renders "(unnamed)" and warns, keeping a content gap visible instead of papering over it Batched to a FIXED number of queries regardless of line count (SKU to product, content-by-ids, media-by-ids, roles-by-ids), replacing a per-line + per-component N+1 on every cart render; a batch or per-line failure leaves that snapshot name and no image in place rather than failing the render
func (r *Resolver) HydrateCartLineNames(ctx context.Context, m *frontmodel.Cart) {
	if r.CatalogSvc == nil || m == nil {
		return
	}
	locale := r.activeLocale(ctx)

	skuSet := make(map[string]struct{})
	for _, line := range m.Lines {
		if line == nil {
			continue
		}
		skuSet[line.Sku] = struct{}{}
		for _, comp := range line.Components {
			if comp != nil {
				skuSet[comp.Sku] = struct{}{}
			}
		}
	}
	if len(skuSet) == 0 {
		return
	}
	skus := make([]string, 0, len(skuSet))
	for s := range skuSet {
		skus = append(skus, s)
	}

	bySKU, err := r.CatalogSvc.GetProductsBySKUs(ctx, skus)
	if err != nil || len(bySKU) == 0 {
		return
	}
	ids := make([]uuid.UUID, 0, len(bySKU))
	for _, p := range bySKU {
		ids = append(ids, p.ID)
	}
	content, _ := r.CatalogSvc.LoadProductsContent(ctx, ids, locale)
	imageBySKU := r.batchLineImages(ctx, bySKU, ids)

	nameFor := func(sku string) (string, bool) {
		p, ok := bySKU[sku]
		if !ok {
			return "", false
		}
		if c, ok := content[p.ID]; ok && c.Name != "" {
			return c.Name, true
		}
		return "", false // no localized name → keep the add-time snapshot
	}

	for _, line := range m.Lines {
		if line == nil {
			continue
		}
		if name, ok := nameFor(line.Sku); ok {
			line.Name = name
		}
		if url := imageBySKU[line.Sku]; url != "" {
			u := url
			line.ImageURL = &u
		}
		for _, comp := range line.Components {
			if comp == nil {
				continue
			}
			if name, ok := nameFor(comp.Sku); ok {
				comp.Name = name
			}
		}
	}
}

// Ownership is enforced by CartSvc.ListSavedItems, and it's best-effort - a lookup miss yields no image or an empty list
func (r *Resolver) HydrateCartSavedItems(ctx context.Context, m *frontmodel.Cart, callerID *uuid.UUID) {
	if r.CartSvc == nil || m == nil {
		return
	}
	cid, err := uuid.Parse(m.ID)
	if err != nil {
		return
	}
	items, err := r.CartSvc.ListSavedItems(ctx, cid, callerID)
	if err != nil {
		return
	}
	m.SavedItems = make([]*frontmodel.SavedItem, 0, len(items))
	if len(items) == 0 {
		return
	}
	imageBySKU := map[string]string{}
	if r.CatalogSvc != nil {
		skus := make([]string, 0, len(items))
		for _, it := range items {
			skus = append(skus, it.ProductSku)
		}
		if bySKU, err := r.CatalogSvc.GetProductsBySKUs(ctx, skus); err == nil && len(bySKU) > 0 {
			ids := make([]uuid.UUID, 0, len(bySKU))
			for _, pr := range bySKU {
				ids = append(ids, pr.ID)
			}
			imageBySKU = r.batchLineImages(ctx, bySKU, ids)
		}
	}
	for _, it := range items {
		si := &frontmodel.SavedItem{
			ID:       it.ID.String(),
			Sku:      it.ProductSku,
			Name:     it.ProductName,
			Quantity: it.Quantity,
		}
		if url := imageBySKU[it.ProductSku]; url != "" {
			u := url
			si.ImageURL = &u
		}
		m.SavedItems = append(m.SavedItems, si)
	}
}

// Two batched queries (media + roles for all product ids at once), preferring the thumbnail role - the one meant for cards/cart/search - then main, then the first ready image
func (r *Resolver) batchLineImages(ctx context.Context, bySKU map[string]catalog.Product, ids []uuid.UUID) map[string]string {
	out := make(map[string]string)
	if len(ids) == 0 {
		return out
	}
	mediaByID, err := r.CatalogSvc.ListProductMediaByProductIDs(ctx, ids)
	if err != nil || len(mediaByID) == 0 {
		return out
	}
	rolesByID, _ := r.CatalogSvc.ListProductMediaRolesByProductIDs(ctx, ids)
	for sku, p := range bySKU {
		items := mediaByID[p.ID]
		if len(items) == 0 {
			continue
		}
		if chosen := pickCartImage(items, rolesByID[p.ID]); chosen != nil {
			out[sku] = r.mediaTransformURL(chosen)
		}
	}
	return out
}

// thumbnail role, then main role, then the first ready image - nil when none qualify
func pickCartImage(items []catalog.ProductMedia, roleMap map[catalog.MediaRole]uuid.UUID) *catalog.ProductMedia {
	for _, role := range []catalog.MediaRole{catalog.MediaRoleThumbnail, catalog.MediaRoleMain} {
		if id, ok := roleMap[role]; ok {
			for i := range items {
				if items[i].ID == id {
					return &items[i]
				}
			}
		}
	}
	for i := range items {
		if items[i].Status == catalog.MediaStatusReady && strings.HasPrefix(items[i].MimeType, "image/") {
			return &items[i]
		}
	}
	return nil
}

// A w160 /media/t derivative only when media serving is configured and the asset sits under the public base, else the raw URL
func (r *Resolver) mediaTransformURL(m *catalog.ProductMedia) string {
	if r.MediaPublicBase != "" && strings.HasPrefix(m.MimeType, "image/") {
		if key, ok := strings.CutPrefix(m.URL, r.MediaPublicBase); ok && key != "" {
			return fmt.Sprintf("/media/t/w%d/%s", media.SnapWidth(160), strings.TrimPrefix(key, "/"))
		}
	}
	return m.URL
}

// Called by every cart-returning resolver so the storefront's totals always match what PlaceOrder computes - no-op when the promotion service isn't wired, and env build/engine eval/applied-display all live in promotion.Service so order placement shares them
func (r *Resolver) HydrateCartPromotions(ctx context.Context, m *frontmodel.Cart, c cart.Cart, countryCode string) {
	if r.PromotionSvc == nil || m == nil {
		return
	}
	store, _ := channel.FromContext(ctx)
	var storeID *uuid.UUID
	var storeSlug, currency string
	if store != nil {
		storeID = &store.ID
		storeSlug = store.Slug
		currency = store.CurrencyCode
	}
	customerID := channel.OptionalCustomerID(ctx)
	// Segment slugs and per-line catalog facts come from EvaluateCart, the same call order placement makes - building the environment separately on each side invites disagreement about which fields to fill, and prices a cart differently from its own checkout
	res, err := r.PromotionSvc.EvaluateCart(ctx, promotion.CartEvalInput{
		CartID:            c.ID,
		SubtotalCents:     int64(m.Subtotal),
		Currency:          currency,
		CountryCode:       countryCode,
		ChannelID:         storeID,
		ChannelSlug:       storeSlug,
		CustomerID:        customerID,
		CustomerGroupSlug: r.resolveCustomerGroupSlug(ctx, customerID),
		Lines:             cart.PromotionLineViews(c),
	})
	if err != nil {
		return
	}
	summary := r.PromotionSvc.BuildAppliedSummary(ctx, res)
	// Bundle component SKUs count too - a customer who added a bundle containing the gift SKU has in effect already received it
	cartSkus := make(map[string]struct{}, len(c.Lines))
	for _, l := range c.Lines {
		cartSkus[l.ProductSku] = struct{}{}
		for _, comp := range l.Components {
			cartSkus[comp.ProductSku] = struct{}{}
		}
	}
	out := make([]*frontmodel.AppliedPromotionCode, len(summary))
	for i, s := range summary {
		out[i] = &frontmodel.AppliedPromotionCode{
			Code:              s.Code,
			RuleName:          s.RuleName,
			PublicLabel:       s.PublicLabel,
			SubtotalDiscount:  int32(s.SubtotalDiscount),
			FreeShipping:      s.FreeShipping,
			InvalidatedReason: s.InvalidatedReason,
			RequiresCode:      s.RequiresCode,
			FreeGifts:         r.resolveFreeGifts(ctx, s.FreeGiftOffers, cartSkus),
		}
	}
	m.AppliedCodes = out
	m.TotalDiscount = int32(res.Result.Ledger.SubtotalDiscountCents())
}

// Lookups are best-effort - a missing product yields a minimal entry with no id and no href, so the offer still surfaces
func (r *Resolver) resolveFreeGifts(
	ctx context.Context,
	offers []promotion.FreeGiftOffer,
	cartSkus map[string]struct{},
) []*frontmodel.PromotionFreeGift {
	if len(offers) == 0 {
		return nil
	}
	out := make([]*frontmodel.PromotionFreeGift, 0, len(offers))
	for _, o := range offers {
		gift := &frontmodel.PromotionFreeGift{
			Sku:      o.ProductSKU,
			Name:     o.ProductSKU,
			Quantity: o.Quantity,
		}
		if _, ok := cartSkus[o.ProductSKU]; ok {
			gift.InCart = true
		}
		if r.CatalogSvc != nil {
			if p, err := r.CatalogSvc.GetProductBySKU(ctx, o.ProductSKU); err == nil {
				idStr := p.ID.String()
				gift.ProductID = &idStr
				// The id route over a prettier url-rewriter slug - it's always available and keeps this catalog-svc-only, and display name isn't resolved at this layer at all (locale-keyed content rows), so the storefront uses the rule's PublicLabel and falls back to the SKU
				href := "/products/" + idStr
				gift.Href = &href
			}
		}
		out = append(out, gift)
	}
	return out
}

// Static group slug only - segment slugs moved to promotion.Service.EvaluateCart so this resolver and order placement can't disagree, and a lookup error leaves the slug empty so a missing row means rules don't fire rather than cart pricing breaking
func (r *Resolver) resolveCustomerGroupSlug(ctx context.Context, customerID *uuid.UUID) string {
	if customerID == nil || r.CustomerSvc == nil {
		return ""
	}
	cust, err := r.CustomerSvc.GetByID(ctx, *customerID)
	if err != nil || cust.Group == nil {
		return ""
	}
	return cust.Group.Slug
}

func toCartLineModel(l cart.CartLine) *frontmodel.CartLine {
	components := make([]*frontmodel.CartLineComponent, len(l.Components))
	for i, c := range l.Components {
		components[i] = &frontmodel.CartLineComponent{
			ID:          c.ID.String(),
			OptionID:    c.BundleOptionID.String(),
			SelectionID: c.BundleSelectionID.String(),
			Sku:         c.ProductSku,
			Name:        c.ProductName,
			UnitPrice:   int32(c.UnitPrice),
			Quantity:    c.Quantity,
			SortOrder:   c.SortOrder,
		}
	}
	options := make([]*frontmodel.CartLineOption, len(l.Options))
	for i, o := range l.Options {
		opt := &frontmodel.CartLineOption{
			ID:              o.ID.String(),
			ProductOptionID: o.ProductOptionID.String(),
			InputType:       o.InputType,
			OptionTitle:     o.OptionTitle,
			ValueLabel:      o.ValueLabel,
			TextValue:       o.TextValue,
			PriceDelta:      int32(o.PriceDelta),
			Quantity:        o.Quantity,
			SortOrder:       o.SortOrder,
			FileURL:         o.FileURL,
		}
		if o.OptionValueID != nil {
			s := o.OptionValueID.String()
			opt.OptionValueID = &s
		}
		options[i] = opt
	}
	return &frontmodel.CartLine{
		ID:         l.ID.String(),
		Name:       l.ProductName,
		Sku:        l.ProductSku,
		UnitPrice:  int32(l.UnitPrice),
		Quantity:   l.Quantity,
		TotalPrice: int32(money.LineTotal(l.UnitPrice, l.Quantity)),
		Components: components,
		Options:    options,
	}
}

// Shared shell for the three save-for-later mutations - parse + store-guard the cart id, run the op, hydrate the returned cart
func (r *mutationResolver) saveForLaterOp(ctx context.Context, cartID string, op func(cid, callerID *uuid.UUID) (cart.Cart, error)) (*frontmodel.Cart, error) {
	cid, err := uuid.Parse(cartID)
	if err != nil {
		return nil, invalidID("cart")
	}
	if err := r.assertCartOnCurrentStore(ctx, cid); err != nil {
		return nil, err
	}
	callerID := channel.OptionalCustomerID(ctx)
	c, err := op(&cid, callerID)
	if err != nil {
		return nil, err
	}
	return r.fullCartModel(ctx, c), nil
}

// Falls back to the configurable parent, whose options ride on every variant, so the presign can enforce the option's own file contract instead of only the global cap
func (r *Resolver) productOptionForUpload(ctx context.Context, p catalog.Product, optionID uuid.UUID) (catalog.ProductOption, error) {
	opts, err := r.CatalogSvc.GetProductOptions(ctx, p.ID, r.activeLocale(ctx))
	if err != nil {
		return catalog.ProductOption{}, err
	}
	for _, o := range opts {
		if o.ID == optionID {
			return o, nil
		}
	}
	if parents, perr := r.CatalogSvc.ListConfigurableParentByChildIDs(ctx, []uuid.UUID{p.ID}); perr == nil {
		if link, ok := parents[p.ID]; ok && link.ParentID != uuid.Nil {
			parentOpts, poerr := r.CatalogSvc.GetProductOptions(ctx, link.ParentID, r.activeLocale(ctx))
			if poerr != nil {
				return catalog.ProductOption{}, poerr
			}
			for _, o := range parentOpts {
				if o.ID == optionID {
					return o, nil
				}
			}
		}
	}
	return catalog.ProductOption{}, apperr.New("OPTION_NOT_ON_PRODUCT", apperr.ClassValidation, "option does not belong to this product")
}

// validateUploadExtension mirrors the add-to-cart allow-list check ("jpg, PNG, .pdf") at presign time
func validateUploadExtension(allowRaw, filename string) error {
	if strings.TrimSpace(allowRaw) == "" {
		return nil
	}
	ext := ""
	if _, e, found := strings.CutLast(filename, "."); found && e != "" {
		ext = strings.ToLower(e)
	}
	for part := range strings.SplitSeq(allowRaw, ",") {
		if e := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(part)), "."); e != "" && e == ext {
			return nil
		}
	}
	return apperr.New("FILE_TYPE_NOT_ALLOWED", apperr.ClassValidation, fmt.Sprintf("file type %q is not allowed for this option", ext))
}

// Built like placement's rate request, so the quote is what placement charges
func (r *Resolver) cartRateRequest(ctx context.Context, sv *channel.Channel, c cart.Cart, country, region, postal string) (shipping.RateRequest, *frontmodel.Cart) {
	country = strings.ToUpper(strings.TrimSpace(country))
	m := toCartModel(c)
	r.HydrateCartPromotions(ctx, m, c, country)
	items := make([]shipping.RateItem, len(c.Lines))
	for i, l := range c.Lines {
		items[i] = shipping.RateItem{SKU: l.ProductSku, Quantity: l.Quantity, UnitPriceCents: l.UnitPrice}
	}
	return shipping.RateRequest{
		ChannelID: sv.ID,
		Destination: shipping.Address{
			CountryCode: country,
			Region:      strings.ToUpper(strings.TrimSpace(region)),
			PostalCode:  strings.TrimSpace(postal),
		},
		SubtotalCents: max(int64(m.Subtotal)-int64(m.TotalDiscount), 0),
		CurrencyCode:  sv.CurrencyCode,
		Items:         items,
	}, m
}
