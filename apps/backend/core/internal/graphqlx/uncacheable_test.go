package graphqlx_test

import (
	"context"
	"sync"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/graphqlx"
)

// gqlgen resolves top-level fields concurrently, so two degraded searches mark the same request at once - run with -race
func TestConcurrentMarksAreSafe(t *testing.T) {
	ctx := graphqlx.WithUncacheableFlag(context.Background())
	if graphqlx.IsUncacheable(ctx) {
		t.Fatal("a fresh request reads as uncacheable")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			graphqlx.MarkUncacheable(ctx)
			_ = graphqlx.IsUncacheable(ctx)
		})
	}
	wg.Wait()
	if !graphqlx.IsUncacheable(ctx) {
		t.Fatal("marked request still reads as cacheable")
	}
}

func TestMarkingWithoutAHolderIsANoOp(t *testing.T) {
	graphqlx.MarkUncacheable(context.Background())
	if graphqlx.IsUncacheable(context.Background()) {
		t.Fatal("a request with no holder reads as uncacheable")
	}
}
