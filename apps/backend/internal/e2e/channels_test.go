package e2e

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/cart"
	"github.com/skeletg/skelet/internal/catalog"
	"github.com/skeletg/skelet/internal/channel"
	"github.com/skeletg/skelet/internal/currency"
	"github.com/skeletg/skelet/internal/giftcard"
	"github.com/skeletg/skelet/internal/inventory"
	"github.com/skeletg/skelet/internal/order"
)

// Scenario: a customer IS (channel, email) - the same address registers separately per store, and
// email verification claims guest orders only within its own channel
func TestCustomersAreChannelScoped(t *testing.T) {
	h := newHarness(t)
	a := h.newStore(t, 5, 1000)
	b := h.newStore(t, 5, 1000)
	email := "twochan-" + uuid.NewString()[:8] + "@e2e.test"

	ca, err := h.d.Customer.Register(h.ctx, a.Channel.ID, email, "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register on A: %v", err)
	}
	cb, err := h.d.Customer.Register(h.ctx, b.Channel.ID, email, "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register same email on B: %v - identity must be per channel", err)
	}
	if ca.ID == cb.ID {
		t.Fatal("one customer row spans two channels")
	}

	// A guest order on A, then verification on B - the claim must not cross the channel line
	cartID := h.newGuestCart(t, a, 1)
	o, err := h.tryPlace(t, a.Channel, cartID, func(in *order.PlaceOrderInput) { in.Email = email })
	if err != nil {
		t.Fatalf("guest order on A: %v", err)
	}
	h.settle(t)

	tokB, err := h.d.Verify.Create(h.ctx, cb.ID)
	if err != nil {
		t.Fatalf("token B: %v", err)
	}
	if err := h.d.Customer.VerifyEmail(h.ctx, tokB); err != nil {
		t.Fatalf("verify B: %v", err)
	}
	h.settle(t)
	if row, _ := h.d.Queries.GetOrderByID(h.ctx, o.ID); row.CustomerID.Valid {
		t.Fatal("B's verification claimed A's guest order across channels")
	}

	tokA, err := h.d.Verify.Create(h.ctx, ca.ID)
	if err != nil {
		t.Fatalf("token A: %v", err)
	}
	if err := h.d.Customer.VerifyEmail(h.ctx, tokA); err != nil {
		t.Fatalf("verify A: %v", err)
	}
	h.settle(t)
	row, err := h.d.Queries.GetOrderByID(h.ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read order: %v", err)
	}
	if !row.CustomerID.Valid || row.CustomerID.UUID != ca.ID {
		t.Errorf("order customer = %v, want claimed by A's %s", row.CustomerID, ca.ID)
	}
}

// Scenario: one product, many storefronts - each channel sells at its own listing price, and a
// channel without a listing doesn't sell it at all
func TestListingPriceAndPublicationArePerChannel(t *testing.T) {
	h := newHarness(t)
	a := h.newStore(t, 10, 2000)
	b := h.newStoreWith(t, 10, 999, "USD")
	h.listOn(t, b.Channel, a.Product.ID, 2500)
	// B's channel must also see the product's stock - locations are the channel's own view
	if err := h.d.Inventory.SetChannelLocations(h.ctx, b.Channel.ID, []inventory.LocationAssignment{
		{LocationID: b.Location.ID, Priority: 0}, {LocationID: a.Location.ID, Priority: 1},
	}); err != nil {
		t.Fatalf("share location with B: %v", err)
	}

	oa := h.placeGuestCOD(t, a, 1)
	if lines := h.orderLines(t, oa.ID); lines[0].UnitPrice != 2000 {
		t.Errorf("A sells at %d, want its 2000", lines[0].UnitPrice)
	}
	ob, err := h.tryPlace(t, b.Channel, h.newGuestCartSKU(t, b, a.Product.SKU, 1))
	if err != nil {
		t.Fatalf("place on B: %v", err)
	}
	if lines := h.orderLines(t, ob.ID); lines[0].UnitPrice != 2500 {
		t.Errorf("B sells at %d, want its 2500", lines[0].UnitPrice)
	}

	// A third store carries no listing - the product simply doesn't exist there
	c := h.newStore(t, 5, 700)
	cc, err := h.d.Cart.CreateGuestCart(h.ctx, c.Channel.ID)
	if err != nil {
		t.Fatalf("cart on C: %v", err)
	}
	if _, err := h.d.Cart.AddToCart(h.ctx, cc.ID, nil, cart.AddCartLineInput{
		ProductSku: a.Product.SKU, Quantity: 1,
	}); err == nil || !strings.Contains(err.Error(), "not published") {
		t.Errorf("unlisted-channel add = %v, want the not-published refusal", err)
	}
	h.settle(t)
}

// Scenario: money speaks the channel's currency - an inherited listing refuses a currency
// mismatch, an explicit price crosses it deliberately, and the order stamps the channel currency
func TestChannelCurrencyGovernsListingsAndOrders(t *testing.T) {
	h := newHarness(t)
	eur := h.newStoreWith(t, 10, 1800, "EUR")

	// A USD-default product cannot INHERIT its price onto a EUR channel
	p, err := h.d.Catalog.CreateProduct(h.ctx, catalog.CreateProductInput{
		ProductTypeID: eur.Product.ProductTypeID, Locale: "en",
		Name: "E2E Inherit " + uuid.NewString()[:8], SKU: "e2e-inh-" + uuid.NewString()[:8],
		DefaultCurrencyCode: "USD", DefaultPrice: 1000,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	if _, err := h.d.Catalog.SetProductListing(h.ctx, catalog.ProductChannelListingInput{
		ProductID: p.ID, ChannelID: eur.Channel.ID, IsPublished: true, // Price nil = inherit
	}); !errors.Is(err, catalog.ErrListingCurrencyMismatch) {
		t.Errorf("inherited cross-currency listing = %v, want ErrListingCurrencyMismatch", err)
	}
	// An explicit price is a deliberate decision and passes
	h.listOn(t, eur.Channel, p.ID, 950)

	o := h.placeGuestCOD(t, eur, 1)
	row, err := h.d.Queries.GetOrderByID(h.ctx, o.ID)
	if err != nil {
		t.Fatalf("order row: %v", err)
	}
	if row.CurrencyCode != "EUR" {
		t.Errorf("order currency = %q, want the channel's EUR", row.CurrencyCode)
	}
	h.settle(t)
}

// Scenario: a gift card is currency-bound - redeeming a USD card on a EUR storefront fails the
// placement outright and drains nothing
func TestGiftCardCurrencyIsScoped(t *testing.T) {
	h := newHarness(t)
	eur := h.newStoreWith(t, 5, 2000, "EUR")
	usdCard, err := h.d.Giftcard.Issue(h.ctx, giftcard.IssueInput{CurrencyCode: "USD", Amount: 1000})
	if err != nil {
		t.Fatalf("issue USD card: %v", err)
	}

	if _, err := h.tryPlace(t, eur.Channel, h.newGuestCart(t, eur, 1), func(in *order.PlaceOrderInput) {
		in.GiftCardCodes = []string{usdCard.Code}
	}); !errors.Is(err, giftcard.ErrCurrencyMismatch) {
		t.Errorf("USD card on EUR channel = %v, want ErrCurrencyMismatch", err)
	}
	if bal, _ := h.d.Giftcard.Balance(h.ctx, usdCard.Code); bal != 1000 {
		t.Errorf("USD card drained to %d by a failed placement, want untouched 1000", bal)
	}

	eurCard, err := h.d.Giftcard.Issue(h.ctx, giftcard.IssueInput{CurrencyCode: "EUR", Amount: 1000})
	if err != nil {
		t.Fatalf("issue EUR card: %v", err)
	}
	o, err := h.tryPlace(t, eur.Channel, h.newGuestCart(t, eur, 1), func(in *order.PlaceOrderInput) {
		in.GiftCardCodes = []string{eurCard.Code}
	})
	if err != nil {
		t.Fatalf("EUR card on EUR channel: %v", err)
	}
	if o.GiftCardAmount != 1000 {
		t.Errorf("EUR card applied %d, want 1000", o.GiftCardAmount)
	}
	h.settle(t)
}

// Scenario: store credit is a per-currency ledger - dollars are invisible to a euro checkout
func TestStoreCreditIsPerCurrency(t *testing.T) {
	h := newHarness(t)
	eur := h.newStoreWith(t, 5, 2000, "EUR")
	email := "eur-" + uuid.NewString()[:8] + "@e2e.test"
	c, err := h.d.Customer.Register(h.ctx, eur.Channel.ID, email, "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := h.d.StoreCredit.Adjust(h.ctx, c.ID, 800, "USD", "e2e usd grant", nil); err != nil {
		t.Fatalf("grant USD: %v", err)
	}

	place := func() order.Order {
		t.Helper()
		o, err := h.tryPlace(t, eur.Channel, h.newCustomerCart(t, eur, c.ID, eur.Product.SKU, 1),
			func(in *order.PlaceOrderInput) {
				in.CustomerID = &c.ID
				in.Email = email
				in.UseStoreCredit = true
			})
		if err != nil {
			t.Fatalf("place: %v", err)
		}
		return o
	}
	if o := place(); o.StoreCreditAmount != 0 {
		t.Errorf("USD credit applied %d on a EUR order, want 0", o.StoreCreditAmount)
	}
	if _, err := h.d.StoreCredit.Adjust(h.ctx, c.ID, 600, "EUR", "e2e eur grant", nil); err != nil {
		t.Fatalf("grant EUR: %v", err)
	}
	if o := place(); o.StoreCreditAmount != 600 {
		t.Errorf("EUR credit applied %d, want 600", o.StoreCreditAmount)
	}
	h.settle(t)
}

// Scenario: salable stock is the CHANNEL's view - a listing without the channel seeing any
// stocked location sells nothing, however much sits in another store's warehouse
func TestSalableIsPerChannelLocations(t *testing.T) {
	h := newHarness(t)
	a := h.newStore(t, 10, 1500)
	b := h.newStoreWith(t, 5, 700, "USD")
	h.listOn(t, b.Channel, a.Product.ID, 1500) // listed on B, stocked only at A's location

	c, err := h.d.Cart.CreateGuestCart(h.ctx, b.Channel.ID)
	if err != nil {
		t.Fatalf("cart on B: %v", err)
	}
	if _, err := h.d.Cart.AddToCart(h.ctx, c.ID, nil, cart.AddCartLineInput{
		ProductSku: a.Product.SKU, Quantity: 1,
	}); err != nil {
		t.Logf("rejected at add-to-cart: %v", err) // the fail-closed add gate saw salable 0 - fine
	} else if _, err := h.tryPlace(t, b.Channel, c.ID); err == nil {
		t.Fatal("B sold stock its channel cannot see")
	} else if !errors.Is(err, inventory.ErrInsufficientStock) {
		t.Errorf("cross-location placement = %v, want ErrInsufficientStock", err)
	}

	// A's own storefront is untouched by B's failure
	oa := h.placeGuestCOD(t, a, 1)
	if got := h.reservedFor(t, a); got != 1 {
		t.Errorf("A reserved = %v after its own sale, want 1", got)
	}
	_ = oa
	h.settle(t)
}

// Scenario: channels only speak registered currencies and locales - and carry the locale sets
// they declare
func TestChannelValidatesCurrencyAndLocales(t *testing.T) {
	h := newHarness(t)

	if _, err := h.d.Channel.Create(h.ctx, channel.CreateChannelInput{
		Slug: "e2e-bad-" + uuid.NewString()[:8], Name: "Bad", CurrencyCode: "ZZZ",
	}); !errors.Is(err, currency.ErrCurrencyNotFound) {
		t.Errorf("unknown currency = %v, want ErrCurrencyNotFound", err)
	}
	if _, err := h.d.Channel.Create(h.ctx, channel.CreateChannelInput{
		Slug: "e2e-badloc-" + uuid.NewString()[:8], Name: "Bad", CurrencyCode: "USD",
		Locales: []string{"en", "zz"},
	}); err == nil {
		t.Error("channel accepted an unregistered locale")
	}

	// A real second locale rides the channel's declared set (idempotent across suite reruns)
	h.ensureLocale(t, "de")
	ch, err := h.d.Channel.Create(h.ctx, channel.CreateChannelInput{
		Slug: "e2e-de-" + uuid.NewString()[:8], Name: "Bilingual", CurrencyCode: "USD",
		DefaultLocale: "en", Locales: []string{"en", "de"},
	})
	if err != nil {
		t.Fatalf("bilingual channel: %v", err)
	}
	carries := false
	for _, l := range ch.Locales {
		if l.Code == "de" {
			carries = true
		}
	}
	if !carries || ch.DefaultLocale != "en" {
		t.Errorf("channel locales = %+v default %q, want de carried with en default", ch.Locales, ch.DefaultLocale)
	}
}
