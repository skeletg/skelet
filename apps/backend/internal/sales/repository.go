package sales

import (
	"context"

	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) SummaryByCurrency(ctx context.Context, f Filter) ([]CurrencySummary, error) {
	rows, err := r.queries.SalesSummaryByCurrency(ctx, db.SalesSummaryByCurrencyParams{
		StartAt:     f.Start,
		EndAt:       f.End,
		AllChannels: f.AllChannels,
		ChannelIds:  f.ChannelIDs,
	})
	if err != nil {
		return nil, err
	}
	out := make([]CurrencySummary, len(rows))
	for i, row := range rows {
		out[i] = CurrencySummary{
			CurrencyCode:    row.CurrencyCode,
			OrderCount:      row.OrderCount,
			GrossRevenue:    row.GrossRevenue,
			SubtotalTotal:   row.SubtotalTotal,
			DiscountTotal:   row.DiscountTotal,
			TaxTotal:        row.TaxTotal,
			ShippingTotal:   row.ShippingTotal,
			UniqueCustomers: row.UniqueCustomers,
		}
	}
	return out, nil
}

func (r *postgresRepository) CostByCurrency(ctx context.Context, f Filter) (map[string]int64, error) {
	rows, err := r.queries.SalesCostByCurrency(ctx, db.SalesCostByCurrencyParams{
		StartAt:     f.Start,
		EndAt:       f.End,
		AllChannels: f.AllChannels,
		ChannelIds:  f.ChannelIDs,
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		out[row.CurrencyCode] = row.CostTotal
	}
	return out, nil
}

func (r *postgresRepository) Timeseries(ctx context.Context, f Filter, bucket string) ([]TimePoint, error) {
	rows, err := r.queries.SalesTimeseries(ctx, db.SalesTimeseriesParams{
		Bucket:      bucket,
		StartAt:     f.Start,
		EndAt:       f.End,
		AllChannels: f.AllChannels,
		ChannelIds:  f.ChannelIDs,
	})
	if err != nil {
		return nil, err
	}
	out := make([]TimePoint, len(rows))
	for i, row := range rows {
		out[i] = TimePoint{
			Period:       row.Period,
			CurrencyCode: row.CurrencyCode,
			OrderCount:   row.OrderCount,
			Revenue:      row.Revenue,
		}
	}
	return out, nil
}

func (r *postgresRepository) ProductSalesBySKU(ctx context.Context, f Filter, sku string) (ProductSales, error) {
	row, err := r.queries.SalesForProductSku(ctx, db.SalesForProductSkuParams{
		ProductSku:  sku,
		StartAt:     f.Start,
		EndAt:       f.End,
		AllChannels: f.AllChannels,
		ChannelIds:  f.ChannelIDs,
	})
	if err != nil {
		return ProductSales{}, err
	}
	return ProductSales{
		SKU:        sku,
		UnitsSold:  row.UnitsSold,
		Revenue:    row.Revenue,
		Cost:       row.Cost,
		Margin:     row.Revenue - row.Cost,
		OrderCount: row.OrderCount,
	}, nil
}

func (r *postgresRepository) TopProducts(ctx context.Context, f Filter, limit int32) ([]ProductSales, error) {
	rows, err := r.queries.SalesTopProducts(ctx, db.SalesTopProductsParams{
		StartAt:     f.Start,
		EndAt:       f.End,
		AllChannels: f.AllChannels,
		ChannelIds:  f.ChannelIDs,
		Lim:         limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ProductSales, len(rows))
	for i, row := range rows {
		out[i] = ProductSales{
			SKU:        row.ProductSku,
			Name:       row.ProductName,
			UnitsSold:  row.UnitsSold,
			Revenue:    row.Revenue,
			Cost:       row.Cost,
			Margin:     row.Revenue - row.Cost,
			OrderCount: row.OrderCount,
		}
	}
	return out, nil
}

func (r *postgresRepository) ByStatus(ctx context.Context, f Filter) ([]StatusCount, error) {
	rows, err := r.queries.SalesByStatus(ctx, db.SalesByStatusParams{
		StartAt:     f.Start,
		EndAt:       f.End,
		AllChannels: f.AllChannels,
		ChannelIds:  f.ChannelIDs,
	})
	if err != nil {
		return nil, err
	}
	out := make([]StatusCount, len(rows))
	for i, row := range rows {
		out[i] = StatusCount{
			Status:     row.Status,
			OrderCount: row.OrderCount,
			Revenue:    row.Revenue,
		}
	}
	return out, nil
}

func (r *postgresRepository) RefundTotals(ctx context.Context, f Filter) ([]RefundSummary, error) {
	rows, err := r.queries.SalesRefundTotals(ctx, db.SalesRefundTotalsParams{
		StartAt:     f.Start,
		EndAt:       f.End,
		AllChannels: f.AllChannels,
		ChannelIds:  f.ChannelIDs,
	})
	if err != nil {
		return nil, err
	}
	out := make([]RefundSummary, len(rows))
	for i, row := range rows {
		out[i] = RefundSummary{
			CurrencyCode:   row.CurrencyCode,
			RefundedAmount: row.RefundedAmount,
			RefundCount:    row.RefundCount,
		}
	}
	return out, nil
}
