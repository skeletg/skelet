package wiring

import (
	"database/sql"
	"log/slog"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/internal/ai"
	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/audit"
	"github.com/skeletg/skelet/apps/backend/internal/b2b"
	"github.com/skeletg/skelet/apps/backend/internal/campaign"
	"github.com/skeletg/skelet/apps/backend/internal/cart"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/consent"
	"github.com/skeletg/skelet/apps/backend/internal/country"
	"github.com/skeletg/skelet/apps/backend/internal/currency"
	"github.com/skeletg/skelet/apps/backend/internal/customer"
	"github.com/skeletg/skelet/apps/backend/internal/download"
	"github.com/skeletg/skelet/apps/backend/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/flow"
	"github.com/skeletg/skelet/apps/backend/internal/form"
	"github.com/skeletg/skelet/apps/backend/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/internal/lens"
	"github.com/skeletg/skelet/apps/backend/internal/locale"
	"github.com/skeletg/skelet/apps/backend/internal/loyalty"
	"github.com/skeletg/skelet/apps/backend/internal/media"
	"github.com/skeletg/skelet/apps/backend/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/internal/menu"
	"github.com/skeletg/skelet/apps/backend/internal/order"
	"github.com/skeletg/skelet/apps/backend/internal/payment"
	"github.com/skeletg/skelet/apps/backend/internal/paymentvault"
	"github.com/skeletg/skelet/apps/backend/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/internal/qna"
	"github.com/skeletg/skelet/apps/backend/internal/referral"
	"github.com/skeletg/skelet/apps/backend/internal/review"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
	"github.com/skeletg/skelet/apps/backend/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/internal/storecredit"
	"github.com/skeletg/skelet/apps/backend/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/internal/suppression"
	"github.com/skeletg/skelet/apps/backend/internal/tax"
	"github.com/skeletg/skelet/apps/backend/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/internal/webform"
	"github.com/skeletg/skelet/apps/backend/internal/wishlist"
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
		wireB2B,
		wireSegments,
		wireMarketing,
		wireWebForms,
		wireSignals,
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
	B2B         *b2b.Service
	Config      *config.Service
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
	d.Order.SetProductPricer(NewOrderProductPricer(NewCartCatalogAdapter(d.Catalog, d.Customer, d.B2B)))
	d.Order.SetRiskHoldThreshold(NewRiskHoldThreshold(d.Config))
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
	Flow          *flow.Service
	Signals       SignalsEnabled
	Suppression   *suppression.Service
	Order         *order.Service
	Customer      *customer.Service
	Campaign      *campaign.Service
	EmailTemplate *emailtemplate.Service
	Catalog       *catalog.Service
	Channel       *channel.Service
	Form          *form.Service
	Consent       *consent.Service
	Segment       *segment.Service
}

func wireMarketing(d marketingDeps) { // all transactional mail is delivered as a Pulse flow, receipts and account emails included
	d.Flow.SetEnabled(bool(d.Signals))
	d.Flow.SetSuppression(d.Suppression)
	d.Flow.SetOrderReader(d.Order)            // lets order flows populate item / total / tracking / ship-to blocks
	d.Customer.SetAuthEmailDispatcher(d.Flow) // verification + password reset hand their one-time link to the flow engine
	d.Campaign.SetSuppression(d.Suppression)
	d.EmailTemplate.SetCatalogReader(NewEmailCatalogReader(d.Catalog, d.Channel))                   // wires the "products" email block to live catalog data
	d.Form.SetHooks(NewContactAdapter(d.Customer), NewConsentAdapter(d.Consent), d.Segment, d.Flow) // storefront email capture → contact + consent + segment + form_submitted flow
}

type signalsDeps struct {
	fx.In
	Lens         *lens.Service
	SignalsRead  SignalsReadQueries
	CommerceRead ReadQueries
	Rollup       *analytics.TrafficRollup
	MediaStorage *media.Storage
}

func wireSignals(d signalsDeps) { // Lens reads on its own pool, and offloads its large gzip blobs to object storage when pulse/lens/blob_storage is "object"
	d.Lens.SetReadQueries(d.SignalsRead.Queries)
	d.Lens.SetCommerceQueries(d.CommerceRead.Queries) // BrowseAbandoners scans commerce, so it needs that pool too
	d.Lens.SetRollup(d.Rollup)
	d.Lens.SetBlobStore(d.MediaStorage)
}

func wireCart(c *cart.Service, bus *events.Bus, ml *medialibrary.Service, ms *media.Storage) {
	c.SetEventBus(bus)
	c.SetFileResolver(NewCartFileAdapter(ml, ms)) // FILE-type options validate + snapshot uploads through the media library and object store
}

// The audience layer, plus the one consumer that reads through it
func wireSegments(seg *segment.Service, bus *events.Bus, conn *sql.DB, promo *promotion.Service) {
	seg.SetEventBus(bus)
	seg.SetDB(conn)
	promo.SetCustomerSegments(seg) // core.customer_in_segment can't match without this - EvaluateCart resolves the slugs itself so the storefront cart and order placement can't disagree
}

func wireB2B(b *b2b.Service, bus *events.Bus, cfg Config) {
	b.SetEventBus(bus)
	b.SetEnabled(cfg.B2BEnabled) // false leaves the whole wholesale surface inert - API, contract pricing, on-account credit, quote expiry
}

func wireWebForms(wf *webform.Service, bus *events.Bus, ms *media.Storage) { // admin-built CMS forms - submissions and their file uploads land through the object store
	wf.SetDeps(bus, NewWebformFileStore(ms))
}

// The services whose only seam is the bus - membership is that criterion, so anything needing a second seam gets its own group above
func wireBusOnlyPublishers(q *qna.Service, w *wishlist.Service, bus *events.Bus) {
	q.SetEventBus(bus) // question submitted / answered
	w.SetEventBus(bus) // item added / removed
}

func wireReviews(r *review.Service, bus *events.Bus, cfg *config.Service, ch *channel.Service, aiSvc *ai.Service) {
	r.SetEventBus(bus)
	r.SetConfigService(cfg) // require_authentication + require_verified_purchase resolve against current admin settings, not the compiled defaults
	r.SetDefaultLocaleProvider(NewCatalogDefaultLocaleProvider(ch))
	r.SetAIService(aiSvc)
}

// Each subsystem is already inert behind its flag - this is so a confused operator can see which one from the boot log
func logInertSubsystems(cfg Config, aiSvc *ai.Service) {
	if !cfg.B2BEnabled {
		slog.Info("B2B disabled - no wholesale API, contract pricing, on-account credit or quote expiry", "flag", "B2B_ENABLED=false")
	}
	if !aiSvc.Enabled() {
		slog.Info("AI content assistant disabled", "flag", "AI_ENABLED=false")
	}
	if cfg.SignalsJobsExternal {
		slog.Info("retention + forwarding + trait-compute deferred to cmd/signals", "flag", "SIGNALS_JOBS_EXTERNAL=true")
	}
}
