package trackingmixpanel

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
		ID: "0193a1b2-0000-7000-8000-000000000042", Name: "purchase", OccurredAt: time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC), Source: "server",
		Ecommerce: &sdk.ShopEcommerce{Currency: "EUR", Value: 45, Tax: 9, Shipping: 5, TransactionID: "ORD-000042",
			Items: []sdk.ShopItem{{ItemID: "SKU-1", ItemName: "Mug", Price: 15, Quantity: 2}, {ItemID: "SKU-2", ItemName: "Tee", Price: 20, Quantity: 1}}},
		User:    sdk.ShopUser{CustomerID: &cust, AnonID: "visitor-1", Email: "a@example.com", Phone: "+381601234567"},
		Context: sdk.ShopContext{IP: "203.0.113.7", SessionID: "1761991200"},
		Consent: &sdk.ShopConsent{Analytics: true},
	}
}

func cfg(region string) sdk.DestinationConfig {
	return sdk.DestinationConfig{Config: map[string]string{"project_id": "123", "service_account": "sa.user", "region": region}, Secret: "sa-secret"}
}

func capture(t *testing.T, status int) (*http.Client, *http.Request, *[]byte) {
	t.Helper()
	var got http.Request
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r
		raw, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	prev := hosts["eu"]
	hosts["eu"] = srv.URL
	t.Cleanup(func() { hosts["eu"] = prev })
	return srv.Client(), &got, &raw
}

// The Import request a purchase becomes, held to a golden file in the shape of Mixpanel's Import Events reference
func TestAPurchaseIsAnImportedOrderCompleted(t *testing.T) {
	client, req, raw := capture(t, http.StatusOK)
	if err := Send(context.Background(), client, cfg("eu"), purchase()); err != nil {
		t.Fatal(err)
	}
	user, pass, _ := req.BasicAuth()
	if req.URL.Path != "/import" || req.URL.Query().Get("strict") != "1" || req.URL.Query().Get("project_id") != "123" || user != "sa.user" || pass != "sa-secret" {
		t.Errorf("request %s, auth %q", req.URL, user)
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
	for _, leak := range []string{"a@example.com", "381601234567", "distinct_id"} {
		if strings.Contains(string(*raw), leak) {
			t.Errorf("%q went to Mixpanel", leak)
		}
	}
}

// Anonymous browsing is the visitor id alone; a sign-in sends both ids, which is the merge
func TestIdentityFollowsSimplifiedIDMerge(t *testing.T) {
	ev := purchase()
	ev.User.CustomerID = nil
	p, ok := properties(ev)
	if !ok || p["$device_id"] != "visitor-1" || p["$user_id"] != nil {
		t.Errorf("anonymous: %v", p)
	}
	ev.User.AnonID = ""
	if _, ok := properties(ev); ok {
		t.Error("an event with no visitor and no customer was given an identity")
	}
	if id := insertID("test-" + uuid.NewString()); len(id) > 36 || strings.Trim(id, "0123456789abcdef") != "" {
		t.Errorf("insert id %q is outside Mixpanel's 36 alphanumerics", id)
	}
	if a, b := insertID(ev.ID), insertID(purchase().ID); a != b {
		t.Error("a retry must reuse the insert id")
	}
}

func TestRefusalsAndRegions(t *testing.T) {
	client, _, _ := capture(t, http.StatusBadRequest)
	if err := Send(context.Background(), client, cfg("eu"), purchase()); !sdk.IsPermanent(err) {
		t.Errorf("a strict-mode 400 should not be retried: %v", err)
	}
	client, _, _ = capture(t, http.StatusTooManyRequests)
	if err := Send(context.Background(), client, cfg("eu"), purchase()); err == nil || sdk.IsPermanent(err) {
		t.Errorf("a 429 should be retried: %v", err)
	}
	if err := Send(context.Background(), client, cfg("mars"), purchase()); !sdk.IsPermanent(err) {
		t.Errorf("an unknown region should fail for good: %v", err)
	}
	ev := purchase()
	ev.Name = "login"
	if err := Send(context.Background(), client, cfg("eu"), ev); err != nil {
		t.Errorf("an unmapped event should be skipped: %v", err)
	}
}
