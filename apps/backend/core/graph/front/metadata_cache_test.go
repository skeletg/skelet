package front

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

func TestMetadataCache(t *testing.T) {
	newCache := func() (*MetadataCache, *time.Time) {
		now := time.Unix(1_000_000, 0)
		c := NewMetadataCache(30 * time.Second)
		c.now = func() time.Time { return now }
		return c, &now
	}
	id := uuid.New()
	counting := func(calls *int, name string) func(context.Context) (catalog.ProductType, error) {
		return func(context.Context) (catalog.ProductType, error) {
			*calls++
			return catalog.ProductType{ID: id, Name: name}, nil
		}
	}

	t.Run("serves a hit until the TTL runs out", func(t *testing.T) {
		c, now := newCache()
		calls := 0
		for range 3 {
			if pt, err := c.productType(context.Background(), id, counting(&calls, "v1")); err != nil || pt.Name != "v1" {
				t.Fatalf("got %q, %v", pt.Name, err)
			}
		}
		if calls != 1 {
			t.Fatalf("fetched %d times inside the TTL, want 1", calls)
		}
		*now = now.Add(30 * time.Second)
		if pt, _ := c.productType(context.Background(), id, counting(&calls, "v2")); pt.Name != "v2" || calls != 2 {
			t.Fatalf("after the TTL got %q with %d fetches, want a refetch", pt.Name, calls)
		}
	})

	t.Run("Invalidate drops every entry", func(t *testing.T) {
		c, _ := newCache()
		calls := 0
		_, _ = c.productType(context.Background(), id, counting(&calls, "v1"))
		_, _ = c.attributeNames(context.Background(), "en", func(context.Context) (attrNameMaps, error) { calls++; return attrNameMaps{}, nil })
		c.Invalidate()
		_, _ = c.productType(context.Background(), id, counting(&calls, "v2"))
		_, _ = c.attributeNames(context.Background(), "en", func(context.Context) (attrNameMaps, error) { calls++; return attrNameMaps{}, nil })
		if calls != 4 {
			t.Fatalf("fetched %d times, want both entries refetched after Invalidate", calls)
		}
	})

	t.Run("a failed fetch isn't cached", func(t *testing.T) {
		c, _ := newCache()
		calls := 0
		_, err := c.productType(context.Background(), id, func(context.Context) (catalog.ProductType, error) {
			calls++
			return catalog.ProductType{}, errors.New("db down")
		})
		if err == nil {
			t.Fatal("want the fetch error back")
		}
		if pt, err := c.productType(context.Background(), id, counting(&calls, "v1")); err != nil || pt.Name != "v1" || calls != 2 {
			t.Fatalf("got %q, %v after %d fetches - want a retry", pt.Name, err, calls)
		}
	})

	t.Run("a fetch that straddles an Invalidate isn't stored", func(t *testing.T) {
		c, _ := newCache()
		calls := 0
		_, _ = c.productType(context.Background(), id, func(context.Context) (catalog.ProductType, error) {
			calls++
			c.Invalidate()
			return catalog.ProductType{ID: id, Name: "pre-edit"}, nil
		})
		if pt, _ := c.productType(context.Background(), id, counting(&calls, "post-edit")); pt.Name != "post-edit" {
			t.Fatalf("got %q, want the post-edit refetch", pt.Name)
		}
	})

	t.Run("a cancelled caller doesn't cancel the shared fetch", func(t *testing.T) {
		c, _ := newCache()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		pt, err := c.productType(ctx, id, func(ctx context.Context) (catalog.ProductType, error) {
			if err := ctx.Err(); err != nil {
				return catalog.ProductType{}, err
			}
			return catalog.ProductType{ID: id, Name: "v1"}, nil
		})
		if err != nil || pt.Name != "v1" {
			t.Fatalf("got %q, %v - want the fetch to run detached from the caller", pt.Name, err)
		}
	})

	t.Run("a nil cache passes straight through", func(t *testing.T) {
		var c *MetadataCache
		calls := 0
		for range 2 {
			_, _ = c.productType(context.Background(), id, counting(&calls, "v1"))
		}
		if calls != 2 {
			t.Fatalf("nil cache fetched %d times, want every call", calls)
		}
	})
}
