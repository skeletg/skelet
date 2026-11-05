package trackingawin

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

	"github.com/skeletg/skelet/apps/backend/sdk"
)

var update = flag.Bool("update", false, "rewrite the golden request files")

func purchase() sdk.ShopEvent {
	return sdk.ShopEvent{
		ID: "e-42", Name: "purchase", OccurredAt: time.Now(),
		ClickIDs: map[string]string{"awc": "1001_1700000000_abc"},
		Ecommerce: &sdk.ShopEcommerce{Currency: "GBP", Value: 111, TransactionID: "#1100011", Coupon: "10_OFF",
			Items: []sdk.ShopItem{{ItemID: "321", ItemName: "Shoe | red", ItemCategory: "Shoes", Price: 100, Quantity: 1}, {ItemID: "654", Price: 11, Quantity: 1}}},
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
	prev := apiHost
	apiHost = srv.URL
	t.Cleanup(func() { apiHost = prev })
	return srv, &got, &raw
}

// The request a sale becomes, held to a golden file in the shape of Awin's Conversion API example
func TestASaleIsTheConversionAPIRequest(t *testing.T) {
	srv, req, raw := server(t, http.StatusOK, `{"message":"","batchId":"b","successfulOrders":[{"orderReference":"%231100011"}],"failedOrders":[]}`)
	d := sdk.DestinationConfig{Config: map[string]string{"advertiser_id": "12345", "commission_group": "shoes", "test": "true"}, Secret: "key"}
	if err := Send(context.Background(), srv.Client(), d, purchase()); err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/s2s/advertiser/12345/orders" || req.Header.Get("x-api-key") != "key" {
		t.Errorf("request %s, key %q", req.URL, req.Header.Get("x-api-key"))
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
}

func TestAFailedOrderInsideASuccessIsARefusal(t *testing.T) {
	reply, _ := os.ReadFile("testdata/partial_failure.json")
	srv, _, _ := server(t, http.StatusPartialContent, string(reply))
	err := Send(context.Background(), srv.Client(), sdk.DestinationConfig{Config: map[string]string{"advertiser_id": "1"}, Secret: "key"}, purchase())
	if !sdk.IsPermanent(err) || !strings.Contains(err.Error(), "Commission group not found") {
		t.Errorf("err %v", err)
	}
}

func TestOnlyAPurchaseWithAClickOrVoucherIsSent(t *testing.T) {
	srv, req, _ := server(t, http.StatusOK, `{}`)
	d := sdk.DestinationConfig{Config: map[string]string{"advertiser_id": "1"}, Secret: "key"}
	ev := purchase()
	ev.ClickIDs, ev.Ecommerce.Coupon = nil, ""
	if err := Send(context.Background(), srv.Client(), d, ev); err != nil || req.URL != nil {
		t.Errorf("an unattributable sale was sent: %v", err)
	}
	view := purchase()
	view.Name = "view_item"
	if err := Send(context.Background(), srv.Client(), d, view); err != nil || req.URL != nil {
		t.Errorf("a view was sent: %v", err)
	}
}
