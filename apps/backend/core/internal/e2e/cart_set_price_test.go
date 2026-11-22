package e2e

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/integration"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// Scenario: support matches a price on a line of a shopper's own cart. It takes its own permission and a reason, and
// can't go below what the store allows. The shopper sees the price beside the regular one, can't buy more at it than
// it was set for, and the order line says the price was set, why, and what the catalog asked
func TestAPriceSetOnAShoppersOwnCart(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 10_000)
	h.setChannelConfig(t, f, "sales/cart/max_price_cut_percent", "20")
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	cartID := h.NewGuestCart(t, f, 2)

	token := func(name string, permissions ...string) string {
		t.Helper()
		created, err := h.D.Integration.Create(h.Ctx, integration.CreateIntegrationInput{Name: name, Permissions: permissions})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		t.Cleanup(func() { _ = h.D.Integration.Delete(h.Ctx, created.Integration.ID) })
		return created.RawToken
	}
	editor := token("e2e cart editor", "orders.view", "orders.update")
	pricer := token("e2e cart pricer", "orders.view", "orders.update", "orders.set_cart_price")

	data, errs := h.adminGqlPost(t, ts, pricer, fmt.Sprintf(`query { cart(id: %q) { priceLimits { maxCutPercent holdsUntil } lines { id } } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("read the cart: %v", errs)
	}
	read := data["cart"].(map[string]any)
	if limits := read["priceLimits"].(map[string]any); limits["maxCutPercent"] != float64(20) || limits["holdsUntil"] == nil {
		t.Fatalf("price limits = %v, want the channel's 20%% and a date a week out", limits)
	}
	lineID := read["lines"].([]any)[0].(map[string]any)["id"].(string)
	set := func(tok string, cents int, reason string) (map[string]any, []string) {
		t.Helper()
		return h.adminGqlPost(t, ts, tok, fmt.Sprintf(
			`mutation { adminSetCartLinePrice(cartId: %q, lineId: %q, unitPrice: %d, reason: %q) { lines { unitPrice priceSource listUnitPrice priceMaxQuantity priceReason priceExpiresAt } } }`,
			cartID, lineID, cents, reason))
	}

	if _, errs := set(editor, 8_500, "Price match"); len(errs) == 0 {
		t.Fatal("a token that may edit carts but not set their prices set one")
	}
	if _, errs := set(pricer, 8_500, ""); !errorsContain(errs, "say why") {
		t.Fatalf("no reason: %v, want one asked for", errs)
	}
	if _, errs := set(pricer, 7_999, "Price match"); !errorsContain(errs, "lowest price") {
		t.Fatalf("past the store's cut: %v, want it refused", errs)
	}
	data, errs = set(pricer, 8_500, "Price match")
	if len(errs) > 0 {
		t.Fatalf("set the price: %v", errs)
	}
	l := data["adminSetCartLinePrice"].(map[string]any)["lines"].([]any)[0].(map[string]any)
	if l["unitPrice"] != float64(8_500) || l["priceSource"] != "ADMIN" || l["listUnitPrice"] != float64(10_000) || l["priceMaxQuantity"] != float64(2) || l["priceReason"] != "Price match" || l["priceExpiresAt"] == nil {
		t.Fatalf("the line after = %v, want 8500 by an admin in place of 10000, for 2, until a date", l)
	}

	// What the shopper sees, and may do
	data, errs = h.gqlPost(t, ts, chID, fmt.Sprintf(`{ cart(id: %q) { subtotal lines { unitPrice listUnitPrice priceMaxQuantity currentUnitPrice } } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("the shopper's cart: %v", errs)
	}
	shown := data["cart"].(map[string]any)
	if sl := shown["lines"].([]any)[0].(map[string]any); shown["subtotal"] != float64(17_000) || sl["listUnitPrice"] != float64(10_000) || sl["priceMaxQuantity"] != float64(2) || sl["currentUnitPrice"] != nil {
		t.Fatalf("the shopper sees %v, want 2 at 8500 beside the regular 10000 and no price-changed notice", shown)
	}
	_, refused := h.gqlPostErrors(t, ts, chID, fmt.Sprintf(`mutation { updateCartLine(cartId: %q, lineId: %q, quantity: 3) { id } }`, cartID, lineID))
	if len(refused) != 1 || refused[0].Extensions["code"] != "CART_LINE_PRICE_QUANTITY" || refused[0].Extensions["maxQuantity"] != float64(2) || !strings.Contains(refused[0].Message, "up to 2") {
		t.Fatalf("a shopper raising the quantity: %+v, want it refused at 2", refused)
	}
	if _, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { addToCart(cartId: %q, sku: %q, quantity: 1) { id } }`, cartID, f.Product.SKU)); len(errs) == 0 {
		t.Fatal("a shopper added one more at the set price")
	}

	o, err := h.TryPlace(t, f.Channel, cartID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	if o.SubtotalAmount != 17_000 || len(o.Lines) != 1 {
		t.Fatalf("order subtotal %d with %d lines, want 2 at the set 8500", o.SubtotalAmount, len(o.Lines))
	}
	if ol := o.Lines[0]; ol.PriceSource != "admin" || ol.PriceReason != "Price match" || ol.ListUnitPrice == nil || *ol.ListUnitPrice != 10_000 {
		t.Errorf("order line %+v, want it to say the price was set, why, and the catalog's 10000", ol)
	}
	h.Settle(t)
}

// Scenario: 10% off carts over 100.00, and a cart with a price-matched jacket beside two mugs. The jacket counts toward
// the 100.00 - the cart holds it - but the 10% comes off the mugs alone, since the jacket's price is the deal. The
// order line says promotions left it alone. Set with promotions allowed, the same line is discounted like any other
func TestPromotionsLeaveASetPriceAlone(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 10_000)
	mug := h.AddProduct(t, f, 2_500, true, 20)
	h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E a tenth off over 100",
			ChannelID: &f.Channel.ID, IsActive: true,
		},
		Conditions: []promotion.Condition{{Type: "core.cart_subtotal_range", Params: json.RawMessage(`{"min_cents":10000}`)}},
		Actions:    []promotion.Action{{Type: "core.percent_off_subtotal", Params: json.RawMessage(`{"bps":1000}`)}},
	})
	place := func(promotionsApply bool) order.Order {
		t.Helper()
		cartID := h.NewGuestCart(t, f, 1)
		c, err := h.D.Cart.AddToCart(h.Ctx, cartID, nil, mug.SKU, 2)
		if err != nil {
			t.Fatalf("add the mugs: %v", err)
		}
		var jacket uuid.UUID
		for _, l := range c.Lines {
			if l.ProductSku == f.Product.SKU {
				jacket = l.ID
			}
		}
		price := int64(8_500)
		if _, err := h.D.Cart.AdminSetLinePrice(h.Ctx, cartID, jacket, cart.SetLinePriceInput{UnitPrice: &price, Reason: "Price match", PromotionsApply: promotionsApply}); err != nil {
			t.Fatalf("set the price: %v", err)
		}
		cod := "cod"
		quote, err := h.D.Order.QuoteCheckout(channel.WithChannel(h.Ctx, &f.Channel), order.PlaceOrderInput{
			CartID: &cartID, BillingAddress: e2eAddress(), ShippingAddress: e2eAddress(), PaymentProviderSlug: &cod,
		})
		if err != nil {
			t.Fatalf("quote: %v", err)
		}
		o, err := h.TryPlace(t, f.Channel, cartID)
		if err != nil {
			t.Fatalf("place: %v", err)
		}
		if quote.DiscountCents != o.DiscountAmount || quote.TotalCents != o.TotalAmount {
			t.Errorf("quoted discount %d total %d, placed %d and %d", quote.DiscountCents, quote.TotalCents, o.DiscountAmount, o.TotalAmount)
		}
		return o
	}

	o := place(false)
	if o.SubtotalAmount != 13_500 || o.DiscountAmount != 500 {
		t.Fatalf("subtotal %d discount %d, want 13500 and a tenth of the mugs' 5000", o.SubtotalAmount, o.DiscountAmount)
	}
	lines := h.linesAddUp(t, o)
	if l := lines[0]; l.ProductSku != f.Product.SKU || l.DiscountCents != 0 || !l.NoPromotions {
		t.Errorf("the set-price line %+v, want no discount and promotions marked as left off", l)
	}
	if l := lines[1]; l.DiscountCents != 500 || l.NoPromotions {
		t.Errorf("the line beside it %+v, want the whole 500", l)
	}

	o = place(true)
	if o.DiscountAmount != 1_350 {
		t.Fatalf("with promotions allowed the discount is %d, want a tenth of the whole 13500", o.DiscountAmount)
	}
	if lines = h.linesAddUp(t, o); lines[0].DiscountCents != 850 || lines[0].NoPromotions || lines[1].DiscountCents != 500 {
		t.Errorf("with promotions allowed: shares %d and %d, want 850 and 500", lines[0].DiscountCents, lines[1].DiscountCents)
	}
	h.Settle(t)
}
