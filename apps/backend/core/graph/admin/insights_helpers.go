package admin

// Helpers for insights.resolvers.go. buildInsights runs a few cheap, deterministic checks against the read-models the admin may see and turns notable changes into SignalsInsight cards. Every check is best-effort + permission-gated; a failure just drops that card. No LLM involved - this is plain analytics.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/agenttools"
	"github.com/skeletg/skelet/apps/backend/core/internal/sales"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
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

func buildInsights(ctx context.Context, r *Resolver, cid uuid.UUID, days int, p agenttools.Perms) []*model.SignalsInsight {
	now := time.Now()
	since := now.AddDate(0, 0, -days)
	prevSince := since.AddDate(0, 0, -days) // immediately-prior equal-length window

	out := make([]*model.SignalsInsight, 0, 6)
	add := func(level, title, detail string) {
		out = append(out, &model.SignalsInsight{Level: level, Title: title, Detail: detail})
	}

	if p.Sales {
		f := sales.Filter{Start: since, End: now, ChannelIDs: []uuid.UUID{cid}}
		cur, cerr := r.SalesSvc.Summary(ctx, f)
		prev, perr := r.SalesSvc.Summary(ctx, sales.Filter{Start: prevSince, End: since, ChannelIDs: []uuid.UUID{cid}})
		if err := errors.Join(cerr, perr); err != nil { // half a comparison would read as a swing, so no card at all
			slog.WarnContext(ctx, "insights: sales summary failed", "err", err)
			cur = nil
		}
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
					c.CurrencyCode, money.FormatCode(c.GrossRevenue, c.CurrencyCode), days, money.FormatCode(pr, c.CurrencyCode), days, d))
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

	if p.Prism {
		if authed, aerr := r.SignalsAPI.Authed(ctx); aerr == nil {
			if ov, err := r.SignalsAPI.Prism.GetExperimentsOverview(authed, &signalsv1.GetExperimentsOverviewRequest{ChannelId: cid.String(), SinceDays: int32(days)}); err == nil && ov.GetDecided() > 0 {
				add("good", "Experiment decided",
					fmt.Sprintf("%d running experiment(s) reached a significant winner - consider rolling out.", ov.GetDecided()))
			}
		}
	}

	if p.Lens {
		if authed, aerr := r.SignalsAPI.Authed(ctx); aerr == nil {
			if ov, err := r.SignalsAPI.Lens.GetOverview(authed, &signalsv1.GetOverviewRequest{ChannelId: cid.String(), SinceDays: int32(days)}); err == nil && ov.GetVisitors() > 0 {
				add("info", "Traffic", fmt.Sprintf("%d visitors / %d page views in the last %dd.", ov.GetVisitors(), ov.GetViews(), days))
			}
		}
	}

	if p.Inventory {
		if low, err := r.InventorySvc.ListChannelLowStockLevels(ctx, cid, 1000); err == nil && len(low) > 0 {
			add("warn", "Low stock", fmt.Sprintf("%d item(s) at or below their low-stock threshold.", len(low)))
		}
	}

	return out
}
