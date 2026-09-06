package flow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/email"
	"github.com/skeletg/skelet/apps/backend/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/internal/ordermail"
)

var errEmailNotFound = errors.New("flow: send_email node references a missing library email")

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

func fulfillmentIDFromRun(run FlowRun) uuid.UUID {
	var rc map[string]any
	if json.Unmarshal(run.Context, &rc) != nil {
		return uuid.Nil
	}
	s, _ := rc["fulfillment_id"].(string)
	id, _ := uuid.Parse(s)
	return id
}

// chosenEmail returns the library email the send_email node renders: the
// node's referenced library email, or one built from its inline content
func (s *Service) chosenEmail(ctx context.Context, ch channel.Channel, node Node) (emailtemplate.LibraryEmail, bool) {
	if id := strings.TrimSpace(node.Data.EmailID); id != "" {
		if eid, err := uuid.Parse(id); err == nil {
			if e, ok, err := s.tmpl.GetLibraryEmail(ctx, eid); err == nil && ok {
				return e, true
			}
		}
		return emailtemplate.LibraryEmail{}, false
	}
	return emailtemplate.LibraryEmail{
		ChannelID: ch.ID, Subject: node.Data.Subject, Blocks: node.Data.Blocks, Styles: node.Data.Styles,
	}, true
}

// renderOrderEmail loads the run's order and renders the node's chosen email with the full order render context for the
// flow's trigger kind, so the order_items / order_totals / tracking / shipping_address blocks populate
func (s *Service) renderOrderEmail(ctx context.Context, ch channel.Channel, f Flow, node Node, run FlowRun) (string, string, string, error) {
	o, err := s.orders.GetByID(ctx, orderIDFromRun(run))
	if err != nil {
		return "", "", "", err
	}
	e, ok := s.chosenEmail(ctx, ch, node)
	if !ok {
		return "", "", "", errEmailNotFound
	}
	store := storefrontURL(ch)
	// general/branding/store_name is the CUSTOMER-facing name; channels.name is the operator's internal
	// label ("US Wholesale B2B"). Emails printed the internal one, which is what the setting exists to avoid
	storeName := ordermail.StoreName(ctx, s.config, ch)
	switch f.TriggerType {
	case TriggerOrderShipped:
		ful, err := s.orders.GetFulfillment(ctx, fulfillmentIDFromRun(run))
		if err != nil {
			return "", "", "", err
		}
		return s.tmpl.RenderStoredOrderShipment(ctx, ch.ID, e, email.OrderShipmentData{
			StoreName: storeName, OrderNumber: o.Number, FirstName: ordermail.FirstName(o),
			Carrier: ful.Carrier, TrackingNumber: ful.TrackingNumber,
			Lines: ordermail.ShipmentLines(o, ful), Currency: o.CurrencyCode, StorefrontURL: store,
		})
	case TriggerOrderCancelled:
		return s.tmpl.RenderStoredOrderCancellation(ctx, ch.ID, e, email.OrderCancellationData{
			StoreName: storeName, OrderNumber: o.Number, FirstName: ordermail.FirstName(o), StorefrontURL: store,
		})
	case TriggerPaymentRefunded:
		return s.tmpl.RenderStoredOrderRefund(ctx, ch.ID, e, email.OrderRefundData{
			StoreName: storeName, OrderNumber: o.Number, FirstName: ordermail.FirstName(o),
			Currency: o.CurrencyCode, RefundedAmount: ordermail.RefundedAmount(ctx, s.orders, o.ID), StorefrontURL: store,
		})
	default: // order_placed → confirmation
		return s.tmpl.RenderStoredOrderConfirmation(ctx, ch.ID, e, email.OrderConfirmationData{
			StoreName: storeName, OrderNumber: o.Number, FirstName: ordermail.FirstName(o), Currency: o.CurrencyCode,
			Lines: ordermail.Lines(o), Subtotal: o.SubtotalAmount, Discount: o.DiscountAmount,
			Shipping: o.ShippingAmount, Tax: o.TaxAmount, Total: o.TotalAmount,
			ShipTo: ordermail.AddressLines(o.ShippingAddress), StorefrontURL: store,
		})
	}
}
