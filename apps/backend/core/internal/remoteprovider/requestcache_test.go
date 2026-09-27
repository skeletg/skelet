package remoteprovider

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

type countingRepo struct {
	*MemRepo
	lists atomic.Int32
}

func (r *countingRepo) ListActiveByKind(ctx context.Context, kind Kind, channelID *uuid.UUID) ([]Provider, error) {
	r.lists.Add(1)
	return r.MemRepo.ListActiveByKind(ctx, kind, channelID)
}

func (r *countingRepo) ListActiveForChannel(ctx context.Context, channelID *uuid.UUID) ([]Provider, error) {
	r.lists.Add(1)
	return r.MemRepo.ListActiveForChannel(ctx, channelID)
}

func TestARequestReadsItsChannelsProvidersOnce(t *testing.T) {
	repo := &countingRepo{MemRepo: NewMemRepo()}
	svc := NewService(repo)
	svc.AllowPrivateTargets = true
	created, err := svc.Create(context.Background(), CreateInput{Kind: KindPricing, Slug: "pricing-test", Name: "P", TargetURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	ch := uuid.New()

	ctx := WithRequestCache(context.Background())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := svc.ListActiveByKind(ctx, KindPricing, &ch); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if _, _, err := svc.ActiveFor(ctx, KindPricing, &ch); err != nil {
		t.Fatal(err)
	}
	if n := repo.lists.Load(); n != 1 {
		t.Fatalf("concurrent reads of one kind in one request hit the repo %d times, want 1", n)
	}

	got, _ := svc.ListActiveByKind(ctx, KindPricing, &ch)
	got[0].Name = "mutated"
	if again, _ := svc.ListActiveByKind(ctx, KindPricing, &ch); again[0].Name != "P" {
		t.Fatal("a caller mutating its slice must not change the cached answer")
	}

	if ps, err := svc.ListActiveByKind(ctx, KindSearch, &ch); err != nil || len(ps) != 0 {
		t.Fatalf("another kind answers from the same read with only its own providers, got %v, %v", ps, err)
	}
	if n := repo.lists.Load(); n != 1 {
		t.Fatalf("every kind for one channel shares the request's single read, got %d repo calls", n)
	}
	if _, err := svc.ListActiveByKind(ctx, KindPricing, nil); err != nil {
		t.Fatal(err)
	}
	if n := repo.lists.Load(); n != 2 {
		t.Fatalf("a different channel scope is its own read, got %d repo calls", n)
	}

	if _, err := svc.SetActive(ctx, created.Provider.ID, false); err != nil {
		t.Fatal(err)
	}
	if ps, _ := svc.ListActiveByKind(ctx, KindPricing, &ch); len(ps) != 0 {
		t.Fatalf("a provider switched off in this request is still listed: %v", ps)
	}

	before := repo.lists.Load()
	for range 2 {
		if _, err := svc.ListActiveByKind(context.Background(), KindPricing, &ch); err != nil {
			t.Fatal(err)
		}
	}
	if n := repo.lists.Load() - before; n != 2 {
		t.Fatalf("outside a request scope every read must hit the repo, got %d of 2", n)
	}
}
