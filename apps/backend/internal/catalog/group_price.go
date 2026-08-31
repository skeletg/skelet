package catalog

// Customer-group tier pricing (quantity breaks) - Rows live in catalog_product_group_prices; resolution (lowest applicable price wins against the effective price) is internal/pricing.BestTier - the cart adapter pre-filters scope and passes the tiers through

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/google/uuid"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

type GroupPrice struct { // one tier row - nil ChannelID/CustomerGroupID = applies to every channel / every group
	ID              uuid.UUID
	ProductID       uuid.UUID
	ChannelID       *uuid.UUID
	CustomerGroupID *uuid.UUID
	MinQty          float64
	Price           int64
}

type GroupPriceInput struct {
	ChannelID       *uuid.UUID
	CustomerGroupID *uuid.UUID
	MinQty          float64
	Price           int64
}

func (s *listingService) ListGroupPrices(ctx context.Context, productID uuid.UUID) ([]GroupPrice, error) { // min-qty ascending
	return s.repo.ListGroupPrices(ctx, productID)
}

// SetGroupPrices replaces the product's tier rows wholesale (the admin editor edits the full set) - Validates qty/price sanity; uniqueness is the DB constraint's job
func (s *listingService) SetGroupPrices(ctx context.Context, productID uuid.UUID, inputs []GroupPriceInput) ([]GroupPrice, error) {
	for _, in := range inputs {
		if in.MinQty <= 0 {
			return nil, fmt.Errorf("tier min quantity must be > 0 (got %g)", in.MinQty)
		}
		if in.Price < 0 {
			return nil, fmt.Errorf("tier price must be >= 0 (got %d)", in.Price)
		}
	}
	// Deterministic order keeps the unique-violation error stable
	sorted := make([]GroupPriceInput, len(inputs))
	copy(sorted, inputs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].MinQty < sorted[j].MinQty })
	return s.repo.ReplaceGroupPrices(ctx, productID, sorted)
}

func (r *postgresRepository) ListGroupPrices(ctx context.Context, productID uuid.UUID) ([]GroupPrice, error) {
	rows, err := r.queries.ListProductGroupPrices(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list group prices: %w", err)
	}
	out := make([]GroupPrice, len(rows))
	for i, row := range rows {
		out[i] = toGroupPrice(row)
	}
	return out, nil
}

func (r *postgresRepository) ReplaceGroupPrices(ctx context.Context, productID uuid.UUID, inputs []GroupPriceInput) ([]GroupPrice, error) {
	var out []GroupPrice
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		if err := q.DeleteProductGroupPrices(ctx, productID); err != nil {
			return fmt.Errorf("clear group prices: %w", err)
		}
		out = make([]GroupPrice, 0, len(inputs))
		for _, in := range inputs {
			row, err := q.InsertProductGroupPrice(ctx, db.InsertProductGroupPriceParams{
				ProductID:       productID,
				ChannelID:       nullableUUIDPtr(in.ChannelID),
				CustomerGroupID: nullableUUIDPtr(in.CustomerGroupID),
				MinQty:          in.MinQty,
				Price:           in.Price,
			})
			if err != nil {
				return fmt.Errorf("insert group price: %w", err)
			}
			out = append(out, toGroupPrice(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func toGroupPrice(row db.CatalogProductGroupPrice) GroupPrice {
	gp := GroupPrice{
		ID:        row.ID,
		ProductID: row.ProductID,
		MinQty:    row.MinQty,
		Price:     row.Price,
	}
	if row.ChannelID.Valid {
		v := row.ChannelID.UUID
		gp.ChannelID = &v
	}
	if row.CustomerGroupID.Valid {
		v := row.CustomerGroupID.UUID
		gp.CustomerGroupID = &v
	}
	return gp
}

func nullableUUIDPtr(p *uuid.UUID) uuid.NullUUID {
	if p == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *p, Valid: true}
}
