package front

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	model1 "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/internal/recommend"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

func (r *mutationResolver) CreateGuestCart(ctx context.Context) (*model1.Cart, error) {
	store, ok := channel.FromContext(ctx)
	if !ok {
		return nil, storeRequired()
	}
	c, err := r.CartSvc.CreateGuestCart(ctx, store.ID)
	if err != nil {
		return nil, err
	}
	return r.fullCartModel(ctx, c), nil
}

func (r *mutationResolver) AddToCart(ctx context.Context, cartID string, sku string, quantity float64) (*model1.Cart, error) {
	cid, err := uuid.Parse(cartID)
	if err != nil {
		return nil, invalidID("cart")
	}
	if err := r.assertCartOnCurrentStore(ctx, cid); err != nil {
		return nil, err
	}
	c, err := r.CartSvc.AddToCart(ctx, cid, channel.OptionalCustomerID(ctx), sku, quantity)
	if err != nil {
		return nil, err
	}
	return r.fullCartModel(ctx, c), nil
}

// Component rows are snapshotted onto the line, not looked up later
func (r *mutationResolver) AddBundleToCart(ctx context.Context, input model1.AddBundleToCartInput) (*model1.Cart, error) {
	cid, err := uuid.Parse(input.CartID)
	if err != nil {
		return nil, invalidID("cart")
	}
	if err := r.assertCartOnCurrentStore(ctx, cid); err != nil {
		return nil, err
	}
	selections := make([]cart.AddBundleSelectionInput, 0, len(input.Selections))
	for _, sel := range input.Selections {
		optID, err := uuid.Parse(sel.OptionID)
		if err != nil {
			return nil, invalidID("option")
		}
		ids := make([]uuid.UUID, 0, len(sel.SelectionIds))
		for _, raw := range sel.SelectionIds {
			id, err := uuid.Parse(raw)
			if err != nil {
				return nil, invalidID("selection")
			}
			ids = append(ids, id)
		}
		selections = append(selections, cart.AddBundleSelectionInput{OptionID: optID, SelectionIDs: ids})
	}
	c, err := r.CartSvc.AddBundleToCart(ctx, cid, channel.OptionalCustomerID(ctx), cart.AddBundleToCartInput{
		ProductSku: input.Sku,
		Quantity:   input.Quantity,
		Selections: selections,
	})
	if err != nil {
		return nil, err
	}
	return r.fullCartModel(ctx, c), nil
}

// Option rows are snapshotted onto the line and priced server-side, never from the client
func (r *mutationResolver) AddConfiguredToCart(ctx context.Context, input model1.AddConfiguredToCartInput) (*model1.Cart, error) {
	cid, err := uuid.Parse(input.CartID)
	if err != nil {
		return nil, invalidID("cart")
	}
	if err := r.assertCartOnCurrentStore(ctx, cid); err != nil {
		return nil, err
	}
	options := make([]cart.CartLineOptionInput, 0, len(input.Options))
	for _, o := range input.Options {
		optID, err := uuid.Parse(o.OptionID)
		if err != nil {
			return nil, invalidID("option")
		}
		valueIDs := make([]uuid.UUID, 0, len(o.ValueIds))
		for _, raw := range o.ValueIds {
			id, err := uuid.Parse(raw)
			if err != nil {
				return nil, invalidID("option value")
			}
			valueIDs = append(valueIDs, id)
		}
		text := ""
		if o.TextValue != nil {
			text = *o.TextValue
		}
		var fileAssetID *uuid.UUID
		if o.FileAssetID != nil && *o.FileAssetID != "" {
			fid, err := uuid.Parse(*o.FileAssetID)
			if err != nil {
				return nil, invalidID("file asset")
			}
			fileAssetID = &fid
		}
		options = append(options, cart.CartLineOptionInput{OptionID: optID, ValueIDs: valueIDs, TextValue: text, FileAssetID: fileAssetID})
	}
	c, err := r.CartSvc.AddConfiguredToCart(ctx, cid, channel.OptionalCustomerID(ctx), cart.AddConfiguredToCartInput{
		ProductSku: input.Sku,
		Quantity:   input.Quantity,
		Options:    options,
	})
	if err != nil {
		return nil, err
	}
	return r.fullCartModel(ctx, c), nil
}

// The client PUTs the bytes, then references the returned assetId via ProductOptionSelectionInput.fileAssetId - extension/size constraints are enforced again at addConfiguredToCart
// Scoped to product-options/<productId> so admins can inspect or clean per-product uploads
func (r *mutationResolver) RequestProductOptionFileUpload(ctx context.Context, sku string, optionID string, filename string, mimeType string, sizeBytes int32) (*model1.ProductOptionFileUpload, error) {
	if err := r.checkRate(ctx, "option_file_upload"); err != nil {
		return nil, err
	}
	if r.MediaLibrarySvc == nil || r.CatalogSvc == nil {
		return nil, errNotConfigured("file uploads")
	}
	optID, err := uuid.Parse(optionID)
	if err != nil {
		return nil, invalidID("option")
	}
	p, err := r.CatalogSvc.GetProductBySKU(ctx, sku)
	if err != nil {
		return nil, err
	}
	// Validate against the option's own contract before presigning - without this a client can park near-global-cap objects that could never attach at addConfiguredToCart
	opt, err := r.productOptionForUpload(ctx, p, optID)
	if err != nil {
		return nil, err
	}
	if !opt.InputType.IsFile() {
		return nil, apperr.New("OPTION_NOT_FILE", apperr.ClassValidation, fmt.Sprintf("option %q does not take a file", opt.Title))
	}
	if opt.MaxFileBytes != nil && *opt.MaxFileBytes > 0 && int64(sizeBytes) > *opt.MaxFileBytes {
		return nil, apperr.New("FILE_TOO_LARGE", apperr.ClassValidation, fmt.Sprintf("file exceeds the option's limit of %d bytes", *opt.MaxFileBytes))
	}
	if err := validateUploadExtension(opt.FileExtensions, filename); err != nil {
		return nil, err
	}
	res, err := r.MediaLibrarySvc.RequestUpload(ctx, medialibrary.RequestUploadInput{
		Path:      "product-options/" + p.ID.String(),
		Filename:  filename,
		MimeType:  mimeType,
		SizeBytes: int64(sizeBytes),
	})
	if err != nil {
		return nil, err
	}
	return &model1.ProductOptionFileUpload{
		AssetID:   res.Asset.ID.String(),
		UploadURL: res.UploadURL,
		ExpiresAt: res.ExpiresAt,
	}, nil
}

func (r *mutationResolver) UpdateCartLine(ctx context.Context, cartID string, lineID string, quantity float64) (*model1.Cart, error) {
	cid, err := uuid.Parse(cartID)
	if err != nil {
		return nil, invalidID("cart")
	}
	lid, err := uuid.Parse(lineID)
	if err != nil {
		return nil, invalidID("line")
	}
	if err := r.assertCartOnCurrentStore(ctx, cid); err != nil {
		return nil, err
	}
	c, err := r.CartSvc.UpdateCartLine(ctx, cid, lid, channel.OptionalCustomerID(ctx), quantity)
	if err != nil {
		return nil, err
	}
	return r.fullCartModel(ctx, c), nil
}

func (r *mutationResolver) RemoveFromCart(ctx context.Context, cartID string, lineID string) (*model1.Cart, error) {
	cid, err := uuid.Parse(cartID)
	if err != nil {
		return nil, invalidID("cart")
	}
	lid, err := uuid.Parse(lineID)
	if err != nil {
		return nil, invalidID("line")
	}
	if err := r.assertCartOnCurrentStore(ctx, cid); err != nil {
		return nil, err
	}
	c, err := r.CartSvc.RemoveFromCart(ctx, cid, lid, channel.OptionalCustomerID(ctx))
	if err != nil {
		return nil, err
	}
	return r.fullCartModel(ctx, c), nil
}

func (r *mutationResolver) SetCartEmail(ctx context.Context, cartID string, email string) (*model1.Cart, error) {
	cid, err := uuid.Parse(cartID)
	if err != nil {
		return nil, invalidID("cart")
	}
	if err := r.assertCartOnCurrentStore(ctx, cid); err != nil {
		return nil, err
	}
	if err := r.CartSvc.SetCartEmail(ctx, cid, channel.OptionalCustomerID(ctx), email); err != nil {
		return nil, err
	}
	c, err := r.CartSvc.GetCart(ctx, cid, channel.OptionalCustomerID(ctx))
	if err != nil {
		return nil, err
	}
	return r.fullCartModel(ctx, c), nil
}

func (r *mutationResolver) SetCartNote(ctx context.Context, cartID string, note string) (*model1.Cart, error) {
	cid, err := uuid.Parse(cartID)
	if err != nil {
		return nil, invalidID("cart")
	}
	if err := r.assertCartOnCurrentStore(ctx, cid); err != nil {
		return nil, err
	}
	if err := r.CartSvc.SetCartNote(ctx, cid, channel.OptionalCustomerID(ctx), note); err != nil {
		return nil, err
	}
	c, err := r.CartSvc.GetCart(ctx, cid, channel.OptionalCustomerID(ctx))
	if err != nil {
		return nil, err
	}
	return r.fullCartModel(ctx, c), nil
}

func (r *mutationResolver) SaveForLater(ctx context.Context, cartID string, lineID string) (*model1.Cart, error) {
	return r.saveForLaterOp(ctx, cartID, func(cid, callerID *uuid.UUID) (cart.Cart, error) {
		lid, err := uuid.Parse(lineID)
		if err != nil {
			return cart.Cart{}, invalidID("line")
		}
		return r.CartSvc.SaveForLater(ctx, *cid, lid, callerID)
	})
}

func (r *mutationResolver) MoveSavedToCart(ctx context.Context, cartID string, savedItemID string) (*model1.Cart, error) {
	return r.saveForLaterOp(ctx, cartID, func(cid, callerID *uuid.UUID) (cart.Cart, error) {
		sid, err := uuid.Parse(savedItemID)
		if err != nil {
			return cart.Cart{}, invalidID("saved item")
		}
		return r.CartSvc.MoveSavedToCart(ctx, *cid, sid, callerID)
	})
}

func (r *mutationResolver) RemoveSavedItem(ctx context.Context, cartID string, savedItemID string) (*model1.Cart, error) {
	return r.saveForLaterOp(ctx, cartID, func(cid, callerID *uuid.UUID) (cart.Cart, error) {
		sid, err := uuid.Parse(savedItemID)
		if err != nil {
			return cart.Cart{}, invalidID("saved item")
		}
		return r.CartSvc.RemoveSavedItem(ctx, *cid, sid, callerID)
	})
}

func (r *mutationResolver) ClearCart(ctx context.Context, cartID string) (*model1.Cart, error) {
	cid, err := uuid.Parse(cartID)
	if err != nil {
		return nil, invalidID("cart")
	}
	if err := r.assertCartOnCurrentStore(ctx, cid); err != nil {
		return nil, err
	}
	if err := r.CartSvc.ClearCart(ctx, cid, channel.OptionalCustomerID(ctx)); err != nil {
		return nil, err
	}
	// Saved items survive, and the full cart comes back like every other cart mutation so clients need no follow-up query
	c, err := r.CartSvc.GetCart(ctx, cid, channel.OptionalCustomerID(ctx))
	if err != nil {
		return nil, err
	}
	return r.fullCartModel(ctx, c), nil
}

func (r *mutationResolver) MergeGuestCart(ctx context.Context, guestCartID string) (*model1.Cart, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	gid, err := uuid.Parse(guestCartID)
	if err != nil {
		return nil, invalidID("cart")
	}
	if err := r.assertCartOnCurrentStore(ctx, gid); err != nil {
		return nil, err
	}
	c, err := r.CartSvc.MergeGuestCart(ctx, claims.CustomerID, gid)
	if err != nil {
		return nil, err
	}
	return r.fullCartModel(ctx, c), nil
}

// The v2 engine allows many codes per cart, so calling this again with a different code stacks rather than replaces
func (r *mutationResolver) ApplyCoupon(ctx context.Context, cartID string, code string) (*model1.Cart, error) {
	if r.PromotionSvc == nil {
		return nil, errNotConfigured("promotions")
	}
	cid, err := uuid.Parse(cartID)
	if err != nil {
		return nil, invalidID("cart")
	}
	if err := r.assertCartOnCurrentStore(ctx, cid); err != nil {
		return nil, err
	}
	c, err := r.CartSvc.GetCart(ctx, cid, channel.OptionalCustomerID(ctx))
	if err != nil {
		return nil, err
	}
	if _, err := r.PromotionSvc.ApplyCode(ctx, cid, c.ChannelID, code, channel.OptionalCustomerID(ctx)); err != nil {
		return nil, err
	}
	return r.fullCartModel(ctx, c), nil
}

// No-op when the code isn't applied - the engine re-evaluates so rules that depended on the removed code drop out of totals
func (r *mutationResolver) RemoveCoupon(ctx context.Context, cartID string, code string) (*model1.Cart, error) {
	if r.PromotionSvc == nil {
		return nil, errNotConfigured("promotions")
	}
	cid, err := uuid.Parse(cartID)
	if err != nil {
		return nil, invalidID("cart")
	}
	if err := r.assertCartOnCurrentStore(ctx, cid); err != nil {
		return nil, err
	}
	c, err := r.CartSvc.GetCart(ctx, cid, channel.OptionalCustomerID(ctx))
	if err != nil {
		return nil, err
	}
	if err := r.PromotionSvc.RemoveCode(ctx, cid, code); err != nil {
		return nil, err
	}
	return r.fullCartModel(ctx, c), nil
}

func (r *queryResolver) MyCart(ctx context.Context) (*model1.Cart, error) {
	claims, err := channel.RequireCustomerForCurrentStore(ctx)
	if err != nil {
		return nil, err
	}
	store, ok := channel.FromContext(ctx)
	if !ok {
		return nil, storeRequired()
	}
	c, err := r.CartSvc.GetOrCreateCart(ctx, claims.CustomerID, store.ID)
	if err != nil {
		return nil, err
	}
	return r.fullCartModel(ctx, c), nil
}

// Guest carts are visible to anyone holding the UUID, customer carts only to the owner, and either way the cart has to belong to the resolved channel So a guest carrying a store-A cookie while browsing store-B sees a fresh empty cart rather than the wrong store's contents nil is what the storefront reads as "no cart" - the next add-to-cart creates one on the correct store via ensureCartID
func (r *queryResolver) Cart(ctx context.Context, id string) (*model1.Cart, error) {
	cid, err := uuid.Parse(id)
	if err != nil {
		return nil, nil
	}
	c, err := r.CartSvc.GetCart(ctx, cid, channel.OptionalCustomerID(ctx))
	if errors.Is(err, cart.ErrCartForbidden) || errors.Is(err, cart.ErrCartNotActive) {
		return nil, nil
	}
	if err != nil {
		return nil, err // a DB failure isn't "no cart" - reading it as one sent the shopper off to start a fresh cart
	}
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil || c.ChannelID != sv.ID {
		return nil, nil
	}
	return r.fullCartModel(ctx, c), nil
}

// Empty list when no zone covers the country - the storefront checkout renders this as the "Standard / Express / Free over $50" picker
func (r *queryResolver) ShippingQuotes(ctx context.Context, countryCode string, cartID *string, regionCode *string, postalCode *string) ([]*model1.ShippingQuote, error) {
	if r.ShippingSvc == nil {
		return nil, nil
	}
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil {
		return nil, storeRequired()
	}
	var c cart.Cart
	switch {
	case cartID != nil && *cartID != "":
		cid, err := uuid.Parse(*cartID)
		if err != nil {
			return nil, invalidID("cart")
		}
		if c, err = r.CartSvc.GetCart(ctx, cid, channel.OptionalCustomerID(ctx)); err != nil {
			return nil, err
		}
		if c.ChannelID != sv.ID {
			return nil, cart.ErrCartStoreMismatch
		}
	case channel.OptionalCustomerID(ctx) != nil:
		// An empty cart quotes what they'd pay checking out right now
		if own, err := r.CartSvc.GetOrCreateCart(ctx, *channel.OptionalCustomerID(ctx), sv.ID); err == nil {
			c = own
		}
	}
	req, _ := r.cartRateRequest(ctx, sv, c, countryCode, ptr.Deref(regionCode), ptr.Deref(postalCode))
	quotes, err := r.ShippingSvc.QuoteAll(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make([]*model1.ShippingQuote, len(quotes))
	for i, q := range quotes {
		out[i] = &model1.ShippingQuote{
			CarrierSlug: q.CarrierSlug,
			CarrierName: q.CarrierName,
			Code:        q.Method.Code,
			Name:        q.Method.Name,
			PriceCents:  int32(q.Method.PriceCents),
		}
	}
	return out, nil
}

func (r *queryResolver) PickupLocations(ctx context.Context) ([]*model1.PickupLocation, error) {
	// Public on purpose - these are shop addresses, the same thing a store locator publishes, and only pickup-enabled ones come back
	if r.InventorySvc == nil {
		return []*model1.PickupLocation{}, nil
	}
	locs, err := r.InventorySvc.PickupLocations(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model1.PickupLocation, len(locs))
	for i, l := range locs {
		out[i] = &model1.PickupLocation{
			Slug:         l.Slug,
			Name:         l.Name,
			Line1:        l.Line1,
			City:         l.City,
			PostalCode:   l.PostalCode,
			CountryCode:  l.CountryCode,
			Instructions: l.PickupInstructions,
		}
	}
	return out, nil
}

func (r *queryResolver) CartEstimate(ctx context.Context, cartID string, countryCode string, regionCode *string) (*model1.CartEstimate, error) {
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil {
		return nil, storeRequired()
	}
	cid, err := uuid.Parse(cartID)
	if err != nil {
		return nil, invalidID("cart")
	}
	c, err := r.CartSvc.GetCart(ctx, cid, channel.OptionalCustomerID(ctx))
	if err != nil {
		return nil, err
	}
	if c.ChannelID != sv.ID {
		return nil, cart.ErrCartStoreMismatch
	}
	req, m := r.cartRateRequest(ctx, sv, c, countryCode, ptr.Deref(regionCode), "")

	out := make([]*model1.ShippingQuote, 0)
	cheapest := int64(0)
	haveShip := false
	if r.ShippingSvc != nil {
		quotes, qerr := r.ShippingSvc.QuoteAll(ctx, req)
		if qerr != nil {
			return nil, qerr
		}
		for _, q := range quotes {
			out = append(out, &model1.ShippingQuote{
				CarrierSlug: q.CarrierSlug, CarrierName: q.CarrierName,
				Code: q.Method.Code, Name: q.Method.Name, PriceCents: int32(q.Method.PriceCents),
			})
			if !haveShip || q.Method.PriceCents < cheapest {
				cheapest = q.Method.PriceCents
				haveShip = true
			}
		}
	}

	var taxCents int64
	if r.OrderSvc != nil {
		if t, terr := r.OrderSvc.EstimateTax(ctx, order.TaxEstimateInput{
			Store:         sv,
			CustomerID:    channel.OptionalCustomerID(ctx),
			Cart:          c,
			DiscountCents: int64(m.TotalDiscount),
			ShippingCents: cheapest,
			CountryCode:   req.Destination.CountryCode,
			RegionCode:    req.Destination.Region,
		}); terr == nil {
			taxCents = t
		}
	}

	total := req.SubtotalCents + cheapest
	if !sv.TaxInclusivePricing {
		total += taxCents
	}

	return &model1.CartEstimate{
		SubtotalCents:         int32(req.SubtotalCents),
		ShippingOptions:       out,
		CheapestShippingCents: int32(cheapest),
		TaxCents:              int32(taxCents),
		EstimatedTotalCents:   int32(total),
	}, nil
}

func (r *queryResolver) FreeShippingThreshold(ctx context.Context, countryCode *string) (int32, error) {
	if r.ShippingSvc == nil {
		return 0, nil
	}
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil {
		return 0, nil
	}
	country := ""
	if countryCode != nil {
		country = strings.ToUpper(strings.TrimSpace(*countryCode))
	}
	cents, err := r.ShippingSvc.FreeShippingThreshold(ctx, sv.ID, country)
	if err != nil {
		return 0, err
	}
	return int32(cents), nil
}

// CROSSSELL links of the cart's products first, also-bought co-occurrences as fallback, deduped, minus what's already in the cart, capped at 8
// Best-effort - a resolution failure yields an empty rail, never an error
func (r *queryResolver) CartRecommendations(ctx context.Context, cartID string) ([]*model.Product, error) {
	empty := []*model.Product{}
	cid, err := uuid.Parse(cartID)
	if err != nil {
		return empty, nil
	}
	if err := r.assertCartOnCurrentStore(ctx, cid); err != nil {
		return empty, nil
	}
	cart, err := r.CartSvc.GetCart(ctx, cid, channel.OptionalCustomerID(ctx))
	if err != nil || len(cart.Lines) == 0 {
		return empty, nil
	}
	skus := make([]string, len(cart.Lines))
	for i, l := range cart.Lines {
		skus[i] = l.ProductSku
	}
	products, err := r.CatalogSvc.GetProductsBySKUs(ctx, skus)
	if err != nil {
		return empty, nil
	}
	inCart := map[uuid.UUID]bool{}
	var cartProductIDs []uuid.UUID
	for _, l := range cart.Lines {
		p, ok := products[l.ProductSku]
		if !ok || inCart[p.ID] {
			continue
		}
		inCart[p.ID] = true
		cartProductIDs = append(cartProductIDs, p.ID)
	}
	seen := map[uuid.UUID]bool{}
	var recs []uuid.UUID
	add := func(ids []uuid.UUID) {
		for _, id := range ids {
			if len(recs) >= 8 || inCart[id] || seen[id] {
				continue
			}
			seen[id] = true
			recs = append(recs, id)
		}
	}
	// one query per source for the whole cart - per-line lookups made a remote recommender an HTTP call for every line
	if linked, lerr := r.RecommendSvc.CrosssellsFor(ctx, cartProductIDs); lerr == nil {
		for _, pid := range cartProductIDs {
			add(linked[pid])
		}
	}
	if len(recs) < 4 {
		if ab, aerr := r.RecommendSvc.AlsoBoughtFor(ctx, cartProductIDs, recommend.AlsoBoughtDefaultLimit); aerr == nil {
			for _, pid := range cartProductIDs {
				add(ab[pid])
			}
		}
	}
	if len(recs) == 0 {
		return empty, nil
	}
	return r.renderProductsByIDs(ctx, recs), nil
}
