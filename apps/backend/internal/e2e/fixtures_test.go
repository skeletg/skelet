package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/cart"
	"github.com/skeletg/skelet/internal/catalog"
	"github.com/skeletg/skelet/internal/channel"
	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/inventory"
	"github.com/skeletg/skelet/internal/locale"
	"github.com/skeletg/skelet/internal/order"
)

// A store built from scratch through the real services: channel, product type, published listing,
// stocked location, cod enabled - everything a placement needs, nothing shared between tests
type store struct {
	Channel  channel.Channel
	Product  catalog.Product
	Location inventory.Location
	Item     inventory.Item
}

func (h *harness) newStore(t *testing.T, stock float64, priceCents int64) *store {
	t.Helper()
	return h.newStoreWith(t, stock, priceCents, "USD")
}

// newStoreWith picks the channel currency - products keep DefaultCurrencyCode USD, which is fine
// because every fixture listing carries an explicit price and so skips the currency-match check
func (h *harness) newStoreWith(t *testing.T, stock float64, priceCents int64, currencyCode string) *store {
	t.Helper()
	return h.newStoreOpt(t, stock, priceCents, func(in *channel.CreateChannelInput) {
		in.CurrencyCode = currencyCode
	})
}

// ensureLocale registers a locale ACTIVE, repairing a persisted inactive row from earlier runs -
// CreateLocaleInput.IsActive has no default, and the clamp skips inactive non-default locales
func (h *harness) ensureLocale(t *testing.T, code string) {
	t.Helper()
	active := true
	if err := h.d.Locale.EnsureExists(h.ctx, code); err != nil {
		if _, cerr := h.d.Locale.Create(h.ctx, locale.CreateLocaleInput{Code: code, Name: code, IsActive: true}); cerr != nil {
			t.Fatalf("create locale %s: %v", code, cerr)
		}
		return
	}
	if _, err := h.d.Locale.Update(h.ctx, locale.UpdateLocaleInput{Code: code, IsActive: &active}); err != nil {
		t.Fatalf("activate locale %s: %v", code, err)
	}
}

func (h *harness) newStoreOpt(t *testing.T, stock float64, priceCents int64, mutate func(*channel.CreateChannelInput)) *store {
	t.Helper()
	ctx := h.ctx
	suffix := uuid.NewString()[:8]

	in := channel.CreateChannelInput{
		Slug:         "e2e-" + suffix,
		Name:         "E2E Store " + suffix,
		CurrencyCode: "USD",
	}
	if mutate != nil {
		mutate(&in)
	}
	ch, err := h.d.Channel.Create(ctx, in)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	pt, err := h.d.Catalog.CreateProductType(ctx, catalog.CreateProductTypeInput{
		Slug:            "e2e-type-" + suffix,
		Name:            "E2E Type " + suffix,
		TracksInventory: true, // what makes CreateProduct auto-provision the inventory item
	})
	if err != nil {
		t.Fatalf("create product type: %v", err)
	}

	p, err := h.d.Catalog.CreateProduct(ctx, catalog.CreateProductInput{
		ProductTypeID:       pt.ID,
		Locale:              "en", // must equal the resolved default locale - "en" on a fresh DB
		Name:                "E2E Widget " + suffix,
		SKU:                 "e2e-sku-" + suffix,
		DefaultCurrencyCode: "USD",
		DefaultPrice:        priceCents,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	// Explicit price skips the listing-currency-match check; IsPublished is what the cart gates on
	if _, err := h.d.Catalog.SetProductListing(ctx, catalog.ProductChannelListingInput{
		ProductID:   p.ID,
		ChannelID:   ch.ID,
		Price:       &priceCents,
		IsPublished: true,
	}); err != nil {
		t.Fatalf("publish listing: %v", err)
	}

	// Salable = levels at the channel's locations - a fresh channel has none, so build the chain
	loc, err := h.d.Inventory.CreateLocation(ctx, inventory.CreateLocationInput{
		Slug: "e2e-loc-" + suffix,
		Name: "E2E Location " + suffix,
	})
	if err != nil {
		t.Fatalf("create location: %v", err)
	}
	if err := h.d.Inventory.SetChannelLocations(ctx, ch.ID, []inventory.LocationAssignment{{LocationID: loc.ID, Priority: 0}}); err != nil {
		t.Fatalf("assign location: %v", err)
	}
	item, err := h.d.Inventory.EnsureItem(ctx, p.ID, true)
	if err != nil {
		t.Fatalf("ensure inventory item: %v", err)
	}
	if _, err := h.d.Inventory.AdjustLevel(ctx, inventory.AdjustLevelInput{
		InventoryItemID: item.ID,
		LocationID:      loc.ID,
		NewQuantity:     stock,
	}); err != nil {
		t.Fatalf("stock level: %v", err)
	}

	// The boot-time Sync predates this channel, so its provider rows don't exist yet - rerun
	// (idempotent upserts), then flip cod on: rows are born disabled
	if err := h.d.Payment.Sync(ctx); err != nil {
		t.Fatalf("payment sync: %v", err)
	}
	if _, err := h.d.Payment.SetEnabled(ctx, ch.ID, "cod", true); err != nil {
		t.Fatalf("enable cod: %v", err)
	}

	return &store{Channel: ch, Product: p, Location: loc, Item: item}
}

// Arrange half of the storefront flow: guest cart with one line, fatal on failure
func (h *harness) newGuestCart(t *testing.T, f *store, qty float64) uuid.UUID {
	t.Helper()
	return h.newGuestCartSKU(t, f, f.Product.SKU, qty)
}

func (h *harness) newGuestCartSKU(t *testing.T, f *store, sku string, qty float64) uuid.UUID {
	t.Helper()
	c, err := h.d.Cart.CreateGuestCart(h.ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("create guest cart: %v", err)
	}
	if _, err := h.d.Cart.AddToCart(h.ctx, c.ID, nil, cart.AddCartLineInput{
		ProductSku: sku,
		Quantity:   qty,
	}); err != nil {
		t.Fatalf("add to cart: %v", err)
	}
	return c.ID
}

// A registered customer's cart - callerID must match the cart's owner or the cart layer refuses
func (h *harness) newCustomerCart(t *testing.T, f *store, customerID uuid.UUID, sku string, qty float64) uuid.UUID {
	t.Helper()
	c, err := h.d.Cart.GetOrCreateCart(h.ctx, customerID, f.Channel.ID)
	if err != nil {
		t.Fatalf("customer cart: %v", err)
	}
	if _, err := h.d.Cart.AddToCart(h.ctx, c.ID, &customerID, cart.AddCartLineInput{
		ProductSku: sku,
		Quantity:   qty,
	}); err != nil {
		t.Fatalf("add to customer cart: %v", err)
	}
	return c.ID
}

// Act half, fallible on purpose - the negative scenarios assert the rejection itself
// Safe from spawned goroutines: it never calls Fatal
func (h *harness) tryPlace(t *testing.T, ch channel.Channel, cartID uuid.UUID, opts ...func(*order.PlaceOrderInput)) (order.Order, error) {
	t.Helper()
	cod := "cod"
	in := order.PlaceOrderInput{
		Email:               "guest-" + uuid.NewString()[:8] + "@e2e.test",
		CartID:              &cartID,
		BillingAddress:      e2eAddress(),
		ShippingAddress:     e2eAddress(),
		PaymentProviderSlug: &cod,
	}
	for _, o := range opts {
		o(&in)
	}
	ctx := channel.WithChannel(h.ctx, &ch) // placement resolves currency and channel from ctx, not the cart
	return h.d.Order.PlaceOrder(ctx, in)
}

// Admin cancel - the customer path refuses guest orders, this one only checks the status table
func (h *harness) cancel(t *testing.T, orderID uuid.UUID) {
	t.Helper()
	if _, err := h.d.Order.UpdateStatus(h.ctx, orderID, "cancelled"); err != nil {
		t.Fatalf("cancel order: %v", err)
	}
}

// The storefront flow every happy-path scenario starts from - returns without settling so callers
// can assert the committed transaction on its own first
func (h *harness) placeGuestCOD(t *testing.T, f *store, qty float64) order.Order {
	t.Helper()
	o, err := h.tryPlace(t, f.Channel, h.newGuestCart(t, f, qty))
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	return o
}

// A second product on an existing store - published=false leaves it invisible to the cart
func (h *harness) addProduct(t *testing.T, f *store, priceCents int64, published bool, stock float64) catalog.Product {
	t.Helper()
	suffix := uuid.NewString()[:8]
	p, err := h.d.Catalog.CreateProduct(h.ctx, catalog.CreateProductInput{
		ProductTypeID:       f.Product.ProductTypeID,
		Locale:              "en",
		Name:                "E2E Extra " + suffix,
		SKU:                 "e2e-extra-" + suffix,
		DefaultCurrencyCode: "USD",
		DefaultPrice:        priceCents,
	})
	if err != nil {
		t.Fatalf("create extra product: %v", err)
	}
	if _, err := h.d.Catalog.SetProductListing(h.ctx, catalog.ProductChannelListingInput{
		ProductID:   p.ID,
		ChannelID:   f.Channel.ID,
		Price:       &priceCents,
		IsPublished: published,
	}); err != nil {
		t.Fatalf("list extra product: %v", err)
	}
	item, err := h.d.Inventory.EnsureItem(h.ctx, p.ID, true)
	if err != nil {
		t.Fatalf("ensure extra item: %v", err)
	}
	if stock > 0 {
		if _, err := h.d.Inventory.AdjustLevel(h.ctx, inventory.AdjustLevelInput{
			InventoryItemID: item.ID,
			LocationID:      f.Location.ID,
			NewQuantity:     stock,
		}); err != nil {
			t.Fatalf("stock extra product: %v", err)
		}
	}
	return p
}

// listOn publishes an existing product onto another store's channel at an explicit price
func (h *harness) listOn(t *testing.T, ch channel.Channel, productID uuid.UUID, priceCents int64) {
	t.Helper()
	if _, err := h.d.Catalog.SetProductListing(h.ctx, catalog.ProductChannelListingInput{
		ProductID: productID, ChannelID: ch.ID, Price: &priceCents, IsPublished: true,
	}); err != nil {
		t.Fatalf("cross-list product: %v", err)
	}
}

func e2eAddress() *order.OrderAddress {
	return &order.OrderAddress{
		Street1:     "1 Test Way",
		City:        "Testville",
		PostalCode:  "12345",
		CountryCode: "US", // migration-seeded and active; State stays nil so the regions gate never fires
	}
}

// Gift cards own no inventory, so no item or stock - the price IS the card's face value
func (h *harness) addGiftcardProduct(t *testing.T, f *store, priceCents int64) catalog.Product {
	t.Helper()
	suffix := uuid.NewString()[:8]
	p, err := h.d.Catalog.CreateProduct(h.ctx, catalog.CreateProductInput{
		ProductTypeID:       f.Product.ProductTypeID,
		Locale:              "en",
		Name:                "E2E Gift Card " + suffix,
		SKU:                 "e2e-gc-" + suffix,
		Kind:                catalog.KindGiftcard,
		DefaultCurrencyCode: "USD",
		DefaultPrice:        priceCents,
	})
	if err != nil {
		t.Fatalf("create gift card product: %v", err)
	}
	if _, err := h.d.Catalog.SetProductListing(h.ctx, catalog.ProductChannelListingInput{
		ProductID: p.ID, ChannelID: f.Channel.ID, Price: &priceCents, IsPublished: true,
	}); err != nil {
		t.Fatalf("list gift card product: %v", err)
	}
	return p
}

// A monthly subscription product - terms are what make placement spawn the subscription row
func (h *harness) addSubscriptionProduct(t *testing.T, f *store, priceCents int64) catalog.Product {
	t.Helper()
	suffix := uuid.NewString()[:8]
	p, err := h.d.Catalog.CreateProduct(h.ctx, catalog.CreateProductInput{
		ProductTypeID:       f.Product.ProductTypeID,
		Locale:              "en",
		Name:                "E2E Subscription " + suffix,
		SKU:                 "e2e-sub-" + suffix,
		Kind:                catalog.KindSubscription,
		DefaultCurrencyCode: "USD",
		DefaultPrice:        priceCents,
	})
	if err != nil {
		t.Fatalf("create subscription product: %v", err)
	}
	if _, err := h.d.Catalog.SetSubscriptionTerms(h.ctx, catalog.SetSubscriptionTermsInput{
		ProductID: p.ID, IntervalUnit: "month", IntervalCount: 1,
	}); err != nil {
		t.Fatalf("subscription terms: %v", err)
	}
	if _, err := h.d.Catalog.SetProductListing(h.ctx, catalog.ProductChannelListingInput{
		ProductID: p.ID, ChannelID: f.Channel.ID, Price: &priceCents, IsPublished: true,
	}); err != nil {
		t.Fatalf("list subscription product: %v", err)
	}
	return p
}

// rearmOutbox resets delivered rows so the next settle redelivers them - simulates the crash
// window between dispatch and mark-delivered that makes the outbox at-least-once
func (h *harness) rearmOutbox(t *testing.T, eventType, containsID string, want int64) {
	t.Helper()
	res, err := h.d.DB.ExecContext(h.ctx,
		`UPDATE event_outbox SET delivered_at = NULL, next_attempt_at = NOW()
		  WHERE event_type = $1 AND payload::text LIKE '%' || $2 || '%'`,
		eventType, containsID)
	if err != nil {
		t.Fatalf("re-arm outbox rows: %v", err)
	}
	if n, _ := res.RowsAffected(); n != want {
		t.Fatalf("re-armed %d %s rows, want %d", n, eventType, want)
	}
}

func (h *harness) orderLines(t *testing.T, orderID uuid.UUID) []db.OrderLine {
	t.Helper()
	lines, err := h.d.Queries.ListOrderLinesByOrder(h.ctx, orderID)
	if err != nil {
		t.Fatalf("order lines: %v", err)
	}
	return lines
}

// ledgerByKind folds the succeeded-only payment sums into a map for one-line assertions
func (h *harness) ledgerByKind(t *testing.T, orderID uuid.UUID) map[string]int64 {
	t.Helper()
	sums, err := h.d.Queries.SumOrderPaymentByKind(h.ctx, orderID)
	if err != nil {
		t.Fatalf("ledger sums: %v", err)
	}
	m := make(map[string]int64, len(sums))
	for _, s := range sums {
		m[s.Kind] = s.Total
	}
	return m
}

// reservedFor sums the reserved counter across the store's levels - the residue check every
// rejected placement asserts against
func (h *harness) reservedFor(t *testing.T, f *store) float64 {
	t.Helper()
	levels, err := h.d.Queries.ListInventoryLevelsByItem(h.ctx, f.Item.ID)
	if err != nil {
		t.Fatalf("levels: %v", err)
	}
	var reserved float64
	for _, l := range levels {
		reserved += l.ReservedQuantity
	}
	return reserved
}

// outboxRows counts rows of one event type whose payload mentions the given id - payload shape is
// the emitting domain's own marshaling, so substring match beats coupling to its field names
func (h *harness) outboxRows(t *testing.T, eventType, containsID string) (total, undelivered int) {
	t.Helper()
	err := h.d.DB.QueryRowContext(h.ctx,
		`SELECT count(*), count(*) FILTER (WHERE delivered_at IS NULL)
		   FROM event_outbox WHERE event_type = $1 AND payload::text LIKE '%' || $2 || '%'`,
		eventType, containsID).Scan(&total, &undelivered)
	if err != nil {
		t.Fatalf("count outbox rows: %v", err)
	}
	return total, undelivered
}
