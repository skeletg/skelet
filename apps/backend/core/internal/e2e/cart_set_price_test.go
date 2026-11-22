package e2e

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/integration"
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
