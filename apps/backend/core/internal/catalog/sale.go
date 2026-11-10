package catalog

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/pricing"
)

// A store's sale reads a page of a dozen by default and two at most - the ranking is kept for MaxSalePage, a minute at a time
const (
	SalePageSize = 12
	MaxSalePage  = 24
	saleScanCap  = 500
	saleCacheTTL = time.Minute
)

// SaleProductIDs is what sells below its strike-through price on the store right now, the biggest saving first. The list is public and
// costs a scan of the store's priced-down listings, so each store's ranking is kept for a minute - a price change shows that much later
func (s *listingService) SaleProductIDs(ctx context.Context, channelID uuid.UUID, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = SalePageSize
	}
	limit = min(limit, MaxSalePage)
	now := time.Now()
	ranked, ok := s.sales.get(channelID, now)
	if !ok {
		cands, err := s.repo.ListSaleCandidates(ctx, channelID, saleScanCap)
		if err != nil {
			return nil, fmt.Errorf("sale products: %w", err)
		}
		ranked = rankSales(cands, now, MaxSalePage)
		s.sales.put(channelID, ranked, now)
	}
	return slices.Clone(ranked[:min(limit, len(ranked))]), nil
}

type saleCache struct {
	mu      sync.Mutex
	entries map[uuid.UUID]saleEntry
}

type saleEntry struct {
	at  time.Time
	ids []uuid.UUID
}

func (c *saleCache) get(channelID uuid.UUID, now time.Time) ([]uuid.UUID, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[channelID]
	return e.ids, ok && now.Sub(e.at) < saleCacheTTL
}

func (c *saleCache) put(channelID uuid.UUID, ids []uuid.UUID, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[uuid.UUID]saleEntry{}
	}
	c.entries[channelID] = saleEntry{at: now, ids: ids}
}

func rankSales(cands []SaleCandidate, now time.Time, limit int) []uuid.UUID {
	type sale struct {
		id     uuid.UUID
		saving int64
		share  float64
	}
	sales := make([]sale, 0, len(cands))
	for _, c := range cands {
		r := pricing.Resolve(c.Pricing, now)
		if r.CompareAt != nil && *r.CompareAt > r.Price {
			saving := *r.CompareAt - r.Price
			sales = append(sales, sale{id: c.ProductID, saving: saving, share: float64(saving) / float64(*r.CompareAt)})
		}
	}
	slices.SortFunc(sales, func(a, b sale) int {
		return cmp.Or(cmp.Compare(b.share, a.share), cmp.Compare(b.saving, a.saving), slices.Compare(a.id[:], b.id[:]))
	})
	out := make([]uuid.UUID, 0, min(limit, len(sales)))
	for _, s := range sales[:min(limit, len(sales))] {
		out = append(out, s.id)
	}
	return out
}
