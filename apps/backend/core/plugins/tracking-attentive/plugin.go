// Package trackingattentive sends product views, cart adds and purchases to Attentive's eCommerce events API, for its
// text and email journeys. Like Klaviyo it never builds a profile for an email that didn't opt into marketing, and it
// sends no phone number: that is an SMS channel, which needs a consent the shop doesn't collect
package trackingattentive

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	Slug = "tracking-attentive"
	Kind = "attentive"
)

var apiHost = "https://api.attentivemobile.com" // a var so tests can point it at a local server

var paths = map[string]string{
	"view_item":   "/v1/events/ecommerce/product-view",
	"add_to_cart": "/v1/events/ecommerce/add-to-cart",
	"purchase":    "/v1/events/ecommerce/purchase",
}

func init() {
	p := sdk.RegisterPlugin(sdk.Manifest{Slug: Slug, Name: "Attentive", Version: "0.1.0", SDKRange: "^0.1.0"})
	p.RegisterDestination(sdk.Destination{
		Kind:          Kind,
		Label:         "Attentive",
		Purposes:      []sdk.DestinationPurpose{sdk.PurposePersonalization},
		DefaultEvents: []string{"add_to_cart", "purchase", "view_item"},
		Fields: []sdk.DestinationField{
			{Key: "api_key", Label: "API key", Help: "A private app's key with the ecommerce:write scope", Required: true, Secret: true},
		},
		Send: Send,
	})
}

func Send(ctx context.Context, client *http.Client, d sdk.DestinationConfig, ev sdk.ShopEvent) error {
	path, ok := paths[ev.Name]
	if !ok || ev.Ecommerce == nil || len(ev.Ecommerce.Items) == 0 {
		return nil
	}
	body, ok := toBody(ev)
	if !ok {
		return nil
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return sdk.Permanent(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiHost+path, bytes.NewReader(raw))
	if err != nil {
		return sdk.Permanent(err)
	}
	req.Header.Set("Authorization", "Bearer "+d.Secret)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return sdk.ResponseError(res)
}

type request struct {
	Items        []item        `json:"items"`
	OrderID      string        `json:"orderId,omitempty"`
	SalesChannel *salesChannel `json:"salesChannel,omitempty"`
	OccurredAt   string        `json:"occurredAt"`
	User         user          `json:"user"`
}

type item struct {
	ProductID        string  `json:"productId"`
	ProductVariantID string  `json:"productVariantId"`
	ProductURL       string  `json:"productUrl,omitempty"`
	Name             string  `json:"name,omitempty"`
	Price            []price `json:"price"`
	Quantity         int64   `json:"quantity,omitempty"`
}

type price struct {
	Value    float64 `json:"value"`
	Currency string  `json:"currency"`
}

type salesChannel struct {
	ChannelType string `json:"channelType"`
}

type user struct {
	Email               string               `json:"email,omitempty"`
	ExternalIdentifiers *externalIdentifiers `json:"externalIdentifiers,omitempty"`
}

type externalIdentifiers struct {
	ClientUserID      string             `json:"clientUserId,omitempty"`
	CustomIdentifiers []customIdentifier `json:"customIdentifiers,omitempty"`
}

type customIdentifier struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ok is false when the event may not go: an email that hasn't opted into marketing, or no one to name
func toBody(ev sdk.ShopEvent) (request, bool) {
	if ev.User.Email != "" && !ev.User.MarketingOptIn {
		return request{}, false
	}
	e := ev.Ecommerce
	r := request{OrderID: e.TransactionID, OccurredAt: ev.OccurredAt.UTC().Format(time.RFC3339), User: user{Email: ev.User.Email}}
	ids := &externalIdentifiers{}
	if ev.User.CustomerID != nil {
		ids.ClientUserID = ev.User.CustomerID.String()
	}
	if ev.User.AnonID != "" {
		ids.CustomIdentifiers = []customIdentifier{{Name: "visitor_id", Value: ev.User.AnonID}}
	}
	if ids.ClientUserID != "" || len(ids.CustomIdentifiers) > 0 {
		r.User.ExternalIdentifiers = ids
	}
	if r.User.Email == "" && r.User.ExternalIdentifiers == nil {
		return request{}, false
	}
	if ev.Name == "purchase" {
		r.SalesChannel = &salesChannel{ChannelType: "WEB"}
	}
	for _, it := range e.Items {
		r.Items = append(r.Items, item{
			// The SKU is the variant the shopper chose - the shop keeps no separate product id per line
			ProductID: it.ItemID, ProductVariantID: it.ItemID, Name: it.ItemName,
			Price: []price{{Value: it.Price, Currency: e.Currency}}, Quantity: int64(math.Max(1, math.Round(it.Quantity))),
		})
	}
	if ev.Name == "view_item" && len(r.Items) == 1 {
		r.Items[0].ProductURL = ev.Context.PageURL
	}
	return r, true
}
