package trackingopenai

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
		ID: "order_12345", Name: "purchase", OccurredAt: at,
		Ecommerce: &sdk.ShopEcommerce{Currency: "USD", Value: 20.99, Tax: 2.5, Shipping: 2.5, TransactionID: "ORD-1",
			Items: []sdk.ShopItem{{ItemID: "sku_123", ItemName: "Starter bundle", Price: 20.99, Quantity: 1}}},
		User:     sdk.ShopUser{CustomerID: &cust, Email: " Test@Example.com ", Phone: "+1 (415) 555-2671"},
		Context:  sdk.ShopContext{IP: "203.0.113.1", UserAgent: "Mozilla/5.0", PageURL: "https://shop.example.com/checkout/confirmation"},
		ClickIDs: map[string]string{"oppref": "oppref_abc", "obref": "123e4567-e89b-42d3-a456-426614174000"},
		Consent:  &sdk.ShopConsent{Ads: true},
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
	prev := endpoint
	endpoint = srv.URL + "/v1/events"
	t.Cleanup(func() { endpoint = prev })
	return srv, &got, &raw
}

// The request a purchase becomes, held to a golden file in the shape of OpenAI's own example request
func TestAPurchaseIsOpenAIsOrderCreated(t *testing.T) {
	srv, req, raw := server(t, http.StatusOK)
	at := time.UnixMilli(1773892800000).UTC()
	if err := Send(context.Background(), srv.Client(), sdk.DestinationConfig{Config: map[string]string{"pixel_id": "PX1"}, Secret: "key"}, purchase(time.Now())); err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/v1/events" || req.URL.Query().Get("pid") != "PX1" || req.Header.Get("Authorization") != "Bearer key" {
		t.Errorf("request %s", req.URL)
	}
	var sent body
	if err := json.Unmarshal(*raw, &sent); err != nil {
		t.Fatal(err)
	}
	sent.Events[0].TimestampMs = at.UnixMilli() // the golden file's time is fixed
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

func TestNormalisationAndAmountsFollowOpenAIsRules(t *testing.T) {
	e := toEvent("order_created", purchase(time.Now()))
	if e.User.EmailsSHA256[0] != sha("test@example.com") || e.User.PhonesSHA256[0] != sha("14155552671") {
		t.Error("email trimmed and lowercased; phone +1 (415) 555-2671 becomes 14155552671, as OpenAI's own example")
	}
	if *e.Data.Amount != 2599 || e.Data.Contents[0].Amount != 2099 {
		t.Errorf("amounts in minor units: %d, %d", *e.Data.Amount, e.Data.Contents[0].Amount)
	}
	if minor(4200, "JPY") != 4200 || minor(1.999, "KWD") != 1999 {
		t.Error("minor units must follow the currency's decimals")
	}
	server := purchase(time.Now())
	server.Context.PageURL = ""
	if toEvent("order_created", server).ActionSource != "other" {
		t.Error("an event with no page can't claim action_source web, which needs one")
	}
}

func TestWhatIsNotSent(t *testing.T) {
	srv, req, _ := server(t, http.StatusBadRequest)
	d := sdk.DestinationConfig{Config: map[string]string{"pixel_id": "PX1"}, Secret: "key"}
	if err := Send(context.Background(), srv.Client(), d, sdk.ShopEvent{ID: "r", Name: "refund", OccurredAt: time.Now()}); err != nil || req.URL != nil {
		t.Errorf("an event with no OpenAI type was sent: %v", err)
	}
	if err := Send(context.Background(), srv.Client(), d, purchase(time.Now().Add(-8*24*time.Hour))); !sdk.IsPermanent(err) || req.URL != nil {
		t.Errorf("an event older than 7 days: %v", err)
	}
	if err := Send(context.Background(), srv.Client(), d, purchase(time.Now())); !sdk.IsPermanent(err) {
		t.Errorf("OpenAI's 400 must be final: %v", err)
	}
}
