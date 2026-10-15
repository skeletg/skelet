package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
)

// gqlPost drives the real front API - the request crosses the entire production middleware
// chain: recover, body limits, rate limiting, auth, channel resolution, locale clamp, dataloaders
func (h *harness) gqlPost(t *testing.T, ts *httptest.Server, channelID, query string) (map[string]any, []string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatalf("marshal query: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+frontAPIPath(t), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if channelID != "" {
		req.Header.Set(channel.HeaderChannelID, channelID)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("post graphql: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Data   map[string]any `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode graphql response (http %d): %v", resp.StatusCode, err)
	}
	var msgs []string
	for _, e := range out.Errors {
		msgs = append(msgs, e.Message)
	}
	return out.Data, msgs
}

func gqlField(t *testing.T, data map[string]any, object, field string) string {
	t.Helper()
	obj, ok := data[object].(map[string]any)
	if !ok {
		t.Fatalf("response has no %s object: %v", object, data)
	}
	v, ok := obj[field].(string)
	if !ok {
		t.Fatalf("%s.%s missing or not a string: %v", object, field, obj)
	}
	return v
}

// Scenario: the storefront checkout over real HTTP - the wire's answers must equal the database's,
// the channel header is identity, and a converted cart can't place twice
func TestFrontGraphQLCheckout(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2300)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()

	// Header-less requests are deployment-defined (the middleware may fall back to an elected default channel), so identity is asserted the
	// deterministic way: the header must scope everything below to THIS store, proven by the DB rows at the end

	chID := f.Channel.ID.String()
	data, errs := h.gqlPost(t, ts, chID, `mutation { createGuestCart { id } }`)
	if len(errs) > 0 {
		t.Fatalf("createGuestCart: %v", errs)
	}
	cartID := gqlField(t, data, "createGuestCart", "id")

	// The price comes from the listing alone - the mutation has no way to send one
	if _, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(
		`mutation { addToCart(cartId: %q, sku: %q, quantity: 1, unitPrice: 1) { id } }`,
		cartID, f.Product.SKU)); len(errs) == 0 {
		t.Fatal("addToCart accepted a client price")
	}
	if _, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(
		`mutation { addToCart(cartId: %q, sku: %q, quantity: 1) { id } }`,
		cartID, f.Product.SKU)); len(errs) > 0 {
		t.Fatalf("addToCart: %v", errs)
	}

	email := "gql-" + uuid.NewString()[:8] + "@e2e.test"
	addr := `{firstName: "E2E", lastName: "Guest", street1: "1 Test Way", city: "Testville", postalCode: "12345", countryCode: "US"}`
	for _, step := range []string{
		fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cartID),
		fmt.Sprintf(`setCartDetails(cartId: %q, input: {email: %q, shippingAddress: %s, billingSameAsShipping: true})`, cartID, email, addr),
		fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, cartID),
	} {
		if _, errs := h.gqlPost(t, ts, chID, `mutation { `+step+` { id } }`); len(errs) > 0 {
			t.Fatalf("%s: %v", step, errs)
		}
	}
	data, errs = h.gqlPost(t, ts, chID, fmt.Sprintf(
		`mutation { placeOrder(cartId: %q, acceptTerms: true) { id number status paymentStatus paymentProvider } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("placeOrder: %v", errs)
	}
	if got := gqlField(t, data, "placeOrder", "status"); got != "PENDING" {
		t.Errorf("wire status = %q, want PENDING", got)
	}
	if got := gqlField(t, data, "placeOrder", "paymentStatus"); got != "UNPAID" {
		t.Errorf("wire paymentStatus = %q, want UNPAID", got)
	}
	if got := gqlField(t, data, "placeOrder", "paymentProvider"); got != "cod" {
		t.Errorf("wire paymentProvider = %q, want cod", got)
	}
	if gqlField(t, data, "placeOrder", "number") == "" {
		t.Error("wire order number is empty")
	}

	// Wire truth must equal database truth - and the spoofed unit price must not have survived
	orderID, err := uuid.Parse(gqlField(t, data, "placeOrder", "id"))
	if err != nil {
		t.Fatalf("order id: %v", err)
	}
	row, err := h.D.Queries.GetOrderByID(h.Ctx, orderID)
	if err != nil {
		t.Fatalf("order row: %v", err)
	}
	if row.PaymentStatus != "unpaid" || row.Email != email {
		t.Errorf("db row = %q/%q, want unpaid/%s", row.PaymentStatus, row.Email, email)
	}
	lines := h.OrderLines(t, orderID)
	if len(lines) != 1 || lines[0].UnitPrice != 2300 {
		t.Errorf("order line price = %+v, want the listing's 2300, never the client's 1", lines)
	}
	if got := h.ReservedFor(t, f); got != 1 {
		t.Errorf("reserved = %v after the HTTP placement, want 1", got)
	}
	h.Settle(t)

	// A converted cart is spent - the same cart id can't place twice
	if _, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id } }`, cartID)); len(errs) == 0 {
		t.Fatal("a converted cart placed a second order")
	}
}
