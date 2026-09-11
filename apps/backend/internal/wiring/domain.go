package wiring

import (
	"context"
	"log"
	"log/slog"
	"os"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	authpkg "github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/backinstock"
	"github.com/skeletg/skelet/apps/backend/internal/blog"
	"github.com/skeletg/skelet/apps/backend/internal/cache"
	"github.com/skeletg/skelet/apps/backend/internal/campaign"
	"github.com/skeletg/skelet/apps/backend/internal/cart"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/cms"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/consent"
	"github.com/skeletg/skelet/apps/backend/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/destination"
	"github.com/skeletg/skelet/apps/backend/internal/email"
	"github.com/skeletg/skelet/apps/backend/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/internal/flow"
	"github.com/skeletg/skelet/apps/backend/internal/geo"
	"github.com/skeletg/skelet/apps/backend/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/internal/lens"
	"github.com/skeletg/skelet/apps/backend/internal/linkgraph"
	"github.com/skeletg/skelet/apps/backend/internal/locale"
	"github.com/skeletg/skelet/apps/backend/internal/mailsend"
	"github.com/skeletg/skelet/apps/backend/internal/media"
	"github.com/skeletg/skelet/apps/backend/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/internal/order"
	"github.com/skeletg/skelet/apps/backend/internal/ordermail"
	"github.com/skeletg/skelet/apps/backend/internal/outbox"
	"github.com/skeletg/skelet/apps/backend/internal/payment"
	"github.com/skeletg/skelet/apps/backend/internal/revalidate"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
	"github.com/skeletg/skelet/apps/backend/internal/sendlog"
	"github.com/skeletg/skelet/apps/backend/internal/signalsboot"
	"github.com/skeletg/skelet/apps/backend/internal/sitecrawl"
	"github.com/skeletg/skelet/apps/backend/internal/sms"
	"github.com/skeletg/skelet/apps/backend/internal/suppression"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
	"github.com/skeletg/skelet/apps/backend/internal/urlrewrite"
)

type RootContext struct{ context.Context } // supplied by the entrypoint rather than provided, since that's where signal handling lives

type SignalsEnabled bool // its own type so the graph can't confuse it with another bool

// SpineQueueMode says whether this process wraps its spine writes into the ingest queue or holds the bare engine - producers wrap so appends land in JetStream before the request returns, the consumer must not or it would feed itself
type SpineQueueMode bool

const (
	QueueProducer SpineQueueMode = true
	QueueConsumer SpineQueueMode = false
)

// The commerce and marketing core - services that depend on each other, not just the datastore
var Domain = fx.Module("domain",
	fx.Provide(
		NewCustomerStoreLookup, // composition-root adapters that feed constructors below
		NewCatalogDefaultLocaleProvider,
		NewCartCatalogAdapter,

		newSignalsStack,
		newRecorder,
		newSignalsEnabled,
		newGeoResolver,
		newTrafficRollup,
		newExperimentService,
		newLensService,

		newMediaStorage,
		newMediaLibraryService,
		newRevalidateClient,
		newRevalidateQueue,

		newCustomerService,
		newCartRepository,
		newOrderService,
		newCartRepricer,
		newOutboxEnqueuer,
		newPaymentAuthorizer,
		newCatalogService,
		newCartService,
		newEventOutbox,

		newSendLogStore,
		newCampaignService,
		newFlowService,
		newOrderMailService,
		newTransactionalSender,
		newDestinationService,
		newBackInStockService,

		newBlogService,
		newCMSService,

		newSiteCrawlService,
		newLinkGraphService,
	),
)

func newSiteCrawlService(q *db.Queries) *sitecrawl.Service {
	return sitecrawl.NewService(q, slog.Default())
}

func newLinkGraphService(q *db.Queries) *linkgraph.Service { return linkgraph.New(q) }

// Assembles this early because customer GDPR erasure routes its Signals-side cleanup through the Recorder
func newSignalsStack(lc fx.Lifecycle, rc RootContext, mode SpineQueueMode, q *db.Queries, rq ReadQueries, sq SignalsQueries, srq SignalsReadQueries) (*signalsboot.Stack, error) {
	stack, err := signalsboot.Build(rc.Context, signalsboot.Options{
		Commerce: q, CommerceRead: rq.Queries,
		Spine: sq.Queries, SpineRead: srq.Queries,
		WrapQueue: bool(mode),
	})
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error { stack.Close(); return nil }})
	return stack, nil
}

func newRecorder(s *signalsboot.Stack) *analytics.Recorder { return s.Recorder }

// false switches off the whole suite (Pulse, Lens, Synapse, Prism, ingest, forwarding); transactional mail keeps working
func newSignalsEnabled(s *signalsboot.Stack) SignalsEnabled { return SignalsEnabled(s.Enabled) }

func newGeoResolver() *geo.Resolver { // a bad GEOIP_DB_PATH is logged rather than fatal - geo just stops appearing on profiles
	resolver, err := geo.Open(os.Getenv("GEOIP_DB_PATH"))
	if err != nil {
		log.Printf("[geo] disabled - could not open GEOIP_DB_PATH: %v", err)
		resolver, _ = geo.Open("")
	}
	return resolver
}

// Coalesces in memory and flushes aggregates, so busy sites never store a row per hit nor scan raw events
func newTrafficRollup(sq SignalsQueries) *analytics.TrafficRollup {
	return analytics.NewTrafficRollup(sq.Queries)
}

func newExperimentService(q *db.Queries, rec *analytics.Recorder) *experiment.Service { // after the spine engine is chosen - its repo reads exposures through the Recorder
	return experiment.NewService(experiment.NewRepository(q, rec))
}

func newLensService(sq SignalsQueries, rec *analytics.Recorder, cfg *config.Service) *lens.Service { // Lens writes and reads via signalsdb on the Signals pool
	return lens.NewService(sq.Queries, rec, cfg)
}

// Fails loudly without bucket/region - silent defaults would produce broken media URLs
func newMediaStorage(rc RootContext) (*media.Storage, error) {
	return media.New(rc.Context, mediaConfigFromEnv()) // root ctx, not Background - this runs inside fx.New, where a hung endpoint would ignore SIGTERM
}

func newMediaLibraryService(q *db.Queries, store *media.Storage) *medialibrary.Service {
	return medialibrary.NewService(medialibrary.NewRepository(q), store, os.Getenv("MEDIA_LIBRARY_ROOT_PREFIX"))
}

// Generic, not CMS-specific - logs every purge and busts the storefront's Data Cache tags
func newRevalidateClient(q *db.Queries) *revalidate.Client {
	return revalidate.New(
		os.Getenv("STOREFRONT_REVALIDATE_URL"),
		os.Getenv("STOREFRONT_REVALIDATE_SECRET"),
		revalidate.NewDBStore(q),
	)
}

func newRevalidateQueue(q *db.Queries, c *revalidate.Client) *revalidate.Queue { // durable rebuild queue, drained by the cache-rebuild scheduler job
	return revalidate.NewQueue(q, c)
}

func newCustomerService(
	q *db.Queries, rec *analytics.Recorder, txMgr *tx.Manager,
	verify *authpkg.VerificationStore, reset *authpkg.PasswordResetStore, refresh CustomerRefreshStore,
	sender email.Sender, lookup CustomerStoreLookup, consentSvc *consent.Service, bus *events.Bus,
) *customer.Service {
	return customer.NewService(
		customer.NewRepository(q, rec, txMgr),
		verify, reset, refresh.RefreshStore,
		sender, lookup, consentSvc, bus,
	)
}

func newCartRepository(q *db.Queries) cart.Repository { return cart.NewRepository(q) }

// Fails the graph rather than the shop - every dependency here is one whose absence would otherwise be silent
func newOrderService(
	q *db.Queries, repo cart.Repository, cust *customer.Service, txMgr *tx.Manager, bus *events.Bus,
	repricer order.CartRepricer, ob order.OutboxEnqueuer,
	auth order.PaymentAuthorizer,
) (*order.Service, error) {
	return order.NewService(order.Deps{
		Repo: order.NewRepository(q), CartRepo: repo, Customer: cust, Tx: txMgr, Bus: bus,
		Repricer: repricer, Outbox: ob, PaymentAuthorizer: auth,
	})
}

// The seams order declares as interfaces because those packages import it - the direction can only run one way
func newCartRepricer(c *cart.Service) order.CartRepricer      { return c }
func newOutboxEnqueuer(o *outbox.Outbox) order.OutboxEnqueuer { return o }
func newPaymentAuthorizer(p *payment.Service) order.PaymentAuthorizer {
	return NewPaymentAuthorizerAdapter(p)
}

func newCatalogService( // catalog depends on channel for the listing-currency-match check
	q *db.Queries, txMgr *tx.Manager, store *media.Storage, ch *channel.Service,
	loc *locale.Service, def CatalogDefaultLocaleProvider, rw *urlrewrite.Service,
	bus *events.Bus, vk CacheRedis, // category tree is a read-through cache so it rides the evictable client
) *catalog.Service {
	return catalog.NewService(catalog.Deps{
		Repo:              catalog.NewRepository(q, txMgr),
		Tx:                txMgr,
		Media:             store,
		Channel:           ch,
		Locales:           loc,
		DefaultLocale:     def,
		URLRewrites:       rw,
		Bus:               bus,
		CategoryTreeCache: cache.New(vk.Client, "categorytree"),
	})
}

// cart needs catalog for its SIMPLE-only line-add invariant and bundle materialization
func newCartService(repo cart.Repository, txMgr *tx.Manager, a CartCatalogAdapter) *cart.Service {
	return cart.NewService(repo, txMgr, a)
}

func newEventOutbox(q *db.Queries, bus *events.Bus) *outbox.Outbox { return outbox.New(q, bus) } // registered events enqueue inside the emitting tx and are delivered at-least-once by the relay

func newCampaignService(
	q *db.Queries, tmpl *emailtemplate.Service, cust *customer.Service,
	seg *segment.Service, consentSvc *consent.Service, ch *channel.Service, cfg *config.Service,
	sender email.Sender, rec *analytics.Recorder, sends *sendlog.Store, wcfg Config,
) *campaign.Service {
	return campaign.NewService(
		campaign.NewRepository(q), tmpl, cust, seg, consentSvc, ch, cfg, sender, rec, sends, []byte(wcfg.JWTSecret),
	)
}

// Order-lifecycle mail - owns the subscriptions, delegates delivery to a flow when one is active
// The flow dispatcher is attached in wireMarketing, since it would cycle through this provider
func newOrderMailService(
	orders *order.Service, ch *channel.Service, tmpl *emailtemplate.Service,
	sender *mailsend.Sender, cfg *config.Service, q *db.Queries,
) *ordermail.Service {
	return ordermail.NewService(orders, ch, tmpl, sender, cfg, OrderMailSendLog{q: q})
}

// Core owns the send log, but it comes off the stack like the Recorder does - that is where the read pool and archive sink are attached
func newSendLogStore(s *signalsboot.Stack) *sendlog.Store { return s.SendLog }

// The one recorded sender every direct path shares - tracking is attached in wireMarketing, where the Recorder exists
func newTransactionalSender(sender email.Sender) *mailsend.Sender { return mailsend.New(sender) }

func newFlowService( // marketing automation flows - event-triggered, scheduler-advanced node graphs
	q *db.Queries, tmpl *emailtemplate.Service, cust *customer.Service,
	consentSvc *consent.Service, ch *channel.Service, cfg *config.Service,
	sender email.Sender, texter sms.Sender, rec *analytics.Recorder, sends *sendlog.Store,
	allow AllowPrivateTargets, wcfg Config,
) *flow.Service {
	return flow.NewService(
		flow.NewRepository(q), tmpl, cust, consentSvc, ch, cfg, sender, texter,
		rec, sends, bool(allow), []byte(wcfg.JWTSecret),
	)
}

// Two handles: the destination rows and the spine are the signals database, the audience export reads segments from commerce
func newDestinationService(sq SignalsQueries, q *db.Queries, rec *analytics.Recorder, allow AllowPrivateTargets) *destination.Service {
	return destination.NewService(sq.Queries, q, rec, bool(allow))
}

// On restock, resolved customers enrol into the back_in_stock flow and guests get a direct mail
func newBackInStockService(
	q *db.Queries, inv *inventory.Service, cust *customer.Service, flows *flow.Service,
	ch *channel.Service, cfg *config.Service, sender email.Sender, suppress *suppression.Service,
) *backinstock.Service {
	return backinstock.New(q, inv, cust, flows, ch, cfg, sender, suppress)
}

func newBlogService( // posts/categories/tags/authors with translations and url-rewrite sync
	q *db.Queries, txMgr *tx.Manager, loc *locale.Service,
	def CatalogDefaultLocaleProvider, rw *urlrewrite.Service, bus *events.Bus,
) *blog.Service {
	return blog.NewService(blog.Deps{
		Repo:          blog.NewRepository(q, txMgr),
		Tx:            txMgr,
		Locales:       loc,
		DefaultLocale: def,
		URLRewrites:   rw,
		Bus:           bus,
	})
}

func newCMSService( // page types and custom-attribute pages
	q *db.Queries, txMgr *tx.Manager, loc *locale.Service,
	def CatalogDefaultLocaleProvider, rw *urlrewrite.Service, bus *events.Bus, cfg *config.Service,
) *cms.Service {
	return cms.NewService(cms.Deps{
		Repo:          cms.NewRepository(q, txMgr),
		Tx:            txMgr,
		Locales:       loc,
		DefaultLocale: def,
		URLRewrites:   rw,
		Bus:           bus,
		Config:        cfg,
	})
}
