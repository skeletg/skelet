// Package trackingbing sends shop events to Microsoft Advertising's UET Conversions API, server to server. The event id
// is the one a UET tag on the page would use, so where both run Microsoft counts each conversion once
package trackingbing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	Slug   = "tracking-bing"
	Kind   = "bing_uet"
	maxAge = 90 * 24 * time.Hour // Microsoft refuses an older eventTime
)

var capiHost = "https://capi.uet.microsoft.com" // a var so tests can point it at a local server

// The retail page type Microsoft uses for remarketing, by GA4 event
var pageTypes = map[string]string{
	"view_item": "product", "view_item_list": "category", "view_cart": "cart", "add_to_cart": "cart",
	"begin_checkout": "cart", "purchase": "purchase",
}

func init() {
	p := sdk.RegisterPlugin(sdk.Manifest{Slug: Slug, Name: "Microsoft Advertising (UET Conversions API)", Version: "0.1.0", SDKRange: "^0.1.0"})
	p.RegisterDestination(sdk.Destination{
		Kind:          Kind,
		Label:         "Microsoft Advertising (Bing)",
		Purposes:      []sdk.DestinationPurpose{sdk.PurposeAds},
		DefaultEvents: []string{"add_to_cart", "begin_checkout", "page_view", "purchase", "sign_up", "view_item"},
		Fields: []sdk.DestinationField{
			{Key: "tag_id", Label: "UET tag ID", Required: true},
			{Key: "api_token", Label: "Conversions API token", Help: "UET > edit the tag > Set up tagging > Use Conversions API > Copy Token", Required: true, Secret: true},
		},
		Send: Send,
	})
}

func Send(ctx context.Context, client *http.Client, d sdk.DestinationConfig, ev sdk.ShopEvent) error {
	if ev.Name == "refund" { // a refund is a restate or retract, made through the Campaign Management API
		return nil
	}
	if time.Since(ev.OccurredAt) > maxAge {
		return sdk.Permanent(fmt.Errorf("%s is older than Microsoft's 90 days", ev.ID))
	}
	e, ok := toEvent(ev)
	if !ok {
		return sdk.Permanent(errors.New("no identifier Microsoft accepts - no visitor, click, account, email or phone"))
	}
	raw, err := json.Marshal(body{Data: []event{e}})
	if err != nil {
		return sdk.Permanent(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, capiHost+"/v1/"+url.PathEscape(d.Config["tag_id"])+"/events", bytes.NewReader(raw))
	if err != nil {
		return sdk.Permanent(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.Secret)
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return sdk.ResponseError(res)
}

type body struct {
	Data []event `json:"data"`
}

type event struct {
	EventType        string      `json:"eventType"`
	EventID          string      `json:"eventId,omitempty"`
	EventName        string      `json:"eventName,omitempty"`
	EventTime        int64       `json:"eventTime"`
	EventSourceURL   string      `json:"eventSourceUrl,omitempty"`
	ReferrerURL      string      `json:"referrerUrl,omitempty"`
	AdStorageConsent string      `json:"adStorageConsent"`
	UserData         userData    `json:"userData"`
	CustomData       *customData `json:"customData,omitempty"`
}

type userData struct {
	AnonymousID     string `json:"anonymousId,omitempty"`
	ExternalID      string `json:"externalId,omitempty"`
	Em              string `json:"em,omitempty"`
	Ph              string `json:"ph,omitempty"`
	ClientIPAddress string `json:"clientIpAddress,omitempty"`
	ClientUserAgent string `json:"clientUserAgent,omitempty"`
	Msclkid         string `json:"msclkid,omitempty"`
}

type customData struct {
	TransactionID   string   `json:"transactionId,omitempty"`
	Value           float64  `json:"value,omitempty"`
	Currency        string   `json:"currency,omitempty"`
	Items           []item   `json:"items,omitempty"`
	ItemIDs         []string `json:"itemIds,omitempty"`
	PageType        string   `json:"pageType,omitempty"`
	EcommTotalValue float64  `json:"ecommTotalValue,omitempty"`
}

type item struct {
	ID       string  `json:"id"`
	Quantity float64 `json:"quantity,omitempty"`
	Price    float64 `json:"price,omitempty"`
	Name     string  `json:"name,omitempty"`
}

// A page view is Microsoft's pageLoad event and needs the page; everything else is a custom event named as in GA4.
// ok is false when the event carries no identifier at all, which Microsoft refuses
func toEvent(ev sdk.ShopEvent) (event, bool) {
	e := event{
		EventType: "custom", EventID: ev.ID, EventName: ev.Name, EventTime: ev.OccurredAt.Unix(),
		EventSourceURL: ev.Context.PageURL, ReferrerURL: ev.Context.Referrer,
		AdStorageConsent: "G", // only an event with advertising consent reaches this destination
		UserData: userData{
			AnonymousID: ev.User.AnonID, Em: hashEmail(ev.User.Email), Ph: hashPhone(ev.User.Phone),
			ClientIPAddress: ev.Context.IP, ClientUserAgent: ev.Context.UserAgent,
			Msclkid: strings.ReplaceAll(ev.ClickIDs["msclkid"], "-", ""),
		},
	}
	if ev.Name == "page_view" && ev.Context.PageURL != "" {
		e.EventType, e.EventName = "pageLoad", ""
	}
	if ev.User.CustomerID != nil { // never a real user id - Microsoft asks for it anonymised
		e.UserData.ExternalID = sha(ev.User.CustomerID.String())
	}
	if c := ev.Ecommerce; c != nil && e.EventType == "custom" {
		cd := &customData{TransactionID: c.TransactionID, Currency: c.Currency, Value: c.Value + c.Tax + c.Shipping, PageType: pageTypes[ev.Name]}
		cd.EcommTotalValue = cd.Value
		for _, it := range c.Items {
			cd.Items = append(cd.Items, item{ID: it.ItemID, Quantity: it.Quantity, Price: it.Price, Name: it.ItemName})
			cd.ItemIDs = append(cd.ItemIDs, it.ItemID)
		}
		e.CustomData = cd
	}
	u := e.UserData
	return e, u.AnonymousID != "" || u.ExternalID != "" || u.Em != "" || u.Ph != "" || u.Msclkid != ""
}

// Microsoft's rule: trimmed, the local part without its dots or a +alias, lowercased, then hashed
func hashEmail(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	local, domain, ok := strings.Cut(s, "@")
	if !ok || local == "" || domain == "" {
		return ""
	}
	local, _, _ = strings.Cut(local, "+")
	return sha(strings.ReplaceAll(local, ".", "") + "@" + domain)
}

// E.164 with its country code, then hashed - a number typed without one can't be made E.164, so it isn't sent
func hashPhone(s string) string {
	s = strings.TrimSpace(s)
	var digits strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			digits.WriteRune(r)
		}
	}
	d := digits.String()
	switch {
	case strings.HasPrefix(s, "+") && d != "":
	case strings.HasPrefix(d, "00") && len(d) > 2:
		d = d[2:]
	default:
		return ""
	}
	return sha("+" + d)
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
