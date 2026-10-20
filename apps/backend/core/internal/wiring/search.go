package wiring

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/graphqlx"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/internal/search"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// SearchTier is the assembled product-discovery stack - Client is nil when OPENSEARCH_URL is unset - /health reports on it, so it stays visible rather than being hidden inside Engine
type SearchTier struct {
	Client   *search.Client
	Products *search.ProductIndexer
	Posts    *search.BlogPostIndexer
	Engine   search.Engine
	Indexes  *search.IndexRegistry // every index the reindex tooling can address (core here, plugin-owned via the SDK host) - non-nil even with no cluster, so callers enumerate without a nil check
}

// Search assembles product discovery - it is a TIER, not one engine: OpenSearch answers when it can, Postgres answers when it can't - including when it was never configured, which is the shipped default
var Search = fx.Module("search", fx.Provide(newSearchTier))

type searchDeps struct {
	fx.In

	Root           RootContext
	Cfg            Config
	Bus            *events.Bus
	Queries        *db.Queries
	Blog           *blog.Service
	Catalog        *catalog.Service
	Channel        *channel.Service
	Inventory      *inventory.Service
	Config         *config.Service
	Locale         *locale.Service
	RemoteProvider *remoteprovider.Service
	RemoteInvoker  *remoteprovider.Invoker
}

func newSearchTier(d searchDeps) SearchTier {
	// Best-effort client - an unreachable cluster leaves the indexers non-nil, writes log and continue, reads return empty until `bin/cli search reindex`
	registry := search.NewIndexRegistry()
	var productIndexer *search.ProductIndexer
	var blogPostIndexer *search.BlogPostIndexer
	attrCache := search.NewAttributeCache(d.Catalog, 30*time.Second)
	for _, ev := range []string{catalog.EventAttributeUpdated, catalog.EventAttributeDeleted, catalog.EventAttributeValueSlugChanged} {
		d.Bus.Subscribe(ev, func(context.Context, events.Event) error {
			attrCache.Invalidate()
			return nil
		})
	}
	var searchClient *search.Client                                           // hoisted so /health can report search reachability, and so the tier can be assembled below - browse is the only listing path, so a silent outage looks like an empty catalogue
	if osURL := strings.TrimSpace(os.Getenv("OPENSEARCH_URL")); osURL != "" { // empty disables search entirely (docker-compose.yml, profile "search")
		var caPEM []byte
		if path := strings.TrimSpace(os.Getenv("OPENSEARCH_CA_CERT_FILE")); path != "" {
			pem, rerr := os.ReadFile(path)
			if rerr != nil {
				log.Printf("[search] CA bundle %q unreadable, falling back to the system pool: %v", path, rerr)
			} else {
				caPEM = pem
			}
		}
		client, err := search.NewClientWithOptions([]string{osURL}, search.ClientOptions{
			Username:  os.Getenv("OPENSEARCH_USERNAME"), // credentials optional so dev keeps working against a security-disabled cluster, but without them there is no way to point at any managed OpenSearch
			Password:  os.Getenv("OPENSEARCH_PASSWORD"),
			CACertPEM: caPEM,
		})
		searchClient = client
		if err != nil {
			searchClient = nil
			log.Printf("[search] disabled: client init: %v", err)
		} else {
			productIndexer = search.NewProductIndexer(searchClient, d.Catalog, d.Channel, d.Inventory, d.Config)
			productIndexer.SetSalesSource(NewSalesVelocitySource(d.Queries))
			productIndexer.SetAttributeCache(attrCache)
			productIndexer.SetRemoteSource(search.NewRemoteSearchSource(d.RemoteProvider, d.RemoteInvoker)) // a remote provider answers the search box via an external engine, failing open to this OpenSearch indexer
			blogPostIndexer = search.NewBlogPostIndexer(searchClient, d.Blog, d.Channel, d.Config)
			_ = registry.Register(search.RegisteredIndex{Slug: "products", Lifecycle: productIndexer}) // register errors are duplicate slugs, impossible for the two core entries
			_ = registry.Register(search.RegisteredIndex{Slug: "blog", Lifecycle: blogPostIndexer})
			ctxBoot, cancelBoot := context.WithTimeout(d.Root.Context, 10*time.Second) // root ctx, not Background - this runs inside fx.New, and a hung cluster would otherwise sit out its full timeout ignoring SIGTERM
			for slug, err := range registry.EnsureAll(ctxBoot) {
				log.Printf("[search] %s index ensure failed: %v", slug, err)
			}
			cancelBoot()
			registerSearchSubscribers(d.Root.Context, d.Bus, productIndexer, blogPostIndexer)
			d.Locale.SetStatusChangeHook(func(ctx context.Context, code string, isActive bool) { // a global locale activate/deactivate reindexes every channel that carries the code - deactivation must drop its per-locale docs, reactivation must rebuild them
				stores, err := d.Channel.List(ctx)
				if err != nil {
					log.Printf("[search] locale %s status change: list channels: %v", code, err)
					return
				}
				for _, sv := range stores {
					carries := strings.EqualFold(sv.DefaultLocale, code)
					for _, l := range sv.Locales {
						if strings.EqualFold(l.Code, code) {
							carries = true
							break
						}
					}
					if !carries {
						continue
					}
					if n, err := productIndexer.ReindexChannel(ctx, sv.ID); err != nil {
						log.Printf("[search] locale %s status change: channel %s reindex (after %d): %v", code, sv.ID, n, err)
					}
				}
			})
		}
	} else {
		log.Printf("[search] no OPENSEARCH_URL - product discovery runs on the Postgres tier (ts_rank relevance, no stemming or merch boosts, facets on selectable attributes only)")
	}

	// A TIER, not one engine - Postgres answers whenever OpenSearch can't (including never-configured, the default), and the degraded hook keeps that fallback out of the response cache
	var productSearchEngine search.Engine = search.NewTieredEngine(
		func() search.Engine {
			if productIndexer == nil {
				return nil // typed-nil guard - a nil *ProductIndexer in an interface is not nil
			}
			return productIndexer
		}(),
		func() search.Engine {
			pg := search.NewPostgresEngine(d.Catalog, d.Config)
			pg.SetAttributeCache(attrCache)
			return pg
		}(),
		func(ctx context.Context, op string, cause error) {
			graphqlx.MarkUncacheable(ctx)
		},
		search.WithFallbackLimit(max(1, d.Cfg.MaxConns/4)),
	)
	return SearchTier{
		Client:   searchClient,
		Products: productIndexer,
		Posts:    blogPostIndexer,
		Engine:   productSearchEngine,
		Indexes:  registry,
	}
}

// All subscriptions are async and best-effort - a failed reindex logs and continues so the mutation that triggered it still succeeds, and operators recover from indexer downtime with `bin/cli search reindex`
type productIndex interface {
	Upsert(ctx context.Context, productID uuid.UUID) error
	Delete(ctx context.Context, productID uuid.UUID) error
	ReindexChannel(ctx context.Context, channelID uuid.UUID) (int, error)
	DeleteChannel(ctx context.Context, channelID uuid.UUID) error
}

type postIndex interface {
	Upsert(ctx context.Context, postID uuid.UUID) error
	Delete(ctx context.Context, postID uuid.UUID) error
}

func registerSearchSubscribers(root context.Context, bus *events.Bus, products productIndex, posts postIndex) {
	upsertProduct := func(ctx context.Context, productID uuid.UUID) {
		if err := products.Upsert(ctx, productID); err != nil {
			log.Printf("[search] product upsert %s: %v", productID, err)
		}
	}
	deleteProduct := func(ctx context.Context, productID uuid.UUID) {
		if err := products.Delete(ctx, productID); err != nil {
			log.Printf("[search] product delete %s: %v", productID, err)
		}
	}
	bus.Subscribe(catalog.EventProductCreated, func(ctx context.Context, e events.Event) error {
		upsertProduct(ctx, e.(*catalog.ProductCreated).ProductID)
		return nil
	})
	bus.Subscribe(catalog.EventProductUpdated, func(ctx context.Context, e events.Event) error {
		upsertProduct(ctx, e.(*catalog.ProductUpdated).ProductID)
		return nil
	})
	bus.Subscribe(catalog.EventProductDeleted, func(ctx context.Context, e events.Event) error {
		deleteProduct(ctx, e.(*catalog.ProductDeleted).ProductID)
		return nil
	})
	bus.Subscribe(catalog.EventProductListingChanged, func(ctx context.Context, e events.Event) error {
		upsertProduct(ctx, e.(*catalog.ProductListingChanged).ProductID)
		return nil
	})

	// Attribute edits change the attr_* fields of every product carrying the attribute; a delete leaves them holding a field that no longer exists
	bus.Subscribe(catalog.EventAttributeUpdated, func(ctx context.Context, e events.Event) error {
		for _, id := range e.(*catalog.AttributeUpdated).ProductIDs {
			upsertProduct(ctx, id)
		}
		return nil
	})
	bus.Subscribe(catalog.EventAttributeDeleted, func(ctx context.Context, e events.Event) error {
		for _, id := range e.(*catalog.AttributeDeleted).ProductIDs {
			upsertProduct(ctx, id)
		}
		return nil
	})

	// Coalesced: StockChanged fires per reservation, so a hot product would re-run its ~dozen-query upsert per order line - deduped ids flush together, and is_in_stock lags stock by at most the interval, which "hide out-of-stock" tolerates
	stockChanges := newStockCoalescer(upsertProduct)
	go stockChanges.run(root)
	bus.Subscribe(inventory.EventStockChanged, func(_ context.Context, e events.Event) error {
		stockChanges.add(e.(*inventory.StockChanged).ProductID)
		return nil
	})

	// Collection membership lands in the doc's collection_ids, which the collection landing page filters on - the event was published but nothing consumed it, so products dragged into a collection never appeared there until some unrelated edit happened to re-stamp them
	bus.Subscribe(catalog.EventCollectionProductsChanged, func(ctx context.Context, e events.Event) error {
		evt := e.(*catalog.CollectionProductsChanged)
		for _, id := range evt.Added {
			upsertProduct(ctx, id)
		}
		for _, id := range evt.Removed {
			upsertProduct(ctx, id)
		}
		return nil
	})

	// Channel locale-set changes invalidate the per-(channel, locale) doc fan-out: removed locales leave stale docs that would keep serving `term locale` queries forever, added locales have no docs until products reindex - upsert's delete-then-rebuild per product covers both directions
	bus.Subscribe(channel.EventLocalesChanged, func(ctx context.Context, e events.Event) error {
		channelID := e.(*channel.LocalesChanged).ChannelID
		n, err := products.ReindexChannel(ctx, channelID)
		if err != nil {
			log.Printf("[search] channel %s locale change reindex (after %d products): %v", channelID, n, err)
			return nil
		}
		log.Printf("[search] channel %s locale change: reindexed %d products", channelID, n)
		return nil
	})
	bus.Subscribe(channel.EventDeleted, func(ctx context.Context, e events.Event) error {
		channelID := e.(*channel.Deleted).ChannelID
		if err := products.DeleteChannel(ctx, channelID); err != nil {
			log.Printf("[search] channel %s delete cleanup: %v", channelID, err)
		}
		return nil
	})

	upsertPost := func(ctx context.Context, postID uuid.UUID) {
		if err := posts.Upsert(ctx, postID); err != nil {
			log.Printf("[search] blog post upsert %s: %v", postID, err)
		}
	}
	deletePost := func(ctx context.Context, postID uuid.UUID) {
		if err := posts.Delete(ctx, postID); err != nil {
			log.Printf("[search] blog post delete %s: %v", postID, err)
		}
	}
	bus.Subscribe(blog.EventPostCreated, func(ctx context.Context, e events.Event) error {
		upsertPost(ctx, e.(*blog.PostCreated).PostID)
		return nil
	})
	bus.Subscribe(blog.EventPostUpdated, func(ctx context.Context, e events.Event) error {
		upsertPost(ctx, e.(*blog.PostUpdated).PostID)
		return nil
	})
	bus.Subscribe(blog.EventPostDeleted, func(ctx context.Context, e events.Event) error {
		deletePost(ctx, e.(*blog.PostDeleted).PostID)
		return nil
	})
	bus.Subscribe(blog.EventPostPublished, func(ctx context.Context, e events.Event) error {
		upsertPost(ctx, e.(*blog.PostPublished).PostID)
		return nil
	})
	bus.Subscribe(blog.EventPostUnpublished, func(ctx context.Context, e events.Event) error {
		upsertPost(ctx, e.(*blog.PostUnpublished).PostID)
		return nil
	})
	bus.Subscribe(blog.EventPostContentChanged, func(ctx context.Context, e events.Event) error {
		upsertPost(ctx, e.(*blog.PostContentChanged).PostID)
		return nil
	})
	bus.Subscribe(blog.EventPostListingChanged, func(ctx context.Context, e events.Event) error {
		upsertPost(ctx, e.(*blog.PostListingChanged).PostID)
		return nil
	})
}

// The flush cadence trades index freshness for write pressure - stock-driven
// visibility changes show up within this window
const stockFlushInterval = 30 * time.Second

type stockCoalescer struct {
	mu      sync.Mutex
	pending map[uuid.UUID]struct{}
	upsert  func(ctx context.Context, productID uuid.UUID)
}

func newStockCoalescer(upsert func(ctx context.Context, productID uuid.UUID)) *stockCoalescer {
	return &stockCoalescer{pending: map[uuid.UUID]struct{}{}, upsert: upsert}
}

func (c *stockCoalescer) add(productID uuid.UUID) {
	c.mu.Lock()
	c.pending[productID] = struct{}{}
	c.mu.Unlock()
}

// Pending ids on shutdown drop rather than flush into dying connections - docs reconverge on the next stock event or reindex, the same recovery story as every subscriber failure
func (c *stockCoalescer) run(ctx context.Context) {
	ticker := time.NewTicker(stockFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		c.mu.Lock()
		batch := c.pending
		c.pending = map[uuid.UUID]struct{}{}
		c.mu.Unlock()
		for id := range batch {
			if ctx.Err() != nil {
				return
			}
			c.upsert(ctx, id)
		}
	}
}
