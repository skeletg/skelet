package activity

import (
	"context"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
)

// Lives here rather than in the suite for the same reason Tracker does - the order-shaped translation belongs on core's side of the boundary
func RegisterConversionAttribution(bus *events.Bus, rec *signalsclient.Client) {
	bus.Subscribe(order.EventPlaced, func(ctx context.Context, e events.Event) error { // credits an order to the customer's recent marketing email engagement - last touch within the window, marketing only, one per order
		evt := e.(*order.OrderPlaced)
		if evt.CustomerID == nil {
			return nil
		}
		_ = rec.RecordConversion(ctx, signalsclient.ConversionSpec{
			CustomerID: *evt.CustomerID, ChannelID: evt.ChannelID, OrderID: evt.OrderID,
			RevenueCents: evt.TotalCents, Currency: evt.Currency, At: events.EmittedAtOr(ctx, time.Now()), // outbox stamp keeps At stable across redeliveries so the replay collapses
		})
		return nil
	})
}
