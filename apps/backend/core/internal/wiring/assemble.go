package wiring

import (
	"context"
	"database/sql"
	"log/slog"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/audit"
	"github.com/skeletg/skelet/apps/backend/core/internal/backinstock"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/country"
	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/download"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/form"
	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/loyalty"
	"github.com/skeletg/skelet/apps/backend/core/internal/mailsend"
	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/internal/menu"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/ordermail"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/internal/paymentvault"
	pluginhost "github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/qna"
	"github.com/skeletg/skelet/apps/backend/core/internal/referral"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/storecredit"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/core/internal/webform"
	"github.com/skeletg/skelet/apps/backend/core/internal/wishlist"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
)

// The wiring the graph can't express through constructors: seams that would be import cycles if the dependency were declared, the ordered event subscriptions, and registries that can only be filled once services exist
// These run as invokes inside fx.New - before app.Start, so before any background loop can dispatch into a bus whose subscribers are still being registered - order is the registration order below, and only matters where a comment says so
var Wiring = fx.Module("wiring",
	fx.Invoke(
		wireValidators,
		wireOrderPlacement,
		WireRemoteProviders,
		RegisterBuiltinCarriers,
		wirePayments,
		wireOrderLookups,
		wireCart,
		wireCatalogInventory,
		wireSEOCrawl,
		WireValkeyCaches,
		wirePluginRuntime,
		wireMarketing,
		wireWebForms,
		wireBusOnlyPublishers,
		wireReviews,
		RegisterDomainSubscribers,
		RegisterAdminActions,
		logInertSubsystems,
	),
)

type validatorDeps struct {
	fx.In
	Customer  *customer.Service
	Config    *config.Service
	Country   *country.Service
	Locale    *locale.Service
	Channel   *channel.Service
	Inventory *inventory.Service
	Catalog   *catalog.Service
	Currency  *currency.Service
}

// Config sources and the cross-domain validators - most domains declare config from init(), these read through a service
func wireValidators(d validatorDeps) {
	d.Customer.SetConfigService(d.Config)
	d.Customer.SetCountryValidator(d.Country) // per-channel country/region options bind on address writes
	RegisterConfigSources(d.Customer, d.Inventory, d.Currency)
	d.Locale.SetCountryValidator(d.Country)
	d.Channel.SetLocaleValidator(d.Locale)
	d.Channel.SetLocationAssigner(d.Inventory)
	// Currency-change guards: channel.Update rejects when listings inherit, and config.Set on the global currency path rejects when listings would be stranded under a new one
	d.Channel.SetListingCounter(d.Catalog)
	d.Channel.SetCurrencyValidator(d.Currency)
}

type orderPlacementDeps struct {
	fx.In
	Order       *order.Service
	Tax         *tax.Service
	Shipping    *shipping.Service
	Promotion   *promotion.Service
	Country     *country.Service
	Catalog     *catalog.Service
	Giftcard    *giftcard.Service
	StoreCredit *storecredit.Service
	Inventory   *inventory.Service
	Channel     *channel.Service
	Customer    *customer.Service
	Config      *config.Service

	SegmentReads *SegmentLookup
}

// Pricing inputs, tenders, and the seams that can reject or amend a cart
func wireOrderPlacement(d orderPlacementDeps) {
	d.Order.SetTax(d.Tax)
	d.Order.SetShipping(d.Shipping)
	d.Order.SetPromotion(d.Promotion)
	d.Order.SetCountryValidator(d.Country)
	d.Order.SetProductTaxClassLookup(d.Catalog) // per-line tax needs each line's product tax class, resolved by SKU at placement time
	d.Order.SetProductCostLookup(d.Catalog)
	d.Order.SetGiftCardTender(NewGiftcardTender(d.Giftcard)) // gift-card codes redeem as a tender, inside the placement tx
	d.Order.SetStoreCreditTender(NewStoreCreditTender(d.StoreCredit))
	d.Order.SetGiftStockChecker(d.Inventory) // lets placement drop a promotion free gift stock can't cover, rather than appending a line the reservation rejects and taking the whole paid order down
	d.Order.SetInventoryEditor(d.Inventory)
	d.Order.SetChannelLoader(NewOrderChannelLoader(d.Channel)) // pre-fulfillment editing: the order's channel drives the tax-inclusive recompute
	d.Order.SetProductPricer(NewOrderProductPricer(NewCartCatalogAdapter(d.Catalog, d.Customer)))
	d.Order.SetRiskHoldThreshold(NewRiskHoldThreshold(d.Config))
	d.Order.SetShippingTax(NewShippingTax(d.Config))
	// EvaluateCart resolves the buyer's segments itself so the storefront and placement can't price a cart differently - without this seam every segment-gated condition sees an empty list and matches nobody
	d.Promotion.SetCustomerSegments(d.SegmentReads)
}

type paymentDeps struct {
	fx.In
	Root         RootContext
	Payment      *payment.Service
	Bus          *events.Bus
	Subscription *subscription.Service
	Vault        *paymentvault.Service
	Shipping     *shipping.Service
	Channel      *channel.Service
	Order        *order.Service
}

// Authorize the amount owed at placement, then the payment→order bridge settles payment_status from the gateway's events and webhooks
func wirePayments(d paymentDeps) error {
	d.Payment.SetEventBus(d.Bus)
	d.Subscription.SetRenewalCharger(NewSubscriptionRenewalCharger(d.Vault, d.Payment)) // renewals charge the saved method off-session, dunning ladder in subscription.CollectDue
	if err := SeedProviderRows(d.Root.Context, d.Payment, d.Shipping, d.Channel); err != nil {
		return err
	}
	d.Order.SetPaymentValidator(d.Payment) // per-channel method selection at checkout (enabled + country allow-list) - after seeding, once the payment service is fully set up
	return nil
}

type orderLookupDeps struct {
	fx.In
	Giftcard     *giftcard.Service
	Bus          *events.Bus
	Loyalty      *loyalty.Service
	Referral     *referral.Service
	Download     *download.Service
	Subscription *subscription.Service
	Order        *order.Service
	Catalog      *catalog.Service
}

// Each issuer resolves an order's lines back to products when its event fires
func wireOrderLookups(d orderLookupDeps) {
	d.Giftcard.SetOrderLookup(NewGiftcardOrderLookup(d.Order, d.Catalog))
	d.Giftcard.SetEventBus(d.Bus)
	d.Loyalty.SetOrderLookup(d.Order)
	d.Referral.SetOrderLookup(d.Order)
	d.Download.SetOrderLookup(NewDownloadOrderLookup(d.Order, d.Catalog))
	d.Subscription.SetOrderLookup(NewSubscriptionOrderLookup(d.Order, d.Catalog))
}

type catalogInventoryDeps struct {
	fx.In
	Inventory  *inventory.Service
	Catalog    *catalog.Service
	Order      *order.Service
	Config     *config.Service
	Bus        *events.Bus
	Audit      *audit.Service
	URLRewrite *urlrewrite.Service
	Menu       *menu.Service
	Locale     *locale.Service
	Channel    *channel.Service
}

// Inventory's provisioning hook and reservation lifecycle, plus the catalog URL behaviour that reads from config
func wireCatalogInventory(d catalogInventoryDeps) {
	d.Inventory.SetProductLookup(NewCatalogProductLookup(d.Catalog))
	d.Inventory.SetOrderLineLookup(NewOrderLineLookup(d.Order, d.Catalog))
	d.Inventory.SetLedgerToggle(NewInventoryLedgerToggle(d.Config))
	d.Inventory.SetFlatModeToggle(NewInventoryFlatModeToggle(d.Config))
	d.Inventory.SetDeductionStepToggle(NewInventoryDeductionStep(d.Config))
	d.Inventory.SetEventBus(d.Bus)
	d.Catalog.SetInventoryEnsurer(NewCatalogInventoryEnsurer(d.Inventory))

	d.Catalog.SetURLConfig(d.Config) // operator-tunable general/urls/*: auto-redirect on change, redirect type, template fallback
	d.Catalog.SetURLCollisionRecorder(NewAuditURLCollisions(d.Audit))
	d.URLRewrite.SetEventBus(d.Bus)
	d.Menu.SetEventBus(d.Bus)
	d.Menu.SetLocaleValidator(d.Locale)
	// Unconditional - the nav purge and the channel webhooks consume these, so living inside the search tier's OpenSearch branch left them dead on the default install
	d.Channel.SetEventBus(d.Bus)
}

type marketingDeps struct {
	fx.In
	Flows         *FlowEnroller
	Customer      *customer.Service
	Form          *form.Service
	Consent       *consent.Service
	SegmentReads  *SegmentLookup
	OrderMail     *ordermail.Service
	SignalsClient *signalsclient.Client

	TransactionalSender *mailsend.Sender
	BackInStock         *backinstock.Service
	Lifecycle           fx.Lifecycle
}

// Appended after the signals client's close, so the unwind lets the in-flight send reports land first
func drainSendReportsOnStop(lc fx.Lifecycle, s *mailsend.Sender) {
	lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
		done := make(chan struct{})
		go func() { s.Wait(); close(done) }()
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}})
}

// Transactional mail is offered to Pulse first, where an operator's flow may own it, and goes out from
// core's own templates when no flow takes it - the engine itself runs in the suite now
func wireMarketing(d marketingDeps) {
	d.Customer.SetAuthEmailDispatcher(d.Flows)                                 // verification + password reset hand their one-time link to the flow engine
	d.Customer.SetAuthEmailFallback(NewCoreAccountMail(d.TransactionalSender)) // and go out from core's own templates when no flow does, so a paused one can't swallow a password reset
	d.OrderMail.SetFlowDispatcher(d.Flows)                                     // receipts prefer an operator's flow, and fall back to a direct send
	// Core delivers the mail and Signals counts it - with the suite off the send still goes, unreported
	d.TransactionalSender.SetRecorder(NewTransactionalRecorder(d.SignalsClient))
	drainSendReportsOnStop(d.Lifecycle, d.TransactionalSender)
	d.BackInStock.SetDirectSender(d.TransactionalSender)
	d.Form.SetHooks(NewContactAdapter(d.Customer), NewConsentAdapter(d.Consent), d.SegmentReads, d.Flows) // storefront email capture → contact + consent + segment + form_submitted flow
}

func wireCart(c *cart.Service, bus *events.Bus, ml *medialibrary.Service, ms *objstore.Storage) {
	c.SetEventBus(bus)
	c.SetFileResolver(NewCartFileAdapter(ml, ms)) // FILE-type options validate + snapshot uploads through the media library and object store
}

// The audience layer, plus the one consumer that reads through it The runtime capabilities a plugin resolves on first USE rather than at registration - the pool behind sdk.Database, and the draft-order seam. Both are late-wired for the same reason config is: plugin init() runs before any of them exists
func wirePluginRuntime(conn *sql.DB, ord *order.Service, cust *customer.Service, c *cart.Service) {
	pluginhost.SetDatabase(conn)
	pluginhost.SetDraftOrderCreator(ord, cust)
	pluginhost.SetCartReader(c)
}

func wireWebForms(wf *webform.Service, bus *events.Bus, ms *objstore.Storage) { // admin-built CMS forms - submissions and their file uploads land through the object store
	wf.SetDeps(bus, NewWebformFileStore(ms))
}

// The services whose only seam is the bus - membership is that criterion, so anything needing a second seam gets its own group above
func wireBusOnlyPublishers(q *qna.Service, w *wishlist.Service, c *consent.Service, bus *events.Bus) {
	q.SetEventBus(bus) // question submitted / answered
	w.SetEventBus(bus) // item added / removed
	c.SetEventBus(bus) // opted in / out
}

func wireReviews(r *review.Service, bus *events.Bus, cfg *config.Service, ch *channel.Service, aiSvc *ai.Service) {
	r.SetEventBus(bus)
	r.SetConfigService(cfg) // require_authentication + require_verified_purchase resolve against current admin settings, not the compiled defaults
	r.SetDefaultLocaleProvider(NewCatalogDefaultLocaleProvider(ch))
	r.SetAIService(aiSvc)
}

// Each subsystem is already inert behind its flag - this is so a confused operator can see which one from the boot log
func logInertSubsystems(cfg Config, aiSvc *ai.Service) {
	if !aiSvc.Enabled() {
		slog.Info("AI content assistant disabled", "flag", "AI_ENABLED=false")
	}
}
