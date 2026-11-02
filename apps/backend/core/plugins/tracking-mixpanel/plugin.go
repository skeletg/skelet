// Package trackingmixpanel sends shop events to Mixpanel's Import API, server to server. Identity follows Mixpanel's
// simplified ID merge: the storefront's analytics visitor id is $device_id, the customer is $user_id, and an event
// carrying both joins them, so Mixpanel derives distinct_id itself
package trackingmixpanel

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

	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	Slug = "tracking-mixpanel"
	Kind = "mixpanel"
)

// The ingestion host must match the project's data residency, or the import lands nowhere useful
var hosts = map[string]string{"": "https://api.mixpanel.com", "us": "https://api.mixpanel.com", "eu": "https://api-eu.mixpanel.com", "in": "https://api-in.mixpanel.com"}

var eventNames = map[string]string{
	"page_view":         "Page Viewed",
	"view_item_list":    "Product List Viewed",
	"select_item":       "Product Clicked",
	"view_item":         "Product Viewed",
	"add_to_cart":       "Product Added",
	"remove_from_cart":  "Product Removed",
	"view_cart":         "Cart Viewed",
	"begin_checkout":    "Checkout Started",
	"add_shipping_info": "Shipping Info Added",
	"add_payment_info":  "Payment Info Added",
	"purchase":          "Order Completed",
	"refund":            "Order Refunded",
	"sign_up":           "Signed Up",
}

func init() {
	p := sdk.RegisterPlugin(sdk.Manifest{Slug: Slug, Name: "Mixpanel", Version: "0.1.0", SDKRange: "^0.1.0"})
	defaults := make([]string, 0, len(eventNames))
	for name := range eventNames {
		defaults = append(defaults, name)
	}
	sort.Strings(defaults)
	p.RegisterDestination(sdk.Destination{
		Kind:          Kind,
		Label:         "Mixpanel",
		Purposes:      []sdk.DestinationPurpose{sdk.PurposeAnalytics},
		DefaultEvents: defaults,
		Fields: []sdk.DestinationField{
			{Key: "project_id", Label: "Project ID", Help: "Project Settings > Overview", Required: true},
			{Key: "service_account", Label: "Service account username", Help: "Organization Settings > Service Accounts, with the Admin or Owner role on this project", Required: true},
			{Key: "service_secret", Label: "Service account secret", Required: true, Secret: true},
			{Key: "region", Label: "Data residency: us (default), eu or in", Help: "Project Settings > Data Residency - it must match, or Mixpanel rejects the import"},
		},
		Send: Send,
	})
}

func Send(ctx context.Context, client *http.Client, d sdk.DestinationConfig, ev sdk.ShopEvent) error {
	name, ok := eventNames[ev.Name]
	if !ok {
		return nil
	}
	host, ok := hosts[d.Config["region"]]
	if !ok {
		return sdk.Permanent(fmt.Errorf("unknown Mixpanel region %q", d.Config["region"]))
	}
	props, ok := properties(ev)
	if !ok {
		return nil // nobody to attribute it to - the aggregate counts cover it, not a made-up user
	}
	raw, err := json.Marshal([]event{{Event: name, Properties: props}})
	if err != nil {
		return sdk.Permanent(err)
	}
	// Import rather than Track: it takes events of any age, so a retried or replayed delivery is never refused for being late
	u := host + "/import?" + url.Values{"strict": {"1"}, "project_id": {d.Config["project_id"]}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return sdk.Permanent(err)
	}
	req.SetBasicAuth(d.Config["service_account"], d.Secret)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return sdk.ResponseError(res) // strict mode answers a malformed event with 400, which no retry fixes
}

type event struct {
	Event      string         `json:"event"`
	Properties map[string]any `json:"properties"`
}

func properties(ev sdk.ShopEvent) (map[string]any, bool) {
	p := map[string]any{
		"time":       ev.OccurredAt.UnixMilli(),
		"$insert_id": insertID(ev.ID), // Mixpanel dedupes on it, so a retry or the browser's copy counts once
		"source":     ev.Source,
	}
	if ev.User.AnonID != "" {
		p["$device_id"] = ev.User.AnonID
	}
	if ev.User.CustomerID != nil {
		p["$user_id"] = ev.User.CustomerID.String()
	}
	if p["$device_id"] == nil && p["$user_id"] == nil {
		return nil, false
	}
	if ev.Context.IP != "" {
		p["ip"] = ev.Context.IP // geolocation; without it Import places the event nowhere
	}
	set := func(k, v string) {
		if v != "" {
			p[k] = v
		}
	}
	set("$current_url", ev.Context.PageURL)
	set("$referrer", ev.Context.Referrer)
	set("title", ev.Context.PageTitle)
	set("session_id", ev.Context.SessionID)
	set("locale", ev.Context.Locale)
	if e := ev.Ecommerce; e != nil {
		p["currency"], p["value"] = e.Currency, e.Value
		set("transaction_id", e.TransactionID)
		set("coupon", e.Coupon)
		if e.Tax != 0 {
			p["tax"] = e.Tax
		}
		if e.Shipping != 0 {
			p["shipping"] = e.Shipping
		}
		if len(e.Items) > 0 {
			p["items"] = e.Items // the GA4 item shape, which Mixpanel reports on as a list of objects
		}
	}
	return p, true
}

// Mixpanel takes at most 36 characters of letters, digits and dashes; a test send's id is longer
func insertID(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:16])
}
