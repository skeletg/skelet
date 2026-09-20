package ordermail

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

// Register attaches the four order-lifecycle events that produce customer mail.
func (s *Service) Register(bus *events.Bus) {
	// All four load the order: a flow has to be handed the snapshot it will render from, since by the
	// time the engine advances the run the order is not readable from where it runs
	bus.Subscribe(order.EventPlaced, func(ctx context.Context, e events.Event) error {
		evt := e.(*order.OrderPlaced)
		o, ch, ok := s.context(ctx, evt.OrderID)
		if !ok {
			return nil
		}
		// Guests enroll keyed by the order email, so they get a receipt too
		if s.flowTook(func(d FlowDispatcher) int {
			return d.OnOrderPlaced(ctx, evt.ChannelID, customerIDOrNil(evt.CustomerID), evt.Email, evt.OrderID, Snapshot(o))
		}) {
			return nil
		}
		s.sendOnce(ctx, evt.OrderID, kindConfirmation, "", func() error { return s.sendConfirmation(ctx, o, ch) })
		return nil
	})

	bus.Subscribe(order.EventFulfillmentCreated, func(ctx context.Context, e events.Event) error {
		evt := e.(*order.OrderFulfillmentCreated)
		o, ch, ok := s.context(ctx, evt.OrderID)
		if !ok {
			return nil
		}
		if s.flowTook(func(d FlowDispatcher) int {
			ful, ferr := s.orders.GetFulfillment(ctx, evt.FulfillmentID)
			if ferr != nil {
				return 0 // no fulfillment, no tracking to show - the direct send below loads it again and reports properly
			}
			return d.OnOrderShipped(ctx, evt.ChannelID, customerIDOrNil(o.CustomerID), o.Email, evt.OrderID, evt.FulfillmentID, ShipmentSnapshot(o, ful))
		}) {
			return nil
		}
		s.sendOnce(ctx, evt.OrderID, kindShipment, evt.FulfillmentID.String(), func() error { return s.sendShipment(ctx, o, ch, evt.FulfillmentID) })
		return nil
	})

	bus.Subscribe(order.EventStatusChanged, func(ctx context.Context, e events.Event) error {
		evt := e.(*order.OrderStatusChanged)
		if evt.ToStatus != order.OrderStatusCancelled {
			return nil
		}
		o, ch, ok := s.context(ctx, evt.OrderID)
		if !ok {
			return nil
		}
		if s.flowTook(func(d FlowDispatcher) int {
			return d.OnOrderCancelled(ctx, evt.ChannelID, customerIDOrNil(o.CustomerID), o.Email, evt.OrderID, Snapshot(o))
		}) {
			return nil
		}
		s.sendOnce(ctx, evt.OrderID, kindCancellation, "", func() error { return s.sendCancellation(ctx, o, ch) })
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
		o, ch, ok := s.context(ctx, evt.OrderID)
		if !ok {
			return nil
		}
		if s.flowTook(func(d FlowDispatcher) int {
			snap := Snapshot(o)
			snap.RefundedAmount = RefundedAmount(ctx, s.orders, o.ID)
			return d.OnPaymentRefunded(ctx, evt.ChannelID, customerIDOrNil(o.CustomerID), o.Email, evt.OrderID, ref, snap)
		}) {
			return nil
		}
		s.sendOnce(ctx, evt.OrderID, kindRefund, ref, func() error { return s.sendRefund(ctx, o, ch) })
		return nil
	})
}

// The outbox redelivers an event whose handler finished but whose delivery was never marked done, and a second receipt reads to a customer as a second order
// Recorded after the send, since a row written for mail that never left would suppress the real one
func (s *Service) sendOnce(ctx context.Context, orderID uuid.UUID, kind, ref string, send func() error) {
	if s.sendLog == nil {
		logSend(send())
		return
	}
	sent, err := s.sendLog.AlreadySent(ctx, orderID, kind, ref)
	if err != nil {
		log.Printf("[ordermail] send log read: %v", err) // sends anyway - a missing receipt costs more than a duplicate
	}
	if sent {
		return
	}
	if err := send(); err != nil {
		logSend(err)
		return
	}
	if err := s.sendLog.RecordSent(ctx, orderID, kind, ref); err != nil {
		log.Printf("[ordermail] send log write: %v", err)
	}
}

// An event-driven send has nobody to report to, so a failure is logged and the subscriber still succeeds
func logSend(err error) {
	if err != nil {
		log.Printf("[ordermail] %v", err)
	}
}

// No dispatcher wired means nothing else can be sending, so every order email is ours
func (s *Service) flowTook(enroll func(FlowDispatcher) int) bool {
	return s.flows != nil && enroll(s.flows) > 0
}

// Guests have no customer id - enrollment keys on the order email instead, so the zero UUID is a
// valid input downstream rather than an error
func customerIDOrNil(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}

func (s *Service) sendConfirmation(ctx context.Context, o order.Order, ch channel.Channel) error {
	subject, htmlBody, textBody, err := s.render.RenderOrderConfirmation(ctx, ch.ID, email.OrderConfirmationData{
		StoreName: StoreName(ctx, s.config, ch), OrderNumber: o.Number, FirstName: FirstName(o), Currency: o.CurrencyCode,
		Lines: Lines(o), Subtotal: o.SubtotalAmount, Discount: o.DiscountAmount,
		Shipping: o.ShippingAmount, Tax: o.TaxAmount, Total: o.TotalAmount,
		ShipTo: AddressLines(o.ShippingAddress), StorefrontURL: StorefrontURL(ch),
	})
	if err != nil {
		return fmt.Errorf("render %s for order %s: %w", email.KeyOrderConfirmation, o.Number, err)
	}
	return s.send(ctx, ch, o, email.KeyOrderConfirmation, subject, htmlBody, textBody)
}

func (s *Service) sendShipment(ctx context.Context, o order.Order, ch channel.Channel, fulfillmentID uuid.UUID) error {
	ful, err := s.orders.GetFulfillment(ctx, fulfillmentID)
	if err != nil {
		return fmt.Errorf("load fulfillment %s for order %s: %w", fulfillmentID, o.Number, err)
	}
	subject, htmlBody, textBody, err := s.render.RenderOrderShipment(ctx, ch.ID, email.OrderShipmentData{
		StoreName: StoreName(ctx, s.config, ch), OrderNumber: o.Number, FirstName: FirstName(o),
		Carrier: ful.Carrier, TrackingNumber: ful.TrackingNumber,
		Lines: ShipmentLines(o, ful), Currency: o.CurrencyCode, StorefrontURL: StorefrontURL(ch),
	})
	if err != nil {
		return fmt.Errorf("render %s for order %s: %w", email.KeyOrderShipment, o.Number, err)
	}
	return s.send(ctx, ch, o, email.KeyOrderShipment, subject, htmlBody, textBody)
}

func (s *Service) sendCancellation(ctx context.Context, o order.Order, ch channel.Channel) error {
	subject, htmlBody, textBody, err := s.render.RenderOrderCancellation(ctx, ch.ID, email.OrderCancellationData{
		StoreName: StoreName(ctx, s.config, ch), OrderNumber: o.Number, FirstName: FirstName(o), StorefrontURL: StorefrontURL(ch),
	})
	if err != nil {
		return fmt.Errorf("render %s for order %s: %w", email.KeyOrderCancellation, o.Number, err)
	}
	return s.send(ctx, ch, o, email.KeyOrderCancellation, subject, htmlBody, textBody)
}

func (s *Service) sendRefund(ctx context.Context, o order.Order, ch channel.Channel) error {
	subject, htmlBody, textBody, err := s.render.RenderOrderRefund(ctx, ch.ID, email.OrderRefundData{
		StoreName: StoreName(ctx, s.config, ch), OrderNumber: o.Number, FirstName: FirstName(o),
		Currency: o.CurrencyCode, RefundedAmount: RefundedAmount(ctx, s.orders, o.ID), StorefrontURL: StorefrontURL(ch),
	})
	if err != nil {
		return fmt.Errorf("render %s for order %s: %w", email.KeyOrderRefund, o.Number, err)
	}
	return s.send(ctx, ch, o, email.KeyOrderRefund, subject, htmlBody, textBody)
}

// RefundedAmount totals what actually left the merchant's account - a pending or failed refund doesn't count
func RefundedAmount(ctx context.Context, orders PaymentLedgerReader, orderID uuid.UUID) int64 {
	var refunded int64
	txns, err := orders.PaymentLedger(ctx, orderID)
	if err != nil {
		return 0
	}
	for _, t := range txns {
		if t.Kind == "refund" && t.Status == "succeeded" {
			refunded += t.AmountCents
		}
	}
	return refunded
}
