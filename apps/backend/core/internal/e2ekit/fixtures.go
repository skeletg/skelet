package e2ekit

import (
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient/roster"
)

// A Store built from scratch through the real services: channel, product type, published listing,
// stocked location, cod enabled - everything a placement needs, nothing shared between tests
type Store struct {
	Channel  channel.Channel
	Product  catalog.Product
	Location inventory.Location
	Item     inventory.Item
}

func (h *Harness) NewStore(t TB, stock float64, priceCents int64) *Store {
	t.Helper()
	return h.NewStoreWith(t, stock, priceCents, "USD")
}

// newStoreWith picks the channel currency - products keep DefaultCurrencyCode USD, which is fine
// because every fixture listing carries an explicit price and so skips the currency-match check
func (h *Harness) NewStoreWith(t TB, stock float64, priceCents int64, currencyCode string) *Store {
	t.Helper()
	return h.NewStoreOpt(t, stock, priceCents, func(in *channel.CreateChannelInput) {
		in.CurrencyCode = currencyCode
	})
}

// ensureLocale registers a locale ACTIVE, repairing a persisted inactive row from earlier runs -
// CreateLocaleInput.IsActive has no default, and the clamp skips inactive non-default locales
func (h *Harness) EnsureLocale(t TB, code string) {
	t.Helper()
	active := true
	if err := h.D.Locale.EnsureExists(h.Ctx, code); err != nil {
		if _, cerr := h.D.Locale.Create(h.Ctx, locale.CreateLocaleInput{Code: code, Name: code, IsActive: true}); cerr != nil {
			t.Fatalf("create locale %s: %v", code, cerr)
		}
		return
	}
	if _, err := h.D.Locale.Update(h.Ctx, locale.UpdateLocaleInput{Code: code, IsActive: &active}); err != nil {
		t.Fatalf("activate locale %s: %v", code, err)
	}
}

func (h *Harness) NewStoreOpt(t TB, stock float64, priceCents int64, mutate func(*channel.CreateChannelInput)) *Store {
	t.Helper()
	ctx := h.Ctx
	suffix := uuid.NewString()[:8]

	in := channel.CreateChannelInput{
		Slug:         "e2e-" + suffix,
		Name:         "E2E Store " + suffix,
		CurrencyCode: "USD",
	}
	if mutate != nil {
		mutate(&in)
	}
	ch, err := h.D.Channel.Create(ctx, in)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	pt, err := h.D.Catalog.CreateProductType(ctx, catalog.CreateProductTypeInput{
		Slug:               "e2e-type-" + suffix,
		Name:               "E2E Type " + suffix,
		TracksInventory:    true, // what makes CreateProduct auto-provision the inventory item
		IsShippingRequired: true, // physical goods - checkout groups them for delivery
	})
	if err != nil {
		t.Fatalf("create product type: %v", err)
	}

	p, err := h.D.Catalog.CreateProduct(ctx, catalog.CreateProductInput{
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
	if _, err := h.D.Catalog.SetProductListing(ctx, catalog.ProductChannelListingInput{
		ProductID:   p.ID,
		ChannelID:   ch.ID,
		Price:       &priceCents,
		IsPublished: true,
	}); err != nil {
		t.Fatalf("publish listing: %v", err)
	}

	// Salable = levels at the channel's locations - a fresh channel has none, so build the chain
	loc, err := h.D.Inventory.CreateLocation(ctx, inventory.CreateLocationInput{
		Slug: "e2e-loc-" + suffix,
		Name: "E2E Location " + suffix,
	})
	if err != nil {
		t.Fatalf("create location: %v", err)
	}
	if err := h.D.Inventory.SetChannelLocations(ctx, ch.ID, []inventory.LocationAssignment{{LocationID: loc.ID, Priority: 0}}); err != nil {
		t.Fatalf("assign location: %v", err)
	}
	item, err := h.D.Inventory.EnsureItem(ctx, p.ID, true)
	if err != nil {
		t.Fatalf("ensure inventory item: %v", err)
	}
	if _, err := h.D.Inventory.AdjustLevel(ctx, inventory.AdjustLevelInput{
		InventoryItemID: item.ID,
		LocationID:      loc.ID,
		NewQuantity:     stock,
	}); err != nil {
		t.Fatalf("stock level: %v", err)
	}

	// The boot-time Sync predates this channel, so its provider rows don't exist yet - rerun
	// (idempotent upserts), then flip cod on: rows are born disabled
	if err := h.D.Payment.Sync(ctx); err != nil {
		t.Fatalf("payment sync: %v", err)
	}
	if _, err := h.D.Payment.SetEnabled(ctx, ch.ID, "cod", true); err != nil {
		t.Fatalf("enable cod: %v", err)
	}

	h.SyncChannelRoster(t) // the beacon surface serves a channel only once core has pushed it across
	return &Store{Channel: ch, Product: p, Location: loc, Item: item}
}

// SyncChannelRoster runs the channel-roster push the scheduler job runs, so the suite serves this
// harness's channels - over the contract, to the Signals process bin/test-e2e started
func (h *Harness) SyncChannelRoster(t TB) {
	t.Helper()
	if err := roster.PushChannels(h.Ctx, h.D.Channel, h.D.Config, h.D.SignalsClient); err != nil {
		t.Fatalf("push channel roster: %v", err)
	}
}

// Arrange half of the storefront flow: guest cart with one line, fatal on failure
func (h *Harness) NewGuestCart(t TB, f *Store, qty float64) uuid.UUID {
	t.Helper()
	return h.NewGuestCartSKU(t, f, f.Product.SKU, qty)
}

func (h *Harness) NewGuestCartSKU(t TB, f *Store, sku string, qty float64) uuid.UUID {
	t.Helper()
	c, err := h.D.Cart.CreateGuestCart(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("create guest cart: %v", err)
	}
	if _, err := h.D.Cart.AddToCart(h.Ctx, c.ID, nil, sku, qty); err != nil {
		t.Fatalf("add to cart: %v", err)
	}
	return c.ID
}

// A registered customer's cart - callerID must match the cart's owner or the cart layer refuses
func (h *Harness) NewCustomerCart(t TB, f *Store, customerID uuid.UUID, sku string, qty float64) uuid.UUID {
	t.Helper()
	c, err := h.D.Cart.GetOrCreateCart(h.Ctx, customerID, f.Channel.ID)
	if err != nil {
		t.Fatalf("customer cart: %v", err)
	}
	if _, err := h.D.Cart.AddToCart(h.Ctx, c.ID, &customerID, sku, qty); err != nil {
		t.Fatalf("add to customer cart: %v", err)
	}
	return c.ID
}

// Act half, fallible on purpose - the negative scenarios assert the rejection itself
// Safe from spawned goroutines: it never calls Fatal
func (h *Harness) TryPlace(t TB, ch channel.Channel, cartID uuid.UUID, opts ...func(*order.PlaceOrderInput)) (order.Order, error) {
	t.Helper()
	cod := "cod"
	in := order.PlaceOrderInput{
		Email:               "guest-" + uuid.NewString()[:8] + "@e2e.test",
		CartID:              &cartID,
		BillingAddress:      Address(),
		ShippingAddress:     Address(),
		PaymentProviderSlug: &cod,
	}
	for _, o := range opts {
		o(&in)
	}
	ctx := channel.WithChannel(h.Ctx, &ch) // placement resolves currency and channel from ctx, not the cart
	return h.D.Order.PlaceOrder(ctx, in)
}

// Admin cancel - the customer path refuses guest orders, this one only checks the status table
func (h *Harness) Cancel(t TB, orderID uuid.UUID) {
	t.Helper()
	if _, err := h.D.Order.UpdateStatus(h.Ctx, orderID, "cancelled"); err != nil {
		t.Fatalf("cancel order: %v", err)
	}
}

// The storefront flow every happy-path scenario starts from - returns without settling so callers
// can assert the committed transaction on its own first
func (h *Harness) PlaceGuestCOD(t TB, f *Store, qty float64) order.Order {
	t.Helper()
	o, err := h.TryPlace(t, f.Channel, h.NewGuestCart(t, f, qty))
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	return o
}

// A second product on an existing Store - published=false leaves it invisible to the cart
func (h *Harness) AddProduct(t TB, f *Store, priceCents int64, published bool, stock float64) catalog.Product {
	t.Helper()
	suffix := uuid.NewString()[:8]
	p, err := h.D.Catalog.CreateProduct(h.Ctx, catalog.CreateProductInput{
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
	if _, err := h.D.Catalog.SetProductListing(h.Ctx, catalog.ProductChannelListingInput{
		ProductID:   p.ID,
		ChannelID:   f.Channel.ID,
		Price:       &priceCents,
		IsPublished: published,
	}); err != nil {
		t.Fatalf("list extra product: %v", err)
	}
	item, err := h.D.Inventory.EnsureItem(h.Ctx, p.ID, true)
	if err != nil {
		t.Fatalf("ensure extra item: %v", err)
	}
	if stock > 0 {
		if _, err := h.D.Inventory.AdjustLevel(h.Ctx, inventory.AdjustLevelInput{
			InventoryItemID: item.ID,
			LocationID:      f.Location.ID,
			NewQuantity:     stock,
		}); err != nil {
			t.Fatalf("stock extra product: %v", err)
		}
	}
	return p
}

// listOn publishes an existing product onto another Store's channel at an explicit price
func (h *Harness) ListOn(t TB, ch channel.Channel, productID uuid.UUID, priceCents int64) {
	t.Helper()
	if _, err := h.D.Catalog.SetProductListing(h.Ctx, catalog.ProductChannelListingInput{
		ProductID: productID, ChannelID: ch.ID, Price: &priceCents, IsPublished: true,
	}); err != nil {
		t.Fatalf("cross-list product: %v", err)
	}
}

func Address() *order.OrderAddress {
	return &order.OrderAddress{
		Street1:     "1 Test Way",
		City:        "Testville",
		PostalCode:  "12345",
		CountryCode: "US", // migration-seeded and active; State stays nil so the regions gate never fires
	}
}

// Gift cards own no inventory, so no item or stock - the price IS the card's face value
func (h *Harness) AddGiftcardProduct(t TB, f *Store, priceCents int64) catalog.Product {
	t.Helper()
	suffix := uuid.NewString()[:8]
	p, err := h.D.Catalog.CreateProduct(h.Ctx, catalog.CreateProductInput{
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
	if _, err := h.D.Catalog.SetProductListing(h.Ctx, catalog.ProductChannelListingInput{
		ProductID: p.ID, ChannelID: f.Channel.ID, Price: &priceCents, IsPublished: true,
	}); err != nil {
		t.Fatalf("list gift card product: %v", err)
	}
	return p
}

// A monthly subscription product - terms are what make placement spawn the subscription row
func (h *Harness) AddSubscriptionProduct(t TB, f *Store, priceCents int64) catalog.Product {
	t.Helper()
	suffix := uuid.NewString()[:8]
	p, err := h.D.Catalog.CreateProduct(h.Ctx, catalog.CreateProductInput{
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
	if _, err := h.D.Catalog.SetSubscriptionTerms(h.Ctx, catalog.SetSubscriptionTermsInput{
		ProductID: p.ID, IntervalUnit: "month", IntervalCount: 1,
	}); err != nil {
		t.Fatalf("subscription terms: %v", err)
	}
	if _, err := h.D.Catalog.SetProductListing(h.Ctx, catalog.ProductChannelListingInput{
		ProductID: p.ID, ChannelID: f.Channel.ID, Price: &priceCents, IsPublished: true,
	}); err != nil {
		t.Fatalf("list subscription product: %v", err)
	}
	return p
}

// rearmOutbox resets delivered rows so the next settle redelivers them - simulates the crash
// window between dispatch and mark-delivered that makes the outbox at-least-once
func (h *Harness) RearmOutbox(t TB, eventType, containsID string, want int64) {
	t.Helper()
	res, err := h.D.DB.ExecContext(h.Ctx,
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

func (h *Harness) OrderLines(t TB, orderID uuid.UUID) []db.OrderLine {
	t.Helper()
	lines, err := h.D.Queries.ListOrderLinesByOrder(h.Ctx, orderID)
	if err != nil {
		t.Fatalf("order lines: %v", err)
	}
	return lines
}

// ledgerByKind folds the succeeded-only payment sums into a map for one-line assertions
func (h *Harness) LedgerByKind(t TB, orderID uuid.UUID) map[string]int64 {
	t.Helper()
	sums, err := h.D.Queries.SumOrderPaymentByKind(h.Ctx, orderID)
	if err != nil {
		t.Fatalf("ledger sums: %v", err)
	}
	m := make(map[string]int64, len(sums))
	for _, s := range sums {
		m[s.Kind] = s.Total
	}
	return m
}

// reservedFor sums the reserved counter across the Store's levels - the residue check every
// rejected placement asserts against
func (h *Harness) ReservedFor(t TB, f *Store) float64 {
	t.Helper()
	levels, err := h.D.Queries.ListInventoryLevelsByItem(h.Ctx, f.Item.ID)
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
func (h *Harness) OutboxRows(t TB, eventType, containsID string) (total, undelivered int) {
	t.Helper()
	err := h.D.DB.QueryRowContext(h.Ctx,
		`SELECT count(*), count(*) FILTER (WHERE delivered_at IS NULL)
		   FROM event_outbox WHERE event_type = $1 AND payload::text LIKE '%' || $2 || '%'`,
		eventType, containsID).Scan(&total, &undelivered)
	if err != nil {
		t.Fatalf("count outbox rows: %v", err)
	}
	return total, undelivered
}
