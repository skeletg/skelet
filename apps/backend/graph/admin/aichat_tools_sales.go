package admin

// AI-chat tool definitions and handlers for the sales / reports domain. The
// shared registry, dispatch and schema helpers live in aichat_tools.go.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/ai"
	"github.com/skeletg/skelet/apps/backend/internal/sales"
)

// salesSignalTools returns the sales / reports tools (reports.view).
func salesSignalTools() []signalTool {
	return []signalTool{
		// ---- Sales (reports.view) ----
		{
			def: ai.Tool{
				Name:        "get_sales_summary",
				Description: "Orders, gross revenue, AOV and unique customers (per currency), plus order counts by status, for the channel over a window.",
				Schema:      windowSchema(nil),
			},
			perm: func(p digestPerms) bool { return p.sales },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, _ toolArgs) string {
				return reuseDigest(func(b *strings.Builder) { writeSalesDigest(ctx, r, cid, since, b) }, "No sales data in this window.")
			},
		},
		{
			def: ai.Tool{
				Name:        "get_top_products",
				Description: "Best-selling products by units sold for the channel over a window. Use 'limit' to drill deeper than the overview's top 5.",
				Schema:      windowSchema(map[string]any{"limit": limitProp("Max rows to return (1-50). Default 10.", 50)}),
			},
			perm: func(p digestPerms) bool { return p.sales },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolTopProducts(ctx, r, cid, since, a.Limit)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_sales_trend",
				Description: "Revenue and order count over time (a time-series) for the channel. Use for 'is revenue growing?' / trend questions. Render the result as a line widget.",
				Schema:      windowSchema(map[string]any{"bucket": enumProp("Time bucket. Default day.", "day", "week")}),
			},
			perm: func(p digestPerms) bool { return p.sales },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolSalesTrend(ctx, r, cid, since, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_refunds",
				Description: "Refund totals (amount + count, per currency) for the channel over a window.",
				Schema:      windowSchema(nil),
			},
			perm: func(p digestPerms) bool { return p.sales },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, _ toolArgs) string {
				return toolRefunds(ctx, r, cid, since)
			},
		},
	}
}

func toolTopProducts(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, limit int) string {
	limit = clampLimit(limit, 10, 50)
	tp, err := r.SalesSvc.TopProducts(ctx, sales.Filter{Start: since, End: time.Now(), ChannelIDs: []uuid.UUID{cid}}, int32(limit))
	if err != nil {
		return "Could not load top products."
	}
	if len(tp) == 0 {
		return "No product sales in this window."
	}
	var b strings.Builder
	b.WriteString("Top products by units sold:\n")
	for i, pr := range tp {
		name := pr.Name
		if name == "" {
			name = pr.SKU
		}
		fmt.Fprintf(&b, "%d. %s - %d units\n", i+1, name, pr.UnitsSold)
	}
	return b.String()
}

func toolSalesTrend(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, a toolArgs) string {
	bucket := bucketOf(a.Bucket)
	pts, err := r.SalesSvc.Timeseries(ctx, sales.Filter{Start: since, End: time.Now(), ChannelIDs: []uuid.UUID{cid}}, bucket)
	if err != nil {
		return "Could not load the sales trend."
	}
	if len(pts) == 0 {
		return "No sales in this window."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Sales by %s (revenue in the currency's major units, then orders):\n", bucket)
	for _, p := range pts {
		fmt.Fprintf(&b, "%s %s: revenue %.2f, orders %d\n", p.Period.Format("2006-01-02"), p.CurrencyCode, float64(p.Revenue)/100, p.OrderCount)
	}
	return b.String()
}

func toolRefunds(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time) string {
	rs, err := r.SalesSvc.RefundTotals(ctx, sales.Filter{Start: since, End: time.Now(), ChannelIDs: []uuid.UUID{cid}})
	if err != nil {
		return "Could not load refunds."
	}
	if len(rs) == 0 {
		return "No refunds in this window."
	}
	var b strings.Builder
	b.WriteString("Refunds:\n")
	for _, x := range rs {
		fmt.Fprintf(&b, "- %s: %s refunded across %d refunds\n", x.CurrencyCode, fmtMoney(x.RefundedAmount, x.CurrencyCode), x.RefundCount)
	}
	return b.String()
}
