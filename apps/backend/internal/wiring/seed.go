package wiring

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/payment"
	"github.com/skeletg/skelet/apps/backend/internal/shipping"
)

func SeedProviderRows(ctx context.Context, payments *payment.Service, carriers *shipping.Service, channels *channel.Service) error { // one row per (channel, registered provider) in both tables, new ones disabled by default - idempotent upserts that must run after their service exists and before traffic, an ordering run() only implied by position
	if err := payments.Sync(ctx); err != nil {
		return fmt.Errorf("payment sync: %w", err)
	}
	chs, err := channels.List(ctx)
	if err != nil {
		return fmt.Errorf("shipping carrier sync: list channels: %w", err)
	}
	ids := make([]uuid.UUID, len(chs))
	for i, c := range chs {
		ids[i] = c.ID
	}
	if err := carriers.SyncCarriers(ctx, ids); err != nil {
		return fmt.Errorf("shipping carrier sync: %w", err)
	}
	return nil
}
