package trackingga4

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
		ChannelID: uuid.MustParse("0193a1b2-0000-7000-8000-00000000c4a1"),
		Ecommerce: &sdk.ShopEcommerce{
			Currency: "EUR", Value: 45, TransactionID: "ORD-000042", Tax: 9, Shipping: 4.9,
			Items: []sdk.ShopItem{{ItemID: "SKU-1", ItemName: "Mug", Price: 15, Quantity: 2}, {ItemID: "SKU-2", ItemName: "Tee", Price: 20, Quantity: 1}},
		},
		User:     sdk.ShopUser{CustomerID: &cust, AnonID: "visitor-1", Email: "a@example.com"},
		Context:  sdk.ShopContext{IP: "203.0.113.7", PageURL: "https://shop.example/checkout", PageTitle: "Checkout", SessionID: "1793526000"},
		ClickIDs: map[string]string{"gclid": "g1"},
		Consent:  &sdk.ShopConsent{Analytics: true, Ads: true},
		Source:   "server",
	}
}

func capture(t *testing.T, reply string) (*httptest.Server, *http.Request, *[]byte) {
	t.Helper()
	var got http.Request
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r
		body, _ = io.ReadAll(r.Body)
		if reply != "" {
			_, _ = w.Write([]byte(reply))
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &got, &body
}

// The request a purchase becomes, held to a golden file - the shape Google documents for the Measurement Protocol
func TestAPurchaseIsTheMeasurementProtocolRequest(t *testing.T) {
	srv, req, body := capture(t, "")
	d := sdk.DestinationConfig{Config: map[string]string{"measurement_id": "G-TEST123", "endpoint": srv.URL}, Secret: "s3cret"}
	if err := Send(context.Background(), srv.Client(), d, purchase()); err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/mp/collect" || req.URL.Query().Get("measurement_id") != "G-TEST123" || req.URL.Query().Get("api_secret") != "s3cret" {
		t.Errorf("request %s", req.URL)
	}
	var pretty map[string]any
	if err := json.Unmarshal(*body, &pretty); err != nil {
		t.Fatal(err)
	}
	got, _ := json.MarshalIndent(pretty, "", "  ")
	const golden = "testdata/purchase.request.json"
	if *update {
		_ = os.WriteFile(golden, append(got, '\n'), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil || strings.TrimSpace(string(want)) != string(got) {
		t.Errorf("request body drifted from %s (run with -update after checking it):\n%s", golden, got)
	}
	if strings.Contains(string(*body), "a@example.com") {
		t.Error("an email went to Google Analytics, which forbids personal data in events")
	}
}

func TestConsentAndTheClientIDFollowTheVisitor(t *testing.T) {
	ev := purchase()
	ev.Consent = &sdk.ShopConsent{Analytics: true}
	ev.User.AnonID, ev.Context.IP = "", ""
	p := toPayload(ev)
	if p.Consent == nil || p.Consent.AdUserData != "DENIED" || p.Consent.AdPersonalization != "DENIED" {
		t.Errorf("consent %+v, want ads and personalization denied", p.Consent)
	}
	if p.ClientID != ev.ID || p.IPOverride != "" {
		t.Errorf("with no visitor the event id stands in as client id: %q, ip %q", p.ClientID, p.IPOverride)
	}
	ev.Consent = nil
	if toPayload(ev).Consent != nil {
		t.Error("an event with no banner behind it must leave GA's consent to its own defaults")
	}
}

// Google's documented validation answer, from the validating-events guide
func TestTheValidationServerRefusalIsFinal(t *testing.T) {
	fixture, err := os.ReadFile("testdata/validation_invalid.json")
	if err != nil {
		t.Fatal(err)
	}
	srv, req, _ := capture(t, string(fixture))
	d := sdk.DestinationConfig{Config: map[string]string{"measurement_id": "G-TEST123", "endpoint": srv.URL + "/", "validate": "true"}, Secret: "s"}
	err = Send(context.Background(), srv.Client(), d, purchase())
	if req.URL.Path != "/debug/mp/collect" || !sdk.IsPermanent(err) || !strings.Contains(err.Error(), "NAME_INVALID") {
		t.Errorf("path %s, err %v - want the validation server's refusal, final", req.URL.Path, err)
	}
	srv2, _, _ := capture(t, `{"validationMessages": []}`)
	d.Config["endpoint"] = srv2.URL
	if err := Send(context.Background(), srv2.Client(), d, purchase()); err != nil {
		t.Errorf("a clean validation must pass: %v", err)
	}
}

// GA4's list, promotion, checkout-choice and search parameters reach the event, and an empty one is left out
func TestEventParametersBeyondThePurchase(t *testing.T) {
	idx := 2
	ev := sdk.ShopEvent{ID: "e-1", Name: "select_promotion", SearchTerm: "", Ecommerce: &sdk.ShopEcommerce{
		Currency: "EUR", PromotionID: "rule-1", PromotionName: "summer", CreativeSlot: "hero", ItemListName: "Mugs",
		Items: []sdk.ShopItem{{ItemID: "SKU-1", Index: &idx}},
	}}
	params := toPayload(ev).Events[0].Params
	if params["promotion_id"] != "rule-1" || params["promotion_name"] != "summer" || params["creative_slot"] != "hero" || params["item_list_name"] != "Mugs" {
		t.Errorf("params %v", params)
	}
	for _, k := range []string{"creative_name", "payment_type", "shipping_tier", "item_list_id", "search_term"} {
		if _, ok := params[k]; ok {
			t.Errorf("an empty %s was sent", k)
		}
	}
	if p := toPayload(sdk.ShopEvent{ID: "e-2", Name: "search", SearchTerm: "blue mug"}).Events[0].Params; p["search_term"] != "blue mug" {
		t.Errorf("search params %v", p)
	}
}
