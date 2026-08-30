package admin

import (
	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/sales"
)

// Whitelisted so the raw value never reaches the date_trunc() call - anything unknown or nil falls back to per-day
func reportsBucket(b *string) string {
	if b == nil {
		return "day"
	}
	switch *b {
	case "day", "week", "month":
		return *b
	default:
		return "day"
	}
}

func reportsLimit(n *int32) int32 { // default 10, max 100 - a caller can't request an unbounded scan
	if n == nil || *n <= 0 {
		return 10
	}
	if *n > 100 {
		return 100
	}
	return *n
}

// Refund totals overlay by currency, and money narrows from int64 cents to the Int64 scalar's Go int - lossless on 64-bit
func salesSummaryToModel(rows []sales.CurrencySummary, refunds []sales.RefundSummary) []*model.SalesSummary {
	refundByCurrency := make(map[string]sales.RefundSummary, len(refunds))
	for _, r := range refunds {
		refundByCurrency[r.CurrencyCode] = r
	}
	out := make([]*model.SalesSummary, len(rows))
	for i, r := range rows {
		var aov int64
		if r.OrderCount > 0 {
			aov = r.GrossRevenue / int64(r.OrderCount)
		}
		ref := refundByCurrency[r.CurrencyCode] // zero value when none
		out[i] = &model.SalesSummary{
			CurrencyCode:      r.CurrencyCode,
			OrderCount:        r.OrderCount,
			GrossRevenue:      int(r.GrossRevenue),
			SubtotalTotal:     int(r.SubtotalTotal),
			DiscountTotal:     int(r.DiscountTotal),
			TaxTotal:          int(r.TaxTotal),
			ShippingTotal:     int(r.ShippingTotal),
			UniqueCustomers:   r.UniqueCustomers,
			AverageOrderValue: int(aov),
			RefundedAmount:    int(ref.RefundedAmount),
			RefundCount:       ref.RefundCount,
			CostTotal:         int(r.CostTotal),
			MarginTotal:       int(r.MarginTotal),
		}
	}
	return out
}

func salesTimeseriesToModel(rows []sales.TimePoint) []*model.SalesTimeseriesPoint {
	out := make([]*model.SalesTimeseriesPoint, len(rows))
	for i, r := range rows {
		out[i] = &model.SalesTimeseriesPoint{
			Period:       r.Period,
			CurrencyCode: r.CurrencyCode,
			OrderCount:   r.OrderCount,
			Revenue:      int(r.Revenue),
		}
	}
	return out
}

func salesTopProductsToModel(rows []sales.ProductSales) []*model.TopProduct {
	out := make([]*model.TopProduct, len(rows))
	for i, r := range rows {
		out[i] = &model.TopProduct{
			Sku:        r.SKU,
			Name:       r.Name,
			UnitsSold:  int(r.UnitsSold),
			Revenue:    int(r.Revenue),
			Cost:       int(r.Cost),
			Margin:     int(r.Margin),
			OrderCount: r.OrderCount,
		}
	}
	return out
}

func salesStatusToModel(rows []sales.StatusCount) []*model.SalesStatusBreakdown {
	out := make([]*model.SalesStatusBreakdown, len(rows))
	for i, r := range rows {
		out[i] = &model.SalesStatusBreakdown{
			Status:     r.Status,
			OrderCount: r.OrderCount,
			Revenue:    int(r.Revenue),
		}
	}
	return out
}
