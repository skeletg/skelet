package ordermail

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// The order emails an operator can send again - the same four the lifecycle subscribers deliver
var resendable = map[string]bool{
	emailtemplate.KeyOrderConfirmation: true,
	emailtemplate.KeyOrderShipment:     true,
	emailtemplate.KeyOrderCancellation: true,
	emailtemplate.KeyOrderRefund:       true,
}

// Resend sends one of an order's lifecycle emails again, skipping the flow engine deliberately - enrolling would restart a journey or hand delivery back to the flow that lost the original
func (s *Service) Resend(ctx context.Context, orderID uuid.UUID, messageKey string, fulfillmentID *uuid.UUID) error {
	if !resendable[messageKey] {
		return fmt.Errorf("%q is not a resendable order email", messageKey)
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
	case emailtemplate.KeyOrderConfirmation:
		return s.sendConfirmation(ctx, o, ch)
	case emailtemplate.KeyOrderCancellation:
		return s.sendCancellation(ctx, o, ch)
	case emailtemplate.KeyOrderRefund:
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
		return uuid.Nil, fmt.Errorf("order %s has no shipment to notify about", o.Number)
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
	return uuid.Nil, fmt.Errorf("fulfillment %s is not on order %s", *want, o.Number)
}
