package agenttools

// Agent tool definitions and handlers for Lens web analytics. The shared
// registry, dispatch and schema helpers live in tools.go.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// lensTools returns the Lens web-analytics tools (lens.view).
func lensTools() []tool {
	return []tool{
		// ---- Lens / web (lens.view) ----
		{
			def: ai.Tool{
				Name:        "get_web_traffic",
				Description: "Lens web traffic: page views, unique visitors, top pages, devices, referrers, and web vitals for the channel over a window.",
				Schema:      windowSchema(nil),
			},
			perm: func(p Perms) bool { return p.Lens },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time, days int, _ toolArgs) string {
				return lensDigest.tool(ctx, deps, cid, since, days)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_top_pages",
				Description: "Most-viewed pages (views + visitors) for the channel over a window, with a custom limit.",
				Schema:      windowSchema(map[string]any{"limit": limitProp("Max pages to return (1-50). Default 10.", 50)}),
			},
			perm: func(p Perms) bool { return p.Lens },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolTopPages(ctx, deps, cid, days, a.Limit)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_bot_traffic",
				Description: "Human-vs-bot split and the top crawler/bot user-agents hitting the channel over a window. Optional 'bot' filters to one agent name.",
				Schema:      windowSchema(map[string]any{"bot": map[string]any{"type": "string", "description": "Optional bot/agent name to filter to."}}),
			},
			perm: func(p Perms) bool { return p.Lens },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolBotTraffic(ctx, deps, cid, days, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_site_search",
				Description: "On-site search analytics: total searches, distinct terms, top terms, and zero-result terms for the channel over a window.",
				Schema:      windowSchema(nil),
			},
			perm: func(p Perms) bool { return p.Lens },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, _ toolArgs) string {
				return toolSiteSearch(ctx, deps, cid, days)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_frustration",
				Description: "UX frustration signals: rage-click and dead-click pages, the worst rage elements, and the top JS errors for the channel over a window.",
				Schema:      windowSchema(map[string]any{"limit": limitProp("Max rows per list (1-20). Default 5.", 20)}),
			},
			perm: func(p Perms) bool { return p.Lens },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolFrustration(ctx, deps, cid, days, a.Limit)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_traffic_trend",
				Description: "Daily page views over time for the channel. Use for traffic trend questions.",
				Schema:      windowSchema(nil),
			},
			perm: func(p Perms) bool { return p.Lens },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, _ toolArgs) string {
				return toolTrafficTrend(ctx, deps, cid, days)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_page_performance",
				Description: "Per-page Core Web Vitals (p75 LCP/CLS/INP/TTFB) for the channel - find the slowest pages.",
				Schema:      windowSchema(map[string]any{"limit": limitProp("Max pages to return (1-50). Default 10.", 50)}),
			},
			perm: func(p Perms) bool { return p.Lens },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolPagePerformance(ctx, deps, cid, days, a.Limit)
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
			perm: func(p Perms) bool { return p.Lens },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolPageStats(ctx, deps, cid, a)
			},
		},
	}
}

func toolTopPages(ctx context.Context, deps *Deps, cid uuid.UUID, days int, limit int) string {
	limit = clampLimit(limit, 10, 50)
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "pages")
	}
	ov, err := deps.Signals.Lens.GetOverview(authed, &signalsv1.GetOverviewRequest{ChannelId: cid.String(), SinceDays: int32(days)})
	if err != nil {
		return signalsUnavailable(ctx, err, "pages")
	}
	if len(ov.GetTopPages()) == 0 {
		return "No page views in this window."
	}
	var b strings.Builder
	// Views only - the overview's rollup has no per-page uniques, and a zero here would read as one
	b.WriteString("Top pages by views:\n")
	for i, p := range ov.GetTopPages() {
		if i >= limit {
			break
		}
		fmt.Fprintf(&b, "%d. %s - %d views\n", i+1, untrusted(p.GetPageKey()), p.GetViews())
	}
	return b.String()
}

func toolBotTraffic(ctx context.Context, deps *Deps, cid uuid.UUID, days int, a toolArgs) string {
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "bot traffic")
	}
	bt, err := deps.Signals.Lens.GetBotTraffic(authed, &signalsv1.GetBotTrafficRequest{
		ChannelId: cid.String(), SinceDays: int32(days), Bot: strings.TrimSpace(a.Bot),
	})
	if err != nil {
		return signalsUnavailable(ctx, err, "bot traffic")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Human views: %d, bot hits: %d (%d distinct bots across %d pages).\n",
		bt.GetHumanViews(), bt.GetBotHits(), bt.GetBotCount(), bt.GetPageCount())
	if len(bt.GetBots()) > 0 {
		b.WriteString("Top bots:\n")
		for i, ba := range bt.GetBots() {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&b, "- %s (%s): %d hits over %d pages\n", ba.GetName(), ba.GetCategory(), ba.GetHits(), ba.GetPages())
		}
	}
	return b.String()
}

func toolSiteSearch(ctx context.Context, deps *Deps, cid uuid.UUID, days int) string {
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "site search")
	}
	sa, err := deps.Signals.Lens.GetSearchAnalytics(authed, &signalsv1.GetSearchAnalyticsRequest{ChannelId: cid.String(), SinceDays: int32(days)})
	if err != nil {
		return signalsUnavailable(ctx, err, "site search")
	}
	if sa.GetTotalSearches() == 0 {
		return "No on-site searches in this window."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d searches, %d distinct terms, %d returned zero results.\n",
		sa.GetTotalSearches(), sa.GetDistinctTerms(), sa.GetZeroResultSearches())
	terms := func(label string, list []*signalsv1.LensSearchTerm) {
		if len(list) == 0 {
			return
		}
		parts := make([]string, 0, len(list))
		for _, t := range list {
			parts = append(parts, fmt.Sprintf("%s (%d)", untrusted(t.GetTerm()), t.GetCount()))
		}
		fmt.Fprintf(&b, "%s: %s\n", label, strings.Join(parts, "; "))
	}
	terms("Top terms", sa.GetTop())
	terms("Zero-result terms", sa.GetZeroResult())
	return b.String()
}

func toolFrustration(ctx context.Context, deps *Deps, cid uuid.UUID, days int, limit int) string {
	limit = clampLimit(limit, 5, 20)
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "frustration signals")
	}
	fr, err := deps.Signals.Lens.GetFrustration(authed, &signalsv1.GetFrustrationRequest{ChannelId: cid.String(), SinceDays: int32(days)})
	if err != nil {
		return signalsUnavailable(ctx, err, "frustration signals")
	}
	var b strings.Builder
	pages := func(label string, list []*signalsv1.LensSignalPage) {
		if len(list) == 0 {
			return
		}
		parts := make([]string, 0, len(list))
		for _, p := range list {
			parts = append(parts, fmt.Sprintf("%s (%d)", untrusted(p.GetPageKey()), p.GetCount()))
		}
		fmt.Fprintf(&b, "%s: %s\n", label, strings.Join(parts, "; "))
	}
	pages("Rage-click pages", fr.GetRagePages())
	pages("Dead-click pages", fr.GetDeadPages())
	if len(fr.GetRageElements()) > 0 {
		b.WriteString("Worst rage elements:\n")
		for _, e := range fr.GetRageElements() {
			label := strings.TrimSpace(e.GetText())
			if label == "" {
				label = e.GetSelector()
			}
			fmt.Fprintf(&b, "- %s (%d)\n", untrusted(label), e.GetCount())
		}
	}
	if errs, err := deps.Signals.Lens.ListErrors(authed, &signalsv1.ListErrorsRequest{
		ChannelId: cid.String(), SinceDays: int32(days), Limit: int32(limit),
	}); err == nil && len(errs.GetErrors()) > 0 {
		b.WriteString("Top JS errors:\n")
		for _, e := range errs.GetErrors() {
			fmt.Fprintf(&b, "- %s (%d hits over %d pages)\n", untrusted(e.GetMessage()), e.GetCount(), e.GetPages())
		}
	}
	if b.Len() == 0 {
		return "No frustration signals in this window."
	}
	return b.String()
}

func toolTrafficTrend(ctx context.Context, deps *Deps, cid uuid.UUID, days int) string {
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "the traffic trend")
	}
	ov, err := deps.Signals.Lens.GetOverview(authed, &signalsv1.GetOverviewRequest{ChannelId: cid.String(), SinceDays: int32(days)})
	if err != nil {
		return signalsUnavailable(ctx, err, "the traffic trend")
	}
	if len(ov.GetDaily()) == 0 {
		return "No traffic in this window."
	}
	var b strings.Builder
	b.WriteString("Daily page views:\n")
	for _, d := range ov.GetDaily() {
		fmt.Fprintf(&b, "%s: %d\n", d.GetDate(), d.GetViews())
	}
	return b.String()
}

// A dash, not a zero - a zero would read as a real measurement
func fmtVital(name string, v *float64, prec int) string {
	if v == nil {
		return name + " -"
	}
	return fmt.Sprintf("%s %.*f", name, prec, *v)
}

func toolPagePerformance(ctx context.Context, deps *Deps, cid uuid.UUID, days int, limit int) string {
	limit = clampLimit(limit, 10, 50)
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "page performance")
	}
	res, err := deps.Signals.Lens.GetPerformance(authed, &signalsv1.GetPerformanceRequest{ChannelId: cid.String(), SinceDays: int32(days)})
	if err != nil {
		return signalsUnavailable(ctx, err, "page performance")
	}
	if len(res.GetPages()) == 0 {
		return "No web-vitals samples in this window."
	}
	var b strings.Builder
	b.WriteString("Per-page web vitals (p75; LCP/INP/TTFB in ms, CLS unitless):\n")
	for i, p := range res.GetPages() {
		if i >= limit {
			break
		}
		fmt.Fprintf(&b, "- %s: %s, %s, %s, %s (%d samples)\n",
			untrusted(p.GetPageKey()), fmtVital("LCP", p.Lcp, 0), fmtVital("INP", p.Inp, 0), fmtVital("TTFB", p.Ttfb, 0), fmtVital("CLS", p.Cls, 2), p.GetSamples())
	}
	return b.String()
}

func toolPageStats(ctx context.Context, deps *Deps, cid uuid.UUID, a toolArgs) string {
	key := strings.TrimSpace(a.PageKey)
	if key == "" {
		return "Provide a 'page_key' (use get_top_pages)."
	}
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "page stats")
	}
	ps, err := deps.Signals.Lens.GetPageStats(authed, &signalsv1.GetPageStatsRequest{ChannelId: cid.String(), PageKey: key})
	if err != nil {
		return signalsUnavailable(ctx, err, "page stats")
	}
	if ps.GetViews() == 0 && ps.GetVisitors() == 0 {
		return fmt.Sprintf("No data for page %q.", key)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Page %s: %d views, %d visitors, %d clicks, avg scroll %d%%.\n",
		key, ps.GetViews(), ps.GetVisitors(), ps.GetClicks(), ps.GetAvgScroll())
	if len(ps.GetReferrers()) > 0 {
		parts := make([]string, 0, 5)
		for i, rf := range ps.GetReferrers() {
			if i >= 5 {
				break
			}
			parts = append(parts, fmt.Sprintf("%s (%d)", untrusted(rf.GetLabel()), rf.GetViews()))
		}
		fmt.Fprintf(&b, "Top referrers: %s\n", strings.Join(parts, "; "))
	}
	if len(ps.GetDevices()) > 0 {
		parts := make([]string, 0, len(ps.GetDevices()))
		for _, d := range ps.GetDevices() {
			parts = append(parts, fmt.Sprintf("%s=%d", d.GetLabel(), d.GetViews()))
		}
		fmt.Fprintf(&b, "Devices: %s\n", strings.Join(parts, ", "))
	}
	return b.String()
}
