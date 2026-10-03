package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/backinstock"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalogio"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

func (h *harness) gqlAs(t *testing.T, ts *httptest.Server, channelID, query string, headers map[string]string) (map[string]any, []string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"query": query})
	req, err := http.NewRequest(http.MethodPost, ts.URL+frontAPIPath(t), bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(channel.HeaderChannelID, channelID)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Data   map[string]any `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var msgs []string
	for _, e := range out.Errors {
		msgs = append(msgs, e.Message)
	}
	return out.Data, msgs
}

func TestReviewsStayOnTheirChannel(t *testing.T) {
	h := newHarness(t)
	a := h.NewStore(t, 5, 1000)
	b := h.NewStore(t, 5, 1000)
	h.ListOn(t, b.Channel, a.Product.ID, 1000)
	for ch, title := range map[uuid.UUID]string{a.Channel.ID: "written on A", b.Channel.ID: "written on B"} {
		if _, err := h.D.DB.Exec(`INSERT INTO reviews (product_id, guest_name, channel_id, locale, title, status) VALUES ($1, 'Guest', $2, 'en', $3, 'approved')`,
			a.Product.ID, ch, title); err != nil {
			t.Fatalf("seed review: %v", err)
		}
	}
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	data, errs := h.gqlAs(t, ts, b.Channel.ID.String(), fmt.Sprintf(`{ product(id: %q) { reviews { title } } }`, a.Product.ID), nil)
	if len(errs) > 0 {
		t.Fatalf("product: %v", errs)
	}
	raw, _ := json.Marshal(data)
	if !strings.Contains(string(raw), "written on B") || strings.Contains(string(raw), "written on A") {
		t.Fatalf("channel B shows %s, want only its own review", raw)
	}
}

// Scenario: a gift card issued for one channel spends and reads only there, and one minted against an order belongs to the order's channel
func TestAGiftCardSpendsOnlyOnItsChannel(t *testing.T) {
	h := newHarness(t)
	a := h.NewStore(t, 5, 1000)
	b := h.NewStore(t, 5, 1000)
	card, err := h.D.Giftcard.Issue(h.Ctx, giftcard.IssueInput{CurrencyCode: "USD", Amount: 5000, ChannelID: &a.Channel.ID})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	q := fmt.Sprintf(`{ giftCardBalance(code: %q) { balanceCents } }`, card.Code)
	if data, _ := h.gqlAs(t, ts, b.Channel.ID.String(), q, nil); data["giftCardBalance"] != nil {
		t.Fatalf("channel B read channel A's card: %v", data)
	}
	if data, _ := h.gqlAs(t, ts, a.Channel.ID.String(), q, nil); data["giftCardBalance"] == nil {
		t.Fatal("channel A could not read its own card")
	}
	cartB := h.NewGuestCart(t, b, 1)
	if _, err := h.TryPlace(t, b.Channel, cartB, func(in *order.PlaceOrderInput) { in.GiftCardCodes = []string{card.Code} }); !errors.Is(err, giftcard.ErrWrongChannel) {
		t.Fatalf("placing on B with A's card = %v, want ErrWrongChannel", err)
	}
	cartA := h.NewGuestCart(t, a, 1)
	o, err := h.TryPlace(t, a.Channel, cartA, func(in *order.PlaceOrderInput) { in.GiftCardCodes = []string{card.Code} })
	if err != nil || o.GiftCardAmount != 1000 {
		t.Fatalf("placing on A with its own card = %+v, %v", o.GiftCardAmount, err)
	}
	refund, err := h.D.Giftcard.Issue(h.Ctx, giftcard.IssueInput{CurrencyCode: "USD", Amount: 100, IssuedOrderID: &o.ID})
	if err != nil {
		t.Fatalf("issue against order: %v", err)
	}
	if refund.ChannelID == nil || *refund.ChannelID != a.Channel.ID {
		t.Fatalf("a card minted against A's order has channel %v, want A", refund.ChannelID)
	}
}

func TestCartsAndGuestOrdersStayOnTheirStore(t *testing.T) {
	h := newHarness(t)
	a := h.NewStore(t, 5, 1000)
	b := h.NewStore(t, 5, 1000)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	cart := h.NewGuestCart(t, a, 1)
	if _, errs := h.gqlAs(t, ts, b.Channel.ID.String(), fmt.Sprintf(`{ cartEstimate(cartId: %q, countryCode: "US") { subtotalCents } }`, cart), nil); len(errs) == 0 {
		t.Fatal("channel B priced channel A's cart")
	}
	email := "iso-" + uuid.NewString()[:8] + "@e2e.test"
	o, err := h.TryPlace(t, a.Channel, h.NewGuestCart(t, a, 1), func(in *order.PlaceOrderInput) { in.Email = email })
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	q := fmt.Sprintf(`{ guestOrder(id: %q, email: %q) { id } }`, o.ID, email)
	if data, _ := h.gqlAs(t, ts, b.Channel.ID.String(), q, nil); data["guestOrder"] != nil {
		t.Fatal("channel B showed channel A's guest order")
	}
	if data, _ := h.gqlAs(t, ts, a.Channel.ID.String(), q, nil); data["guestOrder"] == nil {
		t.Fatal("channel A lost its own guest order")
	}
	if _, errs := h.gqlAs(t, ts, b.Channel.ID.String(), fmt.Sprintf(`mutation { finalizeOrderPayment(orderId: %q) }`, o.ID), nil); len(errs) == 0 {
		t.Fatal("channel B finalized channel A's order")
	}
}

func TestStockIsNotReadableForAnotherStore(t *testing.T) {
	h := newHarness(t)
	a := h.NewStore(t, 5, 1000)
	b := h.NewStore(t, 5, 1000)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	if _, errs := h.gqlAs(t, ts, a.Channel.ID.String(), fmt.Sprintf(`{ product(id: %q) { inStock(channelId: %q) } }`, a.Product.ID, b.Channel.ID), nil); len(errs) == 0 {
		t.Fatal("channel A read channel B's stock")
	}
	if _, errs := h.gqlAs(t, ts, a.Channel.ID.String(), fmt.Sprintf(`{ product(id: %q) { inStock(channelId: %q) } }`, a.Product.ID, a.Channel.ID), nil); len(errs) > 0 {
		t.Fatalf("own stock: %v", errs)
	}
}

func TestARefreshTokenOnTheWrongStoreIsNotSpent(t *testing.T) {
	h := newHarness(t)
	a := h.NewStore(t, 5, 1000)
	b := h.NewStore(t, 5, 1000)
	email := "rt-" + uuid.NewString()[:8] + "@e2e.test"
	c, err := h.D.Customer.Register(h.Ctx, a.Channel.ID, email, "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if tok, terr := h.D.Verify.Create(h.Ctx, c.ID); terr == nil {
		_ = h.D.Customer.VerifyEmail(h.Ctx, tok)
	}
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	data, errs := h.gqlAs(t, ts, a.Channel.ID.String(), fmt.Sprintf(`mutation { customerLogin(input: {email: %q, password: "password123"}) { refreshToken } }`, email), nil)
	if len(errs) > 0 {
		t.Fatalf("login: %v", errs)
	}
	token := gqlField(t, data, "customerLogin", "refreshToken")
	refresh := fmt.Sprintf(`mutation { customerRefreshToken(token: %q) { refreshToken } }`, token)
	if _, errs := h.gqlAs(t, ts, b.Channel.ID.String(), refresh, nil); len(errs) == 0 {
		t.Fatal("channel B rotated channel A's refresh token")
	}
	sum := sha256.Sum256([]byte(token))
	var rotated bool
	if err := h.D.DB.QueryRow(`SELECT rotated_at IS NOT NULL OR successor_token IS NOT NULL FROM auth_refresh_tokens WHERE token_hash = $1`, hex.EncodeToString(sum[:])).Scan(&rotated); err != nil {
		t.Fatalf("read token row: %v", err)
	}
	if rotated {
		t.Fatal("the wrong-store attempt rotated the token, so its owner's next refresh reads as reuse")
	}
	if _, errs := h.gqlAs(t, ts, a.Channel.ID.String(), refresh, nil); len(errs) > 0 {
		t.Fatalf("the wrong-store attempt spent the token: %v", errs)
	}
}

// Scenario: a back-in-stock request is only for a product the store sells, a bounded number per address, and the notice is built from the catalog
func TestBackInStockTakesOnlyTheProductAndTheAddress(t *testing.T) {
	h := newHarness(t)
	a := h.NewStore(t, 0, 1000)
	b := h.NewStore(t, 0, 1000)
	addr := "bis-" + uuid.NewString()[:8] + "@e2e.test"
	if err := h.D.BackInStock.Record(h.Ctx, backinstock.Request{ChannelID: b.Channel.ID, ProductID: a.Product.ID, Email: addr}); !errors.Is(err, backinstock.ErrNotSoldHere) {
		t.Fatalf("a request for a product B doesn't sell = %v", err)
	}
	if err := h.D.BackInStock.Record(h.Ctx, backinstock.Request{ChannelID: a.Channel.ID, ProductID: a.Product.ID, Email: addr}); err != nil {
		t.Fatalf("record: %v", err)
	}
	var sku string
	if err := h.D.DB.QueryRow(`SELECT sku FROM back_in_stock_requests WHERE channel_id = $1 AND email = $2`, a.Channel.ID, addr).Scan(&sku); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if sku != a.Product.SKU {
		t.Fatalf("stored sku %q, want the catalog's sku", sku)
	}
	if _, err := h.D.Inventory.AdjustLevel(h.Ctx, inventory.AdjustLevelInput{InventoryItemID: a.Item.ID, LocationID: a.Location.ID, NewQuantity: 5}); err != nil {
		t.Fatalf("restock: %v", err)
	}
	h.Settle(t)
	sent := h.Mail.To(addr)
	if len(sent) != 1 || !strings.Contains(sent[0].Subject, "E2E Widget") {
		t.Fatalf("restock notices = %+v, want one naming the catalog's product", sent)
	}
	var cappedErr error
	for range 25 {
		p := h.AddProduct(t, a, 1000, true, 0)
		if cappedErr = h.D.BackInStock.Record(h.Ctx, backinstock.Request{ChannelID: a.Channel.ID, ProductID: p.ID, Email: addr}); cappedErr != nil {
			break
		}
	}
	if !errors.Is(cappedErr, backinstock.ErrTooManyRequested) {
		t.Fatalf("one address queued unbounded notices: %v", cappedErr)
	}
}

// Scenario: the merchant feed reports what this channel's locations can sell, and links by the channel's own URL
func TestTheMerchantFeedIsChannelTrue(t *testing.T) {
	h := newHarness(t)
	a := h.NewStore(t, 0, 1000)
	if _, err := h.D.DB.Exec(`UPDATE inventory_items SET quantity = 7 WHERE product_id = $1`, a.Product.ID); err != nil {
		t.Fatalf("stale flat quantity: %v", err)
	}
	path := "a-only-" + uuid.NewString()[:8]
	if _, err := h.D.DB.Exec(`INSERT INTO url_rewrites (entity_type, entity_id, request_path, redirect_type, channel_id) VALUES ('product', $1, $2, 0, $3)`,
		a.Product.ID, path, a.Channel.ID); err != nil {
		t.Fatalf("channel rewrite: %v", err)
	}
	out, _, err := catalogio.NewMerchantFeed(h.D.Queries, nil, h.D.Inventory, h.D.Catalog).Render(context.Background(), catalogio.MerchantFeedOptions{
		ChannelID: a.Channel.ID, BaseURL: "https://a.test", CurrencyCode: "USD", Locale: "en", Title: "A",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	feed := string(out)
	item := ""
	for chunk := range strings.SplitSeq(feed, "<item>") {
		if strings.Contains(chunk, a.Product.SKU) {
			item = chunk
		}
	}
	if !strings.Contains(item, "out_of_stock") {
		t.Errorf("the feed reported stock the channel's locations don't hold:\n%s", item)
	}
	if !strings.Contains(item, "https://a.test/"+path) {
		t.Errorf("the feed linked past the channel's own URL")
	}
}
