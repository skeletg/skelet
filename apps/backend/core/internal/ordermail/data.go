// Package ordermail turns an order's lifecycle into the mail a customer expects: the receipt, the shipment notice, the cancellation and the refund.
package ordermail

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

// FirstName addresses the customer - the shipping name wins, since that's who receives the parcel
func FirstName(o order.Order) string {
	if n := strings.TrimSpace(o.ShippingAddress.FirstName); n != "" {
		return n
	}
	return strings.TrimSpace(o.BillingAddress.FirstName)
}

// AddressLines pre-formats an address for display - the templates print these verbatim
func AddressLines(a order.OrderAddress) []string {
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

func Lines(o order.Order) []email.OrderEmailLine {
	lines := make([]email.OrderEmailLine, len(o.Lines))
	for i, l := range o.Lines {
		lines[i] = email.OrderEmailLine{Name: l.ProductName, SKU: l.ProductSku, Quantity: l.Quantity, UnitPrice: l.UnitPrice, LineTotal: l.TotalPrice, Options: lineOptionSummaries(l)}
	}
	return lines
}

// lineOptionSummaries renders a line's custom-option picks as "Title: Value" strings for email display
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

// ShipmentLines are just the items in this fulfillment, priced from their order line
func ShipmentLines(o order.Order, f order.Fulfillment) []email.OrderEmailLine {
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

func StorefrontURL(ch channel.Channel) string {
	return strings.TrimRight(strings.TrimSpace(ch.PublicURL), "/")
}

// StoreName is the CUSTOMER-facing name, falling back to the channel's internal label when the branding setting is unset. channels.name is the operator's own ("US Wholesale B2B") and printing that in a receipt is what the setting exists to avoid
func StoreName(ctx context.Context, cfg *config.Service, ch channel.Channel) string {
	if cfg == nil {
		return ch.Name
	}
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &ch.ID}
	if name, err := cfg.GetString(ctx, channel.ConfigStoreName, scope); err == nil {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			return trimmed
		}
	}
	return ch.Name
}

// Snapshot captures the order as the four order emails render it, so a flow run advanced minutes
// later shows the order as it was when the event fired rather than whatever it has become
func Snapshot(o order.Order) email.OrderSnapshot {
	return email.OrderSnapshot{
		Number: o.Number, FirstName: FirstName(o), Currency: o.CurrencyCode,
		Lines: Lines(o), Subtotal: o.SubtotalAmount, Discount: o.DiscountAmount,
		Shipping: o.ShippingAmount, Tax: o.TaxAmount, Total: o.TotalAmount,
		ShipTo: AddressLines(o.ShippingAddress),
	}
}

// ShipmentSnapshot narrows the lines to what this fulfillment carried, and names the carrier that took it
func ShipmentSnapshot(o order.Order, f order.Fulfillment) email.OrderSnapshot {
	snap := Snapshot(o)
	snap.Lines = ShipmentLines(o, f)
	snap.Carrier, snap.TrackingNumber = f.Carrier, f.TrackingNumber
	return snap
}
