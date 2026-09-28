package wiring

import (
	"database/sql"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/address"
	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/adminaction"
	"github.com/skeletg/skelet/apps/backend/core/internal/adminprefs"
	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/audit"
	"github.com/skeletg/skelet/apps/backend/core/internal/backinstock"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
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
	"github.com/skeletg/skelet/apps/backend/core/internal/download"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/form"
	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
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
	"github.com/skeletg/skelet/apps/backend/core/internal/outbox"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/internal/paymentvault"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/qna"
	"github.com/skeletg/skelet/apps/backend/core/internal/referral"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/internal/revalidate"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/core/internal/sales"
	"github.com/skeletg/skelet/apps/backend/core/internal/scheduler"
	"github.com/skeletg/skelet/apps/backend/core/internal/searchmerch"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient/enroll"
	"github.com/skeletg/skelet/apps/backend/core/internal/sitecrawl"
	"github.com/skeletg/skelet/apps/backend/core/internal/storecredit"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/core/internal/webform"
	"github.com/skeletg/skelet/apps/backend/core/internal/webhook"
	"github.com/skeletg/skelet/apps/backend/core/internal/wishlist"
	authpkg "github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
)

// Infra and Svcs are what the request-serving layer needs, which is very nearly everything - the one place in the tree where a wide dependency list is the honest shape
// Infra stays narrow anyway, because a field added here makes all three of its consumers depend on it
type Infra struct {
	fx.In

	Config  Config
	DB      *sql.DB
	Valkey  *redis.Client // the noeviction primary - auth/refresh/reset tokens, preview stores, pub/sub
	Cache   CacheRedis    // the evictable cache client - APQ, response cache, read-through caches
	Limits  LimitsRedis   // rate-limit counters, the cache client unless VALKEY_LIMITS_URL names its own
	Queries *db.Queries
}

// Svcs is the same for the domain layer - taken only by newGQL and newRootHandler, which
// need it because the resolver root exposes every domain and the handler mounts every route
type Svcs struct {
	fx.In

	Bus   *events.Bus
	Email email.Sender

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

	Tax          *tax.Service
	Shipping     *shipping.Service
	Promotion    *promotion.Service
	SegmentReads *SegmentLookup
	Sales        *sales.Service
	URLRewrite   *urlrewrite.Service
	Inventory    *inventory.Service
	Giftcard     *giftcard.Service
	StoreCredit  *storecredit.Service
	Loyalty      *loyalty.Service
	Referral     *referral.Service
	Download     *download.Service
	Subscription *subscription.Service
	Address      *address.Service
	SearchMerch  *searchmerch.Service
	QnA          *qna.Service
	Wishlist     *wishlist.Service
	AI           *ai.Service
	Review       *review.Service
	Form         *form.Service
	WebForm      *webform.Service
	Menu         *menu.Service
	Chat         *chat.Service
	Action       *adminaction.Service
	DataJob      *datajob.Service
	Payment      *payment.Service
	PaymentVault *paymentvault.Service
	NotifHub     *notification.Hub
	Notification *notification.Service
	AdminPrefs   *adminprefs.Service

	SignalsEnabled SignalsEnabled
	SignalsClient  *signalsclient.Client

	MediaStorage    *objstore.Storage
	MediaLibrary    *medialibrary.Service
	RevalidateCli   *revalidate.Client
	RevalidateQueue *revalidate.Queue

	Customer  *customer.Service
	CartRepo  cart.Repository
	Cart      *cart.Service
	Order     *order.Service
	OrderMail *ordermail.Service
	Catalog   *catalog.Service
	Outbox    *outbox.Outbox

	Flows       *enroll.Enroller
	BackInStock *backinstock.Service

	Scheduler  *scheduler.Service
	SearchTier SearchTier

	Blog *blog.Service
	CMS  *cms.Service
}

// The GQL servers and rate limiters stay out of Svcs - newGQL consumes Svcs, so a GQL field in there is a dependency cycle
