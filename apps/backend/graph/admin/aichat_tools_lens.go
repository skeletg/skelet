package admin

// AI-chat tool definitions and handlers for Lens web analytics. The shared
// registry, dispatch and schema helpers live in aichat_tools.go.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/ai"
)

// lensSignalTools returns the Lens web-analytics tools (lens.view).
func lensSignalTools() []signalTool {
	return []signalTool{
		// ---- Lens / web (lens.view) ----
		{
			def: ai.Tool{
				Name:        "get_web_traffic",
				Description: "Lens web traffic: page views, unique visitors, top pages, devices, referrers, and web vitals for the channel over a window.",
				Schema:      windowSchema(nil),
			},
			perm: func(p digestPerms) bool { return p.lens },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, _ toolArgs) string {
				return reuseDigest(func(b *strings.Builder) { writeLensDigest(ctx, r, cid, since, b) }, "No web traffic in this window.")
			},
		},
		{
			def: ai.Tool{
				Name:        "get_top_pages",
				Description: "Most-viewed pages (views + visitors) for the channel over a window, with a custom limit.",
				Schema:      windowSchema(map[string]any{"limit": limitProp("Max pages to return (1-50). Default 10.", 50)}),
			},
			perm: func(p digestPerms) bool { return p.lens },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolTopPages(ctx, r, cid, since, a.Limit)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_bot_traffic",
				Description: "Human-vs-bot split and the top crawler/bot user-agents hitting the channel over a window. Optional 'bot' filters to one agent name.",
				Schema:      windowSchema(map[string]any{"bot": map[string]any{"type": "string", "description": "Optional bot/agent name to filter to."}}),
			},
			perm: func(p digestPerms) bool { return p.lens },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolBotTraffic(ctx, r, cid, since, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_site_search",
				Description: "On-site search analytics: total searches, distinct terms, top terms, and zero-result terms for the channel over a window.",
				Schema:      windowSchema(nil),
			},
			perm: func(p digestPerms) bool { return p.lens },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, _ toolArgs) string {
				return toolSiteSearch(ctx, r, cid, since)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_frustration",
				Description: "UX frustration signals: rage-click and dead-click pages, the worst rage elements, and the top JS errors for the channel over a window.",
				Schema:      windowSchema(map[string]any{"limit": limitProp("Max rows per list (1-20). Default 5.", 20)}),
			},
			perm: func(p digestPerms) bool { return p.lens },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolFrustration(ctx, r, cid, since, a.Limit)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_traffic_trend",
				Description: "Daily page views over time for the channel. Use for traffic trend questions; render as a line widget.",
				Schema:      windowSchema(nil),
			},
			perm: func(p digestPerms) bool { return p.lens },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, _ toolArgs) string {
				return toolTrafficTrend(ctx, r, cid, since)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_page_performance",
				Description: "Per-page Core Web Vitals (p75 LCP/CLS/INP/TTFB) for the channel - find the slowest pages.",
				Schema:      windowSchema(map[string]any{"limit": limitProp("Max pages to return (1-50). Default 10.", 50)}),
			},
			perm: func(p digestPerms) bool { return p.lens },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolPagePerformance(ctx, r, cid, since, a.Limit)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_page_stats",
				Description: "Drill into one page by its key (from get_top_pages): views, visitors, clicks, average scroll, top referrers and devices.",
				Schema: objSchema(map[string]any{
					"page_key": map[string]any{"type": "string", "description": "Page key/path from get_top_pages (e.g. /products)."},
				}, "page_key"),
			},
			perm: func(p digestPerms) bool { return p.lens },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolPageStats(ctx, r, cid, a)
			},
		},
	}
}

func toolTopPages(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, limit int) string {
	limit = clampLimit(limit, 10, 50)
	ov, err := r.LensSvc.Overview(ctx, cid, since)
	if err != nil {
		return "Could not load pages."
	}
	if len(ov.TopPages) == 0 {
		return "No page views in this window."
	}
	var b strings.Builder
	b.WriteString("Top pages by views:\n")
	for i, p := range ov.TopPages {
		if i >= limit {
			break
		}
		fmt.Fprintf(&b, "%d. %s - %d views, %d visitors\n", i+1, p.PageKey, p.Views, p.Visitors)
	}
	return b.String()
}

func toolBotTraffic(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, a toolArgs) string {
	bt, err := r.LensSvc.BotTraffic(ctx, cid, since, strings.TrimSpace(a.Bot))
	if err != nil {
		return "Could not load bot traffic."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Human views: %d, bot hits: %d (%d distinct bots across %d pages).\n", bt.HumanViews, bt.BotHits, bt.BotCount, bt.PageCount)
	if len(bt.Bots) > 0 {
		b.WriteString("Top bots:\n")
		for i, ba := range bt.Bots {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&b, "- %s (%s): %d hits over %d pages\n", ba.Name, ba.Category, ba.Hits, ba.Pages)
		}
	}
	return b.String()
}

func toolSiteSearch(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time) string {
	sa, err := r.LensSvc.SearchAnalytics(ctx, cid, since)
	if err != nil {
		return "Could not load site search."
	}
	if sa.TotalSearches == 0 {
		return "No on-site searches in this window."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d searches, %d distinct terms, %d returned zero results.\n", sa.TotalSearches, sa.DistinctTerms, sa.ZeroResultSearches)
	if len(sa.Top) > 0 {
		parts := make([]string, 0, len(sa.Top))
		for _, t := range sa.Top {
			parts = append(parts, fmt.Sprintf("%s (%d)", t.Term, t.Count))
		}
		fmt.Fprintf(&b, "Top terms: %s\n", strings.Join(parts, "; "))
	}
	if len(sa.ZeroResult) > 0 {
		parts := make([]string, 0, len(sa.ZeroResult))
		for _, t := range sa.ZeroResult {
			parts = append(parts, fmt.Sprintf("%s (%d)", t.Term, t.Count))
		}
		fmt.Fprintf(&b, "Zero-result terms: %s\n", strings.Join(parts, "; "))
	}
	return b.String()
}

func toolFrustration(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, limit int) string {
	limit = clampLimit(limit, 5, 20)
	fr, err := r.LensSvc.Frustration(ctx, cid, since, int32(limit))
	if err != nil {
		return "Could not load frustration signals."
	}
	var b strings.Builder
	if len(fr.RagePages) > 0 {
		parts := make([]string, 0, len(fr.RagePages))
		for _, p := range fr.RagePages {
			parts = append(parts, fmt.Sprintf("%s (%d)", p.PageKey, p.Count))
		}
		fmt.Fprintf(&b, "Rage-click pages: %s\n", strings.Join(parts, "; "))
	}
	if len(fr.DeadPages) > 0 {
		parts := make([]string, 0, len(fr.DeadPages))
		for _, p := range fr.DeadPages {
			parts = append(parts, fmt.Sprintf("%s (%d)", p.PageKey, p.Count))
		}
		fmt.Fprintf(&b, "Dead-click pages: %s\n", strings.Join(parts, "; "))
	}
	if len(fr.RageElements) > 0 {
		b.WriteString("Worst rage elements:\n")
		for _, e := range fr.RageElements {
			label := strings.TrimSpace(e.Text)
			if label == "" {
				label = e.Selector
			}
			fmt.Fprintf(&b, "- %s (%d)\n", label, e.Count)
		}
	}
	if errs, err := r.LensSvc.TopErrors(ctx, cid, since, int32(limit)); err == nil && len(errs) > 0 {
		b.WriteString("Top JS errors:\n")
		for _, e := range errs {
			fmt.Fprintf(&b, "- %s (%d hits over %d pages)\n", e.Message, e.Count, e.Pages)
		}
	}
	if b.Len() == 0 {
		return "No frustration signals in this window."
	}
	return b.String()
}

func toolTrafficTrend(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time) string {
	ov, err := r.LensSvc.Overview(ctx, cid, since)
	if err != nil {
		return "Could not load the traffic trend."
	}
	if len(ov.Daily) == 0 {
		return "No traffic in this window."
	}
	var b strings.Builder
	b.WriteString("Daily page views:\n")
	for _, d := range ov.Daily {
		fmt.Fprintf(&b, "%s: %d\n", d.Date, d.Views)
	}
	return b.String()
}

func fmtVital(name string, v *float64, prec int) string {
	if v == nil {
		return name + " -"
	}
	return fmt.Sprintf("%s %.*f", name, prec, *v)
}

func toolPagePerformance(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, limit int) string {
	limit = clampLimit(limit, 10, 50)
	pv, err := r.LensSvc.Performance(ctx, cid, since)
	if err != nil {
		return "Could not load page performance."
	}
	if len(pv) == 0 {
		return "No web-vitals samples in this window."
	}
	var b strings.Builder
	b.WriteString("Per-page web vitals (p75; LCP/INP/TTFB in ms, CLS unitless):\n")
	for i, p := range pv {
		if i >= limit {
			break
		}
		fmt.Fprintf(&b, "- %s: %s, %s, %s, %s (%d samples)\n",
			p.PageKey, fmtVital("LCP", p.LCP, 0), fmtVital("INP", p.INP, 0), fmtVital("TTFB", p.TTFB, 0), fmtVital("CLS", p.CLS, 2), p.Samples)
	}
	return b.String()
}

func toolPageStats(ctx context.Context, r *Resolver, cid uuid.UUID, a toolArgs) string {
	key := strings.TrimSpace(a.PageKey)
	if key == "" {
		return "Provide a 'page_key' (use get_top_pages)."
	}
	ps, err := r.LensSvc.PageStats(ctx, cid, key)
	if err != nil {
		return "Could not load page stats."
	}
	if ps.Views == 0 && ps.Visitors == 0 {
		return fmt.Sprintf("No data for page %q.", key)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Page %s: %d views, %d visitors, %d clicks, avg scroll %d%%.\n", key, ps.Views, ps.Visitors, ps.Clicks, ps.AvgScroll)
	if len(ps.Referrers) > 0 {
		parts := make([]string, 0, 5)
		for i, rf := range ps.Referrers {
			if i >= 5 {
				break
			}
			parts = append(parts, fmt.Sprintf("%s (%d)", rf.Label, rf.Views))
		}
		fmt.Fprintf(&b, "Top referrers: %s\n", strings.Join(parts, "; "))
	}
	if len(ps.Devices) > 0 {
		parts := make([]string, 0, len(ps.Devices))
		for _, d := range ps.Devices {
			parts = append(parts, fmt.Sprintf("%s=%d", d.Label, d.Views))
		}
		fmt.Fprintf(&b, "Devices: %s\n", strings.Join(parts, ", "))
	}
	return b.String()
}
