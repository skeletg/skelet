package admin

// Helpers for insights.resolvers.go. buildInsights runs a few cheap, deterministic
// checks against the read-models the admin may see and turns notable changes into
// SignalsInsight cards. Every check is best-effort + permission-gated; a failure
// just drops that card. No LLM involved - this is plain analytics.

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/sales"
)

func pctDelta(cur, prev int64) float64 {
	if prev == 0 {
		if cur > 0 {
			return 100
		}
		return 0
	}
	return float64(cur-prev) / float64(prev) * 100
}

func buildInsights(ctx context.Context, r *Resolver, cid uuid.UUID, days int, p digestPerms) []*model.SignalsInsight {
	now := time.Now()
	since := now.AddDate(0, 0, -days)
	prevSince := since.AddDate(0, 0, -days) // immediately-prior equal-length window

	out := make([]*model.SignalsInsight, 0, 6)
	add := func(level, title, detail string) {
		out = append(out, &model.SignalsInsight{Level: level, Title: title, Detail: detail})
	}

	if p.sales {
		f := sales.Filter{Start: since, End: now, ChannelIDs: []uuid.UUID{cid}}
		cur, _ := r.SalesSvc.Summary(ctx, f)
		prev, _ := r.SalesSvc.Summary(ctx, sales.Filter{Start: prevSince, End: since, ChannelIDs: []uuid.UUID{cid}})
		prevRev := map[string]int64{}
		for _, c := range prev {
			prevRev[c.CurrencyCode] = c.GrossRevenue
		}
		for _, c := range cur {
			pr := prevRev[c.CurrencyCode]
			if c.GrossRevenue == 0 && pr == 0 {
				continue
			}
			d := pctDelta(c.GrossRevenue, pr)
			level, word := "info", "flat"
			if d <= -10 {
				level, word = "warn", "down"
			} else if d >= 10 {
				level, word = "good", "up"
			}
			add(level, fmt.Sprintf("Revenue %s", word),
				fmt.Sprintf("%s: %s in the last %dd vs %s the prior %dd (%+.0f%%).",
					c.CurrencyCode, fmtMoney(c.GrossRevenue, c.CurrencyCode), days, fmtMoney(pr, c.CurrencyCode), days, d))
		}
		if st, err := r.SalesSvc.ByStatus(ctx, f); err == nil {
			var pending int32
			for _, s := range st {
				switch s.Status {
				case "pending", "confirmed", "processing":
					pending += s.OrderCount
				}
			}
			if pending > 0 {
				add("info", "Orders to fulfill", fmt.Sprintf("%d order(s) awaiting fulfillment.", pending))
			}
		}
	}

	if p.prism {
		if ov, err := r.ExperimentSvc.Overview(ctx, cid, since); err == nil && ov.Decided > 0 {
			add("good", "Experiment decided",
				fmt.Sprintf("%d running experiment(s) reached a significant winner - consider rolling out.", ov.Decided))
		}
	}

	if p.lens {
		if ov, err := r.LensSvc.Overview(ctx, cid, since); err == nil && ov.Visitors > 0 {
			add("info", "Traffic", fmt.Sprintf("%d visitors / %d page views in the last %dd.", ov.Visitors, ov.Views, days))
		}
	}

	if p.inventory {
		if low, err := r.InventorySvc.ListLowStockLevels(ctx, 1000); err == nil && len(low) > 0 {
			add("warn", "Low stock", fmt.Sprintf("%d item(s) at or below their low-stock threshold.", len(low)))
		}
	}

	return out
}
