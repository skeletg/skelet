package agenttools

// Agent tool definitions and handlers for the sales / reports domain. The
// shared registry, dispatch and schema helpers live in tools.go.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/sales"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

// salesTools returns the sales / reports tools (reports.view).
func salesTools() []tool {
	return []tool{
		// ---- Sales (reports.view) ----
		{
			def: ai.Tool{
				Name:        "get_sales_summary",
				Description: "Orders, gross revenue, AOV and unique customers (per currency), plus order counts by status, for the channel over a window.",
				Schema:      windowSchema(nil),
			},
			perm: func(p Perms) bool { return p.Sales },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time, days int, _ toolArgs) string {
				return salesDigest.tool(ctx, deps, cid, since, days)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_top_products",
				Description: "Best-selling products by units sold for the channel over a window. Use 'limit' to drill deeper than the overview's top 5.",
				Schema:      windowSchema(map[string]any{"limit": limitProp("Max rows to return (1-50). Default 10.", 50)}),
			},
			perm: func(p Perms) bool { return p.Sales },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolTopProducts(ctx, deps, cid, since, a.Limit)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_sales_trend",
				Description: "Revenue and order count over time (a time-series) for the channel. Use for 'is revenue growing?' / trend questions.",
				Schema:      windowSchema(map[string]any{"bucket": enumProp("Time bucket. Default day.", "day", "week")}),
			},
			perm: func(p Perms) bool { return p.Sales },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolSalesTrend(ctx, deps, cid, since, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_refunds",
				Description: "Refund totals (amount + count, per currency) for the channel over a window.",
				Schema:      windowSchema(nil),
			},
			perm: func(p Perms) bool { return p.Sales },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time, _ int, _ toolArgs) string {
				return toolRefunds(ctx, deps, cid, since)
			},
		},
	}
}

func toolTopProducts(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time, limit int) string {
	limit = clampLimit(limit, 10, 50)
	tp, err := deps.Sales.TopProducts(ctx, sales.Filter{Start: since, End: time.Now(), ChannelIDs: []uuid.UUID{cid}}, int32(limit))
	if err != nil {
		return toolFailed(ctx, err, "Could not load top products.")
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

func toolSalesTrend(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time, a toolArgs) string {
	bucket := dayOrWeek(a.Bucket)
	pts, err := deps.Sales.Timeseries(ctx, sales.Filter{Start: since, End: time.Now(), ChannelIDs: []uuid.UUID{cid}}, bucket)
	if err != nil {
		return toolFailed(ctx, err, "Could not load the sales trend.")
	}
	if len(pts) == 0 {
		return "No sales in this window."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Sales by %s (revenue in the currency's major units, then orders):\n", bucket)
	for _, p := range pts {
		fmt.Fprintf(&b, "%s: revenue %s, orders %d\n", p.Period.Format("2006-01-02"), money.FormatCode(p.Revenue, p.CurrencyCode), p.OrderCount)
	}
	return b.String()
}

func toolRefunds(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time) string {
	rs, err := deps.Sales.RefundTotals(ctx, sales.Filter{Start: since, End: time.Now(), ChannelIDs: []uuid.UUID{cid}})
	if err != nil {
		return toolFailed(ctx, err, "Could not load refunds.")
	}
	if len(rs) == 0 {
		return "No refunds in this window."
	}
	var b strings.Builder
	b.WriteString("Refunds:\n")
	for _, x := range rs {
		fmt.Fprintf(&b, "- %s: %s refunded across %d refunds\n", x.CurrencyCode, money.FormatCode(x.RefundedAmount, x.CurrencyCode), x.RefundCount)
	}
	return b.String()
}
