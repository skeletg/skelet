package wiring

import (
	"database/sql"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/internal/action"
	"github.com/skeletg/skelet/internal/address"
	"github.com/skeletg/skelet/internal/admin"
	"github.com/skeletg/skelet/internal/adminprefs"
	"github.com/skeletg/skelet/internal/ai"
	"github.com/skeletg/skelet/internal/analytics"
	"github.com/skeletg/skelet/internal/audit"
	authpkg "github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/b2b"
	"github.com/skeletg/skelet/internal/backinstock"
	"github.com/skeletg/skelet/internal/backup"
	"github.com/skeletg/skelet/internal/blog"
	"github.com/skeletg/skelet/internal/campaign"
	"github.com/skeletg/skelet/internal/cart"
	"github.com/skeletg/skelet/internal/catalog"
	"github.com/skeletg/skelet/internal/channel"
	"github.com/skeletg/skelet/internal/chat"
	"github.com/skeletg/skelet/internal/cms"
	"github.com/skeletg/skelet/internal/config"
	"github.com/skeletg/skelet/internal/consent"
	"github.com/skeletg/skelet/internal/country"
	"github.com/skeletg/skelet/internal/currency"
	"github.com/skeletg/skelet/internal/customer"
	"github.com/skeletg/skelet/internal/datajob"
	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/destination"
	"github.com/skeletg/skelet/internal/download"
	"github.com/skeletg/skelet/internal/email"
	"github.com/skeletg/skelet/internal/emailtemplate"
	"github.com/skeletg/skelet/internal/events"
	"github.com/skeletg/skelet/internal/experiment"
	"github.com/skeletg/skelet/internal/flow"
	"github.com/skeletg/skelet/internal/form"
	"github.com/skeletg/skelet/internal/geo"
	"github.com/skeletg/skelet/internal/giftcard"
	"github.com/skeletg/skelet/internal/integration"
	"github.com/skeletg/skelet/internal/inventory"
	"github.com/skeletg/skelet/internal/lens"
	"github.com/skeletg/skelet/internal/linkgraph"
	"github.com/skeletg/skelet/internal/locale"
	"github.com/skeletg/skelet/internal/loyalty"
	"github.com/skeletg/skelet/internal/media"
	"github.com/skeletg/skelet/internal/medialibrary"
	"github.com/skeletg/skelet/internal/menu"
	"github.com/skeletg/skelet/internal/notification"
	"github.com/skeletg/skelet/internal/order"
	"github.com/skeletg/skelet/internal/outbox"
	"github.com/skeletg/skelet/internal/payment"
	"github.com/skeletg/skelet/internal/paymentvault"
	"github.com/skeletg/skelet/internal/promotion"
	"github.com/skeletg/skelet/internal/qna"
	"github.com/skeletg/skelet/internal/referral"
	"github.com/skeletg/skelet/internal/remoteprovider"
	"github.com/skeletg/skelet/internal/revalidate"
	"github.com/skeletg/skelet/internal/review"
	"github.com/skeletg/skelet/internal/sales"
	"github.com/skeletg/skelet/internal/scheduler"
	"github.com/skeletg/skelet/internal/searchmerch"
	"github.com/skeletg/skelet/internal/segment"
	"github.com/skeletg/skelet/internal/shipping"
	"github.com/skeletg/skelet/internal/signalsboot"
	"github.com/skeletg/skelet/internal/sitecrawl"
	"github.com/skeletg/skelet/internal/sms"
	"github.com/skeletg/skelet/internal/storecredit"
	"github.com/skeletg/skelet/internal/subscription"
	"github.com/skeletg/skelet/internal/suppression"
	"github.com/skeletg/skelet/internal/tax"
	"github.com/skeletg/skelet/internal/urlrewrite"
	"github.com/skeletg/skelet/internal/webform"
	"github.com/skeletg/skelet/internal/webhook"
	"github.com/skeletg/skelet/internal/wishlist"
)

// Infra and Svcs are what the request-serving layer needs, which is very nearly everything - the one place in the tree where a wide dependency list is the honest shape
// Infra stays narrow anyway, because a field added here makes all three of its consumers depend on it
type Infra struct {
	fx.In

	Config  Config
	DB      *sql.DB
	Valkey  *redis.Client // the noeviction primary - auth/refresh/reset tokens, preview stores, pub/sub
	Cache   CacheRedis    // the evictable cache client - APQ, response cache, read-through caches, rate limiters
	Queries *db.Queries
}

// Svcs is the same for the domain layer - taken only by newGQL and newRootHandler, which
// need it because the resolver root exposes every domain and the handler mounts every route
type Svcs struct {
	fx.In

	Bus   *events.Bus
	Email email.Sender
	SMS   sms.Sender

	SiteCrawl *sitecrawl.Service
	LinkGraph *linkgraph.Service

	Admin       *admin.Service
	Audit       *audit.Service
	Integration *integration.Service
	Consent     *consent.Service
	Config      *config.Service
	Channel     *channel.Service
	Locale      *locale.Service
	Currency    *currency.Service
	Country     *country.Service

	Verify          *authpkg.VerificationStore
	Reset           *authpkg.PasswordResetStore
	AdminRefresh    AdminRefreshStore
	CustomerRefresh CustomerRefreshStore
	BlogPreview     BlogPreviewStore
	CMSPreview      CMSPreviewStore

	AllowPrivateTargets AllowPrivateTargets
	Webhook             *webhook.Service
	WebhookDispatcher   *webhook.Dispatcher
	RemoteProvider      *remoteprovider.Service
	RemoteInvoker       *remoteprovider.Invoker

	Tax           *tax.Service
	Shipping      *shipping.Service
	Promotion     *promotion.Service
	Segment       *segment.Service
	Sales         *sales.Service
	Suppression   *suppression.Service
	URLRewrite    *urlrewrite.Service
	Inventory     *inventory.Service
	B2B           *b2b.Service
	Giftcard      *giftcard.Service
	StoreCredit   *storecredit.Service
	Loyalty       *loyalty.Service
	Referral      *referral.Service
	Download      *download.Service
	Subscription  *subscription.Service
	Address       *address.Service
	SearchMerch   *searchmerch.Service
	QnA           *qna.Service
	Wishlist      *wishlist.Service
	AI            *ai.Service
	Review        *review.Service
	Form          *form.Service
	WebForm       *webform.Service
	Menu          *menu.Service
	Chat          *chat.Service
	Action        *action.Service
	DataJob       *datajob.Service
	Backup        *backup.Service
	EmailTemplate *emailtemplate.Service
	Payment       *payment.Service
	PaymentVault  *paymentvault.Service
	NotifHub      *notification.Hub
	Notification  *notification.Service
	AdminPrefs    *adminprefs.Service

	SignalsStack   *signalsboot.Stack
	Recorder       *analytics.Recorder
	SignalsEnabled SignalsEnabled
	Geo            *geo.Resolver
	TrafficRollup  *analytics.TrafficRollup
	Experiment     *experiment.Service
	Lens           *lens.Service

	MediaStorage    *media.Storage
	MediaLibrary    *medialibrary.Service
	RevalidateCli   *revalidate.Client
	RevalidateQueue *revalidate.Queue

	Customer *customer.Service
	CartRepo cart.Repository
	Cart     *cart.Service
	Order    *order.Service
	Catalog  *catalog.Service
	Outbox   *outbox.Outbox

	Campaign    *campaign.Service
	Flow        *flow.Service
	Destination *destination.Service
	BackInStock *backinstock.Service

	Scheduler  *scheduler.Service
	SearchTier SearchTier

	Blog *blog.Service
	CMS  *cms.Service
}

// The GQL servers and rate limiters stay out of Svcs - newGQL consumes Svcs, so a GQL field in there is a dependency cycle
