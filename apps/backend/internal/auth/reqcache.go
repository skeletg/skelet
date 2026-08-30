package auth

import (
	"context"
	"sync"
)

type RequestCache struct { // installed by the auth middleware on every request context - callers like the admin service's grant snapshot compute a value at most once per request, even when many resolvers ask concurrently
	mu      sync.Mutex
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	once sync.Once
	val  any
	err  error
}

func newRequestCache() *RequestCache {
	return &RequestCache{entries: make(map[string]*cacheEntry)}
}

func (c *RequestCache) GetOrLoad(key string, load func() (any, error)) (any, error) { // concurrent callers for the same key block until the single in-flight load finishes, then all see the same result
	c.mu.Lock()
	e, ok := c.entries[key]
	if !ok {
		e = &cacheEntry{}
		c.entries[key] = e
	}
	c.mu.Unlock()

	e.once.Do(func() { e.val, e.err = load() })
	return e.val, e.err
}

type requestCacheKey struct{}

func WithRequestCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestCacheKey{}, newRequestCache())
}

func RequestCacheFrom(ctx context.Context) *RequestCache { // nil when the middleware never ran, as in background jobs and tests - callers must treat that as "no caching" and load directly
	c, _ := ctx.Value(requestCacheKey{}).(*RequestCache)
	return c
}
