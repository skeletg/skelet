// Package trackingopenai sends shop events to OpenAI's Conversions API, for ads shown in ChatGPT. The event id is the
// one OpenAI's pixel would use, so where both run each conversion counts once
package trackingopenai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	Slug   = "tracking-openai"
	Kind   = "openai"
	maxAge = 7 * 24 * time.Hour // OpenAI refuses an older timestamp, and with it the whole batch
)

var endpoint = "https://bzr.openai.com/v1/events" // a var so tests can point it at a local server

// GA4's names to OpenAI's standard event types
var eventTypes = map[string]string{
	"page_view":      "page_viewed",
	"view_item":      "contents_viewed",
	"add_to_cart":    "items_added",
	"begin_checkout": "checkout_started",
	"purchase":       "order_created",
	"sign_up":        "registration_completed",
}

func init() {
	p := sdk.RegisterPlugin(sdk.Manifest{Slug: Slug, Name: "OpenAI Ads (Conversions API)", Version: "0.1.0", SDKRange: "^0.1.0"})
	p.RegisterDestination(sdk.Destination{
		Kind:          Kind,
		Label:         "OpenAI Ads (ChatGPT)",
		Purposes:      []sdk.DestinationPurpose{sdk.PurposeAds},
		DefaultEvents: []string{"add_to_cart", "begin_checkout", "purchase", "sign_up", "view_item"},
		Fields: []sdk.DestinationField{
			{Key: "pixel_id", Label: "Pixel ID", Help: "Ads Manager > Conversions", Required: true},
			{Key: "api_key", Label: "API key", Help: "Ads Manager > Conversions", Required: true, Secret: true},
			{Key: "validate_only", Label: "Validate only", Help: "true checks each event and saves nothing - for setting up"},
		},
		Send: Send,
	})
}

func Send(ctx context.Context, client *http.Client, d sdk.DestinationConfig, ev sdk.ShopEvent) error {
	typ, ok := eventTypes[ev.Name]
	if !ok {
		return nil
	}
	if time.Since(ev.OccurredAt) > maxAge {
		return sdk.Permanent(fmt.Errorf("%s is older than OpenAI's 7 days", ev.ID))
	}
	raw, err := json.Marshal(body{ValidateOnly: d.Config["validate_only"] == "true", IntegrationSource: "skelet", Events: []event{toEvent(typ, ev)}})
	if err != nil {
		return sdk.Permanent(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"?"+url.Values{"pid": {d.Config["pixel_id"]}}.Encode(), bytes.NewReader(raw))
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

type body struct {
	ValidateOnly      bool    `json:"validate_only,omitempty"`
	IntegrationSource string  `json:"integration_source"`
	Events            []event `json:"events"`
}

type event struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	TimestampMs  int64     `json:"timestamp_ms"`
	Oppref       string    `json:"oppref,omitempty"`
	SourceURL    string    `json:"source_url,omitempty"`
	ActionSource string    `json:"action_source"`
	User         *user     `json:"user,omitempty"`
	Data         eventData `json:"data"`
}

type user struct {
	EmailsSHA256      []string `json:"emails_sha256,omitempty"`
	PhonesSHA256      []string `json:"phone_numbers_sha256,omitempty"`
	ExternalIDsSHA256 []string `json:"external_ids_sha256,omitempty"`
	Obref             string   `json:"obref,omitempty"`
	IPAddress         string   `json:"ip_address,omitempty"`
	UserAgent         string   `json:"user_agent,omitempty"`
}

type eventData struct {
	Type     string    `json:"type"`
	Amount   *int64    `json:"amount,omitempty"`
	Currency string    `json:"currency,omitempty"`
	Contents []content `json:"contents,omitempty"`
}

type content struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	ContentType string `json:"content_type"`
	Quantity    int64  `json:"quantity,omitempty"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency,omitempty"`
}

func toEvent(typ string, ev sdk.ShopEvent) event {
	out := event{ID: ev.ID, Type: typ, TimestampMs: ev.OccurredAt.UnixMilli(), Oppref: ev.ClickIDs["oppref"], SourceURL: ev.Context.PageURL, ActionSource: "web"}
	if out.SourceURL == "" { // web events must name their page; a purchase reported by the server has none to name
		out.ActionSource = "other"
	}
	u := &user{Obref: ev.ClickIDs["obref"], IPAddress: ev.Context.IP, UserAgent: ev.Context.UserAgent}
	if em := strings.ToLower(strings.TrimSpace(ev.User.Email)); em != "" {
		u.EmailsSHA256 = []string{sha(em)}
	}
	if ph := phoneDigits(ev.User.Phone); len(ph) >= 8 && len(ph) <= 15 {
		u.PhonesSHA256 = []string{sha(ph)}
	}
	if ev.User.CustomerID != nil {
		u.ExternalIDsSHA256 = []string{sha(ev.User.CustomerID.String())}
	}
	if u.Obref != "" || u.IPAddress != "" || u.UserAgent != "" || len(u.EmailsSHA256)+len(u.PhonesSHA256)+len(u.ExternalIDsSHA256) > 0 {
		out.User = u
	}
	// shortcut: page_viewed and registration_completed are sent as customer_action, the shape app lifecycle events take
	// - check OpenAI's Supported Events page against a live account when one is set up
	out.Data = eventData{Type: "customer_action"}
	if e := ev.Ecommerce; e != nil && typ != "page_viewed" && typ != "registration_completed" {
		value := e.Value
		if typ == "order_created" {
			value += e.Tax + e.Shipping
		}
		amount := minor(value, e.Currency)
		out.Data = eventData{Type: "contents", Amount: &amount, Currency: e.Currency}
		for _, it := range e.Items {
			out.Data.Contents = append(out.Data.Contents, content{
				ID: it.ItemID, Name: it.ItemName, ContentType: "product", Quantity: int64(math.Max(1, math.Round(it.Quantity))),
				Amount: minor(it.Price, e.Currency), Currency: e.Currency,
			})
		}
	}
	return out
}

// ISO 4217 currencies whose minor unit isn't a hundredth - a plugin sees only the SDK, so it carries its own copy
var isoDecimals = map[string]int{
	"BIF": 0, "CLP": 0, "DJF": 0, "GNF": 0, "ISK": 0, "JPY": 0, "KMF": 0, "KRW": 0, "PYG": 0,
	"RWF": 0, "UGX": 0, "UYI": 0, "VND": 0, "VUV": 0, "XAF": 0, "XOF": 0, "XPF": 0,
	"BHD": 3, "IQD": 3, "JOD": 3, "KWD": 3, "LYD": 3, "OMR": 3, "TND": 3,
	"CLF": 4, "UYW": 4,
}

// Minor units by the currency's own decimals - 4200 for 42.00 EUR, 4200 for 4200 JPY
func minor(v float64, currency string) int64 {
	decimals, ok := isoDecimals[strings.ToUpper(currency)]
	if !ok {
		decimals = 2
	}
	return int64(math.Round(v * math.Pow10(decimals)))
}

// OpenAI's rule: keep the country code, drop every symbol, a leading + and leading zeros
func phoneDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimLeft(b.String(), "0")
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
