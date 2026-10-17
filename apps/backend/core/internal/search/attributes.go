package search

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

type attributeLister interface {
	ListAttributes(ctx context.Context) ([]catalog.Attribute, error)
}

// AttributeCache serves the read paths' attribute list - every keystroke loaded every attribute with its value rows. The TTL bounds
// what another replica's edit leaves stale, Invalidate clears this one's. Indexing reads the catalogue directly
type AttributeCache struct {
	src attributeLister
	ttl time.Duration

	mu      sync.Mutex
	attrs   []catalog.Attribute
	fetched time.Time
}

func NewAttributeCache(src attributeLister, ttl time.Duration) *AttributeCache {
	return &AttributeCache{src: src, ttl: ttl}
}

func (c *AttributeCache) List(ctx context.Context) ([]catalog.Attribute, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.attrs != nil && time.Since(c.fetched) < c.ttl {
		return slices.Clone(c.attrs), nil
	}
	attrs, err := c.src.ListAttributes(ctx)
	if err != nil {
		return nil, err
	}
	c.attrs, c.fetched = attrs, time.Now()
	return slices.Clone(attrs), nil
}

func (c *AttributeCache) Invalidate() {
	c.mu.Lock()
	c.attrs = nil
	c.mu.Unlock()
}

func listAttributes(ctx context.Context, cache *AttributeCache, src attributeLister) ([]catalog.Attribute, error) {
	if cache != nil {
		return cache.List(ctx)
	}
	return src.ListAttributes(ctx)
}
