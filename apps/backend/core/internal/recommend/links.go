// Package recommend owns the product rails: curated links (related / upsell / crosssell) and the batch-built "customers also bought" list, optionally answered by a remote recommender - Curated links are the operator's voice, also-bought fills rails when curation is thin
package recommend

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type LinkKind string

// Also-bought sizing - the storefront over-fetches by AlsoBoughtOverfetch so its channel filter can't leave a rail short
const (
	AlsoBoughtDefaultLimit int32 = 8
	AlsoBoughtMaxLimit     int32 = 24
	AlsoBoughtOverfetch    int32 = 3
	alsoBoughtFetchMax           = AlsoBoughtMaxLimit * AlsoBoughtOverfetch
	alsoBoughtRebuildTopN        = alsoBoughtFetchMax
	maxProductLinks              = 200
	// Older orders say more about last year's catalog than this one's, and the self-join grows with every order ever kept
	alsoBoughtLookback = 365 * 24 * time.Hour
	// A remote recommender is an HTTP call per product - the cart asks it about its newest lines only
	cartRemoteLookups = 3
)

const (
	LinkRelated   LinkKind = "related"
	LinkUpsell    LinkKind = "upsell"
	LinkCrosssell LinkKind = "crosssell"
)

var ErrInvalidLinkKind = errors.New("invalid product link kind")

func (k LinkKind) IsValid() bool {
	return k == LinkRelated || k == LinkUpsell || k == LinkCrosssell
}

func ClampAlsoBoughtLimit(limit *int32) int32 {
	if limit == nil {
		return AlsoBoughtDefaultLimit
	}
	return min(max(*limit, 1), AlsoBoughtMaxLimit)
}

// All recommend needs of the catalog: a link write is a product change the storefront caches must hear about
type ProductChanges interface {
	ProductChanged(ctx context.Context, productID uuid.UUID, changed ...string)
}

type Service struct {
	queries  *db.Queries
	tx       *txrunner.Manager
	products ProductChanges
	remote   RemoteRecommendationSource
}

func NewService(queries *db.Queries, tx *txrunner.Manager, products ProductChanges) *Service {
	return &Service{queries: queries, tx: tx, products: products}
}

// Optional - nil leaves recommendations on the native co-occurrence table
func (s *Service) SetRemoteRecommendations(src RemoteRecommendationSource) { s.remote = src }

func (s *Service) ListProductLinks(ctx context.Context, productID uuid.UUID, kind LinkKind) ([]uuid.UUID, error) { // curated order
	if !kind.IsValid() {
		return nil, fmt.Errorf("%w: %q", ErrInvalidLinkKind, kind)
	}
	rows, err := s.queries.ListProductLinks(ctx, db.ListProductLinksParams{ProductID: productID, Kind: string(kind)})
	if err != nil {
		return nil, fmt.Errorf("list product links: %w", err)
	}
	out := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		out[i] = row.LinkedProductID
	}
	return out, nil
}

// Replaces one kind's link set wholesale, curated order = slice order - self-links are dropped
func (s *Service) SetProductLinks(ctx context.Context, productID uuid.UUID, kind LinkKind, linked []uuid.UUID) error {
	if !kind.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidLinkKind, kind)
	}
	if len(linked) > maxProductLinks {
		return fmt.Errorf("%w: %d links, at most %d", catalog.ErrTooManyItems, len(linked), maxProductLinks)
	}
	clean := make([]uuid.UUID, 0, len(linked))
	seen := map[uuid.UUID]bool{}
	for _, id := range linked {
		if id == productID || seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	if err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := s.queries.WithTx(t)
		if err := q.DeleteProductLinks(ctx, db.DeleteProductLinksParams{ProductID: productID, Kind: string(kind)}); err != nil {
			return fmt.Errorf("clear product links: %w", err)
		}
		for i, id := range clean {
			if err := q.InsertProductLink(ctx, db.InsertProductLinkParams{
				ProductID: productID, LinkedProductID: id, Kind: string(kind), SortOrder: int32(i),
			}); err != nil {
				if pgdb.IsForeignKeyViolation(err) {
					return fmt.Errorf("%w: linked product %s", catalog.ErrProductNotFound, id)
				}
				return fmt.Errorf("insert product link: %w", err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if s.products != nil {
		s.products.ProductChanged(ctx, productID, "links")
	}
	return nil
}

// Co-occurrence recommendations, best first - a wired remote recommender answers first, so both the PDP and the cart rail ride on it
func (s *Service) ListAlsoBought(ctx context.Context, productID uuid.UUID, limit int32) ([]uuid.UUID, error) {
	limit = clampFetch(limit)
	if s.remote != nil {
		if ids, ok := s.remote.Recommend(ctx, productID, StrategyAlsoBought, limit); ok {
			return ids, nil
		}
	}
	ids, err := s.queries.ListAlsoBought(ctx, db.ListAlsoBoughtParams{ProductID: productID, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("list also bought: %w", err)
	}
	return ids, nil
}

// Clamped before either source answers - a 0 limit reached the remote provider as "no cap"
func clampFetch(limit int32) int32 {
	if limit <= 0 {
		return AlsoBoughtDefaultLimit
	}
	return min(limit, alsoBoughtFetchMax)
}

// Each product's crosssell links in curated order, one query for the whole cart
func (s *Service) CrosssellsFor(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	out := map[uuid.UUID][]uuid.UUID{}
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := s.queries.ListProductLinksForProducts(ctx, db.ListProductLinksForProductsParams{ProductIds: productIDs, Kind: string(LinkCrosssell)})
	if err != nil {
		return nil, fmt.Errorf("list crosssells: %w", err)
	}
	for _, r := range rows {
		out[r.ProductID] = append(out[r.ProductID], r.LinkedProductID)
	}
	return out, nil
}

// Also-bought for every product in one native read - a wired remote recommender answers for the last cartRemoteLookups products, concurrently, where it has an answer
func (s *Service) AlsoBoughtFor(ctx context.Context, productIDs []uuid.UUID, perProduct int32) (map[uuid.UUID][]uuid.UUID, error) {
	out := map[uuid.UUID][]uuid.UUID{}
	if len(productIDs) == 0 {
		return out, nil
	}
	perProduct = clampFetch(perProduct)
	rows, err := s.queries.ListAlsoBoughtForProducts(ctx, db.ListAlsoBoughtForProductsParams{ProductIds: productIDs, PerProduct: perProduct})
	if err != nil {
		return nil, fmt.Errorf("list also bought: %w", err)
	}
	for _, r := range rows {
		out[r.ProductID] = append(out[r.ProductID], r.OtherProductID)
	}
	if s.remote == nil {
		return out, nil
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range productIDs[max(len(productIDs)-cartRemoteLookups, 0):] {
		wg.Go(func() {
			if ids, ok := s.remote.Recommend(ctx, id, StrategyAlsoBought, perProduct); ok {
				mu.Lock()
				out[id] = ids
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return out, nil
}

// Driven by a scheduled job - clear and rebuild are separate statements in one transaction, since fused into one statement via a DELETE CTE the insert never saw the deletion and every rebuild after the first failed on the primary key
func (s *Service) RebuildAlsoBought(ctx context.Context) error {
	return s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := s.queries.WithTx(t)
		if err := q.ClearAlsoBought(ctx); err != nil {
			return fmt.Errorf("clear also bought: %w", err)
		}
		if err := q.RebuildAlsoBought(ctx, db.RebuildAlsoBoughtParams{TopN: alsoBoughtRebuildTopN, Since: time.Now().Add(-alsoBoughtLookback)}); err != nil {
			return fmt.Errorf("rebuild also bought: %w", err)
		}
		return nil
	})
}
