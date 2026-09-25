package dataloaders

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
)

type listingSpy struct{ all, scoped int }

func (s *listingSpy) ListProductChannelListingsByProductIDs(context.Context, []uuid.UUID) (map[uuid.UUID][]catalog.ProductChannelListing, error) {
	s.all++
	return nil, nil
}

func (s *listingSpy) ListProductChannelListingsByProductIDsForChannel(context.Context, []uuid.UUID, uuid.UUID) (map[uuid.UUID][]catalog.ProductChannelListing, error) {
	s.scoped++
	return nil, nil
}

func TestAStorefrontRequestLoadsOnlyItsChannelsListings(t *testing.T) {
	spy := &listingSpy{}
	ctx := channel.WithChannel(context.Background(), &channel.Channel{ID: uuid.New()})
	if _, err := StorefrontListings(ctx, spy, []uuid.UUID{uuid.New()}); err != nil {
		t.Fatal(err)
	}
	productListingsBatchFn(spy)(ctx, []uuid.UUID{uuid.New()})
	if spy.all != 0 || spy.scoped != 2 {
		t.Fatalf("read every channel's listings %d times and the channel's own %d, want 0 and 2", spy.all, spy.scoped)
	}
}
