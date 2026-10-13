package wiring

import (
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/address"
	"github.com/skeletg/skelet/apps/backend/core/internal/adminaction"
	"github.com/skeletg/skelet/apps/backend/core/internal/adminprefs"
	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/delivery"
	"github.com/skeletg/skelet/apps/backend/core/internal/download"
	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/loyalty"
	"github.com/skeletg/skelet/apps/backend/core/internal/menu"
	"github.com/skeletg/skelet/apps/backend/core/internal/notification"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/internal/paymentvault"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion/actions"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion/conditions"
	"github.com/skeletg/skelet/apps/backend/core/internal/qna"
	"github.com/skeletg/skelet/apps/backend/core/internal/referral"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/core/internal/sales"
	"github.com/skeletg/skelet/apps/backend/core/internal/searchmerch"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/internal/signupform"
	"github.com/skeletg/skelet/apps/backend/core/internal/storecredit"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	taxconditions "github.com/skeletg/skelet/apps/backend/core/internal/tax/conditions"
	taxeffects "github.com/skeletg/skelet/apps/backend/core/internal/tax/effects"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/core/internal/webform"
	"github.com/skeletg/skelet/apps/backend/core/internal/wishlist"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// Construction needs nothing but the datastore, settings and each other - cycle breakers, subscriptions and optional providers are still wired by the entrypoint
var Services = fx.Module("services",
	fx.Provide(
		newTaxService,
		newShippingService,
		newPromotionService,
		newSalesService,
		newURLRewriteService,
		newInventoryService,
		newGiftcardService,
		newStoreCreditService,
		newLoyaltyService,
		newReferralService,
		newDownloadService,
		newSubscriptionService,
		newAddressService,
		newSearchMerchService,
		newQnAService,
		newWishlistService,
		newAIService,
		newReviewService,
		newFormService,
		newWebFormService,
		newDeliveryService,
		newMenuService,
		newChatHistory,
		newActionService,
		newPaymentService,
		newPaymentVaultService,
		newNotificationHub,
		newNotificationService,
		newAdminPrefsService,
	),
)

// Optional seams - PlaceOrder falls through to "zero" when nil and no plugin claimed the field - wiring them means a fresh install gets per-country tax, zone shipping and coupons with no plugin
func newTaxService(q *db.Queries) *tax.Service {
	taxconditions.Register(tax.DefaultRegistry)
	taxeffects.Register(tax.DefaultRegistry)
	return tax.NewService(tax.NewRepository(q))
}

func newShippingService(q *db.Queries) *shipping.Service {
	return shipping.NewService(shipping.NewRepository(q))
}

// The built-in rule types install here rather than from an entrypoint's blank import - without them the engine has no conditions and no actions, so a promotion silently matches nothing
func newPromotionService(q *db.Queries, txMgr *txrunner.Manager) *promotion.Service {
	actions.Register(promotion.DefaultRegistry)
	conditions.Register(promotion.DefaultRegistry)
	svc := promotion.NewService(promotion.NewRepository(q))
	svc.SetTx(txMgr)
	return svc
}

func newSalesService(q *db.Queries) *sales.Service { return sales.NewService(sales.NewRepository(q)) }

func newURLRewriteService(q *db.Queries) *urlrewrite.Service {
	return urlrewrite.NewService(urlrewrite.NewRepository(q))
}

func newInventoryService(q *db.Queries, txMgr *txrunner.Manager) *inventory.Service {
	return inventory.NewService(inventory.NewRepository(q, txMgr), txMgr)
}

func newGiftcardService(q *db.Queries, txMgr *txrunner.Manager) *giftcard.Service { // a kind=giftcard purchase issues a redeemable card, one per gift-card line, off OrderPlaced
	return giftcard.NewService(giftcard.NewRepository(q), txMgr)
}

func newStoreCreditService(q *db.Queries, txMgr *txrunner.Manager) *storecredit.Service { // per-customer ledger spent as a tender at checkout
	return storecredit.NewService(q, txMgr)
}

func newLoyaltyService(q *db.Queries, txMgr *txrunner.Manager, cfg *config.Service, sc *storecredit.Service) *loyalty.Service { // points earned on placed orders, redeemed by converting into store credit
	return loyalty.NewService(q, txMgr, cfg, sc)
}

func newReferralService(q *db.Queries, txMgr *txrunner.Manager, cfg *config.Service, sc *storecredit.Service) *referral.Service { // give-get program rewarding both sides with store credit on the referee's first order
	return referral.NewService(q, txMgr, cfg, sc)
}

func newDownloadService(q *db.Queries, txMgr *txrunner.Manager) *download.Service {
	return download.NewService(download.NewRepository(q), txMgr)
}

func newSubscriptionService(q *db.Queries, txMgr *txrunner.Manager) *subscription.Service {
	return subscription.NewService(subscription.NewRepository(q), txMgr)
}

func newAddressService(cfg *config.Service) *address.Service { return address.NewService(cfg) } // builtin format validator by default, remote validator when a channel opts in

func newSearchMerchService(q *db.Queries) *searchmerch.Service { return searchmerch.NewService(q) }
func newQnAService(q *db.Queries) *qna.Service                 { return qna.NewService(q) }
func newWishlistService(q *db.Queries) *wishlist.Service       { return wishlist.NewService(q) }

func newAIService(cfg *config.Service) *ai.Service { return ai.NewService(cfg) } // config-gated and resolved at request time, so it ships dark until an operator sets a provider/key

func newReviewService(q *db.Queries, txMgr *txrunner.Manager) *review.Service {
	return review.NewService(review.NewRepository(q, txMgr), txMgr)
}

func newFormService(q *db.Queries) *signupform.Service { return signupform.New(q) } // storefront email capture - contact + consent + segment + form_submitted flow

func newWebFormService(q *db.Queries, txMgr *txrunner.Manager, cfg *config.Service) *webform.Service { // admin-built CMS forms placed on pages via the `form` block
	return webform.New(webform.NewRepository(q, txMgr), cfg)
}

func newDeliveryService(q *db.Queries, tx *txrunner.Manager) *delivery.Service { // delivery options, kinds, point sources and the admin's PO boxes
	return delivery.NewService(delivery.NewRepository(q, tx))
}

func newMenuService(q *db.Queries, txMgr *txrunner.Manager) *menu.Service {
	return menu.NewService(menu.NewRepository(q, txMgr))
}

func newChatHistory(q *db.Queries) *ai.History { return ai.NewHistory(q) }

func newActionService() *adminaction.Service { return adminaction.NewService() }

func newPaymentService(q *db.Queries, txMgr *txrunner.Manager, ch *channel.Service) *payment.Service {
	return payment.NewService(payment.NewRepository(q), txMgr, ch)
}

func newPaymentVaultService(q *db.Queries, pay *payment.Service, limits LimitsRedis) *paymentvault.Service {
	svc := paymentvault.NewService(q, pay)
	svc.SetSaveBudget(ratelimit.NewRedis(limits.Client, RateLimitPerMin("PAYMENT_METHOD_SAVE_RATE_LIMIT_PER_HOUR", 5), time.Hour)) // every save is a gateway vault call - the card-testing surface
	return svc
}

func newNotificationHub(vk *redis.Client) *notification.Hub { return notification.NewHub(vk) } // live-pushes to dashboard subscriptions across replicas over Valkey pub/sub - latency layer only, the tables stay authoritative

func newAdminPrefsService(q *db.Queries) *adminprefs.Service {
	return adminprefs.NewService(adminprefs.NewRepository(q))
}

func newNotificationService(q *db.Queries, hub *notification.Hub) *notification.Service {
	return notification.New(q, hub)
}
