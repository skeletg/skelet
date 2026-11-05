package trackingattentive

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
		ID: "e-42", Name: "purchase", OccurredAt: time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC),
		Ecommerce: &sdk.ShopEcommerce{Currency: "USD", Value: 19.99, TransactionID: "ORDER-1001",
			Items: []sdk.ShopItem{{ItemID: "AB12345", ItemName: "T-Shirt", Price: 19.99, Quantity: 1}}},
		User: sdk.ShopUser{CustomerID: &cust, AnonID: "visitor-1", Email: "test@gmail.com", Phone: "+13115552368", MarketingOptIn: true},
	}
}

func server(t *testing.T) (*httptest.Server, *http.Request, *[]byte) {
	t.Helper()
	var got http.Request
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r
		raw, _ = io.ReadAll(r.Body)
	}))
	t.Cleanup(srv.Close)
	prev := apiHost
	apiHost = srv.URL
	t.Cleanup(func() { apiHost = prev })
	return srv, &got, &raw
}

// The request a purchase becomes, held to a golden file in the shape of Attentive's purchase example
func TestAPurchaseIsAttentivesPurchaseEvent(t *testing.T) {
	srv, req, raw := server(t)
	if err := Send(context.Background(), srv.Client(), sdk.DestinationConfig{Secret: "jwt"}, purchase()); err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/v1/events/ecommerce/purchase" || req.Header.Get("Authorization") != "Bearer jwt" {
		t.Errorf("request %s, auth %q", req.URL, req.Header.Get("Authorization"))
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
	if strings.Contains(string(*raw), "3115552368") {
		t.Error("a phone number went to Attentive - an SMS channel the shop holds no consent for")
	}
}

func TestEachEventGoesToItsEndpointAndAnUnconsentedEmailToNone(t *testing.T) {
	srv, req, _ := server(t)
	ev := purchase()
	ev.User.MarketingOptIn = false
	if err := Send(context.Background(), srv.Client(), sdk.DestinationConfig{Secret: "jwt"}, ev); err != nil || req.URL != nil {
		t.Fatalf("an email without marketing consent was sent: %v", err)
	}
	for name, path := range paths {
		ev := purchase()
		ev.Name, ev.User.Email = name, ""
		if err := Send(context.Background(), srv.Client(), sdk.DestinationConfig{Secret: "jwt"}, ev); err != nil || req.URL == nil || req.URL.Path != path {
			t.Errorf("%s went to %v (%v), want %s", name, req.URL, err, path)
		}
	}
}
