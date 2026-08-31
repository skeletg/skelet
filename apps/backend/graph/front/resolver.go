package front

import (
	"context"
	"sync"
	"time"

	"github.com/skeletg/skelet/apps/backend/internal/address"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/b2b"
	"github.com/skeletg/skelet/apps/backend/internal/blog"
	"github.com/skeletg/skelet/apps/backend/internal/blog/preview"
	"github.com/skeletg/skelet/apps/backend/internal/cart"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/cms"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/consent"
	"github.com/skeletg/skelet/apps/backend/internal/country"
	"github.com/skeletg/skelet/apps/backend/internal/currency"
	"github.com/skeletg/skelet/apps/backend/internal/customer"
	"github.com/skeletg/skelet/apps/backend/internal/download"
	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/internal/locale"
	"github.com/skeletg/skelet/apps/backend/internal/loyalty"
	"github.com/skeletg/skelet/apps/backend/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/internal/menu"
	"github.com/skeletg/skelet/apps/backend/internal/order"
	"github.com/skeletg/skelet/apps/backend/internal/payment"
	"github.com/skeletg/skelet/apps/backend/internal/paymentvault"
	"github.com/skeletg/skelet/apps/backend/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/internal/qna"
	"github.com/skeletg/skelet/apps/backend/internal/ratelimit"
	"github.com/skeletg/skelet/apps/backend/internal/referral"
	"github.com/skeletg/skelet/apps/backend/internal/review"
	"github.com/skeletg/skelet/apps/backend/internal/search"
	"github.com/skeletg/skelet/apps/backend/internal/searchmerch"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
	"github.com/skeletg/skelet/apps/backend/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/internal/storecredit"
	"github.com/skeletg/skelet/apps/backend/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/internal/tax"
	"github.com/skeletg/skelet/apps/backend/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/internal/webform"
	"github.com/skeletg/skelet/apps/backend/internal/wishlist"
)

// Admin-only services (admin.Service, integration, webhook, inventory writes, …) are deliberately absent so a mis-typed front resolver can't reach an admin code path
type Resolver struct {
	CustomerSvc          *customer.Service
	StoreCreditSvc       *storecredit.Service
	GiftcardSvc          *giftcard.Service
	MediaPublicBase      string // storage public-URL prefix - transformUrl derives object keys by trimming it off full URLs
	LoyaltySvc           *loyalty.Service
	ReferralSvc          *referral.Service
	PaymentVaultSvc      *paymentvault.Service
	AddressSvc           *address.Service
	SearchMerchSvc       *searchmerch.Service
	B2BSvc               *b2b.Service
	SignalsEnabled       bool // mirrors SIGNALS_ENABLED (features.signals)
	QnaSvc               *qna.Service
	CartSvc              *cart.Service
	OrderSvc             *order.Service
	ConfigSvc            *config.Service
	ConsentSvc           *consent.Service
	CatalogSvc           *catalog.Service
	BlogSvc              *blog.Service
	CMSSvc               *cms.Service
	ChannelSvc           *channel.Service
	InventorySvc         *inventory.Service
	LocaleSvc            *locale.Service
	CurrencySvc          *currency.Service
	CountrySvc           *country.Service
	CustomerRefreshStore *auth.RefreshStore
	BlogPreviewStore     *preview.Store      // validates short-lived admin-minted preview tokens, used only by previewBlogPost(token)
	CMSPreviewStore      *preview.Store      // same, for previewCMSPage(token)
	URLRewriteSvc        *urlrewrite.Service // backs the catch-all route resolver - maps a storefront path to its entity (product / category / collection / blog post / CMS page) through url_rewrites
	WishlistSvc          *wishlist.Service
	MenuSvc              *menu.Service
	ProductSearch        search.Engine           // the Engine interface, not the OpenSearch indexer - main wires a tier that falls back to Postgres, so the resolver never branches on availability
	BlogPostSearch       *search.BlogPostIndexer // the indexer doubles as the read handle; optional - nil means the search resolvers return empty results because the cluster is off or was unavailable at boot
	PromotionSvc         *promotion.Service
	SegmentSvc           *segment.Service
	ShippingSvc          *shipping.Service
	TaxSvc               *tax.Service
	PaymentSvc           *payment.Service
	ReviewSvc            *review.Service       // storefront-readable on the PDP AND writable via submit
	MediaLibrarySvc      *medialibrary.Service // backs review photo + video uploads through the same presign/confirm pair the admin uses for product media
	WebFormSvc           *webform.Service
	DownloadSvc          *download.Service
	SubscriptionSvc      *subscription.Service
	JWTSecret            string
	JWTTTL               time.Duration
	VoterTokenSecret     []byte              // signs anon review-vote cookie tokens - the storefront mints `<uuid>.<hmac>` server-side with the same secret and the backend verifies it, so nobody can forge a UUID and stuff the counts
	RateLimiter          ratelimit.Limiter   // per-IP cap on abuse-prone ops (login, password reset, signup, review submit/vote); optional - nil falls through, which is what tests and CLI paths get, while production wires ratelimit.NewRedis so every instance shares one set of counters
	AuthLimiter          ratelimit.Limiter   // lower per-window cap than RateLimiter, reserved for credential surfaces
	LoginThrottle        *auth.LoginThrottle // locks an email after sustained failed logins, counting failures across rate windows; nil disables it

	Bus *events.Bus // storefront-surface domain events (just the throttle-tripped admin-inbox signal today); nil drops them, matching tests and CLI

	catalogRate sync.Map // per-channel catalog-query throttle config at ~15s TTL so the hottest reads never pay a per-request config lookup - zero value ready, nothing to wire in main.go
}

// channel.LocaleClampMiddleware has already snapped the Locale header to the active channel's supported set, so the context value wins in practice and the provider below only fires for channel-less requests
func (r *Resolver) activeLocale(ctx context.Context) string {
	return locale.Active(ctx, defaultChannelLocale{r.ChannelSvc})
}

type defaultChannelLocale struct{ channels *channel.Service } // last resort when no Locale header and no channel resolved: the DEFAULT channel's default_locale, falling to "en" only when nothing is flagged default - GetDefault is Valkey-cached so it stays cheap

func (c defaultChannelLocale) DefaultLocale(ctx context.Context) (string, error) {
	if c.channels != nil {
		if sv, err := c.channels.GetDefault(ctx); err == nil && sv.DefaultLocale != "" {
			return sv.DefaultLocale, nil
		}
	}
	return "en", nil
}
