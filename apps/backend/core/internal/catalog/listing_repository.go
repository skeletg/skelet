package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

func (r *postgresRepository) UpsertProductChannelListing(ctx context.Context, input ProductChannelListingInput) (ProductChannelListing, error) {
	row, err := r.queries.UpsertProductChannelListing(ctx, db.UpsertProductChannelListingParams{
		ProductID:      input.ProductID,
		ChannelID:      input.ChannelID,
		Price:          sqlnull.Int64(input.Price),
		IsPublished:    input.IsPublished,
		Url:            sqlnull.String(input.URL),
		CompareAtPrice: sqlnull.Int64(input.CompareAtPrice),
		SpecialPrice:   sqlnull.Int64(input.SpecialPrice),
		SpecialFrom:    sqlnull.Time(input.SpecialFrom),
		SpecialTo:      sqlnull.Time(input.SpecialTo),
	})
	if err != nil {
		return ProductChannelListing{}, fmt.Errorf("upsert listing: %w", err)
	}
	return toListing(row), nil
}

func (r *postgresRepository) GetProductChannelListing(ctx context.Context, productID, channelID uuid.UUID) (ProductChannelListing, error) {
	row, err := r.queries.GetProductChannelListing(ctx, db.GetProductChannelListingParams{
		ProductID: productID,
		ChannelID: channelID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductChannelListing{}, ErrListingNotFound
		}
		return ProductChannelListing{}, fmt.Errorf("get listing: %w", err)
	}
	return toListing(row), nil
}

func (r *postgresRepository) ListProductChannelListingsByProduct(ctx context.Context, productID uuid.UUID) ([]ProductChannelListing, error) {
	rows, err := r.queries.ListProductChannelListingsByProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list listings by product: %w", err)
	}
	out := make([]ProductChannelListing, len(rows))
	for i, row := range rows {
		out[i] = toListing(row)
	}
	return out, nil
}

func (r *postgresRepository) ListProductChannelListingsByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]ProductChannelListing, error) {
	if len(productIDs) == 0 {
		return map[uuid.UUID][]ProductChannelListing{}, nil
	}
	rows, err := r.queries.ListProductChannelListingsByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("list listings by product ids: %w", err)
	}
	out := make(map[uuid.UUID][]ProductChannelListing, len(productIDs))
	for _, row := range rows {
		out[row.ProductID] = append(out[row.ProductID], toListing(row))
	}
	return out, nil
}

func (r *postgresRepository) ListProductChannelListingsByProductIDsForChannel(ctx context.Context, productIDs []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID][]ProductChannelListing, error) {
	if len(productIDs) == 0 {
		return map[uuid.UUID][]ProductChannelListing{}, nil
	}
	rows, err := r.queries.ListProductChannelListingsByProductIDsForChannel(ctx, db.ListProductChannelListingsByProductIDsForChannelParams{ProductIds: productIDs, ChannelID: channelID})
	if err != nil {
		return nil, fmt.Errorf("list channel listings by product ids: %w", err)
	}
	out := make(map[uuid.UUID][]ProductChannelListing, len(productIDs))
	for _, row := range rows {
		out[row.ProductID] = append(out[row.ProductID], toListing(row))
	}
	return out, nil
}

func (r *postgresRepository) ListProductChannelListingsByChannel(ctx context.Context, channelID uuid.UUID) ([]ProductChannelListing, error) {
	rows, err := r.queries.ListProductChannelListingsByChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list listings by store: %w", err)
	}
	out := make([]ProductChannelListing, len(rows))
	for i, row := range rows {
		out[i] = toListing(row)
	}
	return out, nil
}

func (r *postgresRepository) DeleteProductChannelListing(ctx context.Context, productID, channelID uuid.UUID) error {
	if err := r.queries.DeleteProductChannelListing(ctx, db.DeleteProductChannelListingParams{
		ProductID: productID,
		ChannelID: channelID,
	}); err != nil {
		return fmt.Errorf("delete listing: %w", err)
	}
	return nil
}

func (r *postgresRepository) CountInheritingListingsForChannel(ctx context.Context, channelID uuid.UUID) (int64, error) {
	n, err := r.queries.CountInheritingListingsForChannel(ctx, channelID)
	if err != nil {
		return 0, fmt.Errorf("count inheriting listings for store: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) CountInheritingListingsAcrossStoresWithCurrencyMismatch(ctx context.Context, newCurrency string) (int64, error) {
	n, err := r.queries.CountInheritingListingsAcrossStoresWithCurrencyMismatch(ctx, newCurrency)
	if err != nil {
		return 0, fmt.Errorf("count cross-store inheriting listings: %w", err)
	}
	return n, nil
}

// GetListingProductIDByChannelURL resolves a channel-override url to its product - ErrListingNotFound when no listing on this channel carries the url
func (r *postgresRepository) GetListingProductIDByChannelURL(ctx context.Context, channelID uuid.UUID, url string) (uuid.UUID, error) {
	id, err := r.queries.GetListingProductIDByChannelURL(ctx, db.GetListingProductIDByChannelURLParams{
		ChannelID: channelID,
		Url:       sql.NullString{String: url, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, ErrListingNotFound
		}
		return uuid.Nil, fmt.Errorf("get listing by channel url: %w", err)
	}
	return id, nil
}

func toListing(row db.ProductChannelListing) ProductChannelListing {
	return ProductChannelListing{
		ProductID:      row.ProductID,
		ChannelID:      row.ChannelID,
		Price:          sqlnull.Int64Ptr(row.Price),
		IsPublished:    row.IsPublished,
		URL:            sqlnull.StringPtr(row.Url),
		CompareAtPrice: sqlnull.Int64Ptr(row.CompareAtPrice),
		SpecialPrice:   sqlnull.Int64Ptr(row.SpecialPrice),
		SpecialFrom:    sqlnull.TimePtr(row.SpecialFrom),
		SpecialTo:      sqlnull.TimePtr(row.SpecialTo),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}
