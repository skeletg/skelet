package admin

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/sales"
)

func strptr(s string) *string { return &s }
func i32ptr(n int32) *int32   { return &n }

func TestReportsBucket(t *testing.T) {
	tests := []struct {
		name string
		in   *string
		want string
	}{
		{"nil defaults to day", nil, "day"},
		{"day", strptr("day"), "day"},
		{"week", strptr("week"), "week"},
		{"month", strptr("month"), "month"},
		{"unknown falls back to day", strptr("year"), "day"},
		{"empty falls back to day", strptr(""), "day"},
		{"injection attempt falls back to day", strptr("day); DROP"), "day"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reportsBucket(tt.in); got != tt.want {
				t.Errorf("reportsBucket(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestReportsLimit(t *testing.T) {
	tests := []struct {
		name string
		in   *int32
		want int32
	}{
		{"nil defaults to 10", nil, 10},
		{"zero defaults to 10", i32ptr(0), 10},
		{"negative defaults to 10", i32ptr(-5), 10},
		{"in range", i32ptr(25), 25},
		{"at max", i32ptr(100), 100},
		{"over max clamps to 100", i32ptr(500), 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reportsLimit(tt.in); got != tt.want {
				t.Errorf("reportsLimit(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestSalesSummaryToModel(t *testing.T) {
	rows := []sales.CurrencySummary{
		{CurrencyCode: "USD", OrderCount: 4, GrossRevenue: 10000, SubtotalTotal: 9000, DiscountTotal: 500, TaxTotal: 800, ShippingTotal: 700, UniqueCustomers: 3},
		{CurrencyCode: "EUR", OrderCount: 0, GrossRevenue: 0}, // no orders → AOV must not divide by zero
	}
	refunds := []sales.RefundSummary{
		{CurrencyCode: "USD", RefundedAmount: 1500, RefundCount: 2},
		// EUR has no refunds; GBP refund has no matching summary row and is dropped.
		{CurrencyCode: "GBP", RefundedAmount: 999, RefundCount: 1},
	}

	got := salesSummaryToModel(rows, refunds)
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}

	usd := got[0]
	if usd.CurrencyCode != "USD" {
		t.Fatalf("row 0 currency = %q, want USD", usd.CurrencyCode)
	}
	if usd.AverageOrderValue != 2500 { // 10000 / 4
		t.Errorf("USD AOV = %d, want 2500", usd.AverageOrderValue)
	}
	if usd.RefundedAmount != 1500 || usd.RefundCount != 2 {
		t.Errorf("USD refunds = (%d, %d), want (1500, 2)", usd.RefundedAmount, usd.RefundCount)
	}

	eur := got[1]
	if eur.AverageOrderValue != 0 {
		t.Errorf("EUR AOV = %d, want 0 (no orders)", eur.AverageOrderValue)
	}
	if eur.RefundedAmount != 0 || eur.RefundCount != 0 {
		t.Errorf("EUR refunds = (%d, %d), want (0, 0)", eur.RefundedAmount, eur.RefundCount)
	}
}
