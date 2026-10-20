package front

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

const metadataFetchTimeout = 5 * time.Second

// Product types and localized attribute names shared across requests - every caller gets the same value, so treat it as read-only
type MetadataCache struct {
	ttl    time.Duration
	now    func() time.Time
	flight singleflight.Group

	mu    sync.Mutex
	gen   uint64
	types map[uuid.UUID]metadataEntry[catalog.ProductType]
	names map[string]metadataEntry[attrNameMaps]
}

type metadataEntry[T any] struct {
	v  T
	at time.Time
}

func NewMetadataCache(ttl time.Duration) *MetadataCache {
	return &MetadataCache{
		ttl:   ttl,
		now:   time.Now,
		types: map[uuid.UUID]metadataEntry[catalog.ProductType]{},
		names: map[string]metadataEntry[attrNameMaps]{},
	}
}

func (c *MetadataCache) Invalidate() {
	c.mu.Lock()
	c.gen++
	clear(c.types)
	clear(c.names)
	c.mu.Unlock()
}

func (c *MetadataCache) productType(ctx context.Context, id uuid.UUID, fetch func(context.Context) (catalog.ProductType, error)) (catalog.ProductType, error) {
	if c == nil {
		return fetch(ctx)
	}
	return metadataLoad(ctx, c, c.types, id, "pt:"+id.String(), fetch)
}

func (c *MetadataCache) attributeNames(ctx context.Context, locale string, fetch func(context.Context) (attrNameMaps, error)) (attrNameMaps, error) {
	if c == nil {
		return fetch(ctx)
	}
	return metadataLoad(ctx, c, c.names, locale, "names:"+locale, fetch)
}

// One request's disconnect mustn't fail everyone sharing the fetch, and a fetch that began before an Invalidate isn't stored
func metadataLoad[K comparable, T any](ctx context.Context, c *MetadataCache, m map[K]metadataEntry[T], key K, flightKey string, fetch func(context.Context) (T, error)) (T, error) {
	c.mu.Lock()
	if e, ok := m[key]; ok && c.now().Sub(e.at) < c.ttl {
		c.mu.Unlock()
		return e.v, nil
	}
	gen := c.gen
	c.mu.Unlock()
	v, err, _ := c.flight.Do(flightKey, func() (any, error) {
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), metadataFetchTimeout)
		defer cancel()
		v, err := fetch(fctx)
		if err != nil {
			return v, err
		}
		c.mu.Lock()
		if c.gen == gen {
			m[key] = metadataEntry[T]{v: v, at: c.now()}
		}
		c.mu.Unlock()
		return v, nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return v.(T), nil
}
