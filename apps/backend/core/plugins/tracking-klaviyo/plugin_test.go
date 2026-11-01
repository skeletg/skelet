package trackingklaviyo

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

var update = flag.Bool("update", false, "rewrite the golden request files")

func purchase() sdk.ShopEvent {
	cust := uuid.MustParse("0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b")
	return sdk.ShopEvent{
		ID: "0193a1b2-0000-7000-8000-000000000042", Name: "purchase", OccurredAt: time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC),
		Ecommerce: &sdk.ShopEcommerce{Currency: "EUR", Value: 45, Tax: 9, Shipping: 5, TransactionID: "ORD-000042",
			Items: []sdk.ShopItem{{ItemID: "SKU-1", ItemName: "Mug", Price: 15, Quantity: 2}, {ItemID: "SKU-2", ItemName: "Tee", Price: 20, Quantity: 1}}},
		User:    sdk.ShopUser{CustomerID: &cust, AnonID: "visitor-1", Email: "a@example.com", Phone: "+381601234567", MarketingOptIn: true},
		Consent: &sdk.ShopConsent{Personalization: true},
	}
}

func server(t *testing.T, status int) (*httptest.Server, *http.Request, *[]byte) {
	t.Helper()
	var got http.Request
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r
		raw, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	prev := apiHost
	apiHost = srv.URL
	t.Cleanup(func() { apiHost = prev })
	return srv, &got, &raw
}

// The request a purchase becomes, held to a golden file in the shape of Klaviyo's Create Event reference
func TestAPurchaseIsKlaviyosPlacedOrder(t *testing.T) {
	srv, req, raw := server(t, http.StatusAccepted)
	if err := Send(context.Background(), srv.Client(), sdk.DestinationConfig{Secret: "pk_test"}, purchase()); err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/api/events" || req.Header.Get("Authorization") != "Klaviyo-API-Key pk_test" || req.Header.Get("revision") != revision || req.Header.Get("Content-Type") != "application/vnd.api+json" {
		t.Errorf("request %s, headers %v", req.URL, req.Header)
	}
	var sent any
	_ = json.Unmarshal(*raw, &sent)
	got, _ := json.MarshalIndent(sent, "", "  ")
	const golden = "testdata/purchase.request.json"
	if *update {
		_ = os.WriteFile(golden, append(got, '\n'), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil || strings.TrimSpace(string(want)) != string(got) {
		t.Errorf("request body drifted from %s (run with -update after checking it):\n%s", golden, got)
	}
	if strings.Contains(string(*raw), "381601234567") {
		t.Error("a phone number went to Klaviyo, which treats it as an SMS channel needing its own consent")
	}
}

// The decision this plugin exists to hold: no marketing profile for an email that didn't opt in
func TestAnEmailThatDidNotOptInIsNeverSent(t *testing.T) {
	srv, req, _ := server(t, http.StatusAccepted)
	ev := purchase()
	ev.User.MarketingOptIn = false
	if err := Send(context.Background(), srv.Client(), sdk.DestinationConfig{Secret: "k"}, ev); err != nil || req.URL != nil {
		t.Errorf("an order for an email without marketing consent was sent: %v", err)
	}
	view := sdk.ShopEvent{ID: "v", Name: "view_item", OccurredAt: time.Now(), User: sdk.ShopUser{AnonID: "visitor-1"},
		Ecommerce: &sdk.ShopEcommerce{Currency: "EUR", Value: 15, Items: []sdk.ShopItem{{ItemID: "SKU-1", ItemName: "Mug", Price: 15, Quantity: 1}}}}
	if err := Send(context.Background(), srv.Client(), sdk.DestinationConfig{Secret: "k"}, view); err != nil || req.URL == nil {
		t.Fatalf("a browsing event under the visitor id must go: %v", err)
	}
	if b, _ := toBody("Viewed Product", view); b.Data.Attributes["profile"].(object).Data.Attributes["anonymous_id"] != "visitor-1" {
		t.Error("the browsing event must name the visitor")
	}
	if _, ok := toBody("Viewed Product", sdk.ShopEvent{ID: "x", Name: "view_item"}); ok {
		t.Error("an event with no identifier can't make a profile")
	}
}
