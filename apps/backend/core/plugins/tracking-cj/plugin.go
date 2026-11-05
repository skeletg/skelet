// Package trackingcj reports sales to CJ Affiliate with its inbound server-to-server postback, carrying the cjevent the
// storefront kept in the cje cookie - CJ's own way of tracking without a browser tag
package trackingcj

import (
	"context"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	Slug = "tracking-cj"
	Kind = "cj"
)

var postbackURL = "https://www.emjcd.com/u" // a var so tests can point it at a local server

func init() {
	p := sdk.RegisterPlugin(sdk.Manifest{Slug: Slug, Name: "CJ Affiliate (server-to-server)", Version: "0.1.0", SDKRange: "^0.1.0"})
	p.RegisterDestination(sdk.Destination{
		Kind:             Kind,
		Label:            "CJ Affiliate",
		Purposes:         []sdk.DestinationPurpose{sdk.PurposeAds},
		NecessarySetting: "general/privacy/cj_cookie_necessary",
		DefaultEvents:    []string{"purchase"},
		Fields: []sdk.DestinationField{
			{Key: "cid", Label: "Enterprise ID (CID)", Help: "From your CJ Client Integration Engineer", Required: true},
			{Key: "type", Label: "Action tracker ID (TYPE)", Help: "The action a sale commissions", Required: true},
		},
		Send: Send,
	})
}

// CJ's limits on an order and a SKU id: letters, digits, dashes and underscores
var unsafeID = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func Send(ctx context.Context, client *http.Client, d sdk.DestinationConfig, ev sdk.ShopEvent) error {
	cje := ev.ClickIDs["cjevent"]
	if ev.Name != "purchase" || ev.Ecommerce == nil || cje == "" { // no click, nothing CJ could attribute
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, postbackURL+"?"+Query(d, ev).Encode(), nil)
	if err != nil {
		return sdk.Permanent(err)
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return sdk.ResponseError(res)
}

// Query is the postback's parameters, as CJ's S2S implementation guide defines them
func Query(d sdk.DestinationConfig, ev sdk.ShopEvent) url.Values {
	e := ev.Ecommerce
	q := url.Values{
		"CID": {d.Config["cid"]}, "TYPE": {d.Config["type"]}, "METHOD": {"S2S"},
		"CJEVENT":   {ev.ClickIDs["cjevent"]},
		"eventTime": {ev.OccurredAt.UTC().Format(time.RFC3339)},
		"OID":       {clip(unsafeID.ReplaceAllString(e.TransactionID, "-"), 96)},
		"CURRENCY":  {e.Currency},
	}
	if c := strings.Trim(unsafeID.ReplaceAllString(strings.ReplaceAll(e.Coupon, ",", "\x00"), "-"), "-"); c != "" {
		q.Set("COUPON", strings.ReplaceAll(c, "\x00", ","))
	}
	gross, lineDiscounts := 0.0, 0.0
	for i, it := range e.Items {
		if i == 100 { // CJ takes at most 100 item sets
			break
		}
		n := strconv.Itoa(i + 1)
		qty := max(1, int(math.Round(it.Quantity)))
		q.Set("ITEM"+n, clip(unsafeID.ReplaceAllString(it.ItemID, "-"), 100))
		q.Set("AMT"+n, money(it.Price)) // per unit, before tax and shipping - CJ multiplies by QTY
		q.Set("QTY"+n, strconv.Itoa(qty))
		q.Set("DCNT"+n, money(it.Discount))
		gross += it.Price * float64(qty)
		lineDiscounts += it.Discount
	}
	// Whatever the order took off beyond its lines' own discounts - value is the items after every discount
	if d := round2(gross - lineDiscounts - e.Value); d > 0 && len(e.Items) > 0 {
		q.Set("DISCOUNT", money(d))
	}
	return q
}

func money(v float64) string { return strconv.FormatFloat(round2(v), 'f', 2, 64) }

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
