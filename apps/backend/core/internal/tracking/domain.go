// Package tracking sends shop events server to server to analytics and ad platforms - GA4 and server-side GTM, Meta,
// Bing, Klaviyo, the affiliate networks. Each platform is a destination kind, registered in code; core owns the queue,
// the consent check, retries, the delivery log and the secrets, so a kind only maps an event and makes the call.
// Nothing here depends on Signals, which keeps working on its own feed
package tracking

import (
	"context"
	"math"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Purpose is a cookie category an event needs before a destination may receive it - the SDK's, which plugins declare
type Purpose = sdk.DestinationPurpose

const (
	PurposeNecessary       = sdk.PurposeNecessary // needs no consent - only an affiliate network the merchant made necessary
	PurposeAnalytics       = sdk.PurposeAnalytics
	PurposeAds             = sdk.PurposeAds
	PurposePersonalization = sdk.PurposePersonalization
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
	Login           = "login"
	AddToWishlist   = "add_to_wishlist"
	ViewPromotion   = "view_promotion"
	SelectPromotion = "select_promotion"
	Search          = "search"
	GenerateLead    = "generate_lead" // a sign-up form was submitted, before its address is confirmed
	// A customer withdrew email marketing, or their account was deleted or erased. Server-only. A destination that
	// lists it in its default events is built to remove the person, so it gets it whatever the visitor's cookie choice -
	// taking someone off a list never needs their consent
	MarketingOptOut = "marketing_opt_out"
)

// What a browser may report: what it saw and clicked. Cart changes, checkout choices, purchases, refunds, sign-ins and
// sign-ups happen on the server, which reports them itself - a browser copy would be forgeable and counted twice
var browserEvents = map[string]bool{
	PageView: true, ViewItemList: true, SelectItem: true, ViewItem: true, ViewCart: true, BeginCheckout: true,
	ViewPromotion: true, SelectPromotion: true, Search: true,
}

func BrowserEvent(name string) bool { return browserEvents[name] }

// Event is one shop event in a common envelope: the GA4 ecommerce payload plus who, where and under what consent.
// Nothing in it is hashed - each destination hashes what its platform asks for. It is the SDK's own type, so what core
// queues is exactly what a plugin receives, and a field can't be added on one side only
type (
	Event     = sdk.ShopEvent
	Ecommerce = sdk.ShopEcommerce
	Item      = sdk.ShopItem
	User      = sdk.ShopUser
	Context   = sdk.ShopContext
)

// WithShopper fills in the visitor - each part is already there only under the consent it needs
func WithShopper(e *Event, s *auth.Shopper) {
	if s == nil {
		return
	}
	e.Consent = &sdk.ShopConsent{Analytics: s.Consent.Analytics, Ads: s.Consent.Ads, Personalization: s.Consent.Personalization}
	e.ClickIDs = s.ClickIDs
	e.User.AnonID, e.Context.SessionID = s.AnonID, s.SessionID
	e.Context.IP, e.Context.UserAgent = s.IP, s.UserAgent
}

// Decimal turns minor units into the decimal amount platforms take, by the currency's ISO decimals
func Decimal(minor int64, currency string) float64 {
	return float64(minor) / math.Pow10(money.ISODecimals(currency))
}

// Allowed is the consent check: every purpose the destination needs must be granted. An event with no consent stated
// reaches only a destination that needs none
func Allowed(purposes []Purpose, c *sdk.ShopConsent) bool {
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
	ID         uuid.UUID
	ChannelID  uuid.UUID
	Kind       string
	Name       string
	Enabled    bool
	Events     []string // empty: the kind's defaults
	Config     map[string]string
	Secret     string
	LastSentAt *time.Time // stamped at most once a minute; nil: never
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Health is a destination's recent failures: the latest and how many failed in a day
type Health struct {
	LastError string // already redacted by the worker
	Dead24h   int
}

// Kind is a plugin's destination as it registered it, plus the plugin it came from and where that plugin is on
type Kind struct {
	sdk.Destination
	Plugin string // the plugin that registered it
	// Whether a channel may use it at all - a plugin switched off there sends nothing. Nil: everywhere
	Available func(ctx context.Context, channelID uuid.UUID) bool
}

// takesRegardless is a kind built to act on an opt-out - one that names it among its default events gets it with no
// consent check, since taking someone off a list never needs their consent. Anything else, such as a webhook taking
// every event, still needs the visitor's consent for it
func (k Kind) takesRegardless(name string) bool {
	return name == MarketingOptOut && slices.Contains(k.DefaultEvents, name)
}

// config is the destination as its plugin's Send takes it
func (d Destination) config() sdk.DestinationConfig {
	return sdk.DestinationConfig{ID: d.ID, ChannelID: d.ChannelID, Name: d.Name, Config: d.Config, Secret: d.Secret}
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
