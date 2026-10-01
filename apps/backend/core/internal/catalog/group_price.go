package catalog

// Customer-group tier pricing (quantity breaks) - Rows live in catalog_product_group_prices; resolution (lowest applicable price wins against the effective price) is internal/pricing.BestTier - the cart adapter pre-filters scope and passes the tiers through

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
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
const maxGroupPriceTiers = 200

// A global tier applies only where the channel prices in the product's default currency - its amount carries no currency of its own
func (s *listingService) ApplicableGroupPrices(ctx context.Context, productID uuid.UUID, channelID *uuid.UUID, groupID *uuid.UUID) ([]GroupPrice, error) {
	rows, err := s.repo.ListGroupPrices(ctx, productID)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	globalOK := false
	if channelID != nil && slices.ContainsFunc(rows, func(t GroupPrice) bool { return t.ChannelID == nil }) {
		if globalOK, err = s.channelPricesInProductCurrency(ctx, productID, *channelID); err != nil {
			return nil, err
		}
	}
	out := make([]GroupPrice, 0, len(rows))
	for _, t := range rows {
		switch {
		case t.ChannelID == nil && !globalOK:
			continue
		case t.ChannelID != nil && (channelID == nil || *t.ChannelID != *channelID):
			continue
		case t.CustomerGroupID != nil && (groupID == nil || *t.CustomerGroupID != *groupID):
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func (s *listingService) channelPricesInProductCurrency(ctx context.Context, productID, channelID uuid.UUID) (bool, error) {
	p, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return false, err
	}
	ch, err := s.channelSvc.GetByID(ctx, channelID)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(ch.CurrencyCode, p.DefaultCurrencyCode), nil
}

func (s *listingService) SetGroupPrices(ctx context.Context, productID uuid.UUID, inputs []GroupPriceInput) ([]GroupPrice, error) {
	if len(inputs) > maxGroupPriceTiers {
		return nil, fmt.Errorf("%w: %d tiers, at most %d", ErrTooManyItems, len(inputs), maxGroupPriceTiers)
	}
	for _, in := range inputs {
		if in.MinQty <= 0 {
			return nil, fmt.Errorf("%w: got %g", ErrInvalidTierQuantity, in.MinQty)
		}
		if in.Price < 0 {
			return nil, fmt.Errorf("%w: tier price %d", ErrNegativePrice, in.Price)
		}
	}
	// Deterministic order keeps the unique-violation error stable
	sorted := make([]GroupPriceInput, len(inputs))
	copy(sorted, inputs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].MinQty < sorted[j].MinQty })
	out, err := s.repo.ReplaceGroupPrices(ctx, productID, sorted)
	if err != nil {
		return nil, err
	}
	s.publishProductUpdatedByID(ctx, productID, "groupPrices")
	return out, nil
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
	err := r.atomic(ctx, func(q *db.Queries) error {
		if err := q.DeleteProductGroupPrices(ctx, productID); err != nil {
			return fmt.Errorf("clear group prices: %w", err)
		}
		out = make([]GroupPrice, 0, len(inputs))
		for _, in := range inputs {
			row, err := q.InsertProductGroupPrice(ctx, db.InsertProductGroupPriceParams{
				ProductID:       productID,
				ChannelID:       sqlnull.UUID(in.ChannelID),
				CustomerGroupID: sqlnull.UUID(in.CustomerGroupID),
				MinQty:          in.MinQty,
				Price:           in.Price,
			})
			if err != nil {
				if pgdb.IsForeignKeyViolation(err) {
					return groupPriceFKError(err)
				}
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

func groupPriceFKError(err error) error {
	switch msg := err.Error(); {
	case strings.Contains(msg, "channel_id"):
		return fmt.Errorf("%w: tier channel", channel.ErrChannelNotFound)
	case strings.Contains(msg, "customer_group_id"):
		return fmt.Errorf("%w: tier customer group", ErrCustomerGroupNotFound)
	default:
		return ErrProductNotFound
	}
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
