package e2e

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

func (h *harness) gqlGuestCart(t *testing.T, ts *httptest.Server, f *store, qty int) string {
	t.Helper()
	chID := f.Channel.ID.String()
	data, errs := h.gqlPost(t, ts, chID, `mutation { createGuestCart { id } }`)
	if len(errs) > 0 {
		t.Fatalf("createGuestCart: %v", errs)
	}
	cartID := gqlField(t, data, "createGuestCart", "id")
	if _, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(
		`mutation { addToCart(cartId: %q, productName: "x", productSku: %q, unitPrice: 1, quantity: %d) { id } }`,
		cartID, f.Product.SKU, qty)); len(errs) > 0 {
		t.Fatalf("addToCart: %v", errs)
	}
	return cartID
}

func (h *harness) gqlQuoteCents(t *testing.T, ts *httptest.Server, f *store, args string) float64 {
	t.Helper()
	data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`{ shippingQuotes(%s) { code priceCents } }`, args))
	if len(errs) > 0 {
		t.Fatalf("shippingQuotes(%s): %v", args, errs)
	}
	quotes, _ := data["shippingQuotes"].([]any)
	if len(quotes) != 1 {
		t.Fatalf("shippingQuotes(%s) = %v, want the one standard method", args, data)
	}
	return quotes[0].(map[string]any)["priceCents"].(float64)
}

// Scenario: a guest's checkout quote prices their actual cart, after discounts, the way placement charges it
func TestShippingQuotesPriceTheGuestCart(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2500)
	h.addFlatShipping(t, f, 799, 2000)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	cartID := h.gqlGuestCart(t, ts, f, 1)

	if got := h.gqlQuoteCents(t, ts, f, fmt.Sprintf(`countryCode: "US", cartId: %q`, cartID)); got != 0 {
		t.Errorf("quote for a 2500 cart = %v, want free above 2000", got)
	}

	h.newRule(t, promotion.CreateRuleInput{
		Rule:    promotion.Rule{Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E 1000 off", ChannelID: &f.Channel.ID, IsActive: true},
		Actions: []promotion.Action{{Type: "core.fixed_off_subtotal", Params: json.RawMessage(`{"cents":1000}`)}},
	})
	if got := h.gqlQuoteCents(t, ts, f, fmt.Sprintf(`countryCode: "US", cartId: %q, regionCode: "NY", postalCode: "10001"`, cartID)); got != 799 {
		t.Errorf("quote after 1000 off = %v, want 799 - 1500 is under the free bar", got)
	}
	o := h.PlaceGuestCOD(t, f, 1)
	if o.ShippingAmount != 799 {
		t.Errorf("placement charged shipping %d, want the quoted 799", o.ShippingAmount)
	}
	h.Settle(t)
}

// Scenario: the cart estimate's tax is what placement charges, shipping included
func TestCartEstimateTaxMatchesPlacement(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	h.newTaxRule(t, f.Channel.ID, 1000)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	cartID := h.gqlGuestCart(t, ts, f, 1)

	data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(
		`{ cartEstimate(cartId: %q, countryCode: "US") { taxCents cheapestShippingCents estimatedTotalCents } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("cartEstimate: %v", errs)
	}
	est := data["cartEstimate"].(map[string]any)
	cartUUID := uuid.MustParse(cartID)
	o, err := h.TryPlace(t, f.Channel, cartUUID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	if est["taxCents"].(float64) != float64(o.TaxAmount) || o.TaxAmount != 250 {
		t.Errorf("estimated tax %v, placed %d, want both 250 (goods 200 + shipping 50)", est["taxCents"], o.TaxAmount)
	}
	if est["estimatedTotalCents"].(float64) != float64(o.TotalAmount) {
		t.Errorf("estimated total %v, placed %d", est["estimatedTotalCents"], o.TotalAmount)
	}
	h.Settle(t)
}

// Scenario: the catalog answers each SKU's weight and size for shipping rates
func TestProductDimensionsReachShippingRates(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	w, l := int32(1250), int32(300)
	if _, err := h.D.Catalog.UpdateProduct(h.Ctx, catalog.UpdateProductInput{ID: f.Product.ID, WeightGrams: &w, LengthMm: &l}); err != nil {
		t.Fatalf("set weight: %v", err)
	}
	dims, err := h.D.Catalog.ProductDimensionsBySKUs(h.Ctx, []string{f.Product.SKU, "no-such-sku"})
	if err != nil {
		t.Fatalf("dimensions: %v", err)
	}
	if got := dims[f.Product.SKU]; got.WeightGrams != 1250 || got.LengthMm != 300 || got.WidthMm != 0 {
		t.Errorf("dimensions = %+v, want weight 1250, length 300, the rest unset", got)
	}
	if _, ok := dims["no-such-sku"]; ok {
		t.Error("an unknown sku came back with dimensions")
	}
}
