package e2e

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

// Scenario: the shopper is shown a total, and the price moves before they press the button. Placement with the
// total they saw places nothing - no order, no stock held, the cart still theirs - and says what it comes to now.
// Asked again with that figure, the order is placed at it
func TestPlacementRefusesATotalTheShopperWasNotShown(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2_000)
	h.newTaxRule(t, f.Channel.ID, 1000)
	ctx := channel.WithChannel(h.Ctx, &f.Channel)
	cartID, cod := h.NewGuestCart(t, f, 2), "cod"
	in := order.PlaceOrderInput{CartID: &cartID, BillingAddress: e2eAddress(), ShippingAddress: e2eAddress(), PaymentProviderSlug: &cod}

	quote, err := h.D.Order.QuoteCheckout(ctx, in)
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if quote.TotalCents != 4_400 {
		t.Fatalf("quoted %d, want 2 x 2000 and 10%% tax", quote.TotalCents)
	}

	price := int64(2_500)
	if _, err := h.D.Catalog.SetProductListing(h.Ctx, catalog.ProductChannelListingInput{ProductID: f.Product.ID, ChannelID: f.Channel.ID, Price: &price, IsPublished: true}); err != nil {
		t.Fatalf("move the price: %v", err)
	}

	shown := quote.TotalCents
	_, err = h.TryPlace(t, f.Channel, cartID, func(p *order.PlaceOrderInput) { p.ExpectedTotalCents = &shown })
	var changed *order.TotalChangedError
	if !errors.Is(err, order.ErrTotalChanged) || !errors.As(err, &changed) {
		t.Fatalf("placing at a total that moved: %v, want it refused", err)
	}
	if changed.ExpectedCents != 4_400 || changed.Now.TotalCents != 5_500 || changed.Now.SubtotalCents != 5_000 || changed.Now.TaxCents != 500 {
		t.Errorf("refusal carries %+v, want the 4400 shown and the 5500 it comes to now", changed)
	}
	if got := h.ReservedFor(t, f); got != 0 {
		t.Errorf("%v reserved after a refused placement, want none", got)
	}
	c, err := h.D.Cart.GetCartByIDReadOnly(h.Ctx, cartID)
	if err != nil || c.Status != "active" {
		t.Fatalf("cart after a refused placement: status %q (%v), want it still active", c.Status, err)
	}
	var orders int
	if err := h.D.DB.QueryRowContext(h.Ctx, `SELECT count(*) FROM orders WHERE channel_id = $1`, f.Channel.ID).Scan(&orders); err != nil || orders != 0 {
		t.Fatalf("%d orders after a refused placement (%v), want none", orders, err)
	}

	now := changed.Now.TotalCents
	placed, err := h.TryPlace(t, f.Channel, cartID, func(p *order.PlaceOrderInput) { p.ExpectedTotalCents = &now })
	if err != nil {
		t.Fatalf("placing at the new total: %v", err)
	}
	if placed.TotalAmount != 5_500 {
		t.Errorf("placed at %d, want the 5500 the shopper agreed to", placed.TotalAmount)
	}
	h.Settle(t)
}

// Over the storefront API the refusal has its own code and the new figures beside it, and a placement that names no
// total goes through as it always did
func TestPlaceOrderReportsTheNewTotal(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2_000)
	h.addFlatShipping(t, f, 500, 0)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	cartID := h.NewGuestCart(t, f, 1).String()
	for _, step := range []string{
		fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345") { id }`, cartID),
		fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "builtin", code: "standard") { id }`, cartID),
		details(cartID, fmt.Sprintf(`email: "guard@e2e.test", shippingAddress: %s, billingSameAsShipping: true`, usAddress)) + ` { id }`,
		fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod") { id }`, cartID),
	} {
		if _, errs := h.gqlPost(t, ts, chID, `mutation { `+step+` }`); len(errs) > 0 {
			t.Fatalf("%s: %v", step, errs)
		}
	}
	data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`{ cart(id: %q) { checkout { totals { totalCents } } } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("totals: %v", errs)
	}
	shown := data["cart"].(map[string]any)["checkout"].(map[string]any)["totals"].(map[string]any)["totalCents"].(float64)
	if shown != 2_500 {
		t.Fatalf("shown total %v, want 2000 and 500 shipping", shown)
	}

	place := func(expected string) (map[string]any, []gqlError) {
		t.Helper()
		return h.gqlPostErrors(t, ts, chID, fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true%s) { id total } }`, cartID, expected))
	}
	_, refused := place(", expectedTotalCents: 2400")
	if len(refused) != 1 || refused[0].Extensions["code"] != "CHECKOUT_TOTAL_CHANGED" || refused[0].Extensions["class"] != "CONFLICT" {
		t.Fatalf("a total that isn't the order's: %+v, want CHECKOUT_TOTAL_CHANGED", refused)
	}
	ext := refused[0].Extensions
	if ext["totalCents"] != float64(2_500) || ext["expectedTotalCents"] != float64(2_400) || ext["subtotalCents"] != float64(2_000) || ext["shippingCents"] != float64(500) {
		t.Errorf("refusal extensions %v, want the 2500 it comes to with its parts", ext)
	}
	if !strings.Contains(refused[0].Message, "total changed") {
		t.Errorf("message %q, want it to say the total changed", refused[0].Message)
	}

	data, errs2 := place(fmt.Sprintf(", expectedTotalCents: %d", int(shown)))
	if len(errs2) > 0 || data["placeOrder"].(map[string]any)["total"] != float64(2_500) {
		t.Fatalf("placing at the total shown: %v %v", data, errs2)
	}
	h.Settle(t)
}
