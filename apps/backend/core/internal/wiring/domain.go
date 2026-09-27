package wiring

import (
	"context"
	"log/slog"
	"os"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/backinstock"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/linkgraph"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/mailsend"
	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/ordermail"
	"github.com/skeletg/skelet/apps/backend/core/internal/outbox"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/internal/revalidate"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient/enroll"
	"github.com/skeletg/skelet/apps/backend/core/internal/sitecrawl"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
	authpkg "github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type RootContext struct{ context.Context } // supplied by the entrypoint rather than provided, since that's where signal handling lives

type SignalsEnabled bool // its own type so the graph can't confuse it with another bool

// The commerce and marketing core - services that depend on each other, not just the datastore
var Domain = fx.Module("domain",
	fx.Provide(
		NewCustomerStoreLookup, // composition-root adapters that feed constructors below
		NewCatalogDefaultLocaleProvider,
		NewCartCatalogAdapter,

		newSignalsEnabled,
		newSegmentLookup,
		newSignalsClient,

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

		enroll.New,
		newOrderMailService,
		newTransactionalSender,
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

// false switches off the whole suite (Pulse, Lens, Synapse, Prism, ingest, forwarding); transactional mail keeps working
// Core reads the switch itself - the two processes deploy separately and each reads its own environment
func newSignalsEnabled() SignalsEnabled { return SignalsEnabled(EnvBool("SIGNALS_ENABLED", true)) }

// Fails loudly without bucket/region - silent defaults would produce broken media URLs
func newMediaStorage(rc RootContext) (*objstore.Storage, error) {
	return objstore.New(rc.Context, mediaConfigFromEnv()) // root ctx, not Background - this runs inside fx.New, where a hung endpoint would ignore SIGTERM
}

func newMediaLibraryService(q *db.Queries, store *objstore.Storage) *medialibrary.Service {
	return medialibrary.NewService(medialibrary.NewRepository(q), store, os.Getenv("MEDIA_LIBRARY_ROOT_PREFIX"))
}

// Generic, not CMS-specific - logs every purge and busts each channel's storefront Data Cache tags
func newRevalidateClient(q *db.Queries, channels *channel.Service) *revalidate.Client {
	c := revalidate.New(
		os.Getenv("STOREFRONT_REVALIDATE_URL"),
		os.Getenv("STOREFRONT_REVALIDATE_SECRET"),
		revalidate.NewDBStore(q),
	)
	c.SetChannels(storefrontChannels{channels})
	return c
}

type storefrontChannels struct{ svc *channel.Service }

func (s storefrontChannels) StorefrontChannels(ctx context.Context) ([]revalidate.StorefrontChannel, error) {
	chs, err := s.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]revalidate.StorefrontChannel, len(chs))
	for i, ch := range chs {
		out[i] = revalidate.StorefrontChannel{ID: ch.ID, PublicURL: ch.PublicURL, IsActive: ch.IsActive}
	}
	return out, nil
}

func newRevalidateQueue(q *db.Queries, c *revalidate.Client) *revalidate.Queue { // durable rebuild queue, drained by the cache-rebuild scheduler job
	return revalidate.NewQueue(q, c)
}

func newCustomerService(
	q *db.Queries, signals *signalsclient.Client, txMgr *txrunner.Manager,
	verify *authpkg.VerificationStore, reset *authpkg.PasswordResetStore, refresh CustomerRefreshStore,
	sender email.Sender, lookup CustomerStoreLookup, consentSvc *consent.Service, bus *events.Bus,
) *customer.Service {
	// A nil *Client in the interface would read as present, so Signals being off has to arrive as a nil interface
	var eraser customer.SignalsEraser
	if signals != nil {
		eraser = signals
	}
	return customer.NewService(
		customer.NewRepository(q, eraser, txMgr),
		verify, reset, refresh.RefreshStore,
		sender, lookup, consentSvc, bus,
	)
}

func newCartRepository(q *db.Queries) cart.Repository { return cart.NewRepository(q) }

// Fails the graph rather than the shop - every dependency here is one whose absence would otherwise be silent
func newOrderService(
	q *db.Queries, repo cart.Repository, cust *customer.Service, txMgr *txrunner.Manager, bus *events.Bus,
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
	q *db.Queries, txMgr *txrunner.Manager, store *objstore.Storage, ch *channel.Service,
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
func newCartService(repo cart.Repository, txMgr *txrunner.Manager, a CartCatalogAdapter) *cart.Service {
	return cart.NewService(repo, txMgr, a)
}

func newEventOutbox(q *db.Queries, bus *events.Bus) *outbox.Outbox { return outbox.New(q, bus) } // registered events enqueue inside the emitting tx and are delivered at-least-once by the relay

// Order-lifecycle mail - owns the subscriptions, delegates delivery to a flow when one is active
// The flow dispatcher is attached in wireMarketing, since it would cycle through this provider
func newOrderMailService(
	orders *order.Service, ch *channel.Service,
	sender *mailsend.Sender, cfg *config.Service, q *db.Queries,
) *ordermail.Service {
	return ordermail.NewService(orders, ch, CoreMailRenderer{}, sender, cfg, OrderMailSendLog{q: q})
}

// The one recorded sender every direct path shares - tracking is attached in wireMarketing, where the Recorder exists
func newTransactionalSender(sender email.Sender, cfg *config.Service) *mailsend.Sender {
	return mailsend.New(sender, cfg)
}

// On restock, resolved customers enrol into the back_in_stock flow and guests get a direct mail
func newBackInStockService(
	q *db.Queries, inv *inventory.Service, cust *customer.Service, flows *enroll.Enroller,
	ch *channel.Service, cfg *config.Service, sender email.Sender, sc *signalsclient.Client, store *objstore.Storage,
) *backinstock.Service {
	svc := backinstock.New(q, inv, cust, flows, ch, cfg, sender, sc)
	svc.SetMedia(store)
	return svc
}

func newBlogService( // posts/categories/tags/authors with translations and url-rewrite sync
	q *db.Queries, txMgr *txrunner.Manager, loc *locale.Service,
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
	q *db.Queries, txMgr *txrunner.Manager, loc *locale.Service,
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
