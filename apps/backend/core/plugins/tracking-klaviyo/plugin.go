// Package trackingklaviyo sends shop events to Klaviyo's Events API, building the marketing profiles its flows and
// segments run on. It never builds one an email didn't agree to: an event carrying an email goes only when that email
// opted into marketing, and browsing events go under the visitor or account id alone
package trackingklaviyo

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	Slug = "tracking-klaviyo"
	Kind = "klaviyo"
	// The API revision the requests are built against - Klaviyo keeps each revision working for two years
	revision = "2026-10-15"
)

var apiHost = "https://a.klaviyo.com" // a var so tests can point it at a local server

// GA4's names to Klaviyo's standard ecommerce metrics - the names its prebuilt flows listen for
var metrics = map[string]string{
	"view_item":      "Viewed Product",
	"add_to_cart":    "Added to Cart",
	"begin_checkout": "Started Checkout",
	"purchase":       "Placed Order",
	"refund":         "Refunded Order",
}

func init() {
	p := sdk.RegisterPlugin(sdk.Manifest{Slug: Slug, Name: "Klaviyo", Version: "0.1.0", SDKRange: "^0.1.0"})
	p.RegisterDestination(sdk.Destination{
		Kind:          Kind,
		Label:         "Klaviyo",
		Purposes:      []sdk.DestinationPurpose{sdk.PurposePersonalization},
		DefaultEvents: []string{"add_to_cart", "begin_checkout", "purchase", "refund", "view_item"},
		Fields: []sdk.DestinationField{
			{Key: "private_api_key", Label: "Private API key", Help: "Settings > API keys, with the events:write scope", Required: true, Secret: true},
		},
		Send: Send,
	})
}

func Send(ctx context.Context, client *http.Client, d sdk.DestinationConfig, ev sdk.ShopEvent) error {
	metric, ok := metrics[ev.Name]
	if !ok {
		return nil
	}
	body, ok := toBody(metric, ev)
	if !ok {
		return nil
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return sdk.Permanent(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiHost+"/api/events", bytes.NewReader(raw))
	if err != nil {
		return sdk.Permanent(err)
	}
	req.Header.Set("Authorization", "Klaviyo-API-Key "+d.Secret)
	req.Header.Set("revision", revision)
	req.Header.Set("Content-Type", "application/vnd.api+json")
	req.Header.Set("Accept", "application/vnd.api+json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return sdk.ResponseError(res)
}

type object struct {
	Data data `json:"data"`
}

type data struct {
	Type       string         `json:"type"`
	Attributes map[string]any `json:"attributes"`
}

// ok is false when the event may not go: an email that hasn't opted into marketing, or no identifier at all
func toBody(metric string, ev sdk.ShopEvent) (object, bool) {
	if ev.User.Email != "" && !ev.User.MarketingOptIn {
		return object{}, false
	}
	profile := map[string]any{}
	if ev.User.Email != "" {
		profile["email"] = ev.User.Email
	}
	if ev.User.CustomerID != nil {
		profile["external_id"] = ev.User.CustomerID.String()
	}
	if ev.User.AnonID != "" {
		profile["anonymous_id"] = ev.User.AnonID
	}
	if len(profile) == 0 {
		return object{}, false
	}
	props := map[string]any{}
	attrs := map[string]any{
		"properties": props,
		"time":       ev.OccurredAt.UTC().Format(time.RFC3339),
		"unique_id":  ev.ID, // Klaviyo dedupes on it
		"metric":     object{Data: data{Type: "metric", Attributes: map[string]any{"name": metric}}},
		"profile":    object{Data: data{Type: "profile", Attributes: profile}},
	}
	if e := ev.Ecommerce; e != nil {
		value := e.Value
		if ev.Name == "purchase" {
			value += e.Tax + e.Shipping // the order's worth as the shopper paid it
		}
		attrs["value"], attrs["value_currency"] = value, e.Currency
		if e.TransactionID != "" {
			props["OrderId"] = e.TransactionID
		}
		var items []map[string]any
		var names []string
		for _, it := range e.Items {
			name := it.ItemName
			if name == "" {
				name = it.ItemID
			}
			items = append(items, map[string]any{
				"ProductID": it.ItemID, "SKU": it.ItemID, "ProductName": name,
				"Quantity": it.Quantity, "ItemPrice": it.Price, "RowTotal": it.Price * it.Quantity,
			})
			names = append(names, name)
		}
		if len(items) > 0 {
			props["Items"], props["ItemNames"] = items, names
		}
		if len(e.Items) == 1 && ev.Name == "view_item" { // a product view names the product itself, as Klaviyo's own snippet does
			it := e.Items[0]
			props["ProductID"], props["SKU"], props["ProductName"], props["Price"] = it.ItemID, it.ItemID, items[0]["ProductName"], it.Price
		}
	}
	if ev.Context.PageURL != "" {
		props["URL"] = ev.Context.PageURL
	}
	return object{Data: data{Type: "event", Attributes: attrs}}, true
}
