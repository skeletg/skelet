package flow

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/customer"
	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// OrderReader is the lookup the shipment/cancel/refund triggers need - those events
// carry neither the customer id nor the email, so the order has to be loaded
type OrderReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (order.Order, error)
}

// RegisterEnrollment attaches the order and auth lifecycle events that start
// matching active automation flows. Subscription order is preserved from the
// composition root it came out of, and it must stay ahead of the conversion
// attribution subscriber that also listens on order.EventPlaced.
func RegisterEnrollment(bus *events.Bus, svc *Service, orders OrderReader) {
	// Flow enrollment: the order + auth lifecycle events start matching active automation flows for the customer (guests skipped - flows target known customers) - transactional receipts (confirmation/shipment/cancellation/ refund) are ordinary transactional
	// flows seeded per channel
	bus.Subscribe(order.EventPlaced, func(ctx context.Context, e events.Event) error {
		evt := e.(*order.OrderPlaced)
		custID := customerIDOrNil(evt.CustomerID)
		svc.OnOrderPlaced(ctx, evt.ChannelID, custID, evt.Email, evt.OrderID, evt.OrderNumber) // Guests (custID Nil) enroll keyed by the order email so they get a receipt too
		return nil
	})
	bus.Subscribe(order.EventFulfillmentCreated, func(ctx context.Context, e events.Event) error {
		evt := e.(*order.OrderFulfillmentCreated)
		enrollOrder(ctx, orders, evt.OrderID, func(custID uuid.UUID, o order.Order) {
			svc.OnOrderShipped(ctx, evt.ChannelID, custID, o.Email, evt.OrderID, evt.FulfillmentID, o.Number)
		})
		return nil
	})
	bus.Subscribe(order.EventStatusChanged, func(ctx context.Context, e events.Event) error {
		evt := e.(*order.OrderStatusChanged)
		if evt.ToStatus != order.OrderStatusCancelled {
			return nil
		}
		enrollOrder(ctx, orders, evt.OrderID, func(custID uuid.UUID, o order.Order) {
			svc.OnOrderCancelled(ctx, evt.ChannelID, custID, o.Email, evt.OrderID, o.Number)
		})
		return nil
	})
	bus.Subscribe(order.EventPaymentStatusChanged, func(ctx context.Context, e events.Event) error {
		evt := e.(*order.OrderPaymentStatusChanged)
		if evt.Status != "refunded" && evt.Status != "partially_refunded" {
			return nil
		}
		ref := evt.Reference
		if ref == "" {
			ref = evt.Status
		}
		enrollOrder(ctx, orders, evt.OrderID, func(custID uuid.UUID, o order.Order) {
			svc.OnPaymentRefunded(ctx, evt.ChannelID, custID, o.Email, evt.OrderID, o.Number, ref)
		})
		return nil
	})
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

// Guests have no customer id - enrollment keys on the order email instead, so the
// zero UUID is a valid input downstream rather than an error
func customerIDOrNil(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}

// Shipment, cancellation and refund events carry neither the customer id nor the email, so the order is loaded to
// resolve them. A lookup failure skips enrollment rather than enrolling against a half-known order
func enrollOrder(ctx context.Context, orders OrderReader, orderID uuid.UUID, enroll func(custID uuid.UUID, o order.Order)) {
	o, err := orders.GetByID(ctx, orderID)
	if err != nil {
		return
	}
	enroll(customerIDOrNil(o.CustomerID), o)
}
