package admin

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/adminaction"
	"github.com/skeletg/skelet/apps/backend/core/internal/adminprefs"
	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/audit"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog/preview"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/chat"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/country"
	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/datajob"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/form"
	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/internal/health"
	"github.com/skeletg/skelet/apps/backend/core/internal/integration"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/linkgraph"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/loyalty"
	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/internal/menu"
	"github.com/skeletg/skelet/apps/backend/core/internal/notification"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/ordermail"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/qna"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/internal/revalidate"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/core/internal/sales"
	"github.com/skeletg/skelet/apps/backend/core/internal/scheduler"
	"github.com/skeletg/skelet/apps/backend/core/internal/searchmerch"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/sitecrawl"
	"github.com/skeletg/skelet/apps/backend/core/internal/storecredit"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/core/internal/webform"
	"github.com/skeletg/skelet/apps/backend/core/internal/webhook"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
)

type Resolver struct {
	AdminSvc          *admin.Service
	IntegrationSvc    *integration.Service
	CustomerSvc       *customer.Service
	StoreCreditSvc    *storecredit.Service
	LoyaltySvc        *loyalty.Service
	QnaSvc            *qna.Service
	SearchMerchSvc    *searchmerch.Service
	SignalsEnabled    bool // mirrors SIGNALS_ENABLED - false rejects every Signals-suite resolver (Pulse/Lens/Synapse/Prism/spine)
	CartSvc           *cart.Service
	OrderSvc          *order.Service
	OrderMailSvc      *ordermail.Service // the resend path - flows own the automatic sends, this is the operator's manual one
	ConfigSvc         *config.Service
	ConsentSvc        *consent.Service
	ActionSvc         *adminaction.Service
	DataJobSvc        *datajob.Service // the one seam behind every bulk import and export
	AuditSvc          *audit.Service
	PrefsSvc          *adminprefs.Service
	Queries           *db.Queries
	WebhookSvc        *webhook.Service
	RemoteProviderSvc *remoteprovider.Service
	CatalogSvc        *catalog.Service
	BlogSvc           *blog.Service
	CMSSvc            *cms.Service
	AISvc             *ai.Service
	UrlRewriteSvc     *urlrewrite.Service
	LinkGraphSvc      *linkgraph.Service
	SiteCrawlSvc      *sitecrawl.Service
	ChannelSvc        *channel.Service
	InventorySvc      *inventory.Service
	LocaleSvc         *locale.Service
	CurrencySvc       *currency.Service
	CountrySvc        *country.Service
	MediaLibrarySvc   *medialibrary.Service
	MenuSvc           *menu.Service
	ReviewSvc         *review.Service
	TaxSvc            *tax.Service
	ShippingSvc       *shipping.Service
	PaymentSvc        *payment.Service
	PromotionSvc      *promotion.Service
	GiftcardSvc       *giftcard.Service
	SubscriptionSvc   *subscription.Service
	SalesSvc          *sales.Service
	// Core's plug into the Signals service - Pulse, the chat's suite tools and every other Signals read relay through it
	SignalsAPI       *signalsclient.Client
	NotificationSvc  *notification.Service
	NotificationHub  *notification.Hub // live push fan-out for the notificationAdded subscription
	FormSvc          *form.Service
	WebFormSvc       *webform.Service
	ChatSvc          *chat.Service
	CacheClient      *revalidate.Client
	CacheQueue       *revalidate.Queue
	RefreshStore     *auth.RefreshStore // admin refresh tokens
	BlogPreviewStore *preview.Store     // short-lived blog preview tokens
	CMSPreviewStore  *preview.Store     // short-lived CMS page preview tokens
	JWTSecret        string
	JWTTTL           time.Duration

	RateLimiter   ratelimit.Limiter   // shares the Valkey-backed counters with the storefront limiter so every pod sees the same budget - nil disables the check, which is what tests do
	AuthLimiter   ratelimit.Limiter   // tighter limiter for the admin login surface
	LoginThrottle *auth.LoginThrottle // locks an admin email after sustained failed logins
	Scheduler     *scheduler.Service  // backs the scheduled-jobs admin surface
	Health        *health.Registry    // every subsystem's probe, behind the one operator panel

	DevMode bool // set from APP_ENV=development in main.go - the dev identity-switching resolvers hard-error when false, so that surface is inert in production even though the schema carries it
}

// Folds the `Locale` header that internal/locale.Middleware put on context together with the system default, so read resolvers don't each redo it
// Returns "" when nothing resolves - services read that as "no preference" and fall through their populate*Locale cascades
func (r *Resolver) activeLocale(ctx context.Context) string {
	return locale.Active(ctx, defaultChannelLocale{r.ChannelSvc})
}

// For VIEWING resolvers only - explicit channelId wins, then the ambient scope channel channel.AdminMiddleware put on context, and nil means "all channels"
// Channel-targeting mutations take a required channelId and act on it directly instead
func (r *Resolver) scopedChannelID(ctx context.Context, arg *string) (*uuid.UUID, error) {
	if arg != nil {
		if s := strings.TrimSpace(*arg); s != "" {
			id, err := uuid.Parse(s)
			if err != nil {
				return nil, errInvalidID("channelId", err)
			}
			return &id, nil
		}
	}
	if sv, ok := channel.FromContext(ctx); ok && sv != nil {
		id := sv.ID
		return &id, nil
	}
	return nil, nil
}

// Last resort when no Locale header resolved one - the DEFAULT channel's default_locale, the same anchor the catalog's write-side canonical-locale invariant uses, falling to "en" only when no channel is flagged default
type defaultChannelLocale struct{ channels *channel.Service } // GetDefault is Valkey-cached, so this stays cheap

func (c defaultChannelLocale) DefaultLocale(ctx context.Context) (string, error) {
	if c.channels != nil {
		if sv, err := c.channels.GetDefault(ctx); err == nil && sv.DefaultLocale != "" {
			return sv.DefaultLocale, nil
		}
	}
	return "en", nil
}
