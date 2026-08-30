package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/sales"
)

// Same reports.view gate and channel scoping as salesReport - it exposes the same revenue by another cut
func (r *queryResolver) ProductSales(ctx context.Context, sku string, days *int32, channelID *string) (*model.ProductSales, error) {
	if r.SalesSvc == nil {
		return nil, nil
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reports.view"); err != nil {
		return nil, err
	}
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "reports.view")
	if err != nil {
		return nil, err
	}

	window := int32(30)
	if days != nil && *days > 0 && *days <= 365 { // a caller-supplied window can't become an unbounded scan
		window = *days
	}
	end := time.Now().UTC()
	f := sales.Filter{Start: end.AddDate(0, 0, -int(window)), End: end}

	switch {
	case channelID != nil && *channelID != "":
		cid, err := parseID(*channelID, "channel")
		if err != nil {
			return nil, err
		}
		if !perm.Allows(cid) {
			return nil, auth.ErrForbidden
		}
		f.ChannelIDs = []uuid.UUID{cid}
	case perm.Bypass():
		f.AllChannels = true
	default:
		ids := perm.IDs()
		if len(ids) == 0 {
			return nil, auth.ErrForbidden
		}
		f.ChannelIDs = ids
	}

	row, err := r.SalesSvc.ProductSalesBySKU(ctx, f, sku)
	if err != nil {
		return nil, err
	}
	return &model.ProductSales{
		UnitsSold:  int(row.UnitsSold),
		Revenue:    int(row.Revenue),
		Cost:       int(row.Cost),
		Margin:     int(row.Margin),
		OrderCount: int32(row.OrderCount),
		Days:       window,
	}, nil
}

// SalesReport assembles the whole dashboard report in one pass - per-currency KPIs (+ refunds), a bucketed time series, top products, and a status breakdown - gated by reports.view and scoped to the caller's permitted channels
func (r *queryResolver) SalesReport(ctx context.Context, channelID *string, startDate *time.Time, endDate *time.Time, bucket *string, topProductsLimit *int32) (*model.SalesReport, error) {
	if r.SalesSvc == nil {
		return nil, nil
	}

	// Hard auth gate - LoadPermittedChannels below returns Bypass=true when there is no admin in context (it assumes a guard already ran), so without this an anonymous caller would read every channel's revenue
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reports.view"); err != nil {
		return nil, err
	}

	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "reports.view")
	if err != nil {
		return nil, err
	}

	start, end := defaultAnalyticsRange(startDate, endDate)
	f := sales.Filter{Start: start, End: end}

	switch {
	case channelID != nil && *channelID != "": // explicit single-channel request - must be inside the caller's scope
		cid, err := parseID(*channelID, "channel")
		if err != nil {
			return nil, err
		}
		if !perm.Allows(cid) {
			return nil, auth.ErrForbidden
		}
		f.ChannelIDs = []uuid.UUID{cid}
	case perm.Bypass(): // superuser / global-scoped - every channel
		f.AllChannels = true
	default: // channel-scoped caller - empty permitted set means no reports.view grant anywhere, deny rather than return empty
		ids := perm.IDs()
		if len(ids) == 0 {
			return nil, auth.ErrForbidden
		}
		f.ChannelIDs = ids
	}

	summary, err := r.SalesSvc.Summary(ctx, f)
	if err != nil {
		return nil, err
	}
	refunds, err := r.SalesSvc.RefundTotals(ctx, f)
	if err != nil {
		return nil, err
	}
	timeseries, err := r.SalesSvc.Timeseries(ctx, f, reportsBucket(bucket))
	if err != nil {
		return nil, err
	}
	top, err := r.SalesSvc.TopProducts(ctx, f, reportsLimit(topProductsLimit))
	if err != nil {
		return nil, err
	}
	byStatus, err := r.SalesSvc.ByStatus(ctx, f)
	if err != nil {
		return nil, err
	}

	return &model.SalesReport{
		Summary:         salesSummaryToModel(summary, refunds),
		Timeseries:      salesTimeseriesToModel(timeseries),
		TopProducts:     salesTopProductsToModel(top),
		StatusBreakdown: salesStatusToModel(byStatus),
	}, nil
}
