package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	authpkg "github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Reorder lives in the front resolver rather than a service, so these drive the real GraphQL surface - the first authenticated HTTP scenarios in the suite

func e2eJWTSecret() string { // mirrors e2eConfig, which is what the running graph was built with
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return s
	}
	return "e2e-test-secret"
}

func (h *harness) customerToken(t *testing.T, customerID, channelID uuid.UUID, email string) string {
	t.Helper()
	tok, err := authpkg.GenerateCustomerToken(customerID, channelID, email, e2eJWTSecret(), time.Hour)
	if err != nil {
		t.Fatalf("mint customer token: %v", err)
	}
	return tok
}

// gqlPostAs is gqlPost carrying a customer bearer, so resolvers behind RequireCustomerForCurrentStore run
func (h *harness) gqlPostAs(t *testing.T, ts *httptest.Server, channelID, token, query string) (map[string]any, []string) {
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
	req.Header.Set("X-Channel-Id", channelID)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("post graphql: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
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

const reorderQuery = `mutation { reorder(orderId: %q) {
	added
	skipped { productSku reason }
	cart { id lines { productSku quantity unitPrice } }
} }`

type reorderOutcome struct {
	added   int
	skipped []string
	lines   map[string]float64
}

func (h *harness) reorder(t *testing.T, ts *httptest.Server, ch uuid.UUID, token string, orderID uuid.UUID) reorderOutcome {
	t.Helper()
	data, errs := h.gqlPostAs(t, ts, ch.String(), token, fmt.Sprintf(reorderQuery, orderID.String()))
	if len(errs) > 0 {
		t.Fatalf("reorder returned errors: %v", errs)
	}
	res, ok := data["reorder"].(map[string]any)
	if !ok {
		t.Fatalf("response has no reorder object: %v", data)
	}
	out := reorderOutcome{lines: map[string]float64{}}
	if n, ok := res["added"].(float64); ok {
		out.added = int(n)
	}
	if sk, ok := res["skipped"].([]any); ok {
		for _, s := range sk {
			if m, ok := s.(map[string]any); ok {
				out.skipped = append(out.skipped, fmt.Sprint(m["productSku"]))
			}
		}
	}
	if c, ok := res["cart"].(map[string]any); ok {
		if lines, ok := c["lines"].([]any); ok {
			for _, l := range lines {
				if m, ok := l.(map[string]any); ok {
					qty, _ := m["quantity"].(float64)
					out.lines[fmt.Sprint(m["productSku"])] = qty
				}
			}
		}
	}
	return out
}

func (h *harness) registeredBuyer(t *testing.T, f *store) (uuid.UUID, string, string) {
	t.Helper()
	email := "reorder-" + uuid.NewString()[:8] + "@e2e.test"
	c, err := h.D.Customer.Register(h.Ctx, f.Channel.ID, email, "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return c.ID, email, h.customerToken(t, c.ID, f.Channel.ID, email)
}

// Scenario: one-click reorder puts the order's lines back in the customer's cart, at CURRENT catalog prices rather than the order's snapshot
func TestReorderRebuildsTheCartAtCurrentPrices(t *testing.T) {
	h := newHarness(t)
	ts := httptest.NewServer(h.D.Root)
	defer ts.Close()
	f := h.NewStore(t, 50, 2000)
	buyer, email, token := h.registeredBuyer(t, f)

	o := h.placeFor(t, f, buyer, email, 2)
	h.Settle(t)

	// the catalog price moves after the order was placed
	newPrice := int64(3000)
	if _, err := h.D.Catalog.SetProductListing(h.Ctx, catalog.ProductChannelListingInput{
		ProductID: f.Product.ID, ChannelID: f.Channel.ID, Price: &newPrice, IsPublished: true,
	}); err != nil {
		t.Fatalf("reprice: %v", err)
	}

	got := h.reorder(t, ts, f.Channel.ID, token, o.ID)
	if got.added != 1 {
		t.Errorf("added = %d, want the one line re-added (skipped %v)", got.added, got.skipped)
	}
	if got.lines[f.Product.SKU] != 2 {
		t.Errorf("cart lines = %v, want the SKU at quantity 2", got.lines)
	}

	// the cart re-resolves pricing, so it must carry the NEW price and not the order's 2000
	c, err := h.D.Cart.GetOrCreateCart(h.Ctx, buyer, f.Channel.ID)
	if err != nil {
		t.Fatalf("read cart: %v", err)
	}
	if len(c.Lines) != 1 {
		t.Fatalf("%d cart lines, want 1", len(c.Lines))
	}
	if c.Lines[0].UnitPrice != newPrice {
		t.Errorf("cart unit price = %d, want the current %d rather than the order's snapshot", c.Lines[0].UnitPrice, newPrice)
	}
	h.Settle(t)
}

// A line that no longer validates is reported in `skipped` and the REST still land - a reorder that aborts on the first bad line strands the whole basket
func TestReorderSkipsUnavailableLinesAndKeepsTheRest(t *testing.T) {
	h := newHarness(t)
	ts := httptest.NewServer(h.D.Root)
	defer ts.Close()
	f := h.NewStore(t, 50, 2000)
	gone := h.AddProduct(t, f, 1500, true, 10)
	buyer, email, token := h.registeredBuyer(t, f)

	c, err := h.D.Cart.GetOrCreateCart(h.Ctx, buyer, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	for _, sku := range []string{f.Product.SKU, gone.SKU} {
		if _, err := h.D.Cart.AddToCart(h.Ctx, c.ID, &buyer, cart.AddCartLineInput{ProductSku: sku, Quantity: 1}); err != nil {
			t.Fatalf("add %s: %v", sku, err)
		}
	}
	o, err := h.TryPlace(t, f.Channel, c.ID, func(in *order.PlaceOrderInput) {
		in.CustomerID = &buyer
		in.Email = email
	})
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.Settle(t)

	// unpublish one product, so its line can no longer be re-added
	zero := int64(1500)
	if _, err := h.D.Catalog.SetProductListing(h.Ctx, catalog.ProductChannelListingInput{
		ProductID: gone.ID, ChannelID: f.Channel.ID, Price: &zero, IsPublished: false,
	}); err != nil {
		t.Fatalf("unpublish: %v", err)
	}

	got := h.reorder(t, ts, f.Channel.ID, token, o.ID)
	if got.added != 1 {
		t.Errorf("added = %d, want the one still-available line", got.added)
	}
	if len(got.skipped) != 1 || got.skipped[0] != gone.SKU {
		t.Errorf("skipped = %v, want just the unpublished %s", got.skipped, gone.SKU)
	}
	if got.lines[f.Product.SKU] == 0 {
		t.Errorf("cart lines = %v, want the available SKU present despite the other being skipped", got.lines)
	}
	if _, present := got.lines[gone.SKU]; present {
		t.Errorf("cart lines = %v, want the unavailable SKU absent", got.lines)
	}
	h.Settle(t)
}

// The resolver branches on len(line.Components), which a re-read order only carries because hydrateOrder attaches them - without that a bundle order reorders down the simple branch, which the cart refuses, so the bundle silently lands in `skipped`
func TestReorderRebuildsABundleRatherThanSkippingIt(t *testing.T) {
	h := newHarness(t)
	ts := httptest.NewServer(h.D.Root)
	defer ts.Close()
	f := h.NewStore(t, 50, 2000)
	comp := h.AddProduct(t, f, 1000, true, 30)
	b, opt, sel := h.newBundleProduct(t, f, comp, 2)
	buyer, email, token := h.registeredBuyer(t, f)

	c, err := h.D.Cart.GetOrCreateCart(h.Ctx, buyer, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	if _, err := h.D.Cart.AddBundleToCart(h.Ctx, c.ID, &buyer, cart.AddBundleToCartInput{
		ProductSku: b.SKU, Quantity: 1,
		Selections: []cart.AddBundleSelectionInput{{OptionID: opt.ID, SelectionIDs: []uuid.UUID{sel.ID}}},
	}); err != nil {
		t.Fatalf("add bundle: %v", err)
	}
	o, err := h.TryPlace(t, f.Channel, c.ID, func(in *order.PlaceOrderInput) {
		in.CustomerID = &buyer
		in.Email = email
	})
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.Settle(t)

	got := h.reorder(t, ts, f.Channel.ID, token, o.ID)
	if len(got.skipped) != 0 {
		t.Errorf("skipped = %v, want the bundle rebuilt rather than skipped", got.skipped)
	}
	if got.added != 1 {
		t.Errorf("added = %d, want the bundle line re-added", got.added)
	}
	// and it comes back as a real bundle, with its component rows - the placed cart was converted, so the reorder built a fresh one
	reread, err := h.D.Cart.GetOrCreateCart(h.Ctx, buyer, f.Channel.ID)
	if err != nil {
		t.Fatalf("read cart: %v", err)
	}
	var bundleLine *cart.CartLine
	for i := range reread.Lines {
		if reread.Lines[i].ProductSku == b.SKU {
			bundleLine = &reread.Lines[i]
		}
	}
	if bundleLine == nil {
		t.Fatalf("no bundle line in the reordered cart: %+v", reread.Lines)
	}
	if len(bundleLine.Components) == 0 {
		t.Error("the reordered bundle line carries no components, so it is not a real bundle")
	}
	h.Settle(t)
}

// A configured line rebuilds its option picks, so the personalization the customer paid for comes back rather than a bare product
func TestReorderRebuildsConfiguredOptions(t *testing.T) {
	h := newHarness(t)
	ts := httptest.NewServer(h.D.Root)
	defer ts.Close()
	f := h.NewStore(t, 50, 2000)
	opt, values := h.addSelectOption(t, f.Product.ID, "Mounting", true, 500)
	buyer, email, token := h.registeredBuyer(t, f)

	c, err := h.D.Cart.GetOrCreateCart(h.Ctx, buyer, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	if _, err := h.D.Cart.AddConfiguredToCart(h.Ctx, c.ID, &buyer, cart.AddConfiguredToCartInput{
		ProductSku: f.Product.SKU, Quantity: 1,
		Options: []cart.CartLineOptionInput{{OptionID: opt.ID, ValueIDs: []uuid.UUID{values[0].ID}}},
	}); err != nil {
		t.Fatalf("add configured: %v", err)
	}
	o, err := h.TryPlace(t, f.Channel, c.ID, func(in *order.PlaceOrderInput) {
		in.CustomerID = &buyer
		in.Email = email
	})
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.Settle(t)

	got := h.reorder(t, ts, f.Channel.ID, token, o.ID)
	if len(got.skipped) != 0 || got.added != 1 {
		t.Fatalf("added %d, skipped %v - want the configured line rebuilt", got.added, got.skipped)
	}

	reread, err := h.D.Cart.GetOrCreateCart(h.Ctx, buyer, f.Channel.ID)
	if err != nil {
		t.Fatalf("read cart: %v", err)
	}
	if len(reread.Lines) != 1 {
		t.Fatalf("%d lines, want 1", len(reread.Lines))
	}
	if len(reread.Lines[0].Options) != 1 {
		t.Fatalf("the reordered line carries %d options, want the pick rebuilt", len(reread.Lines[0].Options))
	}
	if reread.Lines[0].Options[0].ProductOptionID != opt.ID {
		t.Errorf("rebuilt option = %v, want %v", reread.Lines[0].Options[0].ProductOptionID, opt.ID)
	}
	// the delta rides along, so the reordered line costs what the configured one did
	if reread.Lines[0].UnitPrice != 2500 {
		t.Errorf("unit price = %d, want the base 2000 plus the 500 delta", reread.Lines[0].UnitPrice)
	}
	h.Settle(t)
}

// Reorder reads someone's order history, so it must refuse an anonymous caller and another customer's order
func TestReorderRefusesAnonymousAndForeignOrders(t *testing.T) {
	h := newHarness(t)
	ts := httptest.NewServer(h.D.Root)
	defer ts.Close()
	f := h.NewStore(t, 50, 2000)
	owner, ownerEmail, ownerToken := h.registeredBuyer(t, f)
	_, _, strangerToken := h.registeredBuyer(t, f)

	o := h.placeFor(t, f, owner, ownerEmail, 1)
	h.Settle(t)

	// anonymous
	_, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(reorderQuery, o.ID.String()))
	if len(errs) == 0 {
		t.Error("an anonymous reorder succeeded, want it refused")
	}

	// another customer's order
	_, errs = h.gqlPostAs(t, ts, f.Channel.ID.String(), strangerToken, fmt.Sprintf(reorderQuery, o.ID.String()))
	if len(errs) == 0 {
		t.Error("a stranger reordered someone else's order")
	}

	// and the owner still can
	got := h.reorder(t, ts, f.Channel.ID, ownerToken, o.ID)
	if got.added != 1 {
		t.Errorf("the owner's reorder added %d, want 1", got.added)
	}
	h.Settle(t)
}

// A malformed id is a client error, not a 500
func TestReorderRejectsAMalformedOrderID(t *testing.T) {
	h := newHarness(t)
	ts := httptest.NewServer(h.D.Root)
	defer ts.Close()
	f := h.NewStore(t, 50, 2000)
	_, _, token := h.registeredBuyer(t, f)

	_, errs := h.gqlPostAs(t, ts, f.Channel.ID.String(), token, fmt.Sprintf(reorderQuery, "not-a-uuid"))
	if len(errs) == 0 {
		t.Error("a malformed order id was accepted")
	}
	h.Settle(t)
}
