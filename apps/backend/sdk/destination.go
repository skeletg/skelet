package sdk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// A destination sends shop events server to server to one analytics or ad platform. Core owns everything around the
// call - the queue, the consent check, retries and dead letters, the delivery log, the SSRF-safe client and the secret -
// so a destination only maps an event to its platform's request and makes it

// DestinationPurpose is a cookie category every event a destination receives must have been granted
type DestinationPurpose string

const (
	PurposeNecessary       DestinationPurpose = "necessary" // needs no consent - only for a network the merchant made necessary
	PurposeAnalytics       DestinationPurpose = "analytics"
	PurposeAds             DestinationPurpose = "ads"
	PurposePersonalization DestinationPurpose = "personalization"
)

// ShopEvent is one shop event: GA4's event names and ecommerce payload, plus who, where and under what consent.
// Nothing is hashed - each destination hashes what its platform asks for
type ShopEvent struct {
	ID         string            `json:"event_id"` // shared with the browser's copy, so pass it as the platform's dedupe key
	Name       string            `json:"name"`     // page_view, view_item, add_to_cart, begin_checkout, purchase, refund, sign_up, ...
	OccurredAt time.Time         `json:"occurred_at"`
	ChannelID  uuid.UUID         `json:"channel_id"`
	Ecommerce  *ShopEcommerce    `json:"ecommerce,omitempty"`
	User       ShopUser          `json:"user"`
	Context    ShopContext       `json:"context"`
	ClickIDs   map[string]string `json:"click_ids,omitempty"` // gclid, gbraid, wbraid, msclkid, ttclid, fbc, fbp, awc, cjevent
	Consent    *ShopConsent      `json:"consent,omitempty"`   // nil: placed with no banner, by an admin or an API client
	Source     string            `json:"source"`              // browser | server
}

type ShopEcommerce struct {
	Currency      string     `json:"currency"`
	Value         float64    `json:"value"` // items after discount, without tax or shipping
	TransactionID string     `json:"transaction_id,omitempty"`
	Tax           float64    `json:"tax,omitempty"`
	Shipping      float64    `json:"shipping,omitempty"`
	Coupon        string     `json:"coupon,omitempty"`
	Items         []ShopItem `json:"items,omitempty"`
}

type ShopItem struct {
	ItemID       string  `json:"item_id"` // the SKU
	ItemName     string  `json:"item_name,omitempty"`
	ItemBrand    string  `json:"item_brand,omitempty"`
	ItemCategory string  `json:"item_category,omitempty"`
	ItemVariant  string  `json:"item_variant,omitempty"`
	Price        float64 `json:"price,omitempty"`
	Quantity     float64 `json:"quantity,omitempty"`
	Discount     float64 `json:"discount,omitempty"`
}

type ShopUser struct {
	CustomerID *uuid.UUID `json:"customer_id,omitempty"`
	AnonID     string     `json:"anon_id,omitempty"`
	Email      string     `json:"email,omitempty"`
	Phone      string     `json:"phone,omitempty"`
	// The email may receive marketing - the account's consent, or a guest's opt-in. A destination that builds a
	// marketing profile from the address must not send the event without it
	MarketingOptIn bool `json:"marketing_opt_in,omitempty"`
}

type ShopContext struct {
	IP        string `json:"ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	PageURL   string `json:"page_url,omitempty"`
	Referrer  string `json:"referrer,omitempty"`
	Locale    string `json:"locale,omitempty"`
	PageTitle string `json:"page_title,omitempty"`
	SessionID string `json:"session_id,omitempty"` // the storefront's analytics session, its start in seconds - GA4's session_id
}

type ShopConsent struct {
	Analytics       bool `json:"analytics"`
	Ads             bool `json:"ads"`
	Personalization bool `json:"personalization"`
}

// DestinationField is one setting on the admin form for a configured destination. A Secret field is stored encrypted,
// never shown back, and arrives in DestinationConfig.Secret - a destination has at most one
type DestinationField struct {
	Key      string
	Label    string
	Help     string
	Required bool
	Secret   bool
}

// DestinationConfig is one configured account: what the admin entered, with the secret decrypted
type DestinationConfig struct {
	ID        uuid.UUID
	ChannelID uuid.UUID
	Name      string
	Config    map[string]string
	Secret    string
}

type Destination struct {
	Kind          string // stored on each configured destination - never rename one that has shipped
	Label         string
	Purposes      []DestinationPurpose // every event it receives must have all of these granted
	DefaultEvents []string             // what a new destination receives; empty means every event
	// A setting that, when on for the channel, makes the destination need no consent - an affiliate network the merchant
	// treats as necessary. Whether that is lawful is the merchant's call, so it is off unless they turn it on
	NecessarySetting string
	Fields           []DestinationField
	// Send makes one call. Return Permanent(err) for a refusal a retry can't fix (a 4xx); anything else is retried
	Send func(ctx context.Context, client *http.Client, d DestinationConfig, ev ShopEvent) error
}

// PermanentError marks a refusal a retry can't fix
type PermanentError struct{ Err error }

func (e PermanentError) Error() string { return e.Err.Error() }
func (e PermanentError) Unwrap() error { return e.Err }

func Permanent(err error) error { return PermanentError{Err: err} }

// ResponseError reads a platform's answer: nil for a 2xx, a permanent error for a 4xx a retry can't fix, and a retryable
// one for a 5xx, a 408 or a 429. The body's first 300 bytes go in the message, for the delivery log
func ResponseError(res *http.Response) error {
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 300))
	err := fmt.Errorf("%s: %d %s", res.Request.URL.Host, res.StatusCode, strings.TrimSpace(string(body)))
	if res.StatusCode >= 400 && res.StatusCode < 500 && res.StatusCode != http.StatusRequestTimeout && res.StatusCode != http.StatusTooManyRequests {
		return Permanent(err)
	}
	return err
}

func IsPermanent(err error) bool {
	var p PermanentError
	return errors.As(err, &p)
}

// RegisteredDestination is a destination with the plugin that registered it
type RegisteredDestination struct {
	PluginSlug  string
	Destination Destination
}

var (
	destinationsMu sync.Mutex
	destinations   []RegisteredDestination
)

// Registers a platform the shop can send events to
//
//	p.RegisterDestination(sdk.Destination{Kind: "ga4", Label: "Google Analytics 4", Purposes: ..., Send: send})
func (pl *Plugin) RegisterDestination(d Destination) {
	if d.Kind == "" || d.Send == nil {
		panic("sdk.RegisterDestination: a destination needs a Kind and a Send")
	}
	secrets := 0
	for _, f := range d.Fields {
		if f.Secret {
			secrets++
		}
	}
	if secrets > 1 {
		panic("sdk.RegisterDestination: " + d.Kind + " declares more than one secret field")
	}
	destinationsMu.Lock()
	defer destinationsMu.Unlock()
	destinations = append(destinations, RegisteredDestination{PluginSlug: pl.slug, Destination: d})
}

// Destinations is every destination the compiled-in plugins registered - core reads it once at boot
func Destinations() []RegisteredDestination {
	destinationsMu.Lock()
	defer destinationsMu.Unlock()
	return append([]RegisteredDestination(nil), destinations...)
}
