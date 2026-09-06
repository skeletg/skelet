package order

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/money"
)

// Records a shipment of a subset of line quantities - Carrier and TrackingNumber seed the OrderFulfilling hook, which a plugin may override
type CreateFulfillmentInput struct {
	OrderID        uuid.UUID
	Lines          []FulfillmentLineInput
	Carrier        string
	TrackingNumber string
	// The declared insurance value, handed to the carrier's label call
	InsuredValueCents int64
}

type FulfillmentLineInput struct {
	OrderLineID uuid.UUID
	Quantity    float64
}

// Records a shipment - validates quantities against the unshipped remainder, writes the fulfillment and lines, derives the order status (fulfilled when every line ships, else partially_fulfilled), and emits OrderStatusChanged plus OrderFulfillmentCreated in the same tx
// Inventory finalizes and the payment bridge captures on first shipment off the latter
func (s *Service) CreateFulfillment(ctx context.Context, in CreateFulfillmentInput) (Order, error) {
	o, err := s.repo.GetOrderByID(ctx, in.OrderID)
	if err != nil {
		return Order{}, fmt.Errorf("order not found")
	}
	if o.Status == OrderStatusCancelled || o.Status == OrderStatusFulfilled {
		return Order{}, apperr.New("FULFILLMENT_ORDER_TERMINAL", apperr.ClassConflict,
			fmt.Sprintf("cannot fulfill an order in status %q", o.Status))
	}
	// Respect the status machine. A terminal-only check let an ON-HOLD order ship
	// straight past the review that put it there, a draft order ship stock it never
	// reserved, and a pending order skip confirmed/processing entirely - all
	// transitions orderStatusTransitions explicitly forbids.
	// FulfillRemaining routes through here, so this is the single gate for both.
	if !validTransition(o.Status, OrderStatusPartiallyFulfilled) && !validTransition(o.Status, OrderStatusFulfilled) {
		return Order{}, apperr.New("FULFILLMENT_STATUS_INVALID", apperr.ClassConflict,
			fmt.Sprintf("an order in status %q cannot ship - confirm and process it first", o.Status))
	}
	if len(in.Lines) == 0 {
		return Order{}, apperr.New("FULFILLMENT_EMPTY", apperr.ClassValidation, "a fulfillment must include at least one line")
	}

	ordered := make(map[uuid.UUID]float64, len(o.Lines))
	for _, l := range o.Lines {
		ordered[l.ID] = l.Quantity
	}
	shipped, err := s.repo.ShippedByOrderLine(ctx, o.ID)
	if err != nil {
		return Order{}, err
	}

	// Validate quantities against the unshipped remainder and learn whether this completes the order - pure, so it's unit-tested
	reqByLine, fully, err := planFulfillment(ordered, shipped, in.Lines)
	if err != nil {
		return Order{}, err
	}

	// Pre-commit OrderFulfilling hook - seed carrier and tracking, and let a subscriber override or abort
	carrier, tracking := in.Carrier, in.TrackingNumber
	if s.bus != nil {
		ing := &OrderFulfilling{OrderID: o.ID, ChannelID: o.ChannelID, FromStatus: o.Status, Carrier: carrier, TrackingNumber: tracking}
		if err := s.bus.PublishSync(ctx, ing); err != nil {
			return Order{}, err
		}
		carrier, tracking = ing.Carrier, ing.TrackingNumber
	}

	newStatus := OrderStatusPartiallyFulfilled
	if fully {
		newStatus = OrderStatusFulfilled
	}

	var evtLines []FulfillmentLineQty
	var fulfillmentID uuid.UUID
	err = s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		count, err := r.CountFulfillments(ctx, o.ID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		f, err := r.CreateFulfillment(ctx, Fulfillment{
			OrderID:           o.ID,
			Status:            FulfillmentShipped,
			Carrier:           carrier,
			TrackingNumber:    tracking,
			Sequence:          int32(count + 1),
			ShippedAt:         &now,
			InsuredValueCents: in.InsuredValueCents,
		})
		if err != nil {
			return err
		}
		fulfillmentID = f.ID
		for lineID, qty := range reqByLine {
			if _, err := r.AddFulfillmentLine(ctx, f.ID, FulfillmentLine{OrderLineID: lineID, Quantity: qty}); err != nil {
				return err
			}
			evtLines = append(evtLines, FulfillmentLineQty{OrderLineID: lineID, Quantity: qty})
		}
		// Guarded on the status read above, so two shipments racing on the same order can't both pass planFulfillment's unshipped check and over-ship
		if _, err := r.UpdateOrderStatusFrom(ctx, o.ID, o.Status, newStatus); err != nil {
			return err
		}
		if carrier != "" || tracking != "" {
			if _, err := r.UpdateOrderTracking(ctx, o.ID, carrier, tracking); err != nil {
				return err
			}
		}
		if _, err := r.AddOrderEvent(ctx, OrderEvent{
			OrderID: o.ID,
			Type:    "fulfillment_created",
			Message: fmt.Sprintf("Shipment #%d created (%s).", count+1, newStatus),
		}); err != nil {
			return fmt.Errorf("add order event: %w", err)
		}
		if s.outbox != nil {
			if err := s.outbox.Enqueue(ctx, sqlTx, &OrderStatusChanged{
				OrderID: o.ID, ChannelID: o.ChannelID, FromStatus: o.Status, ToStatus: newStatus,
			}); err != nil {
				return fmt.Errorf("enqueue order.status_changed: %w", err)
			}
			if err := s.outbox.Enqueue(ctx, sqlTx, &OrderFulfillmentCreated{
				OrderID: o.ID, ChannelID: o.ChannelID, FulfillmentID: f.ID, Lines: evtLines, FullyShipped: fully,
			}); err != nil {
				return fmt.Errorf("enqueue order.fulfillment_created: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return Order{}, err
	}

	// Without the outbox, fall back to best-effort in-process dispatch
	if s.bus != nil && s.outbox == nil {
		s.bus.Publish(ctx, &OrderStatusChanged{OrderID: o.ID, ChannelID: o.ChannelID, FromStatus: o.Status, ToStatus: newStatus})
		s.bus.Publish(ctx, &OrderFulfillmentCreated{OrderID: o.ID, ChannelID: o.ChannelID, FulfillmentID: fulfillmentID, Lines: evtLines, FullyShipped: fully})
	}
	if s.bus != nil && fully {
		s.bus.Publish(ctx, &OrderFulfilled{OrderID: o.ID, ChannelID: o.ChannelID, FromStatus: o.Status})
	}
	return s.repo.GetOrderByID(ctx, o.ID)
}

// ShippedByOrderLine sums non-cancelled shipped quantity per order line - exported for the
// inventory adapter, whose flat-mode cancellation restore must not add shipped units back
func (s *Service) ShippedByOrderLine(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]float64, error) {
	return s.repo.ShippedByOrderLine(ctx, orderID)
}

// Validates a shipment against ordered and already-shipped quantities, returning the deduped per-line amounts and whether this completes every line - pure, so the guards and fully-shipped derivation are unit-testable
// `shipped` excludes cancelled shipments, which is the caller's job
func planFulfillment(ordered, shipped map[uuid.UUID]float64, lines []FulfillmentLineInput) (reqByLine map[uuid.UUID]float64, fully bool, err error) {
	reqByLine = make(map[uuid.UUID]float64, len(lines))
	for _, fl := range lines {
		ord, ok := ordered[fl.OrderLineID]
		if !ok {
			return nil, false, apperr.New("FULFILLMENT_UNKNOWN_LINE", apperr.ClassValidation, "fulfillment line does not belong to this order")
		}
		if fl.Quantity <= 0 {
			return nil, false, apperr.New("FULFILLMENT_INVALID_QTY", apperr.ClassValidation, "fulfillment quantity must be positive")
		}
		reqByLine[fl.OrderLineID] += fl.Quantity
		if shipped[fl.OrderLineID]+reqByLine[fl.OrderLineID] > ord {
			return nil, false, apperr.New("FULFILLMENT_OVERSHIP", apperr.ClassConflict, "fulfillment exceeds the unshipped quantity for a line")
		}
	}
	// Fully shipped iff every ordered line is now covered
	fully = true
	for lineID, ord := range ordered {
		if shipped[lineID]+reqByLine[lineID] < ord {
			fully = false
			break
		}
	}
	return reqByLine, fully, nil
}

// Ships everything still unshipped, the one-click Fulfill flow - a thin wrapper over CreateFulfillment so both paths share the inventory decrement and first-shipment capture - Nothing left is an idempotent no-op
func (s *Service) FulfillRemaining(ctx context.Context, orderID uuid.UUID, carrier, trackingNumber string, insuredValueCents int64) (Order, error) {
	o, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return Order{}, fmt.Errorf("order not found")
	}
	if o.Status == OrderStatusCancelled || o.Status == OrderStatusFulfilled {
		return Order{}, apperr.New("FULFILLMENT_ORDER_TERMINAL", apperr.ClassConflict,
			fmt.Sprintf("cannot fulfill an order in status %q", o.Status))
	}
	shipped, err := s.repo.ShippedByOrderLine(ctx, o.ID)
	if err != nil {
		return Order{}, err
	}
	lines := make([]FulfillmentLineInput, 0, len(o.Lines))
	for _, l := range o.Lines {
		if remaining := l.Quantity - shipped[l.ID]; remaining > 0 {
			lines = append(lines, FulfillmentLineInput{OrderLineID: l.ID, Quantity: remaining})
		}
	}
	if len(lines) == 0 {
		return o, nil
	}
	return s.CreateFulfillment(ctx, CreateFulfillmentInput{
		OrderID:           o.ID,
		Lines:             lines,
		Carrier:           carrier,
		TrackingNumber:    trackingNumber,
		InsuredValueCents: insuredValueCents,
	})
}

// unshippedFraction is the share of the order's line value that never shipped,
// used to scale tender reversals on cancellation. 1.0 when nothing shipped (the
// common case), 0 when everything did. Fails SAFE at 1.0 on a lookup error -
// over-restoring a customer beats silently under-refunding them, and the
// operator can still adjust.
func (s *Service) unshippedFraction(ctx context.Context, o Order) float64 {
	shipped, err := s.repo.ShippedByOrderLine(ctx, o.ID)
	if err != nil || len(shipped) == 0 {
		return 1
	}
	var total, unshipped int64
	for _, l := range o.Lines {
		total += money.LineTotal(l.UnitPrice, l.Quantity)
		if rem := l.Quantity - shipped[l.ID]; rem > 0 {
			unshipped += money.LineTotal(l.UnitPrice, rem)
		}
	}
	if total <= 0 {
		return 1
	}
	return float64(unshipped) / float64(total)
}

func (s *Service) ListFulfillments(ctx context.Context, orderID uuid.UUID) ([]Fulfillment, error) {
	return s.repo.ListFulfillments(ctx, orderID)
}

// One shipment by id, without lines or labels - the label mutation uses it to resolve the owning order for its permission check
func (s *Service) GetFulfillment(ctx context.Context, id uuid.UUID) (Fulfillment, error) {
	return s.repo.GetFulfillment(ctx, id)
}
