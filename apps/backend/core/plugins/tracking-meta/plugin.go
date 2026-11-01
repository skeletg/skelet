// Package trackingmeta sends shop events to Meta's Conversions API, server to server. The event id is the one the
// browser pixel uses too, so where both run Meta counts each event once
package trackingmeta

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	Slug = "tracking-meta"
	Kind = "meta_capi"
	// The Graph API version the requests are built against - Meta keeps each version for about two years
	graphVersion = "v26.0"
	maxAge       = 7 * 24 * time.Hour // older and Meta fails the whole request
)

var graphHost = "https://graph.facebook.com" // a var so tests can point it at a local server

// GA4's names to Meta's standard events - one Meta has no standard event for isn't sent
var eventNames = map[string]string{
	"page_view":        "PageView",
	"view_item":        "ViewContent",
	"add_to_cart":      "AddToCart",
	"begin_checkout":   "InitiateCheckout",
	"add_payment_info": "AddPaymentInfo",
	"purchase":         "Purchase",
	"sign_up":          "CompleteRegistration",
}

func init() {
	p := sdk.RegisterPlugin(sdk.Manifest{Slug: Slug, Name: "Meta Conversions API", Version: "0.1.0", SDKRange: "^0.1.2"})
	defaults := make([]string, 0, len(eventNames))
	for name := range eventNames {
		defaults = append(defaults, name)
	}
	sort.Strings(defaults)
	p.RegisterDestination(sdk.Destination{
		Kind:          Kind,
		Label:         "Meta (Facebook, Instagram)",
		Purposes:      []sdk.DestinationPurpose{sdk.PurposeAds},
		DefaultEvents: defaults,
		Fields: []sdk.DestinationField{
			{Key: "pixel_id", Label: "Pixel ID", Help: "The dataset (pixel) the events belong to", Required: true},
			{Key: "access_token", Label: "Access token", Help: "Events Manager > the dataset > Settings > Conversions API > Generate access token", Required: true, Secret: true},
			{Key: "test_event_code", Label: "Test event code", Help: "From Events Manager > Test events, while setting up - leave empty in production"},
		},
		Send: Send,
	})
}

func Send(ctx context.Context, client *http.Client, d sdk.DestinationConfig, ev sdk.ShopEvent) error {
	name, ok := eventNames[ev.Name]
	if !ok {
		return nil
	}
	if time.Since(ev.OccurredAt) > maxAge {
		return sdk.Permanent(fmt.Errorf("%s is older than Meta's 7 days", ev.ID))
	}
	b := body{Data: []event{toEvent(name, ev)}, TestEventCode: d.Config["test_event_code"]}
	raw, err := json.Marshal(b)
	if err != nil {
		return sdk.Permanent(err)
	}
	u := fmt.Sprintf("%s/%s/%s/events?%s", graphHost, graphVersion, url.PathEscape(d.Config["pixel_id"]), url.Values{"access_token": {d.Secret}}.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return sdk.Permanent(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return sdk.ResponseError(res)
}

type body struct {
	Data          []event `json:"data"`
	TestEventCode string  `json:"test_event_code,omitempty"`
}

type event struct {
	EventName      string      `json:"event_name"`
	EventTime      int64       `json:"event_time"`
	EventID        string      `json:"event_id"`
	ActionSource   string      `json:"action_source"`
	EventSourceURL string      `json:"event_source_url,omitempty"`
	UserData       userData    `json:"user_data"`
	CustomData     *customData `json:"custom_data,omitempty"`
}

type userData struct {
	Em              []string `json:"em,omitempty"`
	Ph              []string `json:"ph,omitempty"`
	ExternalID      []string `json:"external_id,omitempty"`
	ClientIPAddress string   `json:"client_ip_address,omitempty"`
	ClientUserAgent string   `json:"client_user_agent,omitempty"`
	Fbc             string   `json:"fbc,omitempty"`
	Fbp             string   `json:"fbp,omitempty"`
}

type customData struct {
	Currency    string    `json:"currency"`
	Value       float64   `json:"value"`
	OrderID     string    `json:"order_id,omitempty"`
	ContentType string    `json:"content_type"`
	ContentIDs  []string  `json:"content_ids,omitempty"`
	Contents    []content `json:"contents,omitempty"`
	NumItems    float64   `json:"num_items,omitempty"`
}

type content struct {
	ID        string  `json:"id"`
	Quantity  float64 `json:"quantity"`
	ItemPrice float64 `json:"item_price,omitempty"`
}

// Meta matches on email and phone hashed as it specifies, and on the browser's own ids and address unhashed
func toEvent(name string, ev sdk.ShopEvent) event {
	out := event{
		EventName: name, EventTime: ev.OccurredAt.Unix(), EventID: ev.ID, ActionSource: "website", EventSourceURL: ev.Context.PageURL,
		UserData: userData{
			ClientIPAddress: ev.Context.IP, ClientUserAgent: ev.Context.UserAgent,
			Fbc: ev.ClickIDs["fbc"], Fbp: ev.ClickIDs["fbp"],
		},
	}
	if em := strings.ToLower(strings.TrimSpace(ev.User.Email)); em != "" {
		out.UserData.Em = []string{hash(em)}
	}
	if ph := phoneDigits(ev.User.Phone); ph != "" {
		out.UserData.Ph = []string{hash(ph)}
	}
	if ev.User.CustomerID != nil {
		out.UserData.ExternalID = []string{hash(ev.User.CustomerID.String())}
	} else if ev.User.AnonID != "" {
		out.UserData.ExternalID = []string{hash(ev.User.AnonID)}
	}
	if e := ev.Ecommerce; e != nil {
		// What was paid, tax and shipping in - Meta's return on ad spend reads value as the order's worth to the shop
		cd := &customData{Currency: e.Currency, Value: e.Value + e.Tax + e.Shipping, OrderID: e.TransactionID, ContentType: "product"}
		for _, it := range e.Items {
			cd.ContentIDs = append(cd.ContentIDs, it.ItemID)
			cd.Contents = append(cd.Contents, content{ID: it.ItemID, Quantity: it.Quantity, ItemPrice: it.Price})
			cd.NumItems += it.Quantity
		}
		out.CustomData = cd
	}
	return out
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Digits only, without the leading zeros of a trunk or international prefix - Meta needs the country code in front,
// which a number typed without one (a local 064...) doesn't have, so it simply won't match
func phoneDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimLeft(b.String(), "0")
}
