// Package ordercapture captures the authorized amount when goods first ship
package ordercapture

import (
	"context"
	"log"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// Orders is the order-side surface a capture needs
type Orders interface {
	GetByID(ctx context.Context, id uuid.UUID) (order.Order, error)
	RecordCaptureIntentKeyed(ctx context.Context, orderID uuid.UUID, amountCents int64, providerSlug, reason, key string) (order.PaymentTransaction, error)
	FinalizePaymentTransaction(ctx context.Context, txnID uuid.UUID, ok bool, providerReference string) (order.Order, error)
	RecordCaptureFailure(ctx context.Context, orderID uuid.UUID, providerSlug string, cause error)
}

// Payments is the gateway surface - Capture carries the idempotency key from the
// intent, so a redelivered fulfillment event can't double-charge
type Payments interface {
	Capture(ctx context.Context, providerSlug string, req payment.CaptureRequest) (payment.CaptureResult, error)
}

// Register attaches capture-on-first-ship. Async and best-effort at the bus level - the intent row plus its idempotency key are what make it safe to retry
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
		// Through the ledger under one key per order - a replay finds the intent it started and resumes it, where a fresh intent each time was refused as an over-capture by the still-pending first and left that one pending forever
		// The same key reaches the gateway, so resuming can't charge twice
		txn, err := orders.RecordCaptureIntentKeyed(ctx, o.ID, amountDue, o.PaymentProvider, "capture on fulfillment", captureKey(o.ID))
		if err != nil {
			log.Printf("[payment] capture-on-fulfill record intent for order %s: %v", o.ID, err)
			// Loud either way - goods are shipping and the money isn't collected - but a refusal (over-capture, nothing authorized) won't change on replay, so only an infrastructure failure is retried
			if permanent(err) {
				orders.RecordCaptureFailure(ctx, o.ID, o.PaymentProvider, err)
				return nil
			}
			return err
		}
		if txn.Status == order.PaymentTxnSucceeded {
			return nil // an earlier delivery finished it
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

func captureKey(orderID uuid.UUID) string { return "capture:fulfill:" + orderID.String() }

// A refusal the ledger made on its own rules - replaying the event gets the same answer
func permanent(err error) bool {
	switch apperr.ClassOf(err) {
	case apperr.ClassConflict, apperr.ClassValidation, apperr.ClassNotFound:
		return true
	}
	return false
}
