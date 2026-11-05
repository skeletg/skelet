package trackingcj

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// The multi-item example from CJ's S2S implementation guide, without AMOUNT and CUST_STATUS, which its parameter
// tables don't define
const docExample = "https://www.emjcd.com/u?CID=1234567&TYPE=123456&METHOD=S2S&CJEVENT=7890abcdef1234567890abcdef123456&eventTime=2026-08-12T09:45:30Z&DISCOUNT=10&OID=SA112342&CURRENCY=USD&COUPON=CJ26&ITEM1=sku-001&AMT1=29.99&QTY1=2&DCNT1=5.00&ITEM2=sku-002&AMT2=14.99&QTY2=1&DCNT2=0"

func docOrder() (sdk.DestinationConfig, sdk.ShopEvent) {
	d := sdk.DestinationConfig{Config: map[string]string{"cid": "1234567", "type": "123456"}}
	ev := sdk.ShopEvent{
		ID: "e", Name: "purchase", OccurredAt: time.Date(2026, 8, 12, 9, 45, 30, 0, time.UTC),
		ClickIDs: map[string]string{"cjevent": "7890abcdef1234567890abcdef123456"},
		Ecommerce: &sdk.ShopEcommerce{
			Currency: "USD", TransactionID: "SA112342", Coupon: "CJ26",
			Value: 29.99*2 + 14.99 - 5 - 10, // after the line discount and the order's own
			Items: []sdk.ShopItem{{ItemID: "sku-001", Price: 29.99, Quantity: 2, Discount: 5}, {ItemID: "sku-002", Price: 14.99, Quantity: 1}},
		},
	}
	return d, ev
}

func TestTheQueryMatchesCJsOwnExample(t *testing.T) {
	want, _ := url.Parse(docExample)
	got := Query(docOrder())
	for k, vs := range want.Query() {
		g, w := got.Get(k), vs[0]
		if gf, err1 := strconv.ParseFloat(g, 64); err1 == nil {
			if wf, err2 := strconv.ParseFloat(w, 64); err2 == nil && gf == wf {
				continue // 5.00 and 5, 0 and 0.00 are the same amount
			}
		}
		if g != w {
			t.Errorf("%s = %q, CJ's example has %q", k, g, w)
		}
	}
	if len(got) != len(want.Query()) {
		t.Errorf("sent %d parameters, the example has %d: %v", len(got), len(want.Query()), got)
	}
}

func TestOnlyAClickedPurchaseIsSentAndIDsAreMadeSafe(t *testing.T) {
	var calls []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls = append(calls, r) }))
	defer srv.Close()
	prev := postbackURL
	postbackURL = srv.URL + "/u"
	defer func() { postbackURL = prev }()

	d, ev := docOrder()
	noClick := ev
	noClick.ClickIDs = nil
	view := ev
	view.Name = "view_item"
	for _, e := range []sdk.ShopEvent{noClick, view} {
		if err := Send(context.Background(), srv.Client(), d, e); err != nil || len(calls) != 0 {
			t.Fatalf("%s sent: %v", e.Name, err)
		}
	}
	ev.Ecommerce.TransactionID, ev.Ecommerce.Items[0].ItemID = "ORD 42/a", "sku 1"
	if err := Send(context.Background(), srv.Client(), d, ev); err != nil || len(calls) != 1 || calls[0].Method != http.MethodGet {
		t.Fatalf("the purchase: %v, %d calls", err, len(calls))
	}
	q := calls[0].URL.Query()
	if q.Get("OID") != "ORD-42-a" || q.Get("ITEM1") != "sku-1" || q.Get("METHOD") != "S2S" {
		t.Errorf("ids not made safe for CJ: %v", q)
	}
}
