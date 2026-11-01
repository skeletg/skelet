// Package tracking sends shop events server to server to analytics and ad platforms - GA4 and server-side GTM, Meta,
// Bing, Klaviyo, the affiliate networks. Each platform is a destination kind, registered in code; core owns the queue,
// the consent check, retries, the delivery log and the secrets, so a kind only maps an event and makes the call.
// Nothing here depends on Signals, which keeps working on its own feed
package tracking

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Purpose is a cookie category an event needs before a destination may receive it
type Purpose string

const (
	PurposeNecessary       Purpose = "necessary" // needs no consent - only an affiliate network the merchant made necessary
	PurposeAnalytics       Purpose = "analytics"
	PurposeAds             Purpose = "ads"
	PurposePersonalization Purpose = "personalization"
)

// Event names are GA4's, which most platforms map from cleanly
const (
	PageView        = "page_view"
	ViewItemList    = "view_item_list"
	SelectItem      = "select_item"
	ViewItem        = "view_item"
	ViewCart        = "view_cart"
	BeginCheckout   = "begin_checkout"
	AddShippingInfo = "add_shipping_info"
	AddPaymentInfo  = "add_payment_info"
	AddToCart       = "add_to_cart"
	RemoveFromCart  = "remove_from_cart"
	Purchase        = "purchase"
	Refund          = "refund"
	SignUp          = "sign_up"
)

// What a browser may report. Cart changes, purchases, refunds and sign-ups happen on the server, which reports them itself -
// a browser copy would be forgeable and counted twice
var browserEvents = map[string]bool{
	PageView: true, ViewItemList: true, SelectItem: true, ViewItem: true, ViewCart: true,
	BeginCheckout: true, AddShippingInfo: true, AddPaymentInfo: true,
}

func BrowserEvent(name string) bool { return browserEvents[name] }

// Event is one shop event in a common envelope: the GA4 ecommerce payload plus who, where and under what consent.
// Nothing in it is hashed - each destination hashes what its platform asks for
type Event struct {
	ID         string            `json:"event_id"` // shared with the browser's copy so a platform can dedupe; a purchase's is the order id
	Name       string            `json:"name"`
	OccurredAt time.Time         `json:"occurred_at"`
	ChannelID  uuid.UUID         `json:"channel_id"`
	Ecommerce  *Ecommerce        `json:"ecommerce,omitempty"`
	User       User              `json:"user"`
	Context    Context           `json:"context"`
	ClickIDs   map[string]string `json:"click_ids,omitempty"`
	Consent    *auth.Consent     `json:"consent,omitempty"` // nil: no banner applied, so only a destination needing no consent gets it
	Source     string            `json:"source"`            // browser | server
}

type Ecommerce struct {
	Currency      string  `json:"currency"`
	Value         float64 `json:"value"` // the items after discount, without tax or shipping - GA4's meaning
	TransactionID string  `json:"transaction_id,omitempty"`
	Tax           float64 `json:"tax,omitempty"`
	Shipping      float64 `json:"shipping,omitempty"`
	Coupon        string  `json:"coupon,omitempty"`
	Items         []Item  `json:"items,omitempty"`
}

type Item struct {
	ItemID       string  `json:"item_id"` // the SKU, everywhere
	ItemName     string  `json:"item_name,omitempty"`
	ItemBrand    string  `json:"item_brand,omitempty"`
	ItemCategory string  `json:"item_category,omitempty"`
	ItemVariant  string  `json:"item_variant,omitempty"`
	Price        float64 `json:"price,omitempty"`
	Quantity     float64 `json:"quantity,omitempty"`
	Discount     float64 `json:"discount,omitempty"`
}

type User struct {
	CustomerID *uuid.UUID `json:"customer_id,omitempty"`
	AnonID     string     `json:"anon_id,omitempty"`
	Email      string     `json:"email,omitempty"`
	Phone      string     `json:"phone,omitempty"`
	// The email may receive marketing - the account's consent, or a guest's opt-in. Marketing platforms that build a
	// profile from the address need it; false wherever the event carries no email
	MarketingOptIn bool `json:"marketing_opt_in,omitempty"`
}

type Context struct {
	IP        string `json:"ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	PageURL   string `json:"page_url,omitempty"`
	Referrer  string `json:"referrer,omitempty"`
	Locale    string `json:"locale,omitempty"`
}

// WithShopper fills in the visitor - each part is already there only under the consent it needs
func (e *Event) WithShopper(s *auth.Shopper) {
	if s == nil {
		return
	}
	c := s.Consent
	e.Consent = &c
	e.ClickIDs = s.ClickIDs
	e.User.AnonID = s.AnonID
	e.Context.IP, e.Context.UserAgent = s.IP, s.UserAgent
}

// Decimal turns minor units into the decimal amount platforms take, by the currency's ISO decimals
func Decimal(minor int64, currency string) float64 {
	return float64(minor) / math.Pow10(money.ISODecimals(currency))
}

// Allowed is the consent check: every purpose the destination needs must be granted. An event with no consent stated
// reaches only a destination that needs none
func Allowed(purposes []Purpose, c *auth.Consent) bool {
	for _, p := range purposes {
		switch p {
		case PurposeNecessary:
			continue
		case PurposeAnalytics:
			if c == nil || !c.Analytics {
				return false
			}
		case PurposeAds:
			if c == nil || !c.Ads {
				return false
			}
		case PurposePersonalization:
			if c == nil || !c.Personalization {
				return false
			}
		default:
			return false // a purpose this build doesn't know can't be shown to be granted
		}
	}
	return true
}

// Destination is one configured account on a platform, with its secret decrypted
type Destination struct {
	ID        uuid.UUID
	ChannelID uuid.UUID
	Kind      string
	Name      string
	Enabled   bool
	Events    []string // empty: the kind's defaults
	Config    map[string]string
	Secret    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Kind is the code that speaks to one platform
type Kind struct {
	Kind   string
	Label  string
	Plugin string                 // the plugin that registered it, empty for core's own
	Fields []sdk.DestinationField // the admin form for a configured destination
	// Whether a channel may use it at all - a plugin switched off there sends nothing. Nil: everywhere
	Available     func(ctx context.Context, channelID uuid.UUID) bool
	Purposes      []Purpose // what every event it receives needs - the row can't change it
	DefaultEvents []string  // empty: every event
	// A setting that, when on, makes the kind need no consent - an affiliate network the merchant treats as necessary
	NecessarySetting string
	// Send makes one call. A PermanentError is never retried; anything else retries on the ladder
	Send func(ctx context.Context, client *http.Client, d Destination, ev Event) error
}

// PermanentError is a refusal a retry can't fix - a 4xx, a payload the platform rejects
type PermanentError struct{ Err error }

func (e PermanentError) Error() string { return e.Err.Error() }
func (e PermanentError) Unwrap() error { return e.Err }

func Permanent(err error) error { return PermanentError{Err: err} }

func isPermanent(err error) bool {
	var p PermanentError
	return errors.As(err, &p)
}

var (
	kindsMu sync.RWMutex
	kinds   = map[string]Kind{}
)

// RegisterKind is called from init or a plugin's setup - a second registration of the same kind is a programming error
func RegisterKind(k Kind) {
	kindsMu.Lock()
	defer kindsMu.Unlock()
	if k.Kind == "" || k.Send == nil {
		panic("tracking: a kind needs a name and a Send")
	}
	if _, dup := kinds[k.Kind]; dup {
		panic("tracking: kind registered twice: " + k.Kind)
	}
	kinds[k.Kind] = k
}

func KindFor(name string) (Kind, bool) {
	kindsMu.RLock()
	defer kindsMu.RUnlock()
	k, ok := kinds[name]
	return k, ok
}

func Kinds() []Kind {
	kindsMu.RLock()
	defer kindsMu.RUnlock()
	out := make([]Kind, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}
