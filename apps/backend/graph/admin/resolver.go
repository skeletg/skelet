package admin

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/action"
	"github.com/skeletg/skelet/apps/backend/internal/admin"
	"github.com/skeletg/skelet/apps/backend/internal/adminprefs"
	"github.com/skeletg/skelet/apps/backend/internal/ai"
	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/analytics/spinequeue"
	"github.com/skeletg/skelet/apps/backend/internal/audit"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/blog"
	"github.com/skeletg/skelet/apps/backend/internal/blog/preview"
	"github.com/skeletg/skelet/apps/backend/internal/campaign"
	"github.com/skeletg/skelet/apps/backend/internal/cart"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/chat"
	"github.com/skeletg/skelet/apps/backend/internal/cms"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/consent"
	"github.com/skeletg/skelet/apps/backend/internal/country"
	"github.com/skeletg/skelet/apps/backend/internal/currency"
	"github.com/skeletg/skelet/apps/backend/internal/customer"
	"github.com/skeletg/skelet/apps/backend/internal/datajob"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/destination"
	"github.com/skeletg/skelet/apps/backend/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/internal/flow"
	"github.com/skeletg/skelet/apps/backend/internal/form"
	"github.com/skeletg/skelet/apps/backend/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/internal/health"
	"github.com/skeletg/skelet/apps/backend/internal/integration"
	"github.com/skeletg/skelet/apps/backend/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/internal/lens"
	"github.com/skeletg/skelet/apps/backend/internal/linkgraph"
	"github.com/skeletg/skelet/apps/backend/internal/locale"
	"github.com/skeletg/skelet/apps/backend/internal/loyalty"
	"github.com/skeletg/skelet/apps/backend/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/internal/menu"
	"github.com/skeletg/skelet/apps/backend/internal/notification"
	"github.com/skeletg/skelet/apps/backend/internal/order"
	"github.com/skeletg/skelet/apps/backend/internal/ordermail"
	"github.com/skeletg/skelet/apps/backend/internal/payment"
	"github.com/skeletg/skelet/apps/backend/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/internal/qna"
	"github.com/skeletg/skelet/apps/backend/internal/ratelimit"
	"github.com/skeletg/skelet/apps/backend/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/internal/revalidate"
	"github.com/skeletg/skelet/apps/backend/internal/review"
	"github.com/skeletg/skelet/apps/backend/internal/sales"
	"github.com/skeletg/skelet/apps/backend/internal/scheduler"
	"github.com/skeletg/skelet/apps/backend/internal/searchmerch"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
	"github.com/skeletg/skelet/apps/backend/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/internal/sitecrawl"
	"github.com/skeletg/skelet/apps/backend/internal/storecredit"
	"github.com/skeletg/skelet/apps/backend/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/internal/suppression"
	"github.com/skeletg/skelet/apps/backend/internal/tax"
	"github.com/skeletg/skelet/apps/backend/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/internal/webform"
	"github.com/skeletg/skelet/apps/backend/internal/webhook"
)

type Resolver struct {
	AdminSvc             *admin.Service
	IntegrationSvc       *integration.Service
	CustomerSvc          *customer.Service
	StoreCreditSvc       *storecredit.Service
	LoyaltySvc           *loyalty.Service
	QnaSvc               *qna.Service
	SearchMerchSvc       *searchmerch.Service
	SignalsEnabled       bool // mirrors SIGNALS_ENABLED - false rejects every Signals-suite resolver (Pulse/Lens/Synapse/Prism/spine)
	CartSvc              *cart.Service
	OrderSvc             *order.Service
	OrderMailSvc         *ordermail.Service // the resend path - flows own the automatic sends, this is the operator's manual one
	ConfigSvc            *config.Service
	ConsentSvc           *consent.Service
	ActionSvc            *action.Service
	DataJobSvc           *datajob.Service // the one seam behind every bulk import and export
	AuditSvc             *audit.Service
	PrefsSvc             *adminprefs.Service
	Queries              *db.Queries
	WebhookSvc           *webhook.Service
	RemoteProviderSvc    *remoteprovider.Service
	DestinationSvc       *destination.Service
	SignalsQueue         *spinequeue.Queue // nil unless SIGNALS_QUEUE_URL is set - the health panel reads stream depth and, more importantly, whether anything consumes it
	CatalogSvc           *catalog.Service
	BlogSvc              *blog.Service
	CMSSvc               *cms.Service
	AISvc                *ai.Service
	UrlRewriteSvc        *urlrewrite.Service
	LinkGraphSvc         *linkgraph.Service
	SiteCrawlSvc         *sitecrawl.Service
	ChannelSvc           *channel.Service
	InventorySvc         *inventory.Service
	LocaleSvc            *locale.Service
	CurrencySvc          *currency.Service
	CountrySvc           *country.Service
	MediaLibrarySvc      *medialibrary.Service
	MenuSvc              *menu.Service
	ReviewSvc            *review.Service
	TaxSvc               *tax.Service
	ShippingSvc          *shipping.Service
	PaymentSvc           *payment.Service
	PromotionSvc         *promotion.Service
	SegmentSvc           *segment.Service
	GiftcardSvc          *giftcard.Service
	SubscriptionSvc      *subscription.Service
	SalesSvc             *sales.Service
	EmailTmplSvc         *emailtemplate.Service
	CampaignSvc          *campaign.Service
	FlowSvc              *flow.Service
	AnalyticsSvc         *analytics.Recorder
	LensSvc              *lens.Service
	ExperimentSvc        *experiment.Service
	NotificationSvc      *notification.Service
	NotificationHub      *notification.Hub // live push fan-out for the notificationAdded subscription
	SuppressionSvc       *suppression.Service
	FormSvc              *form.Service
	WebFormSvc           *webform.Service
	ChatSvc              *chat.Service
	CacheClient          *revalidate.Client
	CacheQueue           *revalidate.Queue
	RefreshStore         *auth.RefreshStore // admin refresh tokens
	CustomerRefreshStore *auth.RefreshStore // customer refresh tokens
	BlogPreviewStore     *preview.Store     // short-lived blog preview tokens
	CMSPreviewStore      *preview.Store     // short-lived CMS page preview tokens
	JWTSecret            string
	JWTTTL               time.Duration

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
