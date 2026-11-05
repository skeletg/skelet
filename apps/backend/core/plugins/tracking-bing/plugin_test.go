package trackingbing

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

// Microsoft's own worked example from the Conversions API guide
func TestHashesMatchMicrosoftsWorkedExample(t *testing.T) {
	if got := hashEmail(" John@Contoso.com "); got != "ec81f3ac7b2b19675bab9d54cf416f9f18cff87c97da5cca82c0f0891bc40602" {
		t.Errorf("email hash %s", got)
	}
	if got := hashPhone("+1 (425) 555-1234"); got != "c59475d96e9f01d7d18d06cfad84dd02333207f02c0c2c5663ef2782cda0390e" {
		t.Errorf("phone hash %s", got)
	}
	if hashEmail("j.o.h.n+shop@contoso.com") != hashEmail("john@contoso.com") {
		t.Error("dots and a +alias in the local part must go, as Microsoft specifies")
	}
	if hashPhone("0014255551234") != hashPhone("+14255551234") || hashPhone("064 123 4567") != "" {
		t.Error("00 is an international prefix; a number with no country code can't be E.164 and isn't sent")
	}
}

func purchase(at time.Time) sdk.ShopEvent {
	cust := uuid.MustParse("0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b")
	return sdk.ShopEvent{
		ID: "0193a1b2-0000-7000-8000-000000000042", Name: "purchase", OccurredAt: at,
		Ecommerce: &sdk.ShopEcommerce{Currency: "EUR", Value: 45, Tax: 9, Shipping: 5, TransactionID: "ORD-000042",
			Items: []sdk.ShopItem{{ItemID: "SKU-1", ItemName: "Mug", Price: 15, Quantity: 2}, {ItemID: "SKU-2", ItemName: "Tee", Price: 20, Quantity: 1}}},
		User:     sdk.ShopUser{CustomerID: &cust, AnonID: "visitor-1", Email: "john@contoso.com", Phone: "+14255551234"},
		Context:  sdk.ShopContext{IP: "203.0.113.7", UserAgent: "Mozilla/5.0"},
		ClickIDs: map[string]string{"msclkid": "dd4afccc-b1c9-4a4c-ad95-44dd7e5006ab"},
		Consent:  &sdk.ShopConsent{Ads: true},
	}
}

func server(t *testing.T, status int, reply string) (*httptest.Server, *http.Request, *[]byte) {
	t.Helper()
	var got http.Request
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r
		raw, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	prev := capiHost
	capiHost = srv.URL
	t.Cleanup(func() { capiHost = prev })
	return srv, &got, &raw
}

// The request a purchase becomes, held to a golden file in the shape Microsoft documents
func TestAPurchaseIsTheConversionsAPIRequest(t *testing.T) {
	srv, req, raw := server(t, http.StatusOK, `{}`)
	at := time.Unix(1793527200, 0).UTC()
	ev := purchase(time.Now())
	if err := Send(context.Background(), srv.Client(), sdk.DestinationConfig{Config: map[string]string{"tag_id": "123456789"}, Secret: "tok"}, ev); err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/v1/123456789/events" || req.Header.Get("Authorization") != "Bearer tok" {
		t.Errorf("request %s, auth %q", req.URL, req.Header.Get("Authorization"))
	}
	var sent body
	if err := json.Unmarshal(*raw, &sent); err != nil {
		t.Fatal(err)
	}
	sent.Data[0].EventTime = at.Unix() // the golden file's time is fixed
	got, _ := json.MarshalIndent(sent, "", "  ")
	const golden = "testdata/purchase.request.json"
	if *update {
		_ = os.WriteFile(golden, append(got, '\n'), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil || strings.TrimSpace(string(want)) != string(got) {
		t.Errorf("request body drifted from %s (run with -update after checking it):\n%s", golden, got)
	}
}

// Microsoft's documented 401 and 400 answers are final - a retry would only repeat them
func TestMicrosoftsRefusalsAreFinal(t *testing.T) {
	for _, fixture := range []string{"unauthorized.json", "validation_error.json"} {
		reply, err := os.ReadFile("testdata/" + fixture)
		if err != nil {
			t.Fatal(err)
		}
		status := http.StatusBadRequest
		if fixture == "unauthorized.json" {
			status = http.StatusUnauthorized
		}
		srv, _, _ := server(t, status, string(reply))
		err = Send(context.Background(), srv.Client(), sdk.DestinationConfig{Config: map[string]string{"tag_id": "1"}, Secret: "tok"}, purchase(time.Now()))
		if !sdk.IsPermanent(err) || strings.Contains(err.Error(), "tok") {
			t.Errorf("%s: %v - want final, without the token", fixture, err)
		}
	}
}

func TestWhatIsNotSent(t *testing.T) {
	srv, req, _ := server(t, http.StatusOK, `{}`)
	d := sdk.DestinationConfig{Config: map[string]string{"tag_id": "1"}, Secret: "tok"}
	if err := Send(context.Background(), srv.Client(), d, sdk.ShopEvent{ID: "r", Name: "refund", OccurredAt: time.Now()}); err != nil || req.URL != nil {
		t.Errorf("a refund goes through Microsoft's restate API, not this one: %v", err)
	}
	if err := Send(context.Background(), srv.Client(), d, purchase(time.Now().AddDate(0, 0, -91))); !sdk.IsPermanent(err) || req.URL != nil {
		t.Errorf("an event older than 90 days: %v", err)
	}
	if err := Send(context.Background(), srv.Client(), d, sdk.ShopEvent{ID: "n", Name: "view_item", OccurredAt: time.Now()}); !sdk.IsPermanent(err) || req.URL != nil {
		t.Errorf("an event with no identifier at all: %v", err)
	}
	pv := sdk.ShopEvent{ID: "p", Name: "page_view", OccurredAt: time.Now(), User: sdk.ShopUser{AnonID: "v"}, Context: sdk.ShopContext{PageURL: "https://shop.example/"}}
	if e, _ := toEvent(pv); e.EventType != "pageLoad" || e.EventName != "" || e.EventSourceURL == "" {
		t.Errorf("a page view is Microsoft's pageLoad: %+v", e)
	}
}
