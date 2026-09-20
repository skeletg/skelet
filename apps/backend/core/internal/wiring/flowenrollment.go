package wiring

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

// The lifecycle events that start a run - core's events, so wired here and not by the engine
// customer.registered rides the outbox, so a suite that was down is asked again on the relay's next pass
func registerFlowEnrollment(bus *events.Bus, flows *FlowEnroller) {
	bus.Subscribe(customer.EventRegistered, func(ctx context.Context, e events.Event) error {
		evt := e.(*customer.CustomerRegistered)
		return flows.OnCustomerRegistered(ctx, evt.ChannelID, evt.CustomerID)
	})
}
