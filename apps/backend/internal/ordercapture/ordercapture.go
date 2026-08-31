// Package ordercapture captures the authorized amount when goods first ship
//
// order and payment don't import each other, so the orchestration lives here
// rather than on either side. It is money movement: a capture that never runs
// after a shipment is revenue the store silently never collects, which is why
// both failure paths record onto the order timeline instead of only logging
package ordercapture

import (
	"context"
	"log"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/order"
	"github.com/skeletg/skelet/apps/backend/internal/payment"
)

// Orders is the order-side surface a capture needs
type Orders interface {
	GetByID(ctx context.Context, id uuid.UUID) (order.Order, error)
	RecordCaptureIntent(ctx context.Context, orderID uuid.UUID, amountCents int64, providerSlug, reason string) (order.PaymentTransaction, error)
	FinalizePaymentTransaction(ctx context.Context, txnID uuid.UUID, ok bool, providerReference string) (order.Order, error)
	RecordCaptureFailure(ctx context.Context, orderID uuid.UUID, providerSlug string, cause error)
}

// Payments is the gateway surface - Capture carries the idempotency key from the
// intent, so a redelivered fulfillment event can't double-charge
type Payments interface {
	Capture(ctx context.Context, providerSlug string, req payment.CaptureRequest) (payment.CaptureResult, error)
}

// Register attaches capture-on-first-ship. Async and best-effort at the bus level -
// the intent row plus its idempotency key are what make it safe to retry
//
// It fires once per order because of the PaymentStatus==authorized guard: after the
// first capture the status is no longer authorized, so later shipments skip
func Register(bus *events.Bus, orders Orders, payments Payments) {
	bus.Subscribe(order.EventFulfillmentCreated, func(ctx context.Context, e events.Event) error {
		evt, ok := e.(*order.OrderFulfillmentCreated)
		if !ok {
			return nil
		}
		o, err := orders.GetByID(ctx, evt.OrderID)
		if err != nil {
			log.Printf("[payment] capture-on-fulfill load order %s: %v", evt.OrderID, err)
			return err // outbox replays - the status guard makes the retry a no-op once someone else captured
		}
		if o.PaymentStatus != order.PaymentAuthorized || o.PaymentProvider == "" || o.PaymentReference == "" {
			return nil
		}
		amountDue := o.AmountDue() // Order.AmountDue() nets BOTH tenders - subtracting only gift cards asked the gateway for more than was ever authorized on any store-credit order
		if amountDue <= 0 {
			return nil
		}
		// Route through the payment-transaction ledger so the auto-capture is recorded + idempotency-keyed (a replayed fulfilled event can't double-capture: the over-capture guard rejects the second intent once the first succeeded), and the order's status
		// derives from the ledger rather than a blind flip to paid
		txn, err := orders.RecordCaptureIntent(ctx, o.ID, amountDue, o.PaymentProvider, "capture on fulfillment")
		if err != nil {
			log.Printf("[payment] capture-on-fulfill record intent for order %s: %v", o.ID, err) // Loud: goods are shipping and the money isn't collected, so this needs an operator, not a log line nobody reads
			orders.RecordCaptureFailure(ctx, o.ID, o.PaymentProvider, err)
			return err // retrying is safe: the over-capture guard rejects a duplicate intent
		}
		res, capErr := payments.Capture(ctx, o.PaymentProvider, payment.CaptureRequest{
			OrderID:           o.ID,
			ChannelID:         o.ChannelID,
			ProviderReference: o.PaymentReference,
			AmountCents:       amountDue,
			IdempotencyKey:    txn.IdempotencyKey,
		})
		ref := ""
		if capErr == nil {
			ref = res.ProviderReference
		}
		if _, err := orders.FinalizePaymentTransaction(ctx, txn.ID, capErr == nil, ref); err != nil {
			log.Printf("[payment] capture-on-fulfill finalize for order %s: %v", o.ID, err)
			return err // money may have moved at the gateway with the ledger row still pending - replay until the ledger agrees
		}
		if capErr != nil {
			log.Printf("[payment] capture on fulfill for order %s (%s): %v", o.ID, o.PaymentProvider, capErr)
			orders.RecordCaptureFailure(ctx, o.ID, o.PaymentProvider, capErr)
		}
		return nil // a gateway decline is finalized + surfaced to the operator - replaying would just re-decline
	})
}
