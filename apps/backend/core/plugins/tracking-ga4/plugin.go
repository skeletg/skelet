// Package trackingga4 sends shop events to Google Analytics 4 over the Measurement Protocol - to Google directly, to its
// EU endpoint, or to a server-side Google Tag Manager container, which then fans them out to whatever it is set up for
package trackingga4

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	Slug            = "tracking-ga4"
	Kind            = "ga4"
	defaultEndpoint = "https://www.google-analytics.com"
)

func init() {
	p := sdk.RegisterPlugin(sdk.Manifest{Slug: Slug, Name: "Google Analytics 4 (server-side)", Version: "0.1.0", SDKRange: "^0.1.2"})
	p.RegisterDestination(sdk.Destination{
		Kind:     Kind,
		Label:    "Google Analytics 4",
		Purposes: []sdk.DestinationPurpose{sdk.PurposeAnalytics},
		Fields: []sdk.DestinationField{
			{Key: "measurement_id", Label: "Measurement ID", Help: "G-XXXXXXXXXX, from the web data stream", Required: true},
			{Key: "api_secret", Label: "Measurement Protocol API secret", Help: "Admin > Data streams > the stream > Measurement Protocol API secrets", Required: true, Secret: true},
			{Key: "endpoint", Label: "Endpoint", Help: "Empty for Google. https://region1.google-analytics.com keeps collection in the EU; a server-side GTM container's URL sends through it"},
			{Key: "validate", Label: "Validate only", Help: "true sends to Google's validation server instead, which checks each event and records nothing - for setting up"},
		},
		Send: Send,
	})
}

func Send(ctx context.Context, client *http.Client, d sdk.DestinationConfig, ev sdk.ShopEvent) error {
	body, err := json.Marshal(toPayload(ev))
	if err != nil {
		return sdk.Permanent(err)
	}
	endpoint := strings.TrimRight(d.Config["endpoint"], "/")
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	path := "/mp/collect"
	validate := d.Config["validate"] == "true"
	if validate {
		path = "/debug/mp/collect"
	}
	q := url.Values{"measurement_id": {d.Config["measurement_id"]}, "api_secret": {d.Secret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+path+"?"+q.Encode(), bytes.NewReader(body))
	if err != nil {
		return sdk.Permanent(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if err := sdk.ResponseError(res); err != nil || !validate {
		return err
	}
	// The collect endpoint answers 2xx whatever it was sent, so only the validation server can say an event is wrong
	var v struct {
		ValidationMessages []struct {
			FieldPath, Description, ValidationCode string
		} `json:"validationMessages"`
	}
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		return fmt.Errorf("read the validation answer: %w", err)
	}
	if len(v.ValidationMessages) > 0 {
		m := v.ValidationMessages[0]
		return sdk.Permanent(fmt.Errorf("%s: %s (%s)", m.ValidationCode, m.Description, m.FieldPath))
	}
	return nil
}

type payload struct {
	ClientID        string         `json:"client_id"`
	UserID          string         `json:"user_id,omitempty"`
	TimestampMicros int64          `json:"timestamp_micros"`
	Consent         *consent       `json:"consent,omitempty"`
	IPOverride      string         `json:"ip_override,omitempty"`
	Events          []payloadEvent `json:"events"`
}

type consent struct {
	AdUserData        string `json:"ad_user_data"`
	AdPersonalization string `json:"ad_personalization"`
}

type payloadEvent struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params"`
}

// toPayload is the event as the Measurement Protocol takes it. The visitor id is the GA client id - with no visitor (an
// order placed off the storefront) the event's own id stands in, so it still lands, as a visitor of its own
func toPayload(ev sdk.ShopEvent) payload {
	p := payload{ClientID: ev.User.AnonID, TimestampMicros: ev.OccurredAt.UnixMicro(), IPOverride: ev.Context.IP}
	if p.ClientID == "" {
		p.ClientID = ev.ID
	}
	if ev.User.CustomerID != nil {
		p.UserID = ev.User.CustomerID.String()
	}
	if ev.Consent != nil {
		p.Consent = &consent{AdUserData: granted(ev.Consent.Ads), AdPersonalization: granted(ev.Consent.Personalization)}
	}
	params := map[string]any{"event_id": ev.ID}
	if ev.Context.PageURL != "" {
		params["page_location"] = ev.Context.PageURL
	}
	if ev.Context.Referrer != "" {
		params["page_referrer"] = ev.Context.Referrer
	}
	if ev.Context.Locale != "" {
		params["language"] = ev.Context.Locale
	}
	if e := ev.Ecommerce; e != nil {
		params["currency"], params["value"] = e.Currency, e.Value
		setIf(params, "transaction_id", e.TransactionID)
		setIf(params, "coupon", e.Coupon)
		if e.Tax != 0 {
			params["tax"] = e.Tax
		}
		if e.Shipping != 0 {
			params["shipping"] = e.Shipping
		}
		if len(e.Items) > 0 {
			params["items"] = e.Items // already GA4's item fields
		}
	}
	p.Events = []payloadEvent{{Name: ev.Name, Params: params}}
	return p
}

func granted(b bool) string {
	if b {
		return "GRANTED"
	}
	return "DENIED"
}

func setIf(m map[string]any, k, v string) {
	if v != "" {
		m[k] = v
	}
}
