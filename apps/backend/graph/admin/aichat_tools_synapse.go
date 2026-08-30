package admin

// AI-chat tool definitions and handlers for Synapse customer analytics. The
// shared registry, dispatch and schema helpers live in aichat_tools.go.

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/internal/ai"
	"github.com/skeletg/skelet/internal/analytics"
)

// synapseSignalTools returns the Synapse customer-behaviour tools (customers.view).
func synapseSignalTools() []signalTool {
	return []signalTool{
		// ---- Synapse / behaviour (customers.view) ----
		{
			def: ai.Tool{
				Name:        "get_customer_behaviour",
				Description: "Synapse behavioural analytics: events, conversions, identified vs anonymous visitors, tracked revenue, top events, and segments for the channel over a window.",
				Schema:      windowSchema(nil),
			},
			perm: func(p digestPerms) bool { return p.synapse },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, _ toolArgs) string {
				return reuseDigest(func(b *strings.Builder) { writeSynapseDigest(ctx, r, cid, since, b) }, "No customer-behaviour data in this window.")
			},
		},
		{
			def: ai.Tool{
				Name:        "get_event_trend",
				Description: "An event's volume over time (a time-series), optionally broken down by an event property. Use for 'how has <event> trended?'. Render as a line widget. Source event names from the top events in the snapshot / get_customer_behaviour.",
				Schema: windowSchema(map[string]any{
					"event":     map[string]any{"type": "string", "description": "Event name (e.g. page_view, $experiment, add_to_cart)."},
					"breakdown": map[string]any{"type": "string", "description": "Optional event-property key to split into series (e.g. device)."},
					"measure":   enumProp("count = total events; unique = distinct people. Default count.", "count", "unique"),
					"unit":      enumProp("Time bucket. Default day.", "day", "week"),
				}, "event"),
			},
			perm: func(p digestPerms) bool { return p.synapse },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolEventTrend(ctx, r, cid, since, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_funnel",
				Description: "Step-by-step conversion funnel: how many distinct people reached each event, in order. Provide 2+ event names in 'steps'.",
				Schema: objSchema(map[string]any{
					"steps":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Ordered list of event names (2+)."},
					"window_days": windowProp(),
				}, "steps"),
			},
			perm: func(p digestPerms) bool { return p.synapse },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolFunnel(ctx, r, cid, since, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_retention",
				Description: "Cohort retention: of people who did 'start_event', what % did 'return_event' in each later period. Render the curve as a line widget.",
				Schema: objSchema(map[string]any{
					"start_event":  map[string]any{"type": "string", "description": "The event that forms the cohort."},
					"return_event": map[string]any{"type": "string", "description": "The event whose return is measured."},
					"unit":         enumProp("Period length. Default day.", "day", "week"),
					"periods":      limitProp("Number of periods to track (1-12). Default 8.", 12),
					"window_days":  windowProp(),
				}, "start_event", "return_event"),
			},
			perm: func(p digestPerms) bool { return p.synapse },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolRetention(ctx, r, cid, since, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_segments",
				Description: "List the store's customer segments with their id and member count. Use this first to get a segment_id for get_segment_detail.",
				Schema:      objSchema(map[string]any{}),
			},
			perm: func(p digestPerms) bool { return p.synapse },
			run: func(ctx context.Context, r *Resolver, _ uuid.UUID, _ time.Time, _ int, _ toolArgs) string {
				return toolListSegments(ctx, r)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_segment_detail",
				Description: "Deep-dive a specific segment (by segment_id from list_segments): member orders/revenue, lifecycle mix, RFM distribution, top attribute affinities, and member-count growth.",
				Schema: objSchema(map[string]any{
					"segment_id":  map[string]any{"type": "string", "description": "Segment UUID from list_segments."},
					"window_days": windowProp(),
				}, "segment_id"),
			},
			perm: func(p digestPerms) bool { return p.synapse },
			run: func(ctx context.Context, r *Resolver, _ uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolSegmentDetail(ctx, r, days, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_stickiness",
				Description: "Engagement stickiness for an event: WAU, MAU, return rate, and the daily-active series. Use 'event' from the snapshot's top events.",
				Schema: windowSchema(map[string]any{
					"event": map[string]any{"type": "string", "description": "Event name (e.g. page_view, add_to_cart)."},
				}, "event"),
			},
			perm: func(p digestPerms) bool { return p.synapse },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, days int, a toolArgs) string {
				return toolStickiness(ctx, r, cid, since, days, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_user_flow",
				Description: "The top paths people take AFTER a focus event - 'what do users do after add_to_cart?'. Returns cohort size, how many continued, and the most common next-step sequences.",
				Schema: objSchema(map[string]any{
					"focus_event": map[string]any{"type": "string", "description": "The event to start the flow from."},
					"window_days": windowProp(),
				}, "focus_event"),
			},
			perm: func(p digestPerms) bool { return p.synapse },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolUserFlow(ctx, r, cid, since, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_affinity_attributes",
				Description: "List the tracked product attributes (slug + name) you can break down customer affinities by. Use a slug with get_affinity_breakdown.",
				Schema:      objSchema(map[string]any{}),
			},
			perm: func(p digestPerms) bool { return p.synapse },
			run: func(ctx context.Context, r *Resolver, _ uuid.UUID, _ time.Time, _ int, _ toolArgs) string {
				return toolListAffinityAttrs(ctx, r)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_affinity_breakdown",
				Description: "Top values of a tracked attribute by distinct-customer reach (e.g. most-wanted brands/sizes). Use an attribute slug from list_affinity_attributes.",
				Schema: windowSchema(map[string]any{
					"attribute": map[string]any{"type": "string", "description": "Attribute slug from list_affinity_attributes."},
					"limit":     limitProp("Max values to return (1-50). Default 15.", 50),
				}, "attribute"),
			},
			perm: func(p digestPerms) bool { return p.synapse },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolAffinityBreakdown(ctx, r, cid, since, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_active_customers",
				Description: "Most-active customers (id, name, events) and anonymous visitors (anon_id) for the channel. Use an id with get_customer_profile / get_visitor_profile.",
				Schema:      windowSchema(nil),
			},
			perm: func(p digestPerms) bool { return p.synapse },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, _ toolArgs) string {
				return toolListActiveCustomers(ctx, r, cid, since)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_customer_profile",
				Description: "One customer's profile (by customer_id from list_active_customers): first/last seen, events, conversions, revenue, top events, viewed products, and device/referrer/country context.",
				Schema: windowSchema(map[string]any{
					"customer_id": map[string]any{"type": "string", "description": "Customer UUID from list_active_customers."},
				}, "customer_id"),
			},
			perm: func(p digestPerms) bool { return p.synapse },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolCustomerProfile(ctx, r, cid, since, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_visitor_profile",
				Description: "One anonymous visitor's profile (by anon_id from list_active_customers): activity, viewed products, and device/referrer context (no identity).",
				Schema: windowSchema(map[string]any{
					"anon_id": map[string]any{"type": "string", "description": "Anonymous visitor id from list_active_customers."},
				}, "anon_id"),
			},
			perm: func(p digestPerms) bool { return p.synapse },
			run: func(ctx context.Context, r *Resolver, _ uuid.UUID, since time.Time, _ int, a toolArgs) string {
				return toolVisitorProfile(ctx, r, since, a)
			},
		},
	}
}

func toolEventTrend(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, a toolArgs) string {
	event := strings.TrimSpace(a.Event)
	if event == "" {
		return "Specify an 'event' name (see the top events in the snapshot)."
	}
	measure := "count"
	if a.Measure == "unique" {
		measure = "unique"
	}
	unit := unitOf(a.Unit)
	breakdown := strings.TrimSpace(a.Breakdown)
	ins, err := r.AnalyticsSvc.Insights(ctx, cid, event, breakdown, measure, unit, since, time.Now())
	if err != nil {
		return "Could not load the event trend."
	}
	if len(ins.Points) == 0 {
		return fmt.Sprintf("No %q events in this window.", event)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q %s by %s", event, measure, unit)
	if breakdown != "" {
		fmt.Fprintf(&b, " (broken down by %q)", breakdown)
	}
	b.WriteString(":\n")
	for _, p := range ins.Points {
		if breakdown != "" {
			fmt.Fprintf(&b, "%s\t%s: %d\n", p.Bucket.Format("2006-01-02"), p.Key, p.Value)
		} else {
			fmt.Fprintf(&b, "%s: %d\n", p.Bucket.Format("2006-01-02"), p.Value)
		}
	}
	return b.String()
}

func toolFunnel(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, a toolArgs) string {
	steps := cleanSteps(a.Steps)
	if len(steps) < 2 {
		return "Provide at least 2 ordered event names in 'steps'."
	}
	fs, err := r.AnalyticsSvc.Funnel(ctx, cid, steps, since)
	if err != nil {
		return "Could not compute the funnel."
	}
	if len(fs) == 0 {
		return "No funnel data for those steps."
	}
	first := fs[0].Count
	var b strings.Builder
	b.WriteString("Funnel (distinct people reaching each step):\n")
	for i, s := range fs {
		pct := 0.0
		if first > 0 {
			pct = float64(s.Count) / float64(first) * 100
		}
		fmt.Fprintf(&b, "%d. %s - %d (%.1f%% of step 1)\n", i+1, s.EventName, s.Count, pct)
	}
	return b.String()
}

func toolRetention(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, a toolArgs) string {
	start := strings.TrimSpace(a.StartEvent)
	ret := strings.TrimSpace(a.ReturnEvent)
	if start == "" || ret == "" {
		return "Provide both 'start_event' and 'return_event'."
	}
	unitDur := 24 * time.Hour
	unitLabel := "day"
	if a.Unit == "week" {
		unitDur = 7 * 24 * time.Hour
		unitLabel = "week"
	}
	periods := clampLimit(a.Periods, 8, 12)
	res, err := r.AnalyticsSvc.Retention(ctx, cid, start, ret, since, time.Now(), periods, unitDur)
	if err != nil {
		return "Could not compute retention."
	}
	if res.Cohort == 0 {
		return fmt.Sprintf("No one did %q in this window.", start)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Retention: %d people did %q; %% who later did %q each %s after:\n", res.Cohort, start, ret, unitLabel)
	for _, bk := range res.Buckets {
		pct := 0.0
		if bk.Eligible > 0 {
			pct = float64(bk.Count) / float64(bk.Eligible) * 100
		}
		fmt.Fprintf(&b, "%s %d: %.1f%% (%d of %d)\n", unitLabel, bk.Period, pct, bk.Count, bk.Eligible)
	}
	return b.String()
}

func toolListSegments(ctx context.Context, r *Resolver) string {
	segs, _, err := r.SegmentSvc.List(ctx, 50, 0)
	if err != nil {
		return "Could not list segments."
	}
	if len(segs) == 0 {
		return "No segments are defined."
	}
	ids := make([]uuid.UUID, len(segs))
	for i, s := range segs {
		ids[i] = s.ID
	}
	counts, _ := r.SegmentSvc.MemberCounts(ctx, ids)
	var b strings.Builder
	b.WriteString("Segments (id - name - members):\n")
	for _, s := range segs {
		fmt.Fprintf(&b, "%s - %s - %d members\n", s.ID, s.Name, counts[s.ID])
	}
	return b.String()
}

func toolSegmentDetail(ctx context.Context, r *Resolver, days int, a toolArgs) string {
	sid, err := uuid.Parse(strings.TrimSpace(a.SegmentID))
	if err != nil {
		return "Provide a valid 'segment_id' (use list_segments)."
	}
	// Affinities need the segment's channel (the spine scan is channel-bounded);
	// behavioral segments carry it in their stored definition.
	var segChannel uuid.UUID
	if seg, err := r.SegmentSvc.Get(ctx, sid); err == nil {
		if def, ok := parseSegmentDef(seg.RefreshSQL); ok {
			segChannel, _ = uuid.Parse(def.ChannelID)
		}
	}
	var b strings.Builder
	if orders, rev, err := r.AnalyticsSvc.SegmentMemberStats(ctx, sid); err == nil {
		fmt.Fprintf(&b, "Members' orders: %d, revenue: %d cents.\n", orders, rev)
	}
	if mix, err := r.AnalyticsSvc.SegmentLifecycleMix(ctx, sid); err == nil && len(mix) > 0 {
		parts := make([]string, 0, len(mix))
		for _, m := range mix {
			parts = append(parts, fmt.Sprintf("%s=%d", m.Stage, m.Count))
		}
		fmt.Fprintf(&b, "Lifecycle mix: %s\n", strings.Join(parts, ", "))
	}
	if rfm, err := r.AnalyticsSvc.SegmentRfmDistribution(ctx, sid); err == nil && len(rfm) > 0 {
		parts := make([]string, 0, len(rfm))
		for _, x := range rfm {
			parts = append(parts, fmt.Sprintf("%s score %d: %d", x.Dimension, x.Score, x.Count))
		}
		fmt.Fprintf(&b, "RFM distribution: %s\n", strings.Join(parts, "; "))
	}
	if aff, err := r.AnalyticsSvc.SegmentTopAffinities(ctx, sid, segChannel, time.Now().AddDate(0, 0, -90), 8); err == nil && len(aff) > 0 {
		parts := make([]string, 0, len(aff))
		for _, x := range aff {
			parts = append(parts, fmt.Sprintf("%s=%s (%d)", x.Attr, x.Value, x.Members))
		}
		fmt.Fprintf(&b, "Top affinities: %s\n", strings.Join(parts, "; "))
	}
	if g, err := r.AnalyticsSvc.SegmentGrowth(ctx, sid, days); err == nil && len(g) > 0 {
		b.WriteString("Member-count growth:\n")
		for _, pt := range g {
			fmt.Fprintf(&b, "%s: %d\n", pt.Day.Format("2006-01-02"), pt.Count)
		}
	}
	if b.Len() == 0 {
		return "No detail for that segment (check the id)."
	}
	return b.String()
}

// valueCountList joins a top-N of []analytics.ValueCount into "value (count)".
func valueCountList(vals []analytics.ValueCount, n int) string {
	parts := make([]string, 0, n)
	for i, v := range vals {
		if i >= n {
			break
		}
		parts = append(parts, fmt.Sprintf("%s (%d)", v.Value, v.Count))
	}
	return strings.Join(parts, "; ")
}

func toolStickiness(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, days int, a toolArgs) string {
	event := strings.TrimSpace(a.Event)
	if event == "" {
		return "Specify an 'event' (see the snapshot's top events)."
	}
	st, err := r.AnalyticsSvc.Stickiness(ctx, cid, event, since, time.Now(), days)
	if err != nil {
		return "Could not load stickiness."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q engagement: WAU %d, MAU %d. Active now %d, prior window %d, returning %d (return rate %.1f%%).\n",
		event, st.WAU, st.MAU, st.ActiveNow, st.PriorActive, st.Returning, st.ReturnRate)
	if len(st.Days) > 0 {
		b.WriteString("Daily active:\n")
		for _, d := range st.Days {
			fmt.Fprintf(&b, "%s: %d\n", d.Day.Format("2006-01-02"), d.Count)
		}
	}
	return b.String()
}

func toolUserFlow(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, a toolArgs) string {
	focus := strings.TrimSpace(a.FocusEvent)
	if focus == "" {
		return "Specify a 'focus_event'."
	}
	fr, err := r.AnalyticsSvc.Flow(ctx, cid, focus, since, 4, 6)
	if err != nil {
		return "Could not compute the flow."
	}
	if fr.Cohort == 0 {
		return fmt.Sprintf("No one did %q in this window.", focus)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "After %q: %d people, %d continued (avg %.1f next steps).\n", focus, fr.Cohort, fr.Continued, fr.AvgSteps)
	if len(fr.TopPaths) > 0 {
		b.WriteString("Top paths:\n")
		for _, p := range fr.TopPaths {
			fmt.Fprintf(&b, "- %s (%d)\n", strings.Join(p.Events, " → "), p.Count)
		}
	}
	return b.String()
}

func toolListAffinityAttrs(ctx context.Context, r *Resolver) string {
	attrs := synapseAffinityAttrs(ctx, r)
	if len(attrs) == 0 {
		return "No tracked affinity attributes."
	}
	var b strings.Builder
	b.WriteString("Tracked attributes (slug - name):\n")
	for _, a := range attrs {
		fmt.Fprintf(&b, "%s - %s\n", a.Slug, a.Name)
	}
	return b.String()
}

func toolAffinityBreakdown(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, a toolArgs) string {
	slug := strings.TrimSpace(a.Attribute)
	if slug == "" {
		return "Provide an 'attribute' slug (use list_affinity_attributes)."
	}
	limit := clampLimit(a.Limit, 15, 50)
	vals, err := r.AnalyticsSvc.AffinityValueCounts(ctx, cid, slug, since, limit)
	if err != nil {
		return "Could not load the affinity breakdown."
	}
	if len(vals) == 0 {
		return fmt.Sprintf("No affinity data for %q (check the slug via list_affinity_attributes).", slug)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Top %q values by distinct-customer reach:\n", slug)
	for i, v := range vals {
		fmt.Fprintf(&b, "%d. %s - %d customers\n", i+1, v.Value, v.Actors)
	}
	return b.String()
}

func toolListActiveCustomers(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time) string {
	ov, err := r.AnalyticsSvc.SynapseOverview(ctx, cid, since)
	if err != nil {
		return "Could not load active customers."
	}
	var b strings.Builder
	if len(ov.ActiveCustomers) > 0 {
		ids := make([]uuid.UUID, 0, len(ov.ActiveCustomers))
		for _, c := range ov.ActiveCustomers {
			ids = append(ids, c.CustomerID)
		}
		names, _ := r.CustomerSvc.GetByIDs(ctx, ids)
		b.WriteString("Most active customers (id - name - events - last seen):\n")
		for _, c := range ov.ActiveCustomers {
			name := "(unknown)"
			if cust, ok := names[c.CustomerID]; ok {
				if n := strings.TrimSpace(cust.FirstName + " " + cust.LastName); n != "" {
					name = n
				} else if cust.Email != "" {
					name = cust.Email
				}
			}
			fmt.Fprintf(&b, "%s - %s - %d events - %s\n", c.CustomerID, name, c.Events, c.LastSeen.Format("2006-01-02"))
		}
	}
	if len(ov.ActiveVisitors) > 0 {
		b.WriteString("Most active anonymous visitors (anon_id - events - last page):\n")
		for i, v := range ov.ActiveVisitors {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&b, "%s - %d events - %s\n", v.AnonID, v.Events, v.LastPage)
		}
	}
	if b.Len() == 0 {
		return "No active customers or visitors in this window."
	}
	return b.String()
}

func toolCustomerProfile(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, a toolArgs) string {
	custID, err := uuid.Parse(strings.TrimSpace(a.CustomerID))
	if err != nil {
		return "Provide a valid 'customer_id' (use list_active_customers)."
	}
	// CustomerProfile is NOT channel-scoped - guard against reading a customer
	// outside the pinned channel.
	ids, err := r.AnalyticsSvc.ChannelCustomerIDs(ctx, cid)
	if err != nil {
		return "Could not verify the customer."
	}
	found := slices.Contains(ids, custID)
	if !found {
		return "That customer isn't active in this channel."
	}
	p, err := r.AnalyticsSvc.CustomerProfile(ctx, custID, since)
	if err != nil {
		return "Could not load the customer profile."
	}
	label := "Customer " + custID.String()
	if c, e := r.CustomerSvc.GetByID(ctx, custID); e == nil {
		if n := strings.TrimSpace(c.FirstName + " " + c.LastName); n != "" {
			label = n
		} else if c.Email != "" {
			label = c.Email
		}
	}
	return formatProfile(label, p)
}

func toolVisitorProfile(ctx context.Context, r *Resolver, since time.Time, a toolArgs) string {
	anon := strings.TrimSpace(a.AnonID)
	if anon == "" {
		return "Provide an 'anon_id' (use list_active_customers)."
	}
	p, err := r.AnalyticsSvc.VisitorProfile(ctx, anon, since)
	if err != nil {
		return "Could not load the visitor profile."
	}
	return formatProfile("Anonymous visitor "+anon, p)
}

// formatProfile renders a bounded summary of a customer/visitor profile. It deliberately OMITS raw IPs and the full
// event timeline to limit the PII that reaches the model provider
func formatProfile(label string, p analytics.Profile) string {
	var b strings.Builder
	b.WriteString(label + "\n")
	if p.FirstSeen != nil {
		fmt.Fprintf(&b, "First seen %s. ", p.FirstSeen.Format("2006-01-02"))
	}
	if p.LastSeen != nil {
		fmt.Fprintf(&b, "Last seen %s.", p.LastSeen.Format("2006-01-02"))
	}
	fmt.Fprintf(&b, "\n%d events, %d page views, %d conversions, %d sessions.\n", p.TotalEvents, p.PageViews, p.Conversions, p.Sessions)
	for _, rev := range p.Revenue {
		fmt.Fprintf(&b, "Revenue: %s across %d orders.\n", fmtMoney(rev.Cents, rev.Currency), rev.Orders)
	}
	if len(p.Counts) > 0 {
		parts := make([]string, 0, 8)
		for i, e := range p.Counts {
			if i >= 8 {
				break
			}
			parts = append(parts, fmt.Sprintf("%s=%d", e.Name, e.Count))
		}
		fmt.Fprintf(&b, "Top events: %s\n", strings.Join(parts, ", "))
	}
	if len(p.TopProducts) > 0 {
		parts := make([]string, 0, 5)
		for i, pr := range p.TopProducts {
			if i >= 5 {
				break
			}
			parts = append(parts, fmt.Sprintf("%s (%d views)", pr.Name, pr.Views))
		}
		fmt.Fprintf(&b, "Viewed products: %s\n", strings.Join(parts, "; "))
	}
	if s := valueCountList(p.Devices, 3); s != "" {
		fmt.Fprintf(&b, "Devices: %s\n", s)
	}
	if s := valueCountList(p.Referrers, 3); s != "" {
		fmt.Fprintf(&b, "Referrers: %s\n", s)
	}
	if s := valueCountList(p.Countries, 3); s != "" {
		fmt.Fprintf(&b, "Countries: %s\n", s)
	}
	if s := valueCountList(p.Sources, 3); s != "" {
		fmt.Fprintf(&b, "Sources: %s\n", s)
	}
	if s := valueCountList(p.Searches, 5); s != "" {
		fmt.Fprintf(&b, "Searches: %s\n", s)
	}
	return b.String()
}
