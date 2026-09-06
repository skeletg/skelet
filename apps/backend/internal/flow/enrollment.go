package flow

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/customer"
	"github.com/skeletg/skelet/apps/backend/internal/events"
)

// RegisterEnrollment attaches the lifecycle events that start matching active automation flows
func RegisterEnrollment(bus *events.Bus, svc *Service) {
	bus.Subscribe(customer.EventRegistered, func(ctx context.Context, e events.Event) error {
		evt := e.(*customer.CustomerRegistered)
		svc.OnCustomerRegistered(ctx, evt.ChannelID, evt.CustomerID)
		return nil
	})
	bus.Subscribe(analytics.BusEmailOpened, func(ctx context.Context, e events.Event) error { // Email engagement (human opens/clicks) enrols engagement-triggered flows
		evt := e.(*analytics.EmailEngaged)
		svc.OnEmailEngaged(ctx, evt.ChannelID, evt.CustomerID, TriggerEmailOpened)
		return nil
	})
	bus.Subscribe(analytics.BusEmailClicked, func(ctx context.Context, e events.Event) error {
		evt := e.(*analytics.EmailEngaged)
		svc.OnEmailEngaged(ctx, evt.ChannelID, evt.CustomerID, TriggerEmailClicked)
		return nil
	})
}
