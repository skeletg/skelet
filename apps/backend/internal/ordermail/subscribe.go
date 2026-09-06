package ordermail

import (
	"context"
	"log"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/email"
	"github.com/skeletg/skelet/apps/backend/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// Register attaches the four order-lifecycle events that produce customer mail.
//
// Each asks the flow engine first and sends only when no flow took it, so an operator's edited
// journey wins and nothing double-sends. Enrollment is asynchronous, so a positive count means a
// run started rather than a mail landed - the alternative is a duplicate receipt, which reads to a
// customer as a duplicate order.
//
// Must stay ahead of the conversion-attribution subscriber, which also listens on order.EventPlaced.
func (s *Service) Register(bus *events.Bus) {
	// The order body isn't on the event, and a flow that takes delivery makes loading it wasted work
	bus.Subscribe(order.EventPlaced, func(ctx context.Context, e events.Event) error {
		evt := e.(*order.OrderPlaced)
		// Guests enroll keyed by the order email, so they get a receipt too
		if s.flowTook(func(d FlowDispatcher) int {
			return d.OnOrderPlaced(ctx, evt.ChannelID, customerIDOrNil(evt.CustomerID), evt.Email, evt.OrderID, evt.OrderNumber)
		}) {
			return nil
		}
		if o, ch, ok := s.context(ctx, evt.OrderID); ok {
			s.sendConfirmation(ctx, o, ch)
		}
		return nil
	})

	// The remaining three carry neither the customer nor the email, so the order is loaded either way
	bus.Subscribe(order.EventFulfillmentCreated, func(ctx context.Context, e events.Event) error {
		evt := e.(*order.OrderFulfillmentCreated)
		o, ch, ok := s.context(ctx, evt.OrderID)
		if !ok {
			return nil
		}
		if s.flowTook(func(d FlowDispatcher) int {
			return d.OnOrderShipped(ctx, evt.ChannelID, customerIDOrNil(o.CustomerID), o.Email, evt.OrderID, evt.FulfillmentID, o.Number)
		}) {
			return nil
		}
		s.sendShipment(ctx, o, ch, evt.FulfillmentID)
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
			return d.OnOrderCancelled(ctx, evt.ChannelID, customerIDOrNil(o.CustomerID), o.Email, evt.OrderID, o.Number)
		}) {
			return nil
		}
		s.sendCancellation(ctx, o, ch)
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
			return d.OnPaymentRefunded(ctx, evt.ChannelID, customerIDOrNil(o.CustomerID), o.Email, evt.OrderID, o.Number, ref)
		}) {
			return nil
		}
		s.sendRefund(ctx, o, ch)
		return nil
	})
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

func (s *Service) sendConfirmation(ctx context.Context, o order.Order, ch channel.Channel) {
	subject, htmlBody, textBody, err := s.render.RenderOrderConfirmation(ctx, ch.ID, email.OrderConfirmationData{
		StoreName: StoreName(ctx, s.config, ch), OrderNumber: o.Number, FirstName: FirstName(o), Currency: o.CurrencyCode,
		Lines: Lines(o), Subtotal: o.SubtotalAmount, Discount: o.DiscountAmount,
		Shipping: o.ShippingAmount, Tax: o.TaxAmount, Total: o.TotalAmount,
		ShipTo: AddressLines(o.ShippingAddress), StorefrontURL: StorefrontURL(ch),
	})
	if err != nil {
		log.Printf("[ordermail] render %s for order %s: %v", emailtemplate.KeyOrderConfirmation, o.Number, err)
		return
	}
	s.send(ctx, ch, o, emailtemplate.KeyOrderConfirmation, subject, htmlBody, textBody)
}

func (s *Service) sendShipment(ctx context.Context, o order.Order, ch channel.Channel, fulfillmentID uuid.UUID) {
	ful, err := s.orders.GetFulfillment(ctx, fulfillmentID)
	if err != nil {
		log.Printf("[ordermail] load fulfillment %s for order %s: %v", fulfillmentID, o.Number, err)
		return
	}
	subject, htmlBody, textBody, err := s.render.RenderOrderShipment(ctx, ch.ID, email.OrderShipmentData{
		StoreName: StoreName(ctx, s.config, ch), OrderNumber: o.Number, FirstName: FirstName(o),
		Carrier: ful.Carrier, TrackingNumber: ful.TrackingNumber,
		Lines: ShipmentLines(o, ful), Currency: o.CurrencyCode, StorefrontURL: StorefrontURL(ch),
	})
	if err != nil {
		log.Printf("[ordermail] render %s for order %s: %v", emailtemplate.KeyOrderShipment, o.Number, err)
		return
	}
	s.send(ctx, ch, o, emailtemplate.KeyOrderShipment, subject, htmlBody, textBody)
}

func (s *Service) sendCancellation(ctx context.Context, o order.Order, ch channel.Channel) {
	subject, htmlBody, textBody, err := s.render.RenderOrderCancellation(ctx, ch.ID, email.OrderCancellationData{
		StoreName: StoreName(ctx, s.config, ch), OrderNumber: o.Number, FirstName: FirstName(o), StorefrontURL: StorefrontURL(ch),
	})
	if err != nil {
		log.Printf("[ordermail] render %s for order %s: %v", emailtemplate.KeyOrderCancellation, o.Number, err)
		return
	}
	s.send(ctx, ch, o, emailtemplate.KeyOrderCancellation, subject, htmlBody, textBody)
}

func (s *Service) sendRefund(ctx context.Context, o order.Order, ch channel.Channel) {
	subject, htmlBody, textBody, err := s.render.RenderOrderRefund(ctx, ch.ID, email.OrderRefundData{
		StoreName: StoreName(ctx, s.config, ch), OrderNumber: o.Number, FirstName: FirstName(o),
		Currency: o.CurrencyCode, RefundedAmount: RefundedAmount(ctx, s.orders, o.ID), StorefrontURL: StorefrontURL(ch),
	})
	if err != nil {
		log.Printf("[ordermail] render %s for order %s: %v", emailtemplate.KeyOrderRefund, o.Number, err)
		return
	}
	s.send(ctx, ch, o, emailtemplate.KeyOrderRefund, subject, htmlBody, textBody)
}

// RefundedAmount totals what actually left the merchant's account - a pending or failed refund doesn't count
func RefundedAmount(ctx context.Context, orders OrderReader, orderID uuid.UUID) int64 {
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
