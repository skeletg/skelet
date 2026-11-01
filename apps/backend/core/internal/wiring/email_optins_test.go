package wiring

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

type fakeOptIns struct {
	recorded []string
	claimed  []uuid.UUID
}

func (f *fakeOptIns) RecordEmailOptIn(_ context.Context, _ uuid.UUID, email, source, reference string) error {
	f.recorded = append(f.recorded, email+" "+source+" "+reference)
	return nil
}

func (f *fakeOptIns) ClaimEmailOptIns(_ context.Context, customerID, _ uuid.UUID, _ string) (int, error) {
	f.claimed = append(f.claimed, customerID)
	return 1, nil
}

type fakeAccounts map[string]uuid.UUID // verified accounts by email

func (f fakeAccounts) VerifiedCustomerIDByEmail(_ context.Context, _ uuid.UUID, email string) (uuid.UUID, bool, error) {
	id, ok := f[email]
	return id, ok, nil
}

func TestAGuestCheckoutOptInIsKeptAndClaimedOnVerification(t *testing.T) {
	bus := events.NewBus()
	f := &fakeOptIns{}
	registerEmailOptIns(bus, f, fakeAccounts{})
	ctx := context.Background()
	ch, cust := uuid.New(), uuid.New()

	_ = bus.DispatchAsyncNow(ctx, &order.OrderPlaced{OrderID: uuid.New(), OrderNumber: "ORD-000001", Email: "g@example.com", ChannelID: ch})
	_ = bus.DispatchAsyncNow(ctx, &order.OrderPlaced{OrderID: uuid.New(), OrderNumber: "ORD-000002", Email: "g@example.com", ChannelID: ch, MarketingOptIn: true})
	if len(f.recorded) != 1 || f.recorded[0] != "g@example.com "+consent.SourceCheckout+" ORD-000002" {
		t.Errorf("recorded %v, want only the ticked order, with its number as proof", f.recorded)
	}
	_ = bus.DispatchAsyncNow(ctx, &customer.CustomerEmailVerified{CustomerID: cust, ChannelID: ch, Email: "g@example.com"})
	if len(f.claimed) != 1 || f.claimed[0] != cust {
		t.Errorf("claimed for %v, want the verified customer", f.claimed)
	}
}

func TestAGuestOptInForAnAlreadyVerifiedAccountAppliesAtOnce(t *testing.T) {
	bus := events.NewBus()
	f := &fakeOptIns{}
	owner := uuid.New()
	registerEmailOptIns(bus, f, fakeAccounts{"owner@example.com": owner})
	ctx := context.Background()
	_ = bus.DispatchAsyncNow(ctx, &order.OrderPlaced{OrderNumber: "ORD-1", Email: "owner@example.com", ChannelID: uuid.New(), MarketingOptIn: true})
	_ = bus.DispatchAsyncNow(ctx, &order.OrderPlaced{OrderNumber: "ORD-2", Email: "nobody@example.com", ChannelID: uuid.New(), MarketingOptIn: true})
	if len(f.recorded) != 2 || len(f.claimed) != 1 || f.claimed[0] != owner {
		t.Errorf("recorded %v, claimed %v - want both kept and only the verified owner's applied", f.recorded, f.claimed)
	}
}
