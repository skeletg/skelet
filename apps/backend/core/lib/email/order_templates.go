package email

import (
	"bytes"
	"fmt"
	"html/template"
	ttemplate "text/template"

	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

// formatMoney renders minor units as "12.34 USD" for email bodies, at the currency's ISO decimals so a yen total isn't divided by 100 - no symbol table, since the code is unambiguous and locale-agnostic
func formatMoney(minor int64, currency string) string {
	return money.Format(minor, money.ISODecimals(currency)) + " " + currency
}

var (
	emailFuncs     = template.FuncMap{"money": formatMoney}
	emailTextFuncs = ttemplate.FuncMap{"money": formatMoney}
)

// OrderEmailLine is one line item rendered in an order email
type OrderEmailLine struct {
	Name      string
	SKU       string
	Quantity  float64
	UnitPrice int64
	LineTotal int64
	// Options are the line's custom-option picks rendered as
	// "Title: Value" strings (engraving text, size, uploaded file name)
	Options []string
}

// OrderConfirmationData is the render context for the order-confirmation email
type OrderConfirmationData struct {
	StoreName     string
	OrderNumber   string
	FirstName     string
	Currency      string
	Lines         []OrderEmailLine
	Subtotal      int64
	Discount      int64
	Shipping      int64
	Tax           int64
	Total         int64
	ShipTo        []string // pre-formatted shipping address lines
	StorefrontURL string
}

// OrderShipmentData is the render context for a shipment email (one per
// fulfillment) - Lines are just the items in this shipment
type OrderShipmentData struct {
	StoreName      string
	OrderNumber    string
	FirstName      string
	Carrier        string
	TrackingNumber string
	Lines          []OrderEmailLine
	Currency       string
	StorefrontURL  string
}

type OrderCancellationData struct {
	StoreName     string
	OrderNumber   string
	FirstName     string
	StorefrontURL string
}

// OrderRefundData is the render context for a refund-confirmation email
type OrderRefundData struct {
	StoreName      string
	OrderNumber    string
	FirstName      string
	Currency       string
	RefundedAmount int64
	StorefrontURL  string
}

// OrderSnapshot is an order as it stood when the event fired, carrying everything the four order emails render from
// These are event emails: the customer wants the order at the moment it was placed, shipped, cancelled or refunded
type OrderSnapshot struct {
	Number    string           `json:"number"`
	FirstName string           `json:"first_name"`
	Currency  string           `json:"currency"`
	Lines     []OrderEmailLine `json:"lines,omitempty"`
	Subtotal  int64            `json:"subtotal,omitempty"`
	Discount  int64            `json:"discount,omitempty"`
	Shipping  int64            `json:"shipping,omitempty"`
	Tax       int64            `json:"tax,omitempty"`
	Total     int64            `json:"total,omitempty"`
	ShipTo    []string         `json:"ship_to,omitempty"`
	// Set on a shipment, from the fulfillment that triggered it rather than the order's latest
	Carrier        string `json:"carrier,omitempty"`
	TrackingNumber string `json:"tracking_number,omitempty"`
	// Set on a refund, as it stood when the refund landed
	RefundedAmount int64 `json:"refunded_amount,omitempty"`
}

// The transactional messages core sends, named so a send is identifiable in a log or a resend request
const (
	KeyOrderConfirmation  = "order_confirmation"
	KeyOrderShipment      = "order_shipment"
	KeyOrderCancellation  = "order_cancellation"
	KeyOrderRefund        = "order_refund"
	KeyVerification       = "customer_verification"
	KeyPasswordReset      = "password_reset"
	KeySignupConfirmation = "signup_confirmation"
	KeyAccountExists      = "account_exists"
)

const greetingFallback = "there"

var orderConfirmationHTML = template.Must(template.New("order_conf_html").Funcs(emailFuncs).Parse(`<!DOCTYPE html>
<html><body style="font-family:sans-serif;color:#222;">
<h2 style="margin:0 0 4px;">{{.StoreName}}</h2>
<p>Hi {{.FirstName}},</p>
<p>Thanks for your order! We've received <strong>order {{.OrderNumber}}</strong> and it's being processed.</p>
<table cellpadding="6" cellspacing="0" style="border-collapse:collapse;width:100%;max-width:520px;">
  <thead><tr style="text-align:left;border-bottom:1px solid #ddd;"><th>Item</th><th>Qty</th><th style="text-align:right;">Total</th></tr></thead>
  <tbody>
  {{range .Lines}}<tr style="border-bottom:1px solid #f0f0f0;"><td>{{.Name}}<br><span style="color:#888;font-size:12px;">{{.SKU}}</span>{{range .Options}}<br><span style="color:#666;font-size:12px;">{{.}}</span>{{end}}</td><td>{{.Quantity}}</td><td style="text-align:right;">{{money .LineTotal $.Currency}}</td></tr>
  {{end}}</tbody>
</table>
<table cellpadding="2" cellspacing="0" style="margin-top:12px;max-width:520px;width:100%;">
  <tr><td>Subtotal</td><td style="text-align:right;">{{money .Subtotal .Currency}}</td></tr>
  {{if .Discount}}<tr><td>Discount</td><td style="text-align:right;">-{{money .Discount .Currency}}</td></tr>{{end}}
  <tr><td>Shipping</td><td style="text-align:right;">{{money .Shipping .Currency}}</td></tr>
  <tr><td>Tax</td><td style="text-align:right;">{{money .Tax .Currency}}</td></tr>
  <tr><td><strong>Total</strong></td><td style="text-align:right;"><strong>{{money .Total .Currency}}</strong></td></tr>
</table>
{{if .ShipTo}}<p style="margin-top:12px;"><strong>Shipping to</strong><br>{{range .ShipTo}}{{.}}<br>{{end}}</p>{{end}}
{{if .StorefrontURL}}<p><a href="{{.StorefrontURL}}">Visit {{.StoreName}}</a></p>{{end}}
</body></html>`))

var orderConfirmationText = ttemplate.Must(ttemplate.New("order_conf_text").Funcs(emailTextFuncs).Parse(
	`Hi {{.FirstName}},

Thanks for your order! We've received order {{.OrderNumber}} and it's being processed.

{{range .Lines}}- {{.Name}} ({{.SKU}}) x{{.Quantity}}  {{money .LineTotal $.Currency}}
{{range .Options}}    {{.}}
{{end}}{{end}}
Subtotal: {{money .Subtotal .Currency}}
{{if .Discount}}Discount: -{{money .Discount .Currency}}
{{end}}Shipping: {{money .Shipping .Currency}}
Tax: {{money .Tax .Currency}}
Total: {{money .Total .Currency}}
{{if .ShipTo}}
Shipping to:
{{range .ShipTo}}{{.}}
{{end}}{{end}}`))

var orderShipmentHTML = template.Must(template.New("order_ship_html").Funcs(emailFuncs).Parse(`<!DOCTYPE html>
<html><body style="font-family:sans-serif;color:#222;">
<h2 style="margin:0 0 4px;">{{.StoreName}}</h2>
<p>Hi {{.FirstName}},</p>
<p>Good news - items from <strong>order {{.OrderNumber}}</strong> are on the way.</p>
{{if .Carrier}}<p>Carrier: {{.Carrier}}{{if .TrackingNumber}} &middot; Tracking: <strong>{{.TrackingNumber}}</strong>{{end}}</p>{{end}}
<table cellpadding="6" cellspacing="0" style="border-collapse:collapse;width:100%;max-width:520px;">
  <thead><tr style="text-align:left;border-bottom:1px solid #ddd;"><th>Item</th><th>Qty</th></tr></thead>
  <tbody>{{range .Lines}}<tr style="border-bottom:1px solid #f0f0f0;"><td>{{.Name}}<br><span style="color:#888;font-size:12px;">{{.SKU}}</span>{{range .Options}}<br><span style="color:#666;font-size:12px;">{{.}}</span>{{end}}</td><td>{{.Quantity}}</td></tr>
  {{end}}</tbody>
</table>
{{if .StorefrontURL}}<p><a href="{{.StorefrontURL}}">Visit {{.StoreName}}</a></p>{{end}}
</body></html>`))

var orderShipmentText = ttemplate.Must(ttemplate.New("order_ship_text").Funcs(emailTextFuncs).Parse(
	`Hi {{.FirstName}},

Good news - items from order {{.OrderNumber}} are on the way.
{{if .Carrier}}Carrier: {{.Carrier}}{{if .TrackingNumber}} - Tracking: {{.TrackingNumber}}{{end}}
{{end}}
{{range .Lines}}- {{.Name}} ({{.SKU}}) x{{.Quantity}}
{{range .Options}}    {{.}}
{{end}}{{end}}`))

var orderCancellationHTML = template.Must(template.New("order_cancel_html").Parse(`<!DOCTYPE html>
<html><body style="font-family:sans-serif;color:#222;">
<h2 style="margin:0 0 4px;">{{.StoreName}}</h2>
<p>Hi {{.FirstName}},</p>
<p>Your <strong>order {{.OrderNumber}}</strong> has been cancelled. If a payment was taken, any refund will follow separately.</p>
<p>If you didn't expect this, please contact us.</p>
{{if .StorefrontURL}}<p><a href="{{.StorefrontURL}}">Visit {{.StoreName}}</a></p>{{end}}
</body></html>`))

var orderCancellationText = ttemplate.Must(ttemplate.New("order_cancel_text").Parse(
	`Hi {{.FirstName}},

Your order {{.OrderNumber}} has been cancelled. If a payment was taken, any refund will follow separately.

If you didn't expect this, please contact us.
`))

var orderRefundHTML = template.Must(template.New("order_refund_html").Funcs(emailFuncs).Parse(`<!DOCTYPE html>
<html><body style="font-family:sans-serif;color:#222;">
<h2 style="margin:0 0 4px;">{{.StoreName}}</h2>
<p>Hi {{.FirstName}},</p>
<p>We've issued a refund of <strong>{{money .RefundedAmount .Currency}}</strong> for <strong>order {{.OrderNumber}}</strong>. It may take a few business days to appear on your statement.</p>
{{if .StorefrontURL}}<p><a href="{{.StorefrontURL}}">Visit {{.StoreName}}</a></p>{{end}}
</body></html>`))

var orderRefundText = ttemplate.Must(ttemplate.New("order_refund_text").Funcs(emailTextFuncs).Parse(
	`Hi {{.FirstName}},

We've issued a refund of {{money .RefundedAmount .Currency}} for order {{.OrderNumber}}. It may take a few business days to appear on your statement.
`))

// RenderOrderConfirmation produces the subject, HTML and text bodies for an
// order-confirmation email
func RenderOrderConfirmation(d OrderConfirmationData) (subject, html, text string, err error) {
	if d.FirstName == "" {
		d.FirstName = greetingFallback
	}
	var hb, tb bytes.Buffer
	if err := orderConfirmationHTML.Execute(&hb, d); err != nil {
		return "", "", "", fmt.Errorf("render html: %w", err)
	}
	if err := orderConfirmationText.Execute(&tb, d); err != nil {
		return "", "", "", fmt.Errorf("render text: %w", err)
	}
	return fmt.Sprintf("Order %s confirmed", d.OrderNumber), hb.String(), tb.String(), nil
}

// RenderOrderShipment produces a shipment email
func RenderOrderShipment(d OrderShipmentData) (subject, html, text string, err error) {
	if d.FirstName == "" {
		d.FirstName = greetingFallback
	}
	var hb, tb bytes.Buffer
	if err := orderShipmentHTML.Execute(&hb, d); err != nil {
		return "", "", "", fmt.Errorf("render html: %w", err)
	}
	if err := orderShipmentText.Execute(&tb, d); err != nil {
		return "", "", "", fmt.Errorf("render text: %w", err)
	}
	return fmt.Sprintf("Order %s has shipped", d.OrderNumber), hb.String(), tb.String(), nil
}

// RenderOrderCancellation produces a cancellation email
func RenderOrderCancellation(d OrderCancellationData) (subject, html, text string, err error) {
	if d.FirstName == "" {
		d.FirstName = greetingFallback
	}
	var hb, tb bytes.Buffer
	if err := orderCancellationHTML.Execute(&hb, d); err != nil {
		return "", "", "", fmt.Errorf("render html: %w", err)
	}
	if err := orderCancellationText.Execute(&tb, d); err != nil {
		return "", "", "", fmt.Errorf("render text: %w", err)
	}
	return fmt.Sprintf("Order %s cancelled", d.OrderNumber), hb.String(), tb.String(), nil
}

// RenderOrderRefund produces a refund-confirmation email
func RenderOrderRefund(d OrderRefundData) (subject, html, text string, err error) {
	if d.FirstName == "" {
		d.FirstName = greetingFallback
	}
	var hb, tb bytes.Buffer
	if err := orderRefundHTML.Execute(&hb, d); err != nil {
		return "", "", "", fmt.Errorf("render html: %w", err)
	}
	if err := orderRefundText.Execute(&tb, d); err != nil {
		return "", "", "", fmt.Errorf("render text: %w", err)
	}
	return fmt.Sprintf("Refund for order %s", d.OrderNumber), hb.String(), tb.String(), nil
}
