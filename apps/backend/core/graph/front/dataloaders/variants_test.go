package dataloaders

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

type variantSpy struct {
	options                map[uuid.UUID][]catalog.ProductOption
	links                  map[uuid.UUID]catalog.ConfigurableParentLink
	optionCalls, linkCalls atomic.Int32
}

func (s *variantSpy) GetProductOptionsByProductIDs(_ context.Context, ids []uuid.UUID, _ string) (map[uuid.UUID][]catalog.ProductOption, error) {
	s.optionCalls.Add(1)
	out := map[uuid.UUID][]catalog.ProductOption{}
	for _, id := range ids {
		if o, ok := s.options[id]; ok {
			out[id] = o
		}
	}
	return out, nil
}

func (s *variantSpy) ListConfigurableParentByChildIDs(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]catalog.ConfigurableParentLink, error) {
	s.linkCalls.Add(1)
	out := map[uuid.UUID]catalog.ConfigurableParentLink{}
	for _, id := range ids {
		if l, ok := s.links[id]; ok {
			out[id] = l
		}
	}
	return out, nil
}

func (s *variantSpy) ListReadyProductMediaByProductIDs(context.Context, []uuid.UUID) (map[uuid.UUID][]catalog.ProductMedia, error) {
	return nil, nil
}

func (s *variantSpy) ListProductMediaRolesByProductIDs(context.Context, []uuid.UUID) (map[uuid.UUID]map[catalog.MediaRole]uuid.UUID, error) {
	return nil, nil
}

func TestEveryVariantOfAGroupResolvesFromOneFetch(t *testing.T) {
	parent := uuid.New()
	children := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	parentOpt := catalog.ProductOption{ID: uuid.New(), ProductID: parent}
	ownOpt := catalog.ProductOption{ID: uuid.New(), ProductID: children[1]}
	spy := &variantSpy{
		options: map[uuid.UUID][]catalog.ProductOption{parent: {parentOpt}, children[1]: {ownOpt}},
		links:   map[uuid.UUID]catalog.ConfigurableParentLink{},
	}
	for i, c := range children {
		spy.links[c] = catalog.ConfigurableParentLink{ParentID: parent, VariantSortOrder: int32(i)}
	}
	l := New(Deps{}) // a nil service panics if the batch loaders are reached
	l.catalog = spy
	l.RegisterVariants(parent, children)
	ctx := context.Background()

	var wg sync.WaitGroup
	for _, c := range append([]uuid.UUID{parent}, children...) {
		wg.Go(func() {
			if _, err := l.LoadProductOptions(ctx, ProductOptionsKey{ProductID: c, Locale: "en"}); err != nil {
				t.Errorf("options %s: %v", c, err)
			}
			if _, err := l.LoadConfigurableParent(ctx, c); err != nil {
				t.Errorf("parent %s: %v", c, err)
			}
		})
	}
	wg.Wait()
	if n, m := spy.optionCalls.Load(), spy.linkCalls.Load(); n != 1 || m != 1 {
		t.Fatalf("want one fetch per field for the whole group, got options=%d links=%d", n, m)
	}

	got, _ := l.LoadProductOptions(ctx, ProductOptionsKey{ProductID: children[1], Locale: "en"})
	if len(got) != 1 || got[0].ID != ownOpt.ID {
		t.Fatalf("a variant keeps its own options, got %+v", got)
	}
	if got, _ := l.LoadProductOptions(ctx, ProductOptionsKey{ProductID: children[0], Locale: "en"}); got == nil || len(got) != 0 {
		t.Fatalf("a variant without options gets an empty non-nil slice, got %v", got)
	}
	if link, _ := l.LoadConfigurableParent(ctx, children[2]); link.ParentID != parent || link.VariantSortOrder != 2 {
		t.Fatalf("links come from the fetch, not a guess, got %+v", link)
	}

	if _, err := l.LoadProductOptions(ctx, ProductOptionsKey{ProductID: children[0], Locale: "de"}); err != nil {
		t.Fatal(err)
	}
	if n := spy.optionCalls.Load(); n != 2 {
		t.Fatalf("a second locale is its own fetch, got %d calls", n)
	}
}
