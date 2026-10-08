package wiring

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/audit"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/download"
	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/mailsend"
	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/internal/paymentvault"
	"github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/internal/pricing"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/storecredit"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// CustomerStoreLookup adapts channel.Service to customer.StoreLookup, projecting the fields the customer service needs to render outbound emails (storefront URL, "from" address) - lives in wiring so customer doesn't import channel directly
type CustomerStoreLookup struct {
	svc *channel.Service
	cfg *config.Service
}

func (l CustomerStoreLookup) GetByID(ctx context.Context, id uuid.UUID) (customer.StoreSnapshot, error) {
	sv, err := l.svc.GetByID(ctx, id)
	if err != nil {
		return customer.StoreSnapshot{}, err
	}
	// EmailFromAddress / EmailFromName live in the config registry (general/contact/*), so read them scoped to this channel - best-effort: missing values fall back to the registry default (empty string) and the customer service applies its own no-reply@skelet.local fallback when rendering email headers
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &sv.ID}
	fromAddr, _ := l.cfg.GetString(ctx, "general/contact/email_from_address", scope)
	fromName, _ := l.cfg.GetString(ctx, "general/contact/email_from_name", scope)
	return customer.StoreSnapshot{
		ID:               sv.ID,
		PublicURL:        sv.PublicURL,
		EmailFromAddress: fromAddr,
		EmailFromName:    fromName,
	}, nil
}

// CatalogDefaultLocaleProvider supplies the catalog's system-wide canonical locale: the DEFAULT channel's default_locale, deliberately NOT the active request channel's - it anchors write-side invariants (ErrLocaleMustBeDefault decides which content row is canonical) so it must be one globally stable value, not whatever channel the admin's header points at - "en" is the last resort when no default channel is flagged, and GetDefault is Valkey-cached so the per-fallback lookup is cheap
type CatalogDefaultLocaleProvider struct {
	channels *channel.Service
}

func (p CatalogDefaultLocaleProvider) DefaultLocale(ctx context.Context) (string, error) {
	if p.channels == nil {
		return channel.FallbackLocale, nil
	}
	return p.channels.InstallDefaultLocale(ctx), nil
}

type CatalogInventoryEnsurer struct { // adapts inventory.Service to the slim catalog.InventoryEnsurer - drops the returned Item value the catalog doesn't need
	svc *inventory.Service
}

func (e CatalogInventoryEnsurer) EnsureItem(ctx context.Context, productID uuid.UUID, tracked bool) error {
	_, err := e.svc.EnsureItem(ctx, productID, tracked)
	return err
}

func (e CatalogInventoryEnsurer) SyncProductTracking(ctx context.Context, productID uuid.UUID, tracked bool) error {
	_, err := e.svc.SyncProductTracking(ctx, productID, tracked)
	return err
}

// AuditURLCollisions adapts audit.Service to catalog.URLCollisionRecorder, writing a product URL collision (intended path taken → a suffix was used) to the audit log under the "catalog.url_collision" action so an operator can review + resolve them later
type AuditURLCollisions struct {
	audit *audit.Service
}

func (a AuditURLCollisions) RecordURLCollision(ctx context.Context, entityType, sku string, entityID uuid.UUID, requested, assigned string) {
	a.audit.Record(ctx, audit.Entry{
		Action:  "catalog.url_collision",
		Success: true,
		Detail: map[string]any{
			"entity_type": entityType,
			"entity_id":   entityID.String(),
			"sku":         sku,
			"requested":   requested,
			"assigned":    assigned,
		},
	})
}

// CartCatalogAdapter glues catalog.Service to cart.ProductResolver - it passes through the kind lookup and translates catalog's typed BundleOption values into cart-package snapshot types so the cart package never needs to import catalog domain types
type CartCatalogAdapter struct {
	svc       *catalog.Service
	customers *customer.Service // resolves a customer's group for tier pricing - nil in contexts without customer access (then group tiers don't apply)
	channels  *channel.Service  // the channel's currency decides which FIXED option surcharges can be charged
}

// CartFileAdapter resolves an uploaded media asset for a FILE-type product option pick - it reads the asset row (filename, mime, public URL) and HEADs object storage for the authoritative byte size + existence, so the cart validates against the real upload rather than client-claimed values
type CartFileAdapter struct {
	media   *medialibrary.Service
	storage *objstore.Storage
}

func (a CartFileAdapter) ResolveFileAsset(ctx context.Context, productID, id uuid.UUID) (cart.FileAsset, error) {
	asset, err := a.media.GetAsset(ctx, id)
	if err != nil {
		return cart.FileAsset{}, err
	}
	// Scope fence: a pick may only reference an upload the product-option presign flow minted for THIS product - not arbitrary media-library assets (admin catalog images, other uploads), and not another product's option upload
	if asset.DirectoryPrefix != "product-options/"+productID.String() {
		return cart.FileAsset{}, fmt.Errorf("asset is not an option upload for this product")
	}
	size, _, exists, err := a.storage.Head(ctx, asset.ObjectKey)
	if err != nil {
		return cart.FileAsset{}, fmt.Errorf("verify uploaded file: %w", err)
	}
	if !exists {
		return cart.FileAsset{}, fmt.Errorf("uploaded file not received")
	}
	return cart.FileAsset{
		Filename:  asset.Filename,
		MimeType:  asset.MimeType,
		SizeBytes: size,
		URL:       asset.URL,
	}, nil
}

// WebformFileStore adapts objstore.Storage to webform.FileStore: presign uploads for form file fields and Head-validate them, translating the (size, mime, exists, err) shape into the (size, mime, err) the webform service expects
type WebformFileStore struct {
	storage *objstore.Storage
}

func (a WebformFileStore) PresignUpload(ctx context.Context, key, contentType string, sizeBytes int64, ttl time.Duration) (string, error) {
	return a.storage.PresignUpload(ctx, key, contentType, sizeBytes, ttl)
}

func (a WebformFileStore) MaxUploadBytes() int64 {
	return a.storage.MaxUploadBytes()
}

func (a WebformFileStore) Head(ctx context.Context, key string) (int64, string, error) {
	size, mime, exists, err := a.storage.Head(ctx, key)
	if err != nil {
		return 0, "", err
	}
	if !exists {
		return 0, "", fmt.Errorf("uploaded file not received")
	}
	return size, mime, nil
}

// OrderChannelLoader glues channel.Service to order.ChannelLoader so a pre-fulfillment edit can load the order's channel (tax-inclusive + currency config) by id rather than relying on request context
type OrderChannelLoader struct {
	svc *channel.Service
}

func (a OrderChannelLoader) LoadChannel(ctx context.Context, id uuid.UUID) (channel.Channel, error) {
	return a.svc.GetByID(ctx, id)
}

// OrderProductPricer glues the catalog pricing seam to order.ProductPricer so an edit can price an added simple line server-side (same canonical pricing the cart uses) - it reuses CartCatalogAdapter's ResolveSimplePricing and maps the cart pricing snapshot into the order package's type
type OrderProductPricer struct {
	svc CartCatalogAdapter
}

// HasRequiredOptions implements order.OptionAwarePricer: drafts/edits reject SKUs whose option set includes required entries
func (a OrderProductPricer) HasRequiredOptions(ctx context.Context, channelID uuid.UUID, sku string) (bool, error) {
	res, err := a.svc.ResolveProductOptions(ctx, channelID, sku, nil)
	if err != nil {
		if errors.Is(err, cart.ErrOptionsUnsupportedKind) {
			return false, nil // the kind never carries options - not an error, just nothing to gate
		}
		return false, err
	}
	return cart.HasVisiblyRequiredOptions(res), nil
}

func (a OrderProductPricer) ResolveSimplePricing(ctx context.Context, channelID uuid.UUID, sku string) (order.SimplePricing, error) {
	p, err := a.svc.ResolveSimplePricing(ctx, channelID, sku, nil)
	if err != nil {
		return order.SimplePricing{}, err
	}
	return order.SimplePricing{UnitPriceCents: p.UnitPrice, ProductName: p.ProductName, CostCents: p.CostCents}, nil
}

// ResolveSimplePricing returns the canonical name + price for a directly purchasable SKU on the given channel - used by cart.Service.AddToCart to ignore client-supplied pricing so a tampered mutation can't set price to 1 cent - resolves the listing override if any, else the product's default_price - rejects unlisted products and kinds that can't be a direct cart line (ProductKind.IsCartLineEligible)
func (a CartCatalogAdapter) ResolveSimplePricing(ctx context.Context, channelID uuid.UUID, sku string, customerID *uuid.UUID) (cart.SimplePricing, error) {
	p, err := a.svc.GetProductBySKU(ctx, sku)
	if err != nil {
		return cart.SimplePricing{}, err
	}
	if !p.Kind.IsCartLineEligible() {
		return cart.SimplePricing{}, fmt.Errorf("%w: sku %q is kind %q", cart.ErrProductNotPurchasable, sku, p.Kind)
	}
	listings, err := a.svc.ListProductChannelListings(ctx, p.ID)
	if err != nil {
		return cart.SimplePricing{}, err
	}
	var chListing *catalog.ProductChannelListing
	published := false
	for i := range listings {
		if listings[i].ChannelID != channelID {
			continue
		}
		published = listings[i].IsPublished
		chListing = &listings[i]
		break
	}
	if !published {
		return cart.SimplePricing{}, fmt.Errorf("sku %q: %w", sku, cart.ErrProductNotPublished)
	}
	// Effective price folds the listing override plus any active scheduled special (internal/pricing) - the same resolution the storefront shows
	resolved := pricing.Resolve(catalog.EffectivePricingInputs(p, chListing), time.Now())
	// Use the slug as the line's display name - matches how bundle lines fill ProductName - storefront re-resolves the full localized name from the SKU at render time, so this just needs to be stable
	out := cart.SimplePricing{
		ProductName: p.SKU,
		UnitPrice:   resolved.Price,
		MinQty:      p.MinOrderQty,
		MaxQty:      p.MaxOrderQty,
		QtyStep:     p.QuantityStep,
	}
	if p.CostCents != nil {
		out.CostCents = *p.CostCents
	}
	// A plugin's per-customer price REPLACES list + tier pricing outright - a negotiated contract rate is a promise about what this buyer pays, so an opportunistic quantity break must not undercut or beat it Skipped for guests: a price keyed on who is buying has nothing to resolve against
	if customerID != nil && plugin.HasPriceResolvers() {
		if cents, ok := plugin.ResolveUnitPrice(ctx, sdk.PriceRequest{
			CustomerID: *customerID,
			ProductID:  p.ID,
			ChannelID:  channelID,
			SKU:        p.SKU,
		}); ok {
			out.UnitPrice = cents
			return out, nil
		}
	}
	// Tier (quantity-break) prices, pre-filtered to this channel and the caller's customer group so the cart can apply BestTier scope-free
	var groupID *uuid.UUID
	if customerID != nil && a.customers != nil {
		if c, cerr := a.customers.GetByID(ctx, *customerID); cerr == nil && c.CustomerGroupID != uuid.Nil {
			g := c.CustomerGroupID
			groupID = &g
		}
	}
	if tiers, terr := a.svc.ApplicableGroupPrices(ctx, p.ID, &channelID, groupID); terr == nil {
		for _, t := range tiers {
			out.Tiers = append(out.Tiers, pricing.Tier{MinQty: t.MinQty, Price: t.Price})
		}
	}
	return out, nil
}

func (a CartCatalogAdapter) ResolveProductOptions(ctx context.Context, channelID uuid.UUID, sku string, customerID *uuid.UUID) (cart.ProductOptionsResolution, error) {
	p, err := a.svc.GetProductBySKU(ctx, sku)
	if err != nil {
		return cart.ProductOptionsResolution{}, err
	}
	if p.Kind != catalog.KindSimple {
		// Sentinel, not a plain error - AddToCart's required-options gate fails closed on real resolve errors and must be able to tell "this kind never has options" apart from a catalog hiccup
		return cart.ProductOptionsResolution{}, fmt.Errorf("sku %q is kind %q, not simple: %w", sku, p.Kind, cart.ErrOptionsUnsupportedKind)
	}
	// Reuse the simple-pricing path for the published-on-store check + the channel price / display name - the customer has to ride along or a configured line is priced at list while every other cart mutation prices it at the buyer's contract/group rate
	pricing, err := a.ResolveSimplePricing(ctx, channelID, sku, customerID)
	if err != nil {
		return cart.ProductOptionsResolution{}, err
	}
	if a.channels == nil {
		return cart.ProductOptionsResolution{}, errors.New("resolve product options: no channel lookup wired")
	}
	ch, err := a.channels.GetByID(ctx, channelID)
	if err != nil {
		return cart.ProductOptionsResolution{}, err
	}
	loc := locale.Active(ctx, nil)
	opts, err := a.svc.GetProductOptions(ctx, p.ID, loc)
	if err != nil {
		return cart.ProductOptionsResolution{}, err
	}
	res := cart.ProductOptionsResolution{
		ProductID:   p.ID,
		ProductName: pricing.ProductName,
		UnitPrice:   pricing.UnitPrice,
		BasePrice:   pricing.UnitPrice,
		Options:     make([]cart.ProductOptionConfig, 0, len(opts)),
	}
	if err := a.appendCartOptions(ctx, ch, &res, opts); err != nil {
		return cart.ProductOptionsResolution{}, err
	}

	// A variant of a configurable also carries its parent's product-wide options (e.g. a gift message that applies to every variant) - merge them in so buildConfiguredLineOptions accepts and prices both sets - a variant belongs to at most one configurable (UNIQUE(child_id)) so this is a single extra lookup, and option IDs are globally unique so appending never collides with the variant's own
	parents, err := a.svc.ListConfigurableParentByChildIDs(ctx, []uuid.UUID{p.ID})
	if err != nil {
		return cart.ProductOptionsResolution{}, err
	}
	if link, ok := parents[p.ID]; ok && link.ParentID != uuid.Nil {
		parentOpts, err := a.svc.GetProductOptions(ctx, link.ParentID, loc)
		if err != nil {
			return cart.ProductOptionsResolution{}, err
		}
		if err := a.appendCartOptions(ctx, ch, &res, parentOpts); err != nil {
			return cart.ProductOptionsResolution{}, err
		}
		// The merged set displays and snapshots in walk order - stable-sort so parent and variant options interleave by their authored sort_order like the PDP shows them, while same-product relative order (the visible_if evaluation invariant) is preserved
		sort.SliceStable(res.Options, func(i, j int) bool {
			return res.Options[i].SortOrder < res.Options[j].SortOrder
		})
	}
	return res, nil
}

// appendCartOptions maps catalog product options into the cart's option-config shape and appends them to res - shared so a variant's own options and its configurable parent's options funnel through the same conversion
func (a CartCatalogAdapter) appendCartOptions(ctx context.Context, ch channel.Channel, res *cart.ProductOptionsResolution, opts []catalog.ProductOption) error {
	channelID := ch.ID
	// Channel-priced source products, deduped - a linked value charges what its source costs on THIS channel (listing override/special), mirroring the bundle component BILLING rule; a source this channel doesn't sell makes the value unpickable
	srcPrice := map[string]int64{}
	unavailable := map[string]bool{}
	for _, o := range opts {
		for _, v := range o.Values {
			if v.SourceProductID == nil || v.SourceProductSKU == "" {
				continue
			}
			if _, done := srcPrice[v.SourceProductSKU]; done || unavailable[v.SourceProductSKU] {
				continue
			}
			sp, err := a.ResolveSimplePricing(ctx, channelID, v.SourceProductSKU, nil)
			switch {
			case errors.Is(err, cart.ErrProductNotPublished), errors.Is(err, cart.ErrProductNotPurchasable):
				unavailable[v.SourceProductSKU] = true
			case err != nil:
				return fmt.Errorf("price option source %q: %w", v.SourceProductSKU, err)
			default:
				srcPrice[v.SourceProductSKU] = sp.UnitPrice
			}
		}
	}
	for _, o := range opts {
		cfg := cart.ProductOptionConfig{
			ID:             o.ID,
			InputType:      string(o.InputType),
			IsRequired:     o.IsRequired,
			Title:          o.Title,
			SortOrder:      o.SortOrder,
			MaxChars:       o.MaxChars,
			MaxFileBytes:   o.MaxFileBytes,
			FileExtensions: o.FileExtensions,
		}
		var priced bool
		cfg.PriceValue, priced = o.ChannelPriceValue(channelID, ch.CurrencyCode)
		cfg.PriceUnavailable = !priced
		if o.PriceType != nil {
			cfg.PriceType = string(*o.PriceType)
		}
		cfg.VisibleIf = toCartVisibilityGroup(o.VisibleIf)
		for _, v := range o.Values {
			price, priced := v.ChannelPriceValue(channelID, ch.CurrencyCode, o.Currency)
			vc := cart.ProductOptionValueConfig{
				ID:               v.ID,
				Title:            v.Title,
				PriceValue:       price,
				PriceType:        string(v.PriceType),
				Quantity:         v.Quantity,
				PriceUnavailable: !priced,
			}
			if v.SourceProductID != nil {
				vc.HasSource = true
				vc.SourceProductID = *v.SourceProductID
				p, ok := srcPrice[v.SourceProductSKU]
				vc.SourceProductPrice = p
				vc.SourceUnavailable = !ok
			}
			vc.ForcedLineQty = v.ForcedLineQty
			cfg.Values = append(cfg.Values, vc)
		}
		res.Options = append(res.Options, cfg)
	}
	return nil
}

func toCartVisibilityGroup(g *catalog.OptionConditionGroup) *cart.OptionVisibilityGroup {
	if g == nil {
		return nil
	}
	out := &cart.OptionVisibilityGroup{Logic: g.Logic}
	for _, r := range g.Rules {
		out.Rules = append(out.Rules, cart.OptionVisibilityRule{
			OptionID: r.OptionID,
			Op:       r.Op,
			ValueIDs: r.ValueIDs,
		})
	}
	for i := range g.Groups {
		sub := toCartVisibilityGroup(&g.Groups[i])
		out.Groups = append(out.Groups, *sub)
	}
	return out
}

func (a CartCatalogAdapter) ResolveBundleBySKU(ctx context.Context, channelID uuid.UUID, sku string) (cart.BundleSnapshot, error) {
	p, err := a.svc.GetProductBySKU(ctx, sku)
	if err != nil {
		return cart.BundleSnapshot{}, err
	}
	if p.Kind != catalog.KindBundle {
		return cart.BundleSnapshot{}, fmt.Errorf("product %q is kind %q, not bundle", p.SKU, string(p.Kind))
	}
	listings, err := a.svc.ListProductChannelListings(ctx, p.ID)
	if err != nil {
		return cart.BundleSnapshot{}, err
	}
	if !publishedOn(listings, channelID) {
		return cart.BundleSnapshot{}, fmt.Errorf("bundle %q: %w", sku, cart.ErrProductNotPublished)
	}
	opts, err := a.svc.GetBundleOptions(ctx, p.ID, locale.Active(ctx, nil)) // active request locale (header-driven via locale.Middleware), falls back to default when not set
	if err != nil {
		return cart.BundleSnapshot{}, err
	}
	snap := cart.BundleSnapshot{
		ProductID:   p.ID,
		ProductName: p.SKU, // cart falls back to SKU if this is empty; slug is a reasonable display
		Options:     make([]cart.BundleOptionSnapshot, len(opts)),
	}
	for i, o := range opts {
		os := cart.BundleOptionSnapshot{
			ID:            o.ID,
			InputType:     string(o.InputType),
			IsRequired:    o.IsRequired,
			MinSelections: o.MinSelections,
			MaxSelections: o.MaxSelections,
			Selections:    make([]cart.BundleSelectionSnapshot, len(o.Selections)),
		}
		for j, sel := range o.Selections {
			os.Selections[j] = cart.BundleSelectionSnapshot{
				ID:                sel.ID,
				ProductID:         sel.ProductID,
				ProductSKU:        sel.ProductSKU,
				ProductName:       sel.ProductName,
				UnitPrice:         sel.ProductDefaultPrice,
				QuantityPerBundle: sel.Quantity,
				IsDefault:         sel.IsDefault,
			}
		}
		snap.Options[i] = os
	}
	return snap, nil
}

type CatalogProductLookup struct { // satisfies inventory.ProductLookup for the OrderPlacing reservation listener
	svc *catalog.Service
}

func (l CatalogProductLookup) GetProductIDBySKU(ctx context.Context, sku string) (uuid.UUID, error) {
	p, err := l.svc.GetProductBySKU(ctx, sku)
	if err != nil {
		return uuid.UUID{}, err
	}
	return p.ID, nil
}

func (l CatalogProductLookup) KitComponents(ctx context.Context, productID uuid.UUID) ([]inventory.KitComponentRef, error) {
	comps, err := l.svc.KitComponentRefs(ctx, productID)
	if err != nil {
		return nil, err
	}
	out := make([]inventory.KitComponentRef, len(comps))
	for i, c := range comps {
		out[i] = inventory.KitComponentRef{ProductID: c.ComponentProductID, Quantity: c.Quantity}
	}
	return out, nil
}

// OrderLineLookup adapts order.Service + catalog.Service for the inventory service's flat-mode cancellation path - mirrors the cart-to-ReserveLine projection in inventory/events.go: simple lines resolve their SKU to a product id, bundle and kit lines expand into one entry per component (with the component's product id + total qty)
type OrderLineLookup struct {
	orderSvc   *order.Service
	catalogSvc *catalog.Service
}

func (l OrderLineLookup) GetOrderLines(ctx context.Context, orderID uuid.UUID) ([]inventory.OrderLine, error) {
	o, err := l.orderSvc.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	// Flat mode's restore subtracts these, so a lookup failure must fail rather than report zero shipped
	shipped, err := l.orderSvc.ShippedByOrderLine(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("shipped quantities for order %s: %w", orderID, err)
	}
	out := make([]inventory.OrderLine, 0, len(o.Lines))
	for _, line := range o.Lines {
		done := shipped[line.ID]
		if len(line.Components) > 0 {
			// Shipments are per ORDER LINE, so a half-shipped bundle counts half of each component gone
			factor := 0.0
			if line.Quantity > 0 {
				factor = done / line.Quantity
			}
			for _, comp := range line.Components {
				out = append(out, inventory.OrderLine{
					ProductID:         comp.ProductID,
					Quantity:          comp.Quantity,
					FulfilledQuantity: comp.Quantity * factor,
					LineID:            line.ID,
					LineQuantity:      line.Quantity,
					LineFulfilled:     done,
				})
			}
			continue
		}
		parts := line.StockParts
		if len(parts) == 0 && line.ProductSku == "" {
			continue
		}
		var p catalog.Product
		if len(parts) == 0 {
			p, err = l.catalogSvc.GetProductBySKU(ctx, line.ProductSku)
			if err != nil {
				// Best-effort: log and skip so a stale SKU doesn't strand a cancellation - same posture as the Reserve-time linesFromCart
				log.Printf("[inventory] resolve sku %q for order %s release: %v", line.ProductSku, orderID, err)
				continue
			}
			// Orders placed before kit lines were snapshotted took today's recipe, the best record there is
			if p.Kind == catalog.KindKit {
				refs, err := l.catalogSvc.KitComponentRefs(ctx, p.ID)
				if err != nil {
					return nil, fmt.Errorf("kit components for %q: %w", line.ProductSku, err)
				}
				for _, r := range refs {
					parts = append(parts, order.OrderLineStockPart{ProductID: r.ComponentProductID, Quantity: r.Quantity, Source: cart.StockPartKit})
				}
			}
		}
		for _, part := range parts {
			out = append(out, inventory.OrderLine{
				ProductID:         part.ProductID,
				Quantity:          part.Quantity * line.Quantity,
				FulfilledQuantity: part.Quantity * done,
				LineID:            line.ID,
				LineQuantity:      line.Quantity,
				LineFulfilled:     done,
			})
		}
		if !ownsStock(parts) || line.ProductSku == "" {
			continue
		}
		if p.ID == uuid.Nil {
			if p, err = l.catalogSvc.GetProductBySKU(ctx, line.ProductSku); err != nil {
				log.Printf("[inventory] resolve sku %q for order %s release: %v", line.ProductSku, orderID, err)
				continue
			}
		}
		out = append(out, inventory.OrderLine{
			ProductID:         p.ID,
			Quantity:          line.Quantity,
			FulfilledQuantity: done,
			LineID:            line.ID,
			LineQuantity:      line.Quantity,
			LineFulfilled:     done,
		})
	}
	return out, nil
}

func ownsStock(parts []order.OrderLineStockPart) bool {
	for _, p := range parts {
		if p.Source == cart.StockPartKit {
			return false
		}
	}
	return true
}

// OrderKitLookup gives order placement a kit's per-unit recipe to snapshot onto the line
type OrderKitLookup struct{ svc *catalog.Service }

func (l OrderKitLookup) KitComponentsBySKU(ctx context.Context, sku string) ([]cart.CartLineStockPart, error) {
	p, err := l.svc.GetProductBySKU(ctx, sku)
	if err != nil {
		if errors.Is(err, catalog.ErrProductNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if p.Kind != catalog.KindKit {
		return nil, nil
	}
	refs, err := l.svc.KitComponentRefs(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	out := make([]cart.CartLineStockPart, len(refs))
	for i, r := range refs {
		out[i] = cart.CartLineStockPart{ProductID: r.ComponentProductID, Quantity: r.Quantity, Source: cart.StockPartKit}
	}
	return out, nil
}

// GiftcardTender adapts giftcard.Service to order.GiftCardTender so the order service can redeem gift-card codes as a tender inside its placement transaction without importing the giftcard package
type GiftcardTender struct{ svc *giftcard.Service }

func (g GiftcardTender) RedeemTx(ctx context.Context, tx *sql.Tx, code string, channelID uuid.UUID, amountDue int64, currency string, orderID uuid.UUID) (int64, error) {
	return g.svc.RedeemUpToTx(ctx, tx, code, channelID, amountDue, currency, orderID)
}

// StoreCreditTender adapts storecredit.Service to order.StoreCreditTender so placement can spend the customer's balance inside its transaction without importing the storecredit package
type StoreCreditTender struct{ svc *storecredit.Service }

func (t StoreCreditTender) RedeemTx(ctx context.Context, tx *sql.Tx, customerID uuid.UUID, amountDue int64, currency string, orderID uuid.UUID) (int64, error) {
	return t.svc.RedeemUpToTx(ctx, tx, customerID, amountDue, currency, orderID)
}

type DownloadOrderLookup struct { // adapts order + catalog to download.OrderDownloadLookup: the downloadable files of a placed order's product lines
	orderSvc   *order.Service
	catalogSvc *catalog.Service
}

func (l DownloadOrderLookup) DownloadsForOrder(ctx context.Context, orderID uuid.UUID) ([]download.GrantableDownload, error) {
	o, err := l.orderSvc.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	var out []download.GrantableDownload
	for _, line := range o.Lines {
		if line.ProductSku == "" {
			continue
		}
		p, err := l.catalogSvc.GetProductBySKU(ctx, line.ProductSku)
		if err != nil {
			continue
		}
		dls, err := l.catalogSvc.ListProductDownloads(ctx, p.ID)
		if err != nil || len(dls) == 0 {
			continue
		}
		for _, d := range dls {
			out = append(out, download.GrantableDownload{
				ProductID:    p.ID,
				DownloadID:   d.ID,
				Title:        d.Title,
				URL:          d.URL,
				MaxDownloads: d.MaxDownloads,
			})
		}
	}
	return out, nil
}

// GiftcardOrderLookup adapts order + catalog to giftcard.OrderGiftcardLookup: it returns the gift-card lines of a placed order (face value + quantity), resolving each line's kind via the catalog
type GiftcardOrderLookup struct {
	orderSvc   *order.Service
	catalogSvc *catalog.Service
}

func (l GiftcardOrderLookup) GiftcardLines(ctx context.Context, orderID uuid.UUID) ([]giftcard.IssuableLine, error) {
	o, err := l.orderSvc.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	var out []giftcard.IssuableLine
	for _, line := range o.Lines {
		if line.ProductSku == "" {
			continue
		}
		kind, err := l.catalogSvc.ProductKindBySKU(ctx, line.ProductSku)
		if err != nil {
			log.Printf("[giftcard] resolve sku %q for order %s issuance: %v", line.ProductSku, orderID, err)
			continue
		}
		if kind == string(catalog.KindGiftcard) {
			out = append(out, giftcard.IssuableLine{UnitPrice: line.UnitPrice, Quantity: line.Quantity})
		}
	}
	return out, nil
}

// PaymentAuthorizerAdapter bridges order placement to the payment service (implements order.PaymentAuthorizer): it authorizes the amount owed, carrying the storefront's tokenization handle in the request's typed PaymentToken slot - the resulting gateway event/webhook settles the order's payment_status via the bridge in registerEventSubscribers
type PaymentAuthorizerAdapter struct {
	paymentSvc *payment.Service
}

// authorizeRequestFrom maps the order-side input onto the payment request - pure so the token plumbing stays table-testable, since this exact seam once dropped every card token on the floor
func authorizeRequestFrom(in order.PaymentAuthorizeInput) payment.AuthorizeRequest {
	return payment.AuthorizeRequest{
		OrderID:      in.OrderID,
		ChannelID:    in.ChannelID,
		CurrencyCode: in.CurrencyCode,
		AmountCents:  in.AmountCents,
		CustomerID:   in.CustomerID,
		Email:        in.Email,
		PaymentToken: in.Token,
		VaultedToken: in.VaultedToken,
	}
}

func (a PaymentAuthorizerAdapter) Authorize(ctx context.Context, in order.PaymentAuthorizeInput) (order.PaymentAuthorizeOutcome, error) {
	// Offline methods (cash on delivery, bank transfer, …) are never charged through a gateway at placement - the order stays unpaid until an operator records the payment - skip authorization entirely so no PaymentAuthorized event fires and no "authorization failed" timeline noise is recorded
	if reg, err := payment.GetProvider(in.ProviderSlug); err == nil && reg.IsOffline {
		return order.PaymentAuthorizeOutcome{}, nil
	}
	res, err := a.paymentSvc.Authorize(ctx, in.ProviderSlug, authorizeRequestFrom(in))
	if err != nil {
		return order.PaymentAuthorizeOutcome{}, err
	}
	return order.PaymentAuthorizeOutcome{
		ActionKind:        string(res.Action.Kind),
		ActionValue:       res.Action.Value,
		Authorized:        !res.Action.Required(),
		ProviderReference: res.ProviderReference,
	}, nil
}

// SubscriptionRenewalCharger collects a renewal invoice against the subscription's saved payment method: vault row -> provider token -> off-session authorize+capture - implements subscription.RenewalCharger
type SubscriptionRenewalCharger struct {
	vault    *paymentvault.Service
	payments *payment.Service
}

func (c SubscriptionRenewalCharger) ChargeInvoice(ctx context.Context, sub subscription.Subscription, inv subscription.Invoice) (string, error) {
	if sub.CustomerID == nil {
		return "", fmt.Errorf("subscription has no customer")
	}
	if sub.PaymentMethodID == nil {
		return "", fmt.Errorf("no saved payment method selected")
	}
	if sub.ChannelID == nil {
		return "", fmt.Errorf("subscription has no channel")
	}
	m, err := c.vault.Get(ctx, *sub.CustomerID, *sub.PaymentMethodID)
	if err != nil {
		return "", err
	}
	return c.payments.ChargeOffSession(ctx, m.ProviderSlug, payment.AuthorizeRequest{
		OrderID:      inv.ID, // gateway reference/idempotency handle; no commerce order exists
		ChannelID:    *sub.ChannelID,
		CurrencyCode: inv.CurrencyCode,
		AmountCents:  inv.Amount,
		CustomerID:   sub.CustomerID,
		VaultedToken: m.Token,
	})
}

// SubscriptionOrderLookup adapts order + catalog to subscription.OrderSubscriptionLookup: it returns the subscription lines of a placed order (terms resolved from the catalog, price/sku/title snapshotted from the line) so the issuer can start a subscription per line
type SubscriptionOrderLookup struct {
	orderSvc   *order.Service
	catalogSvc *catalog.Service
}

func (l SubscriptionOrderLookup) SubscriptionsForOrder(ctx context.Context, orderID uuid.UUID) ([]subscription.StartableSubscription, error) {
	o, err := l.orderSvc.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	var out []subscription.StartableSubscription
	for _, line := range o.Lines {
		if line.ProductSku == "" {
			continue
		}
		p, err := l.catalogSvc.GetProductBySKU(ctx, line.ProductSku)
		if err != nil || p.Kind != catalog.KindSubscription {
			continue
		}
		terms, err := l.catalogSvc.GetSubscriptionTerms(ctx, p.ID)
		if err != nil {
			log.Printf("[subscription] no terms for product %s (order %s): %v", p.ID, orderID, err) // no terms configured - can't start a subscription, skip
			continue
		}
		// Fold custom-option picks into the snapshot title so recurring fulfillment keeps the personalization ("Coffee Club (Grind: Espresso)") - the subscription model has no options field
		title := line.ProductName
		var picks []string
		for _, opt := range line.Options {
			v := opt.ValueLabel
			if v == "" {
				v = opt.TextValue
			}
			if v != "" {
				picks = append(picks, opt.OptionTitle+": "+v)
			}
		}
		if len(picks) > 0 {
			title = title + " (" + strings.Join(picks, "; ") + ")"
		}
		out = append(out, subscription.StartableSubscription{
			ProductID:     p.ID,
			Price:         line.UnitPrice,
			CurrencyCode:  o.CurrencyCode,
			Quantity:      line.Quantity,
			SKU:           line.ProductSku,
			Title:         title,
			IntervalUnit:  terms.IntervalUnit,
			IntervalCount: terms.IntervalCount,
			TrialDays:     terms.TrialDays,
		})
	}
	return out, nil
}

// InventoryLedgerToggle reads sales/inventory/ledger_enabled from config - returns true (the safe default) when the lookup fails so a misconfigured registry never silently turns reservations off
type InventoryLedgerToggle struct {
	cfg *config.Service
}

func (t InventoryLedgerToggle) LedgerEnabled(ctx context.Context) bool {
	v, err := t.cfg.GetBool(ctx, "sales/inventory/ledger_enabled", config.DefaultScope)
	if err != nil {
		return true
	}
	return v
}

// InventoryFlatModeToggle reads sales/inventory/flat_mode - returns false on lookup failure so a registry hiccup defaults to the multi-location code path (the safer assumption - flipping into flat mode silently would skip Reserve writes)
type InventoryFlatModeToggle struct {
	cfg *config.Service
}

func (t InventoryFlatModeToggle) FlatModeEnabled(ctx context.Context) bool {
	v, err := t.cfg.GetBool(ctx, "sales/inventory/flat_mode", config.DefaultScope)
	if err != nil {
		return false
	}
	return v
}

// InventoryDeductionStep reads sales/inventory/deduction_step - anything but "placement", including a failed lookup, keeps the fulfillment-time default, so a registry hiccup can't silently move the deduction
type InventoryDeductionStep struct {
	cfg *config.Service
}

func (t InventoryDeductionStep) DeductAtPlacement(ctx context.Context) bool {
	v, err := t.cfg.GetString(ctx, "sales/inventory/deduction_step", config.DefaultScope)
	return err == nil && v == "placement"
}

// InventoryDefaultLocation reads the location new channels are assigned to
type InventoryDefaultLocation struct {
	cfg *config.Service
}

func (d InventoryDefaultLocation) DefaultLocationSlug(ctx context.Context) (string, error) {
	return d.cfg.GetString(ctx, inventory.ConfigDefaultLocation, config.DefaultScope)
}

type InventoryPickupLister struct{ svc *inventory.Service } // adapts inventory.Service to shipping.PickupLocationLister for the builtin click-and-collect carrier

func (l InventoryPickupLister) PickupLocations(ctx context.Context) ([]shipping.PickupLocation, error) {
	locs, err := l.svc.PickupLocations(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]shipping.PickupLocation, len(locs))
	for i, loc := range locs {
		out[i] = shipping.PickupLocation{
			Slug:         loc.Slug,
			Name:         loc.Name,
			Line1:        loc.Line1,
			City:         loc.City,
			Instructions: loc.PickupInstructions,
		}
	}
	return out, nil
}

// SalesVelocitySource feeds the search ranker's trailing-30d units-sold signal from the orders ledger
type SalesVelocitySource struct {
	queries *db.Queries
}

// Satisfies search.BatchSalesSource so a server-triggered ReindexAll gets the same one-query-per-page treatment the CLI does
func (s SalesVelocitySource) SalesVelocity30dBySKUs(ctx context.Context, skus []string) (map[string]map[uuid.UUID]float64, error) {
	rows, err := s.queries.SalesVelocity30dBySKUs(ctx, skus)
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[uuid.UUID]float64, len(rows))
	for _, r := range rows {
		if out[r.ProductSku] == nil {
			out[r.ProductSku] = map[uuid.UUID]float64{}
		}
		out[r.ProductSku][r.ChannelID] = r.Units
	}
	return out, nil
}

func (s SalesVelocitySource) SalesVelocity30d(ctx context.Context, sku string) (map[uuid.UUID]float64, error) {
	rows, err := s.queries.SalesVelocity30dBySKU(ctx, sku)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]float64, len(rows))
	for _, r := range rows {
		out[r.ChannelID] = r.Units
	}
	return out, nil
}

// Constructors for the adapters above - they exist so callers never have to name the unexported fields, and so each adapter can be handed to fx as a provider

func NewCustomerStoreLookup(ch *channel.Service, cfg *config.Service) CustomerStoreLookup {
	return CustomerStoreLookup{svc: ch, cfg: cfg}
}

func NewCatalogDefaultLocaleProvider(ch *channel.Service) CatalogDefaultLocaleProvider {
	return CatalogDefaultLocaleProvider{channels: ch}
}

func NewCatalogInventoryEnsurer(inv *inventory.Service) CatalogInventoryEnsurer {
	return CatalogInventoryEnsurer{svc: inv}
}

func NewAuditURLCollisions(auditSvc *audit.Service) AuditURLCollisions {
	return AuditURLCollisions{audit: auditSvc}
}

func NewCartCatalogAdapter(cat *catalog.Service, customers *customer.Service, channels *channel.Service) CartCatalogAdapter {
	return CartCatalogAdapter{svc: cat, customers: customers, channels: channels}
}

func NewCartFileAdapter(lib *medialibrary.Service, store *objstore.Storage) CartFileAdapter {
	return CartFileAdapter{media: lib, storage: store}
}

func NewWebformFileStore(store *objstore.Storage) WebformFileStore {
	return WebformFileStore{storage: store}
}

func NewOrderChannelLoader(ch *channel.Service) OrderChannelLoader {
	return OrderChannelLoader{svc: ch}
}

func NewOrderProductPricer(a CartCatalogAdapter) OrderProductPricer {
	return OrderProductPricer{svc: a}
}

func NewCatalogProductLookup(cat *catalog.Service) CatalogProductLookup {
	return CatalogProductLookup{svc: cat}
}

func NewOrderLineLookup(ord *order.Service, cat *catalog.Service) OrderLineLookup {
	return OrderLineLookup{orderSvc: ord, catalogSvc: cat}
}

func NewGiftcardTender(gc *giftcard.Service) GiftcardTender { return GiftcardTender{svc: gc} }

func NewStoreCreditTender(sc *storecredit.Service) StoreCreditTender {
	return StoreCreditTender{svc: sc}
}

func NewDownloadOrderLookup(ord *order.Service, cat *catalog.Service) DownloadOrderLookup {
	return DownloadOrderLookup{orderSvc: ord, catalogSvc: cat}
}

func NewGiftcardOrderLookup(ord *order.Service, cat *catalog.Service) GiftcardOrderLookup {
	return GiftcardOrderLookup{orderSvc: ord, catalogSvc: cat}
}

func NewPaymentAuthorizerAdapter(pay *payment.Service) PaymentAuthorizerAdapter {
	return PaymentAuthorizerAdapter{paymentSvc: pay}
}

func NewSubscriptionRenewalCharger(vault *paymentvault.Service, pay *payment.Service) SubscriptionRenewalCharger {
	return SubscriptionRenewalCharger{vault: vault, payments: pay}
}

func NewSubscriptionOrderLookup(ord *order.Service, cat *catalog.Service) SubscriptionOrderLookup {
	return SubscriptionOrderLookup{orderSvc: ord, catalogSvc: cat}
}

func NewInventoryLedgerToggle(cfg *config.Service) InventoryLedgerToggle {
	return InventoryLedgerToggle{cfg: cfg}
}

func NewInventoryFlatModeToggle(cfg *config.Service) InventoryFlatModeToggle {
	return InventoryFlatModeToggle{cfg: cfg}
}

func NewInventoryDeductionStep(cfg *config.Service) InventoryDeductionStep {
	return InventoryDeductionStep{cfg: cfg}
}

func NewInventoryDefaultLocation(cfg *config.Service) InventoryDefaultLocation {
	return InventoryDefaultLocation{cfg: cfg}
}

// Same shape as the toggles above - a per-channel config read the order service can call without importing config
func NewRiskHoldThreshold(cfg *config.Service) func(context.Context, uuid.UUID) int {
	return func(ctx context.Context, channelID uuid.UUID) int {
		scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
		v, _ := cfg.GetInt(ctx, "sales/checkout/risk_hold_threshold", scope)
		return int(v)
	}
}

type ShippingDimensions struct{ catalog *catalog.Service }

func (a ShippingDimensions) DimensionsBySKUs(ctx context.Context, skus []string) (map[string]shipping.Dimensions, error) {
	dims, err := a.catalog.ProductDimensionsBySKUs(ctx, skus)
	if err != nil {
		return nil, err
	}
	out := make(map[string]shipping.Dimensions, len(dims))
	for sku, d := range dims {
		out[sku] = shipping.Dimensions(d)
	}
	return out, nil
}

func NewShippingTax(cfg *config.Service) func(context.Context, uuid.UUID) (string, bool) {
	return func(ctx context.Context, channelID uuid.UUID) (string, bool) {
		scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
		taxable, err := cfg.GetBool(ctx, "sales/checkout/shipping_taxable", scope)
		if err != nil {
			taxable = true
		}
		class, _ := cfg.GetString(ctx, "sales/checkout/shipping_tax_class", scope)
		return strings.TrimSpace(class), taxable
	}
}

func NewInventoryPickupLister(inv *inventory.Service) InventoryPickupLister {
	return InventoryPickupLister{svc: inv}
}

func NewSalesVelocitySource(q *db.Queries) SalesVelocitySource {
	return SalesVelocitySource{queries: q}
}

// ContactAdapter narrows customer.Service.EnsureContact to the id-returning shape the signup-form service depends on (composition-root glue)
type ContactAdapter struct{ svc *customer.Service }

func (a ContactAdapter) EnsureContact(ctx context.Context, channelID uuid.UUID, email, firstName string, verified bool) (uuid.UUID, error) {
	c, _, err := a.svc.EnsureContact(ctx, channelID, email, firstName, verified)
	return c.ID, err
}

type ConsentAdapter struct{ svc *consent.Service } // drops SetConsent's returned Consent to match the error-only setter the signup-form service expects

func (a ConsentAdapter) SetConsent(ctx context.Context, customerID uuid.UUID, kind string, optedIn bool, source string) error {
	_, err := a.svc.SetConsent(ctx, customerID, kind, optedIn, source)
	return err
}

func NewContactAdapter(c *customer.Service) ContactAdapter { return ContactAdapter{svc: c} }

func NewConsentAdapter(c *consent.Service) ConsentAdapter { return ConsentAdapter{svc: c} }

// Tells the suite a transactional mail went out - the send itself is core's, this is only the analytics half
type TransactionalRecorder struct{ rec *signalsclient.Client }

func NewTransactionalRecorder(rec *signalsclient.Client) TransactionalRecorder {
	return TransactionalRecorder{rec: rec}
}

func (l TransactionalRecorder) RecordSent(ctx context.Context, sendID uuid.UUID, spec mailsend.Spec) error {
	return l.rec.RecordEmailSent(ctx, signalsclient.SendRef{
		ID: sendID, ChannelID: spec.ChannelID, CustomerID: spec.CustomerID, Source: signalsclient.SourceTransactional,
		Email: spec.To, MessageKey: spec.MessageKey, OrderID: spec.OrderID,
	})
}

// OrderMailSendLog is ordermail.SendLog over the order_email_log table, whose unique key on (order_id, kind, ref) does the deduplication
type OrderMailSendLog struct {
	q *db.Queries
}

func (a OrderMailSendLog) AlreadySent(ctx context.Context, orderID uuid.UUID, kind, ref string) (bool, error) {
	return a.q.OrderEmailExists(ctx, db.OrderEmailExistsParams{OrderID: orderID, Kind: kind, Ref: ref})
}

func (a OrderMailSendLog) RecordSent(ctx context.Context, orderID uuid.UUID, kind, ref string) error {
	return a.q.InsertOrderEmailLog(ctx, db.InsertOrderEmailLogParams{OrderID: orderID, Kind: kind, Ref: ref})
}

func publishedOn(listings []catalog.ProductChannelListing, channelID uuid.UUID) bool {
	for _, l := range listings {
		if l.ChannelID == channelID {
			return l.IsPublished
		}
	}
	return false
}
