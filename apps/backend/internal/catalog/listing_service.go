package catalog

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/cache"
)

// Price=nil expresses inheritance from the product's default price, and the inheritance rule (currency match) is enforced here so an inheriting listing always has a coherent currency
func (s *listingService) SetProductListing(ctx context.Context, input ProductChannelListingInput) (ProductChannelListing, error) {
	input.URL = normalizeListingOverride(input.URL)
	if input.Price == nil {
		product, err := s.repo.GetProductByID(ctx, input.ProductID)
		if err != nil {
			return ProductChannelListing{}, err
		}
		store, err := s.channelSvc.GetByID(ctx, input.ChannelID)
		if err != nil {
			return ProductChannelListing{}, err
		}
		if store.CurrencyCode != product.DefaultCurrencyCode {
			return ProductChannelListing{}, ErrListingCurrencyMismatch
		}
	}
	listing, err := s.repo.UpsertProductChannelListing(ctx, input)
	if err != nil {
		return ProductChannelListing{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &ProductListingChanged{
			ProductID:   listing.ProductID,
			ChannelID:   listing.ChannelID,
			IsPublished: listing.IsPublished,
			Cleared:     false,
		})
	}
	return listing, nil
}

// Blank input collapses to nil ("inherit") and leading slashes are stripped so stored paths match the rewrite convention
func normalizeListingOverride(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.Trim(strings.TrimSpace(*p), "/")
	if v == "" {
		return nil
	}
	return &v
}

// ErrListingNotFound when this channel has no override matching the path - the caller then falls back to the global url_rewrites / slug lookup
func (s *listingService) ResolveProductIDForChannelPath(ctx context.Context, channelID uuid.UUID, path string) (uuid.UUID, error) {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return uuid.Nil, ErrListingNotFound
	}
	key := "p:" + channelID.String() + ":" + path
	if s.channelPathCache != nil {
		if hit, ok := cache.GetJSON[cachedChannelPath](ctx, s.channelPathCache, key); ok {
			if hit.NotFound {
				return uuid.Nil, ErrListingNotFound
			}
			if id, err := uuid.Parse(hit.ProductID); err == nil {
				return id, nil
			}
		}
	}
	id, err := s.resolveChannelPathUncached(ctx, channelID, path)
	if s.channelPathCache != nil {
		if errors.Is(err, ErrListingNotFound) {
			s.channelPathCache.SetJSON(ctx, key, cachedChannelPath{NotFound: true}, channelPathNotFoundCacheTTL)
		} else if err == nil {
			s.channelPathCache.SetJSON(ctx, key, cachedChannelPath{ProductID: id.String()}, channelPathCacheTTL)
		}
	}
	return id, err
}

const (
	channelPathCacheTTL         = 15 * time.Minute
	channelPathNotFoundCacheTTL = 5 * time.Minute
)

type cachedChannelPath struct { // the Valkey value for a channel pretty-URL lookup - either the resolved product id or a negative marker
	ProductID string `json:"product_id,omitempty"`
	NotFound  bool   `json:"not_found,omitempty"`
}

func (s *listingService) resolveChannelPathUncached(ctx context.Context, channelID uuid.UUID, path string) (uuid.UUID, error) {
	return s.repo.GetListingProductIDByChannelURL(ctx, channelID, path)
}

// Removing the listing un-publishes the product on that store
func (s *listingService) ClearProductListing(ctx context.Context, productID, channelID uuid.UUID) error {
	if err := s.repo.DeleteProductChannelListing(ctx, productID, channelID); err != nil {
		return err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &ProductListingChanged{
			ProductID:   productID,
			ChannelID:   channelID,
			IsPublished: false,
			Cleared:     true,
		})
	}
	return nil
}

func (s *listingService) ListProductChannelListings(ctx context.Context, productID uuid.UUID) ([]ProductChannelListing, error) {
	return s.repo.ListProductChannelListingsByProduct(ctx, productID)
}

func (s *listingService) ListProductChannelListingsByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]ProductChannelListing, error) {
	return s.repo.ListProductChannelListingsByProductIDs(ctx, productIDs)
}

func (s *listingService) ListProductChannelListingsByChannel(ctx context.Context, channelID uuid.UUID) ([]ProductChannelListing, error) {
	return s.repo.ListProductChannelListingsByChannel(ctx, channelID)
}

func (s *listingService) GetProductChannelListing(ctx context.Context, productID, channelID uuid.UUID) (ProductChannelListing, error) {
	return s.repo.GetProductChannelListing(ctx, productID, channelID)
}

func (s *listingService) CountInheritingListingsForChannel(ctx context.Context, channelID uuid.UUID) (int64, error) { // backs the channel-update guard
	return s.repo.CountInheritingListingsForChannel(ctx, channelID)
}

func (s *listingService) CountInheritingListingsAcrossStoresWithCurrencyMismatch(ctx context.Context, newCurrency string) (int64, error) { // backs the global default-currency guard
	return s.repo.CountInheritingListingsAcrossStoresWithCurrencyMismatch(ctx, newCurrency)
}
