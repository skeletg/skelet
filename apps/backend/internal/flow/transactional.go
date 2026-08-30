package flow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/channel"
	"github.com/skeletg/skelet/internal/config"
	"github.com/skeletg/skelet/internal/email"
	"github.com/skeletg/skelet/internal/emailtemplate"
	"github.com/skeletg/skelet/internal/order"
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
	storeName := s.customerFacingStoreName(ctx, ch)
	switch f.TriggerType {
	case TriggerOrderShipped:
		ful, err := s.orders.GetFulfillment(ctx, fulfillmentIDFromRun(run))
		if err != nil {
			return "", "", "", err
		}
		return s.tmpl.RenderStoredOrderShipment(ctx, ch.ID, e, email.OrderShipmentData{
			StoreName: storeName, OrderNumber: o.Number, FirstName: orderFirstName(o),
			Carrier: ful.Carrier, TrackingNumber: ful.TrackingNumber,
			Lines: shipmentLines(o, ful), Currency: o.CurrencyCode, StorefrontURL: store,
		})
	case TriggerOrderCancelled:
		return s.tmpl.RenderStoredOrderCancellation(ctx, ch.ID, e, email.OrderCancellationData{
			StoreName: storeName, OrderNumber: o.Number, FirstName: orderFirstName(o), StorefrontURL: store,
		})
	case TriggerPaymentRefunded:
		return s.tmpl.RenderStoredOrderRefund(ctx, ch.ID, e, email.OrderRefundData{
			StoreName: storeName, OrderNumber: o.Number, FirstName: orderFirstName(o),
			Currency: o.CurrencyCode, RefundedAmount: s.refundedAmount(ctx, o.ID), StorefrontURL: store,
		})
	default: // order_placed → confirmation
		return s.tmpl.RenderStoredOrderConfirmation(ctx, ch.ID, e, email.OrderConfirmationData{
			StoreName: storeName, OrderNumber: o.Number, FirstName: orderFirstName(o), Currency: o.CurrencyCode,
			Lines: orderLines(o), Subtotal: o.SubtotalAmount, Discount: o.DiscountAmount,
			Shipping: o.ShippingAmount, Tax: o.TaxAmount, Total: o.TotalAmount,
			ShipTo: addressLines(o.ShippingAddress), StorefrontURL: store,
		})
	}
}

func (s *Service) refundedAmount(ctx context.Context, orderID uuid.UUID) int64 {
	var refunded int64
	if txns, err := s.orders.PaymentLedger(ctx, orderID); err == nil {
		for _, t := range txns {
			if t.Kind == "refund" && t.Status == "succeeded" {
				refunded += t.AmountCents
			}
		}
	}
	return refunded
}

// --- order → email data builders (mirror the former ordermail helpers) ---

func orderFirstName(o order.Order) string {
	if n := strings.TrimSpace(o.ShippingAddress.FirstName); n != "" {
		return n
	}
	return strings.TrimSpace(o.BillingAddress.FirstName)
}

func addressLines(a order.OrderAddress) []string {
	out := []string{strings.TrimSpace(a.FirstName + " " + a.LastName)}
	if a.Company != nil && *a.Company != "" {
		out = append(out, *a.Company)
	}
	out = append(out, a.Street1)
	if a.Street2 != nil && *a.Street2 != "" {
		out = append(out, *a.Street2)
	}
	region := ""
	if a.State != nil {
		region = *a.State
	}
	out = append(out, strings.TrimSpace(a.City+" "+region+" "+a.PostalCode))
	out = append(out, a.CountryCode)
	return out
}

func orderLines(o order.Order) []email.OrderEmailLine {
	lines := make([]email.OrderEmailLine, len(o.Lines))
	for i, l := range o.Lines {
		lines[i] = email.OrderEmailLine{Name: l.ProductName, SKU: l.ProductSku, Quantity: l.Quantity, UnitPrice: l.UnitPrice, LineTotal: l.TotalPrice, Options: lineOptionSummaries(l)}
	}
	return lines
}

// lineOptionSummaries renders a line's custom-option picks as
// "Title: Value" strings for email display
func lineOptionSummaries(l order.OrderLine) []string {
	if len(l.Options) == 0 {
		return nil
	}
	out := make([]string, 0, len(l.Options))
	for _, o := range l.Options {
		v := o.ValueLabel
		if v == "" {
			v = o.TextValue
		}
		if v == "" && o.FileURL != "" {
			v = "uploaded file"
		}
		if v == "" {
			continue
		}
		out = append(out, o.OptionTitle+": "+v)
	}
	return out
}

func shipmentLines(o order.Order, f order.Fulfillment) []email.OrderEmailLine {
	byID := make(map[uuid.UUID]order.OrderLine, len(o.Lines))
	for _, l := range o.Lines {
		byID[l.ID] = l
	}
	lines := make([]email.OrderEmailLine, 0, len(f.Lines))
	for _, fl := range f.Lines {
		ol := byID[fl.OrderLineID]
		lines = append(lines, email.OrderEmailLine{Name: ol.ProductName, SKU: ol.ProductSku, Quantity: fl.Quantity, UnitPrice: ol.UnitPrice, Options: lineOptionSummaries(ol)})
	}
	return lines
}

// The customer-facing store name, falling back to the channel's internal label when the branding
// setting is unset - so an operator who never touched it sees exactly what they saw before
func (s *Service) customerFacingStoreName(ctx context.Context, ch channel.Channel) string {
	if s.config == nil {
		return ch.Name
	}
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &ch.ID}
	if name, err := s.config.GetString(ctx, "general/branding/store_name", scope); err == nil {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			return trimmed
		}
	}
	return ch.Name
}
