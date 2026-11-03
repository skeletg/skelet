package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
)

// mcpCall drives the real storefront MCP endpoint the way a hosted agent does - one stateless POST per tool call
func mcpCall(t *testing.T, ts *httptest.Server, channelID, bearer, tool string, args any) (string, bool) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
	req, err := http.NewRequest(http.MethodPost, ts.URL+strings.TrimSuffix(frontAPIPath(t), "/graphql")+"/mcp", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	req.Header.Set(channel.HeaderChannelID, channelID)
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || len(out.Result.Content) == 0 {
		t.Fatalf("%s: http %d, %v", tool, resp.StatusCode, err)
	}
	return out.Result.Content[0].Text, out.Result.IsError
}

var (
	cartIDInText = regexp.MustCompile(`cart_id ([0-9a-f-]{36})`)
	handoffLink  = regexp.MustCompile(`https://shop-[0-9a-f]{8}\.e2e\.test/handoff/([A-Za-z0-9_-]{22})`)
)

// Scenario: a shopper's AI agent builds a cart over the storefront MCP endpoint and hands it off - the endpoint stays anonymous
// even when a customer's token comes along, the agent cart answers nothing on the front API, and the link lands the lines in
// the shopper's own cart, a guest's or a signed-in customer's
func TestAnAgentBuildsACartAndHandsItToTheShopper(t *testing.T) {
	h := newHarness(t)
	f := h.NewStoreOpt(t, 10, 2300, func(in *channel.CreateChannelInput) {
		in.PublicURL = "https://shop-" + uuid.NewString()[:8] + ".e2e.test"
	})
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	customerID, _, token := h.registeredBuyer(t, f)

	if out, isErr := mcpCall(t, ts, chID, token, "get_product", map[string]any{"product": f.Product.SKU}); isErr || !strings.Contains(out, "Put it in the cart with sku "+f.Product.SKU) {
		t.Fatalf("get_product = %v %q", isErr, out)
	}
	out, isErr := mcpCall(t, ts, chID, token, "update_cart", map[string]any{"lines": []map[string]any{{"sku": f.Product.SKU, "quantity": 2}}})
	m := cartIDInText.FindStringSubmatch(out)
	if isErr || m == nil || !strings.Contains(out, "2 x USD 23.00") {
		t.Fatalf("update_cart = %v %q", isErr, out)
	}
	agentID := uuid.MustParse(m[1])
	agent, err := h.D.Cart.GetCartByIDReadOnly(h.Ctx, agentID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Source != cart.SourceAgent || agent.CustomerID != nil {
		t.Fatalf("agent cart = source %q customer %v, want an anonymous agent cart whatever token came along", agent.Source, agent.CustomerID)
	}
	if data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`query { cart(id: %q) { id } }`, agentID)); len(errs) == 0 && data["cart"] != nil {
		t.Fatalf("the front API read an agent cart: %v", data)
	}

	out, isErr = mcpCall(t, ts, chID, token, "start_checkout", map[string]any{"cart_id": agentID})
	link := handoffLink.FindStringSubmatch(out)
	if isErr || link == nil {
		t.Fatalf("start_checkout = %v %q", isErr, out)
	}
	if out, isErr := mcpCall(t, ts, chID, token, "update_cart", map[string]any{"cart_id": agentID, "lines": []map[string]any{{"sku": f.Product.SKU, "quantity": 5}}}); !isErr || !strings.Contains(out, "went to the shopper") {
		t.Fatalf("a handed-off cart changed: %q", out)
	}

	redeem := fmt.Sprintf(`mutation { redeemCartHandoff(token: %q) { guestCart cart { id lines { sku quantity } } } }`, link[1])
	data, errs := h.gqlPost(t, ts, chID, redeem)
	if len(errs) > 0 {
		t.Fatalf("guest redeem: %v", errs)
	}
	got := data["redeemCartHandoff"].(map[string]any)
	lines := got["cart"].(map[string]any)["lines"].([]any)
	if got["guestCart"] != true || len(lines) != 1 || lines[0].(map[string]any)["quantity"] != 2.0 {
		t.Fatalf("guest redeem = %v", got)
	}

	data, errs = h.gqlPostAs(t, ts, chID, token, redeem)
	if len(errs) > 0 {
		t.Fatalf("customer redeem: %v", errs)
	}
	own, err := h.D.Cart.GetOrCreateCart(h.Ctx, customerID, f.Channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	got = data["redeemCartHandoff"].(map[string]any)
	if got["guestCart"] != false || got["cart"].(map[string]any)["id"] != own.ID.String() {
		t.Fatalf("customer redeem = %v, want their own cart %s", got, own.ID)
	}
}
