package ordermail

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

type fakeOrders struct {
	o            order.Order
	err          error
	fulfillments []order.Fulfillment
	fulErr       error
}

func (f fakeOrders) GetByID(context.Context, uuid.UUID) (order.Order, error) { return f.o, f.err }
func (f fakeOrders) GetFulfillment(context.Context, uuid.UUID) (order.Fulfillment, error) {
	return order.Fulfillment{}, nil
}

func (f fakeOrders) ListFulfillments(context.Context, uuid.UUID) ([]order.Fulfillment, error) {
	return f.fulfillments, f.fulErr
}

func (f fakeOrders) PaymentLedger(context.Context, uuid.UUID) ([]order.PaymentTransaction, error) {
	return nil, nil
}

type fakeChannels struct {
	ch  channel.Channel
	err error
}

func (f fakeChannels) GetByID(context.Context, uuid.UUID) (channel.Channel, error) {
	return f.ch, f.err
}

// The guest path: a placed order with no customer id keys on the order email, so uuid.Nil is a
// valid value downstream rather than a bug
func TestCustomerIDOrNil(t *testing.T) {
	if got := customerIDOrNil(nil); got != uuid.Nil {
		t.Errorf("guest should resolve to uuid.Nil, got %v", got)
	}
	id := uuid.New()
	if got := customerIDOrNil(&id); got != id {
		t.Errorf("known customer should pass through, got %v want %v", got, id)
	}
}

// A half-known order renders a half-filled receipt, which is worse than a missing one
func TestContextSkipsOnLookupFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		svc  *Service
	}{
		{"order lookup fails", &Service{orders: fakeOrders{err: errors.New("gone")}, channels: fakeChannels{}}},
		{"channel lookup fails", &Service{orders: fakeOrders{}, channels: fakeChannels{err: errors.New("gone")}}},
	} {
		if _, _, ok := tc.svc.context(context.Background(), uuid.New()); ok {
			t.Errorf("%s: context reported ok, want the mail skipped", tc.name)
		}
	}
}

func TestContextResolvesOrderAndChannel(t *testing.T) {
	chID := uuid.New()
	svc := &Service{
		orders:   fakeOrders{o: order.Order{Email: "guest@example.com", ChannelID: chID}},
		channels: fakeChannels{ch: channel.Channel{ID: chID, Name: "Shop"}},
	}
	o, ch, ok := svc.context(context.Background(), uuid.New())
	if !ok {
		t.Fatal("context failed on two good lookups")
	}
	if o.Email != "guest@example.com" {
		t.Errorf("order email = %q, want the address the receipt goes to", o.Email)
	}
	if ch.ID != chID {
		t.Errorf("channel = %v, want the order's own %v", ch.ID, chID)
	}
}

// Without a dispatcher nothing else can be sending, so every order email is this package's
func TestFlowTookIsFalseWithoutADispatcher(t *testing.T) {
	s := &Service{}
	if s.flowTook(func(FlowDispatcher) int { return 99 }) {
		t.Error("no dispatcher wired, but flowTook reported a flow had it")
	}
}
