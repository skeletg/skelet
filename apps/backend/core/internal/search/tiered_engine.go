package search

import (
	"context"
	"errors"
	"log"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// DegradedFunc is called whenever the primary engine fails and the backup answers instead. Wired in main.go to mark the
// response uncacheable, so a blip can't pin a degraded page in the response cache for its whole TTL after recovery
type DegradedFunc func(ctx context.Context, op string, cause error)

var ErrFallbackSaturated = errors.New("search: fallback tier at capacity")

var errPrimaryOpen = errors.New("search: primary skipped while its circuit is open")

const (
	defaultFallbackLimit = 4
	fallbackWait         = 500 * time.Millisecond
	shedLogEvery         = 10 * time.Second
)

// TieredEngine serves reads from a primary engine and falls back to a backup. The fallback lives HERE rather than in the resolver so every caller gets it automatically and the decision is testable in one place.
type TieredEngine struct {
	primary  Engine
	backup   Engine
	degraded DegradedFunc
	breaker  *breaker
	limit    int
	fallback chan struct{}
	wait     time.Duration
	shed     atomic.Int64
	shedLog  atomic.Int64
}

type TierOption func(*TieredEngine)

func WithFallbackLimit(n int) TierOption {
	return func(t *TieredEngine) {
		if n > 0 {
			t.limit = n
		}
	}
}

func NewTieredEngine(primary, backup Engine, degraded DegradedFunc, opts ...TierOption) *TieredEngine {
	t := &TieredEngine{primary: primary, backup: backup, degraded: degraded, breaker: newBreaker(), limit: defaultFallbackLimit, wait: fallbackWait}
	for _, o := range opts {
		o(t)
	}
	t.fallback = make(chan struct{}, t.limit)
	return t
}

func (t *TieredEngine) Name() string {
	if t.primary != nil {
		return t.primary.Name() + "+" + t.backup.Name()
	}
	return t.backup.Name()
}

// note reports the degradation once, in one place, so it can't be forgotten at a call site
func (t *TieredEngine) note(ctx context.Context, op string, err error) {
	if err == nil {
		return
	}
	log.Printf("[search] %s failed on primary, serving from %s: %v", op, t.backup.Name(), err)
	if t.degraded != nil {
		t.degraded(ctx, op, err)
	}
}

// The backup shares the commerce pool, so answering for a failed primary is capped - unbounded, a search outage under load starves carts and checkout
func tiered[T any](ctx context.Context, t *TieredEngine, op string, run func(Engine) (T, error)) (T, error) {
	var zero T
	if t.primary == nil {
		return run(t.backup)
	}
	if try, probe := t.breaker.allow(); try {
		res, err := run(t.primary)
		if err == nil {
			if _, recovered := t.breaker.record(probe, false); recovered {
				log.Printf("[search] %s is answering again, closing its circuit", t.primary.Name())
			}
			return res, nil
		}
		// A caller that's gone isn't a primary outage - falling back re-ran the query on Postgres for nobody and flagged it degraded
		if ctx.Err() != nil {
			t.breaker.release(probe)
			return zero, ctx.Err()
		}
		if tripped, _ := t.breaker.record(probe, true); tripped {
			log.Printf("[search] %s keeps failing, skipping it for %s: %v", t.primary.Name(), breakerCooldown, err)
		}
		t.note(ctx, op, err)
	} else if t.degraded != nil {
		t.degraded(ctx, op, errPrimaryOpen)
	}
	if !t.acquire(ctx) {
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		t.noteShed()
		return zero, ErrFallbackSaturated
	}
	defer func() { <-t.fallback }()
	return run(t.backup)
}

func (t *TieredEngine) acquire(ctx context.Context) bool {
	select {
	case t.fallback <- struct{}{}:
		return true
	default:
	}
	timer := time.NewTimer(t.wait)
	defer timer.Stop()
	select {
	case t.fallback <- struct{}{}:
		return true
	case <-timer.C:
	case <-ctx.Done():
	}
	return false
}

func (t *TieredEngine) noteShed() {
	t.shed.Add(1)
	now, last := time.Now().UnixNano(), t.shedLog.Load()
	if now-last < int64(shedLogEvery) || !t.shedLog.CompareAndSwap(last, now) {
		return
	}
	log.Printf("[search] %s fallback full at %d concurrent, shed %d requests", t.backup.Name(), t.limit, t.shed.Swap(0))
}

type searchPage struct {
	hits  []ProductHit
	total int64
}

func (t *TieredEngine) SearchProducts(ctx context.Context, query string, channelID uuid.UUID, locale string, limit, offset int, boosts map[uuid.UUID]float64) ([]ProductHit, int64, error) {
	page, err := tiered(ctx, t, "search", func(e Engine) (searchPage, error) {
		hits, total, err := e.SearchProducts(ctx, query, channelID, locale, limit, offset, boosts)
		return searchPage{hits, total}, err
	})
	return page.hits, page.total, err
}

func (t *TieredEngine) BrowseProducts(ctx context.Context, channelID uuid.UUID, locale, query string, filter BrowseFilter, sortSlug string, limit, offset int, boosts map[uuid.UUID]float64) (BrowseResult, error) {
	return tiered(ctx, t, "browse", func(e Engine) (BrowseResult, error) {
		return e.BrowseProducts(ctx, channelID, locale, query, filter, sortSlug, limit, offset, boosts)
	})
}

// ListSortOptions comes from whichever engine will actually answer, so the
// storefront never offers a sort the serving engine can't honour
func (t *TieredEngine) ListSortOptions(ctx context.Context) []SortOption {
	if t.primary != nil {
		return t.primary.ListSortOptions(ctx)
	}
	return t.backup.ListSortOptions(ctx)
}

var _ Engine = (*TieredEngine)(nil)
