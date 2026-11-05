package trackingmeta

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

func purchase(at time.Time) sdk.ShopEvent {
	cust := uuid.MustParse("0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b")
	return sdk.ShopEvent{
		ID: "0193a1b2-0000-7000-8000-000000000042", Name: "purchase", OccurredAt: at,
		Ecommerce: &sdk.ShopEcommerce{
			Currency: "EUR", Value: 45, TransactionID: "ORD-000042", Tax: 9, Shipping: 5,
			Items: []sdk.ShopItem{{ItemID: "SKU-1", Price: 15, Quantity: 2}, {ItemID: "SKU-2", Price: 20, Quantity: 1}},
		},
		User:     sdk.ShopUser{CustomerID: &cust, Email: "  Joe@Example.com ", Phone: "+1 (650) 555-1212"},
		Context:  sdk.ShopContext{IP: "203.0.113.7", UserAgent: "Mozilla/5.0", PageURL: "https://shop.example/checkout"},
		ClickIDs: map[string]string{"fbc": "fb.1.1700000000000.IwAR", "fbp": "fb.1.1700000000000.123", "gclid": "g1"},
		Consent:  &sdk.ShopConsent{Ads: true},
	}
}

func server(t *testing.T, status int) (*httptest.Server, *http.Request, *[]byte) {
	t.Helper()
	var got http.Request
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, body = *r, nil
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	prev := graphHost
	graphHost = srv.URL
	t.Cleanup(func() { graphHost = prev })
	return srv, &got, &body
}

// The request a purchase becomes, held to a golden file in the shape Meta documents for the Conversions API
func TestAPurchaseIsTheConversionsAPIRequest(t *testing.T) {
	srv, req, raw := server(t, http.StatusOK)
	d := sdk.DestinationConfig{Config: map[string]string{"pixel_id": "1234567890", "test_event_code": "TEST123"}, Secret: "tok"}
	at := time.Unix(1793527200, 0).UTC()
	ev := purchase(at)
	if time.Since(at) > maxAge { // the golden file's time is fixed; send a fresh copy and compare it at the fixed one
		ev.OccurredAt = time.Now()
	}
	if err := Send(context.Background(), srv.Client(), d, ev); err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/"+graphVersion+"/1234567890/events" || req.URL.Query().Get("access_token") != "tok" {
		t.Errorf("request %s", req.URL)
	}
	var sent body
	if err := json.Unmarshal(*raw, &sent); err != nil {
		t.Fatal(err)
	}
	sent.Data[0].EventTime = at.Unix()
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

func TestEmailAndPhoneAreNormalisedAndHashedTheWayMetaSpecifies(t *testing.T) {
	e := toEvent("Purchase", purchase(time.Now()))
	if e.UserData.Em[0] != hash("joe@example.com") {
		t.Error("email must be trimmed, lowercased, then hashed")
	}
	if e.UserData.Ph[0] != hash("16505551212") {
		t.Error("phone must be digits only with its country code, then hashed - Meta's own example normalises (650)555-1212 to 16505551212")
	}
	if e.UserData.ClientIPAddress != "203.0.113.7" || e.UserData.Fbc != "fb.1.1700000000000.IwAR" || e.UserData.Fbp == "" || len(e.UserData.Em[0]) != 64 {
		t.Errorf("ip, fbc and fbp go unhashed: %+v", e.UserData)
	}
	if phoneDigits("00381 64 123") != "38164123" {
		t.Error("an international 00 prefix must go, leaving the country code")
	}
}

func TestWhatIsNotSentAndWhatIsNotRetried(t *testing.T) {
	srv, req, _ := server(t, http.StatusBadRequest)
	d := sdk.DestinationConfig{Config: map[string]string{"pixel_id": "1"}, Secret: "tok"}
	if err := Send(context.Background(), srv.Client(), d, sdk.ShopEvent{ID: "r", Name: "refund", OccurredAt: time.Now()}); err != nil || req.URL != nil {
		t.Errorf("an event Meta has no standard name for must be skipped: %v", err)
	}
	if err := Send(context.Background(), srv.Client(), d, purchase(time.Now().Add(-8*24*time.Hour))); !sdk.IsPermanent(err) || req.URL != nil {
		t.Errorf("an event older than 7 days must be refused before sending: %v", err)
	}
	if err := Send(context.Background(), srv.Client(), d, purchase(time.Now())); !sdk.IsPermanent(err) {
		t.Errorf("Meta's 400 must be final: %v", err)
	}
	if strings.Contains(Send(context.Background(), srv.Client(), d, purchase(time.Now())).Error(), "tok") {
		t.Error("the access token leaked into the error the delivery log keeps")
	}
}
