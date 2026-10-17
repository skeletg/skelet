package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
)

// Price=nil expresses inheritance from the product's default price, and the inheritance rule (currency match) is enforced here so an inheriting listing always has a coherent currency
func (s *listingService) SetProductListing(ctx context.Context, input ProductChannelListingInput) (ProductChannelListing, error) {
	input.URL = normalizeListingOverride(input.URL)
	for name, v := range map[string]*int64{"price": input.Price, "compare_at_price": input.CompareAtPrice, "special_price": input.SpecialPrice} {
		if v != nil && *v < 0 {
			return ProductChannelListing{}, fmt.Errorf("%w: %s", ErrNegativePrice, name)
		}
	}
	if input.Price == nil {
		if err := s.requireInheritableCurrency(ctx, input.ProductID, input.ChannelID); err != nil {
			return ProductChannelListing{}, err
		}
	}
	listing, err := s.repo.UpsertProductChannelListing(ctx, input)
	if err != nil {
		return ProductChannelListing{}, err
	}
	s.forgetChannelPaths(ctx, input.ChannelID)
	s.publishListingChanged(ctx, listing)
	return listing, nil
}

// SetProductListingPublished flips only is_published - a new row inherits the product price, so it must pass the currency rule
func (s *listingService) SetProductListingPublished(ctx context.Context, productID, channelID uuid.UUID, published bool) (ProductChannelListing, error) {
	if _, err := s.repo.GetProductChannelListing(ctx, productID, channelID); errors.Is(err, ErrListingNotFound) {
		if err := s.requireInheritableCurrency(ctx, productID, channelID); err != nil {
			return ProductChannelListing{}, err
		}
	} else if err != nil {
		return ProductChannelListing{}, err
	}
	listing, err := s.repo.UpsertProductChannelListingPublished(ctx, productID, channelID, published)
	if err != nil {
		return ProductChannelListing{}, err
	}
	s.forgetChannelPaths(ctx, channelID)
	s.publishListingChanged(ctx, listing)
	return listing, nil
}

func (s *listingService) requireInheritableCurrency(ctx context.Context, productID, channelID uuid.UUID) error {
	product, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return err
	}
	store, err := s.channelSvc.GetByID(ctx, channelID)
	if err != nil {
		return err
	}
	if store.CurrencyCode != product.DefaultCurrencyCode {
		return ErrListingCurrencyMismatch
	}
	return nil
}

func (s *listingService) publishListingChanged(ctx context.Context, listing ProductChannelListing) {
	if s.bus != nil {
		s.bus.Publish(ctx, &ProductListingChanged{
			ProductID:   listing.ProductID,
			ChannelID:   listing.ChannelID,
			IsPublished: listing.IsPublished,
			Cleared:     false,
		})
	}
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

// ErrListingNotFound when this channel has no published override matching the path - the caller then falls back to the global url_rewrites / slug lookup
func (s *listingService) ResolveProductIDForChannelPath(ctx context.Context, channelID uuid.UUID, path string) (uuid.UUID, error) {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" || len(path) > maxChannelPathLen {
		return uuid.Nil, ErrListingNotFound // an over-long path can't be a stored override, and caching it would let random paths flood the negative cache
	}
	if s.channelPathCache == nil {
		return s.resolveChannelPathUncached(ctx, channelID, path)
	}
	// The version is read BEFORE the database, so a write that commits in between bumps it and this fill lands under a key nobody reads
	key := channelPathKey(channelID, s.channelPathVersion(ctx, channelID), path)
	if hit, ok := cache.GetJSON[cachedChannelPath](ctx, s.channelPathCache, key); ok {
		if hit.NotFound {
			return uuid.Nil, ErrListingNotFound
		}
		if id, err := uuid.Parse(hit.ProductID); err == nil {
			return id, nil
		}
	}
	id, err := s.resolveChannelPathUncached(ctx, channelID, path)
	if errors.Is(err, ErrListingNotFound) {
		s.channelPathCache.SetJSON(ctx, key, cachedChannelPath{NotFound: true}, channelPathNotFoundCacheTTL)
	} else if err == nil {
		s.channelPathCache.SetJSON(ctx, key, cachedChannelPath{ProductID: id.String()}, channelPathCacheTTL)
	}
	return id, err
}

func (s *listingService) forgetChannelPaths(ctx context.Context, channelID uuid.UUID) {
	if s.channelPathCache == nil {
		return
	}
	s.channelPathCache.SetJSON(ctx, channelPathVersionKey(channelID), uuid.NewString(), channelPathVersionTTL)
}

func (s *listingService) channelPathVersion(ctx context.Context, channelID uuid.UUID) string {
	v, _ := cache.GetJSON[string](ctx, s.channelPathCache, channelPathVersionKey(channelID))
	return v
}

func channelPathVersionKey(channelID uuid.UUID) string { return "pv:" + channelID.String() }

func channelPathKey(channelID uuid.UUID, version, path string) string {
	return "p:" + channelID.String() + ":" + version + ":" + path
}

const (
	channelPathCacheTTL         = 15 * time.Minute
	channelPathNotFoundCacheTTL = 5 * time.Minute
	channelPathVersionTTL       = 24 * time.Hour // outlives every entry, so an expired version can't resurrect one
	maxChannelPathLen           = 512
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
	s.forgetChannelPaths(ctx, channelID)
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

func (s *listingService) CountProductChannelListingsByChannel(ctx context.Context) (map[uuid.UUID]int64, error) {
	return s.repo.CountProductChannelListingsByChannel(ctx)
}

func (s *listingService) ListProductChannelListingsByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]ProductChannelListing, error) {
	return s.repo.ListProductChannelListingsByProductIDs(ctx, productIDs)
}

func (s *listingService) ListProductChannelListingsByProductIDsForChannel(ctx context.Context, productIDs []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID][]ProductChannelListing, error) {
	return s.repo.ListProductChannelListingsByProductIDsForChannel(ctx, productIDs, channelID)
}

func (s *listingService) ListProductChannelListingsByChannel(ctx context.Context, channelID uuid.UUID) ([]ProductChannelListing, error) {
	return s.repo.ListProductChannelListingsByChannel(ctx, channelID)
}
