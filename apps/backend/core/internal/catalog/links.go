package catalog

// Curated product links (related / upsell / crosssell) + the batch-built "customers also bought" co-occurrence list - Curated links are the operator's voice - also-bought is the fallback the storefront fills rails with when curation is thin

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

type LinkKind string

const (
	LinkRelated   LinkKind = "related"
	LinkUpsell    LinkKind = "upsell"
	LinkCrosssell LinkKind = "crosssell"
)

func (k LinkKind) IsValid() bool {
	return k == LinkRelated || k == LinkUpsell || k == LinkCrosssell
}

func (s *listingService) ListProductLinks(ctx context.Context, productID uuid.UUID, kind LinkKind) ([]uuid.UUID, error) { // curated order
	if !kind.IsValid() {
		return nil, fmt.Errorf("invalid link kind %q", kind)
	}
	return s.repo.ListProductLinks(ctx, productID, kind)
}

// Replaces one kind's link set wholesale, curated order = slice order - self-links are dropped
func (s *listingService) SetProductLinks(ctx context.Context, productID uuid.UUID, kind LinkKind, linked []uuid.UUID) error {
	if !kind.IsValid() {
		return fmt.Errorf("invalid link kind %q", kind)
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
	return s.repo.ReplaceProductLinks(ctx, productID, kind, clean)
}

func (s *listingService) ListAlsoBought(ctx context.Context, productID uuid.UUID, limit int32) ([]uuid.UUID, error) { // co-occurrence recommendations, best first
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	return s.repo.ListAlsoBought(ctx, productID, limit)
}

func (s *listingService) RebuildAlsoBought(ctx context.Context) error { // driven by a scheduled job
	return s.repo.RebuildAlsoBought(ctx)
}

func (r *postgresRepository) ListProductLinks(ctx context.Context, productID uuid.UUID, kind LinkKind) ([]uuid.UUID, error) {
	rows, err := r.queries.ListProductLinks(ctx, db.ListProductLinksParams{ProductID: productID, Kind: string(kind)})
	if err != nil {
		return nil, fmt.Errorf("list product links: %w", err)
	}
	out := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		out[i] = row.LinkedProductID
	}
	return out, nil
}

func (r *postgresRepository) ReplaceProductLinks(ctx context.Context, productID uuid.UUID, kind LinkKind, linked []uuid.UUID) error {
	return r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		if err := q.DeleteProductLinks(ctx, db.DeleteProductLinksParams{ProductID: productID, Kind: string(kind)}); err != nil {
			return fmt.Errorf("clear product links: %w", err)
		}
		for i, id := range linked {
			if err := q.InsertProductLink(ctx, db.InsertProductLinkParams{
				ProductID:       productID,
				LinkedProductID: id,
				Kind:            string(kind),
				SortOrder:       int32(i),
			}); err != nil {
				return fmt.Errorf("insert product link: %w", err)
			}
		}
		return nil
	})
}

func (r *postgresRepository) ListAlsoBought(ctx context.Context, productID uuid.UUID, limit int32) ([]uuid.UUID, error) {
	ids, err := r.queries.ListAlsoBought(ctx, db.ListAlsoBoughtParams{ProductID: productID, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("list also bought: %w", err)
	}
	return ids, nil
}

// Clear and rebuild are separate statements in one transaction - fused into one statement via a DELETE CTE, the insert
// never saw the deletion (one snapshot per statement) and every rebuild after the first failed on the primary key
func (r *postgresRepository) RebuildAlsoBought(ctx context.Context) error {
	return r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		if err := q.ClearAlsoBought(ctx); err != nil {
			return fmt.Errorf("clear also bought: %w", err)
		}
		if err := q.RebuildAlsoBought(ctx); err != nil {
			return fmt.Errorf("rebuild also bought: %w", err)
		}
		return nil
	})
}
