package wiring

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

type fakeOptIns struct{ recorded []string }

func (f *fakeOptIns) RecordEmailOptIn(_ context.Context, _ uuid.UUID, email, source, reference string) error {
	f.recorded = append(f.recorded, email+" "+source+" "+reference)
	return nil
}

// Only the ticked order is kept, with its number as proof - and nothing more: anyone can tick the box under someone else's
// address, so the opt-in never reaches an account, existing or created later (emailOptIns has no way to apply one)
func TestAGuestCheckoutOptInIsKeptAsProofOnly(t *testing.T) {
	bus := events.NewBus()
	f := &fakeOptIns{}
	registerEmailOptIns(bus, f)
	ctx := context.Background()
	ch := uuid.New()

	_ = bus.DispatchAsyncNow(ctx, &order.OrderPlaced{OrderID: uuid.New(), OrderNumber: "ORD-000001", Email: "g@example.com", ChannelID: ch})
	_ = bus.DispatchAsyncNow(ctx, &order.OrderPlaced{OrderID: uuid.New(), OrderNumber: "ORD-000002", Email: "g@example.com", ChannelID: ch, MarketingOptIn: true})
	if len(f.recorded) != 1 || f.recorded[0] != "g@example.com "+consent.SourceCheckout+" ORD-000002" {
		t.Errorf("recorded %v, want only the ticked order, with its number as proof", f.recorded)
	}
}
