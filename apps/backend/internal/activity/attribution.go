package activity

import (
	"context"
	"time"

	"github.com/skeletg/skelet/internal/analytics"
	"github.com/skeletg/skelet/internal/events"
	"github.com/skeletg/skelet/internal/order"
)

// Lives here rather than in analytics for the same reason Tracker does - analytics is a leaf and must not import the commerce packages, so the order-shaped translation belongs on this side of the boundary
func RegisterConversionAttribution(bus *events.Bus, rec *analytics.Recorder) {
	bus.Subscribe(order.EventPlaced, func(ctx context.Context, e events.Event) error { // credits an order to the customer's recent marketing email engagement - last touch within the window, marketing only, one per order
		evt := e.(*order.OrderPlaced)
		if evt.CustomerID == nil {
			return nil
		}
		_, _ = rec.RecordConversion(ctx, analytics.ConversionSpec{
			CustomerID: *evt.CustomerID, ChannelID: evt.ChannelID, OrderID: evt.OrderID,
			RevenueCents: evt.TotalCents, Currency: evt.Currency, At: events.EmittedAtOr(ctx, time.Now()), // outbox stamp keeps At stable across redeliveries so the replay collapses
		})
		return nil
	})
}
