package activity

import (
	"context"
	"fmt"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
)

// Lives here rather than in the suite for the same reason Tracker does - the order-shaped translation belongs on core's side of the boundary
type conversionRecorder interface {
	RecordConversion(ctx context.Context, cs signalsclient.ConversionSpec) error
}

func RegisterConversionAttribution(bus *events.Bus, rec conversionRecorder) {
	bus.Subscribe(order.EventPlaced, func(ctx context.Context, e events.Event) error { // credits an order to the customer's recent marketing email engagement - last touch within the window, marketing only, one per order
		evt := e.(*order.OrderPlaced)
		if evt.CustomerID == nil {
			return nil
		}
		cctx, cancel := context.WithTimeout(ctx, signalsWriteTimeout)
		defer cancel()
		err := rec.RecordConversion(cctx, signalsclient.ConversionSpec{
			CustomerID: *evt.CustomerID, ChannelID: evt.ChannelID, OrderID: evt.OrderID,
			RevenueCents: evt.TotalCents, Currency: evt.Currency, At: events.EmittedAtOr(ctx, time.Now()), // outbox stamp keeps At stable across redeliveries so the replay collapses
		})
		// An unreachable suite hands the order back to the outbox, whose replay the conversion:<order> key collapses - a refusal is final
		if signalsclient.Unreachable(err) {
			return fmt.Errorf("record conversion for order %s: %w", evt.OrderID, err)
		}
		return nil
	})
}
