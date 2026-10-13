package ordermail

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

var (
	ErrNotResendable         = errors.New("not a resendable order email")
	ErrNoShipment            = errors.New("this order has no shipment to notify about")
	ErrFulfillmentNotOnOrder = errors.New("that shipment is not on this order")
)

func init() {
	apperr.RegisterSentinel(ErrNotResendable, "ORDER_EMAIL_NOT_RESENDABLE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrNoShipment, "ORDER_EMAIL_NO_SHIPMENT", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrFulfillmentNotOnOrder, "ORDER_EMAIL_FULFILLMENT_NOT_ON_ORDER", apperr.ClassNotFound)
}

// The order emails an operator can send again - the same four the lifecycle subscribers deliver
var resendable = map[string]bool{
	email.KeyOrderConfirmation: true,
	email.KeyOrderShipment:     true,
	email.KeyOrderCancellation: true,
	email.KeyOrderRefund:       true,
}

// Resend sends one of an order's lifecycle emails again, skipping the flow engine deliberately - enrolling would restart a journey or hand delivery back to the flow that lost the original
func (s *Service) Resend(ctx context.Context, orderID uuid.UUID, messageKey string, fulfillmentID *uuid.UUID) error {
	if !resendable[messageKey] {
		return fmt.Errorf("%w: %q", ErrNotResendable, messageKey)
	}
	o, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("load order: %w", err)
	}
	ch, err := s.channels.GetByID(ctx, o.ChannelID)
	if err != nil {
		return fmt.Errorf("load channel: %w", err)
	}
	switch messageKey {
	case email.KeyOrderConfirmation:
		return s.sendConfirmation(ctx, o, ch)
	case email.KeyOrderCancellation:
		return s.sendCancellation(ctx, o, ch)
	case email.KeyOrderRefund:
		return s.sendRefund(ctx, o, ch)
	default:
		fid, ferr := s.resendTarget(ctx, o, fulfillmentID)
		if ferr != nil {
			return ferr
		}
		return s.sendShipment(ctx, o, ch, fid)
	}
}

// An unnamed fulfillment resends the newest, and a named one is matched against this order's own so the caller's channel gate can't be sidestepped
func (s *Service) resendTarget(ctx context.Context, o order.Order, want *uuid.UUID) (uuid.UUID, error) {
	fs, err := s.orders.ListFulfillments(ctx, o.ID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("load fulfillments for order %s: %w", o.Number, err)
	}
	if len(fs) == 0 {
		return uuid.Nil, fmt.Errorf("%w: order %s", ErrNoShipment, o.Number)
	}
	if want == nil {
		newest := fs[0]
		for _, f := range fs[1:] {
			if f.CreatedAt.After(newest.CreatedAt) {
				newest = f
			}
		}
		return newest.ID, nil
	}
	for _, f := range fs {
		if f.ID == *want {
			return f.ID, nil
		}
	}
	return uuid.Nil, fmt.Errorf("%w: %s on order %s", ErrFulfillmentNotOnOrder, *want, o.Number)
}
