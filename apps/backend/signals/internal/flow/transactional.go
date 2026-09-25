package flow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"

	emailpkg "github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

var (
	errEmailNotFound   = errors.New("flow: send_email node references a missing library email")
	errNoOrderSnapshot = errors.New("flow: run carries no order snapshot to render from")
)

// flowTransactional reports whether the flow is designated transactional (its
// sends bypass the marketing-consent gate) - Parsed from the trigger config
func flowTransactional(f Flow) bool {
	if len(f.TriggerConfig) == 0 {
		return false
	}
	var tc TriggerConfig
	if json.Unmarshal(f.TriggerConfig, &tc) != nil {
		return false
	}
	return tc.Transactional
}

// isOrderTrigger reports whether a trigger carries an order in its run context
func isOrderTrigger(t string) bool {
	switch t {
	case TriggerOrderPlaced, TriggerOrderShipped, TriggerOrderCancelled, TriggerPaymentRefunded:
		return true
	}
	return false
}

// orderIDFromRun extracts the order id carried in the run context (Nil if none)
func orderIDFromRun(run FlowRun) uuid.UUID {
	if len(run.Context) == 0 {
		return uuid.Nil
	}
	var rc map[string]any
	if json.Unmarshal(run.Context, &rc) != nil {
		return uuid.Nil
	}
	s, _ := rc["order_id"].(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return id
}

// chosenEmail returns the library email the send_email node renders: the
// node's referenced library email, or one built from its inline content
func (s *Service) chosenEmail(ctx context.Context, ch Store, node Node) (emailtemplate.LibraryEmail, bool) {
	if id := strings.TrimSpace(node.Data.EmailID); id != "" {
		return s.ownEmail(ctx, ch.ID, id)
	}
	return emailtemplate.LibraryEmail{
		ChannelID: ch.ID, Subject: node.Data.Subject, Blocks: node.Data.Blocks, Styles: node.Data.Styles,
	}, true
}

// renderOrderEmail renders the node's chosen email from the snapshot the enrollment carried, so the
// order_items / order_totals / tracking / shipping_address blocks populate without reading the order back
func (s *Service) renderOrderEmail(ctx context.Context, ch Store, f Flow, node Node, run FlowRun) (string, string, string, error) {
	snap, ok := orderSnapshotFromRun(run)
	if !ok {
		return "", "", "", errNoOrderSnapshot
	}
	e, ok := s.chosenEmail(ctx, ch, node)
	if !ok {
		return "", "", "", errEmailNotFound
	}
	store := storefrontURL(ch)
	// The roster's name is the customer-facing one core resolved from its branding settings, never the
	// operator's internal channel label
	storeName := ch.Name
	switch f.TriggerType {
	case TriggerOrderShipped:
		return s.tmpl.RenderStoredOrderShipment(ctx, ch.ID, e, emailpkg.OrderShipmentData{
			StoreName: storeName, OrderNumber: snap.Number, FirstName: snap.FirstName,
			Carrier: snap.Carrier, TrackingNumber: snap.TrackingNumber,
			Lines: snap.Lines, Currency: snap.Currency, StorefrontURL: store,
		})
	case TriggerOrderCancelled:
		return s.tmpl.RenderStoredOrderCancellation(ctx, ch.ID, e, emailpkg.OrderCancellationData{
			StoreName: storeName, OrderNumber: snap.Number, FirstName: snap.FirstName, StorefrontURL: store,
		})
	case TriggerPaymentRefunded:
		return s.tmpl.RenderStoredOrderRefund(ctx, ch.ID, e, emailpkg.OrderRefundData{
			StoreName: storeName, OrderNumber: snap.Number, FirstName: snap.FirstName,
			Currency: snap.Currency, RefundedAmount: snap.RefundedAmount, StorefrontURL: store,
		})
	default: // order_placed → confirmation
		return s.tmpl.RenderStoredOrderConfirmation(ctx, ch.ID, e, emailpkg.OrderConfirmationData{
			StoreName: storeName, OrderNumber: snap.Number, FirstName: snap.FirstName, Currency: snap.Currency,
			Lines: snap.Lines, Subtotal: snap.Subtotal, Discount: snap.Discount,
			Shipping: snap.Shipping, Tax: snap.Tax, Total: snap.Total,
			ShipTo: snap.ShipTo, StorefrontURL: store,
		})
	}
}

func hasOrderSnapshot(run FlowRun) bool {
	_, ok := orderSnapshotFromRun(run)
	return ok
}

// The snapshot the enrollment stored - a run without one predates the field or was enrolled by
// something that never had an order, and rendering an order email from nothing is worse than not sending
func orderSnapshotFromRun(run FlowRun) (emailpkg.OrderSnapshot, bool) {
	if len(run.Context) == 0 {
		return emailpkg.OrderSnapshot{}, false
	}
	var rc struct {
		Order *emailpkg.OrderSnapshot `json:"order"`
	}
	if json.Unmarshal(run.Context, &rc) != nil || rc.Order == nil {
		return emailpkg.OrderSnapshot{}, false
	}
	return *rc.Order, true
}
