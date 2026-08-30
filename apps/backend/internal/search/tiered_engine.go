package search

import (
	"context"
	"log"

	"github.com/google/uuid"
)

// DegradedFunc is called whenever the primary engine fails and the backup answers instead. Wired in main.go to mark the
// response uncacheable, so a blip can't pin a degraded page in the response cache for its whole TTL after recovery
type DegradedFunc func(ctx context.Context, op string, cause error)

// TieredEngine serves reads from a primary engine and falls back to a backup.
// The fallback lives HERE rather than in the resolver so every caller gets it
// automatically and the decision is testable in one place.
//
// Primary may be nil - a deployment with no OpenSearch configured runs entirely on
// the backup, which is the shipped production default.
type TieredEngine struct {
	primary  Engine
	backup   Engine
	degraded DegradedFunc
}

func NewTieredEngine(primary, backup Engine, degraded DegradedFunc) *TieredEngine {
	return &TieredEngine{primary: primary, backup: backup, degraded: degraded}
}

func (t *TieredEngine) Name() string {
	if t.primary != nil {
		return t.primary.Name() + "+" + t.backup.Name()
	}
	return t.backup.Name()
}

// note reports the degradation once, in one place, so it can't be forgotten at a call site
//
// Only a CONFIGURED primary that failed counts. A deployment with no primary at all
// is a steady state, not a degradation - reporting it marked every browse and search
// response uncacheable forever, which quietly disabled the front response cache on
// the two hottest storefront paths for the shipped default.
func (t *TieredEngine) note(ctx context.Context, op string, err error) {
	if err == nil {
		return
	}
	log.Printf("[search] %s failed on primary, serving from %s: %v", op, t.backup.Name(), err)
	if t.degraded != nil {
		t.degraded(ctx, op, err)
	}
}

func (t *TieredEngine) SearchProducts(ctx context.Context, query string, channelID uuid.UUID, locale string, limit, offset int, boosts map[uuid.UUID]float64) ([]ProductHit, int64, error) {
	if t.primary != nil {
		hits, total, err := t.primary.SearchProducts(ctx, query, channelID, locale, limit, offset, boosts)
		if err == nil {
			return hits, total, nil
		}
		t.note(ctx, "search", err)
	}
	return t.backup.SearchProducts(ctx, query, channelID, locale, limit, offset, boosts)
}

func (t *TieredEngine) BrowseProducts(ctx context.Context, channelID uuid.UUID, locale, query string, filter BrowseFilter, sortSlug string, limit, offset int, boosts map[uuid.UUID]float64) (BrowseResult, error) {
	if t.primary != nil {
		res, err := t.primary.BrowseProducts(ctx, channelID, locale, query, filter, sortSlug, limit, offset, boosts)
		if err == nil {
			return res, nil
		}
		t.note(ctx, "browse", err)
	}
	return t.backup.BrowseProducts(ctx, channelID, locale, query, filter, sortSlug, limit, offset, boosts)
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
