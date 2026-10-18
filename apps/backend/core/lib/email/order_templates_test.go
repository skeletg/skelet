package email

import (
	"strings"
	"testing"
)

func TestMoney(t *testing.T) {
	for _, tt := range []struct {
		cents    int64
		currency string
		want     string
	}{
		{0, "USD", "0.00 USD"},
		{6500, "USD", "65.00 USD"},
		{1, "EUR", "0.01 EUR"},
		{123456, "GBP", "1234.56 GBP"},
		{5000, "JPY", "5000 JPY"},
		{1234, "KWD", "1.234 KWD"},
	} {
		if got := formatMoney(tt.cents, tt.currency); got != tt.want {
			t.Errorf("formatMoney(%d,%q) = %q, want %q", tt.cents, tt.currency, got, tt.want)
		}
	}
}

func TestRenderOrderConfirmation(t *testing.T) {
	subject, html, text, err := RenderOrderConfirmation(OrderConfirmationData{
		StoreName:   "Wheel Store",
		OrderNumber: "ORD-001",
		FirstName:   "Jane",
		Currency:    "USD",
		Lines:       []OrderEmailLine{{Name: "Classic Tee", SKU: "TEE-1", Quantity: 2, UnitPrice: 2500, LineTotal: 5000}},
		Subtotal:    5000, Shipping: 500, Tax: 0, Total: 5500,
		ShipTo: []string{"Jane Doe", "1 Market St", "San Francisco CA 94105"},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(subject, "ORD-001") {
		t.Errorf("subject missing order number: %q", subject)
	}
	for _, want := range []string{"Jane", "Classic Tee", "TEE-1", "55.00 USD", "1 Market St"} {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q", want)
		}
		if want == "Classic Tee" && !strings.Contains(text, want) {
			t.Errorf("text missing %q", want)
		}
	}
}

func TestRenderOrderShipment(t *testing.T) {
	_, html, text, err := RenderOrderShipment(OrderShipmentData{
		StoreName: "S", OrderNumber: "ORD-2", FirstName: "Al",
		Carrier: "USPS", TrackingNumber: "1Z999",
		Lines: []OrderEmailLine{{Name: "Cap", SKU: "CAP-1", Quantity: 1}}, Currency: "USD",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"USPS", "1Z999", "Cap"} {
		if !strings.Contains(html, want) || !strings.Contains(text, want) {
			t.Errorf("missing %q in shipment email", want)
		}
	}
}

func TestRenderOrderCancellationAndRefund(t *testing.T) {
	if _, html, _, err := RenderOrderCancellation(OrderCancellationData{StoreName: "S", OrderNumber: "ORD-3"}); err != nil || !strings.Contains(html, "ORD-3") {
		t.Fatalf("cancellation render: err=%v html-ok=%v", err, strings.Contains(html, "ORD-3"))
	}
	subject, html, _, err := RenderOrderRefund(OrderRefundData{StoreName: "S", OrderNumber: "ORD-4", Currency: "USD", RefundedAmount: 1500})
	if err != nil {
		t.Fatalf("refund render: %v", err)
	}
	if !strings.Contains(subject, "ORD-4") || !strings.Contains(html, "15.00 USD") {
		t.Errorf("refund email missing data: subject=%q", subject)
	}
}

// Empty first name falls back to a generic greeting
func TestRenderFallbackGreeting(t *testing.T) {
	_, html, _, _ := RenderOrderConfirmation(OrderConfirmationData{OrderNumber: "X", Currency: "USD"})
	if !strings.Contains(html, "Hi "+greetingFallback) {
		t.Errorf("expected fallback greeting %q in %q", greetingFallback, html)
	}
}
