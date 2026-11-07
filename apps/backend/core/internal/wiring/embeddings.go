package wiring

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/embedding"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/search"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func newEmbeddingIndexer(q *db.Queries, tx *txrunner.Manager, svc *ai.Service) *embedding.Indexer {
	return embedding.New(q, tx, svc.Embedder())
}

// One per process, so the shoppers' questions it has already turned into vectors are shared by every turn
func newEmbeddingSearcher(q *db.Queries, tx *txrunner.Manager, svc *ai.Service, limits LimitsRedis, cfg *config.Service) *embedding.Searcher {
	s := embedding.NewSearcher(q, tx, svc.Embedder())
	s.SetPace(ratelimit.NewRedis(limits.Client, 0, time.Minute), channelInt(cfg, search.MeaningPacePath, search.MeaningPaceDefault),
		channelInt(cfg, search.MeaningStorePacePath, search.MeaningStorePaceDefault))
	return s
}

// A per-store number from the settings, read on every call - an unreadable one counts as its default
func channelInt(cfg *config.Service, path string, def int) func(ctx context.Context, channelID uuid.UUID) int {
	return func(ctx context.Context, channelID uuid.UUID) int {
		v, err := cfg.GetInt(ctx, path, config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID})
		if err != nil {
			return def
		}
		return int(min(max(v, 0), math.MaxInt32))
	}
}

// storefrontMeaning hands the storefront's listings what the embeddings find
type storefrontMeaning struct{ s *embedding.Searcher }

func (m storefrontMeaning) Near(ctx context.Context, channelID uuid.UUID, locales []string, query string, limit int) ([]search.Near, error) {
	hits, err := m.s.Products(ctx, channelID, locales, query, limit)
	out := make([]search.Near, len(hits))
	for i, h := range hits {
		out[i] = search.Near{ID: h.ProductID, Distance: h.Distance}
	}
	return out, err
}

type embeddingQueue interface {
	Enqueue(ctx context.Context, source string, ids ...uuid.UUID) error
	EnqueueCategory(ctx context.Context, categoryID uuid.UUID) error
	EnqueueProductType(ctx context.Context, productTypeID uuid.UUID) error
}

// Every save that can change what a product or page says queues it - the pass skips one whose words came out the same. These
// events aren't durable, so a failed enqueue is tried again through a short outage; past that the vector waits for the next edit
func registerEmbeddingSubscribers(bus *events.Bus, x embeddingQueue) {
	product := func(ctx context.Context, ids ...uuid.UUID) error {
		return x.Enqueue(ctx, embedding.SourceProduct, ids...)
	}
	page := func(ctx context.Context, ids ...uuid.UUID) error { return x.Enqueue(ctx, embedding.SourcePage, ids...) }
	onEvent(bus, catalog.EventProductCreated, func(ctx context.Context, e *catalog.ProductCreated) error { return product(ctx, e.ProductID) })
	onEvent(bus, catalog.EventProductUpdated, func(ctx context.Context, e *catalog.ProductUpdated) error { return product(ctx, e.ProductID) })
	onEvent(bus, catalog.EventProductDeleted, func(ctx context.Context, e *catalog.ProductDeleted) error { return product(ctx, e.ProductID) })
	onEvent(bus, catalog.EventProductListingChanged, func(ctx context.Context, e *catalog.ProductListingChanged) error { return product(ctx, e.ProductID) })
	onEvent(bus, catalog.EventAttributeUpdated, func(ctx context.Context, e *catalog.AttributeUpdated) error { return product(ctx, e.ProductIDs...) })
	onEvent(bus, catalog.EventAttributeDeleted, func(ctx context.Context, e *catalog.AttributeDeleted) error { return product(ctx, e.ProductIDs...) })
	onEvent(bus, catalog.EventCategoryUpdated, func(ctx context.Context, e *catalog.CategoryUpdated) error {
		return x.EnqueueCategory(ctx, e.CategoryID)
	})
	onEvent(bus, catalog.EventProductTypeRenamed, func(ctx context.Context, e *catalog.ProductTypeRenamed) error {
		return x.EnqueueProductType(ctx, e.ProductTypeID)
	})
	onEvent(bus, cms.EventPagePublished, func(ctx context.Context, e *cms.PagePublished) error { return page(ctx, e.PageID) })
	onEvent(bus, cms.EventPageUnpublished, func(ctx context.Context, e *cms.PageUnpublished) error { return page(ctx, e.PageID) })
	onEvent(bus, cms.EventPageChanged, func(ctx context.Context, e *cms.PageChanged) error { return page(ctx, e.PageID) })
}

func onEvent[E events.Event](bus *events.Bus, name string, handle func(context.Context, E) error) {
	bus.Subscribe(name, func(ctx context.Context, e events.Event) error {
		ev, ok := e.(E)
		if !ok {
			return fmt.Errorf("embedding: %s carried a %T", name, e)
		}
		err := handle(ctx, ev)
		for _, wait := range enqueueRetries {
			if err == nil {
				return nil
			}
			select {
			case <-ctx.Done():
				return errors.Join(err, ctx.Err())
			case <-time.After(wait):
			}
			err = handle(ctx, ev)
		}
		if err != nil {
			slog.ErrorContext(ctx, "embedding: queueing a change failed, its vector waits for the next edit", "event", name, "err", err)
		}
		return err
	})
}

var enqueueRetries = []time.Duration{time.Second, 5 * time.Second, 20 * time.Second}
