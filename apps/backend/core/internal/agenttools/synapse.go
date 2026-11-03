package agenttools

// Agent tool definitions and handlers for Synapse customer analytics. The
// shared registry, dispatch and schema helpers live in tools.go.

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// synapseTools returns the Synapse customer-behaviour tools (customers.view).
func synapseTools() []tool {
	return []tool{
		// ---- Synapse / behaviour (customers.view) ----
		{
			def: ai.Tool{
				Name:        "get_customer_behaviour",
				Description: "Synapse behavioural analytics: events, conversions, identified vs anonymous visitors, tracked revenue, top events, and segments for the channel over a window.",
				Schema:      windowSchema(nil),
			},
			perm: func(p Perms) bool { return p.Synapse },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time, days int, _ toolArgs) string {
				return synapseDigest.tool(ctx, deps, cid, since, days)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_event_trend",
				Description: "An event's volume over time (a time-series), optionally broken down by an event property. Use for 'how has <event> trended?'. Source event names from the top events in the snapshot / get_customer_behaviour.",
				Schema: windowSchema(map[string]any{
					"event":     map[string]any{"type": "string", "description": "Event name (e.g. page_view, $experiment, add_to_cart)."},
					"breakdown": map[string]any{"type": "string", "description": "Optional event-property key to split into series (e.g. device)."},
					"measure":   enumProp("count = total events; unique = distinct people. Default count.", "count", "unique"),
					"unit":      enumProp("Time bucket. Default day.", "day", "week"),
				}, "event"),
			},
			perm: func(p Perms) bool { return p.Synapse },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolEventTrend(ctx, deps, cid, days, a)
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
			perm: func(p Perms) bool { return p.Synapse },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolFunnel(ctx, deps, cid, days, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_retention",
				Description: "Cohort retention: of people who did 'start_event', what % did 'return_event' in each later period.",
				Schema: objSchema(map[string]any{
					"start_event":  map[string]any{"type": "string", "description": "The event that forms the cohort."},
					"return_event": map[string]any{"type": "string", "description": "The event whose return is measured."},
					"unit":         enumProp("Period length. Default day.", "day", "week"),
					"periods":      limitProp("Number of periods to track (1-12). Default 8.", 12),
					"window_days":  windowProp(),
				}, "start_event", "return_event"),
			},
			perm: func(p Perms) bool { return p.Synapse },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolRetention(ctx, deps, cid, days, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_segments",
				Description: "List the store's customer segments with their id and member count. Use this first to get a segment_id for get_segment_detail.",
				Schema:      objSchema(map[string]any{}),
			},
			perm: func(p Perms) bool { return p.Synapse },
			run: func(ctx context.Context, deps *Deps, _ uuid.UUID, _ time.Time, _ int, _ toolArgs) string {
				return toolListSegments(ctx, deps)
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
			perm: func(p Perms) bool { return p.Synapse },
			run: func(ctx context.Context, deps *Deps, _ uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolSegmentDetail(ctx, deps, days, a)
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
			perm: func(p Perms) bool { return p.Synapse },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolStickiness(ctx, deps, cid, days, a)
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
			perm: func(p Perms) bool { return p.Synapse },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolUserFlow(ctx, deps, cid, days, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_affinity_attributes",
				Description: "List the tracked product attributes (slug + name) you can break down customer affinities by. Use a slug with get_affinity_breakdown.",
				Schema:      objSchema(map[string]any{}),
			},
			perm: func(p Perms) bool { return p.Synapse },
			run: func(ctx context.Context, deps *Deps, _ uuid.UUID, _ time.Time, _ int, _ toolArgs) string {
				return toolListAffinityAttrs(ctx, deps)
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
			perm: func(p Perms) bool { return p.Synapse },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolAffinityBreakdown(ctx, deps, cid, days, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_active_customers",
				Description: "Most-active customers (id, name, events) and anonymous visitors (anon_id) for the channel. Use an id with get_customer_profile / get_visitor_profile.",
				Schema:      windowSchema(nil),
			},
			perm: func(p Perms) bool { return p.Synapse },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, _ toolArgs) string {
				return toolListActiveCustomers(ctx, deps, cid, days)
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
			perm: func(p Perms) bool { return p.Synapse },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolCustomerProfile(ctx, deps, cid, days, a)
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
			perm: func(p Perms) bool { return p.Synapse },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, a toolArgs) string {
				return toolVisitorProfile(ctx, deps, cid, days, a)
			},
		},
	}
}

func toolEventTrend(ctx context.Context, deps *Deps, cid uuid.UUID, days int, a toolArgs) string {
	event := strings.TrimSpace(a.Event)
	if event == "" {
		return "Specify an 'event' name (see the top events in the snapshot)."
	}
	measure := "count"
	if a.Measure == "unique" {
		measure = "unique"
	}
	unit := dayOrWeek(a.Unit)
	breakdown := strings.TrimSpace(a.Breakdown)
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "the event trend")
	}
	ins, err := deps.Signals.Synapse.GetInsights(authed, &signalsv1.GetInsightsRequest{
		ChannelId: cid.String(), Event: event, Breakdown: breakdown,
		Measure: measure, Unit: unit, SinceDays: int32(days),
	})
	if err != nil {
		return signalsUnavailable(ctx, err, "the event trend")
	}
	if len(ins.GetPoints()) == 0 {
		return fmt.Sprintf("No %q events in this window.", event)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q %s by %s", event, measure, unit)
	if breakdown != "" {
		fmt.Fprintf(&b, " (broken down by %q)", breakdown)
	}
	b.WriteString(":\n")
	for _, p := range ins.GetPoints() {
		if breakdown != "" {
			fmt.Fprintf(&b, "%s\t%s: %d\n", p.GetBucket().AsTime().Format("2006-01-02"), untrusted(p.GetKey()), p.GetValue())
		} else {
			fmt.Fprintf(&b, "%s: %d\n", p.GetBucket().AsTime().Format("2006-01-02"), p.GetValue())
		}
	}
	return b.String()
}

func toolFunnel(ctx context.Context, deps *Deps, cid uuid.UUID, days int, a toolArgs) string {
	steps := cleanSteps(a.Steps)
	if len(steps) < 2 {
		return "Provide at least 2 ordered event names in 'steps'."
	}
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "the funnel")
	}
	res, err := deps.Signals.Synapse.GetFunnel(authed, &signalsv1.GetFunnelRequest{
		ChannelId: cid.String(), Steps: steps, SinceDays: int32(days),
	})
	if err != nil {
		return signalsUnavailable(ctx, err, "the funnel")
	}
	fs := res.GetSteps()
	if len(fs) == 0 {
		return "No funnel data for those steps."
	}
	first := fs[0].GetCount()
	var b strings.Builder
	b.WriteString("Funnel (distinct people reaching each step):\n")
	for i, s := range fs {
		pct := 0.0
		if first > 0 {
			pct = float64(s.GetCount()) / float64(first) * 100
		}
		fmt.Fprintf(&b, "%d. %s - %d (%.1f%% of step 1)\n", i+1, s.GetEventName(), s.GetCount(), pct)
	}
	return b.String()
}

func toolRetention(ctx context.Context, deps *Deps, cid uuid.UUID, days int, a toolArgs) string {
	start := strings.TrimSpace(a.StartEvent)
	ret := strings.TrimSpace(a.ReturnEvent)
	if start == "" || ret == "" {
		return "Provide both 'start_event' and 'return_event'."
	}
	unit := dayOrWeek(a.Unit)
	periods := clampLimit(a.Periods, 8, 12)
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "retention")
	}
	res, err := deps.Signals.Synapse.GetRetention(authed, &signalsv1.GetRetentionRequest{
		ChannelId: cid.String(), StartEvent: start, ReturnEvent: ret,
		SinceDays: int32(days), Periods: int32(periods), Unit: unit,
	})
	if err != nil {
		return signalsUnavailable(ctx, err, "retention")
	}
	if res.GetCohort() == 0 {
		return fmt.Sprintf("No one did %q in this window.", start)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Retention: %d people did %q; %% who later did %q each %s after:\n", res.GetCohort(), start, ret, unit)
	for _, bk := range res.GetBuckets() {
		pct := 0.0
		if bk.GetEligible() > 0 {
			pct = float64(bk.GetCount()) / float64(bk.GetEligible()) * 100
		}
		fmt.Fprintf(&b, "%s %d: %.1f%% (%d of %d)\n", unit, bk.GetPeriod(), pct, bk.GetCount(), bk.GetEligible())
	}
	return b.String()
}

func toolListSegments(ctx context.Context, deps *Deps) string {
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "segments")
	}
	res, err := deps.Signals.Segments.ListSegments(authed, &signalsv1.ListSegmentsRequest{PageSize: 50})
	if err != nil {
		return signalsUnavailable(ctx, err, "segments")
	}
	if len(res.GetSegments()) == 0 {
		return "No segments are defined."
	}
	var b strings.Builder
	b.WriteString("Segments (id - name - members):\n")
	for _, s := range res.GetSegments() {
		fmt.Fprintf(&b, "%s - %s - %d members\n", s.GetId(), s.GetName(), s.GetMemberCount())
	}
	return b.String()
}

func toolSegmentDetail(ctx context.Context, deps *Deps, days int, a toolArgs) string {
	sid, err := uuid.Parse(strings.TrimSpace(a.SegmentID))
	if err != nil {
		return "Provide a valid 'segment_id' (use list_segments)."
	}
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "the segment")
	}
	d, err := deps.Signals.Synapse.GetBehavioralSegment(authed, &signalsv1.GetBehavioralSegmentRequest{Id: sid.String()})
	if err != nil {
		if status.Code(err) == codes.FailedPrecondition {
			return "That segment has no behavioural definition, so there is no detail to show."
		}
		return signalsUnavailable(ctx, err, "the segment")
	}
	currency := signalsclient.ChannelCurrency(ctx, deps.Channel, d.GetSegment().GetChannelId())
	var b strings.Builder
	fmt.Fprintf(&b, "Segment %q: %d members, %d orders, revenue %s (AOV %s).\n",
		d.GetSegment().GetName(), d.GetSegment().GetMemberCount(), d.GetOrders(),
		money.FormatCode(d.GetRevenueCents(), currency), money.FormatCode(d.GetAovCents(), currency))
	if mix := d.GetLifecycleMix(); len(mix) > 0 {
		parts := make([]string, 0, len(mix))
		for _, m := range mix {
			parts = append(parts, fmt.Sprintf("%s=%d", m.GetStage(), m.GetCount()))
		}
		fmt.Fprintf(&b, "Lifecycle mix: %s\n", strings.Join(parts, ", "))
	}
	if rfm := d.GetRfmDistribution(); len(rfm) > 0 {
		parts := make([]string, 0, len(rfm))
		for _, x := range rfm {
			parts = append(parts, fmt.Sprintf("%s score %d: %d", x.GetDimension(), x.GetScore(), x.GetCount()))
		}
		fmt.Fprintf(&b, "RFM distribution: %s\n", strings.Join(parts, "; "))
	}
	if aff := d.GetTopAffinities(); len(aff) > 0 {
		parts := make([]string, 0, len(aff))
		for _, x := range aff {
			parts = append(parts, fmt.Sprintf("%s=%s (%d)", x.GetAttr(), x.GetValue(), x.GetMembers()))
		}
		fmt.Fprintf(&b, "Top affinities: %s\n", strings.Join(parts, "; "))
	}
	// The window the tool was asked for, out of the 90 days the service returns
	if g := d.GetGrowth(); len(g) > 0 {
		if len(g) > days {
			g = g[len(g)-days:]
		}
		b.WriteString("Member-count growth:\n")
		for _, pt := range g {
			fmt.Fprintf(&b, "%s: %d\n", pt.GetDay(), pt.GetCount())
		}
	}
	return b.String()
}

// valueCountList joins a top-N of a profile's context values into "value (count)".
func valueCountList(vals []*signalsv1.ValueCount, n int) string {
	parts := make([]string, 0, n)
	for i, v := range vals {
		if i >= n {
			break
		}
		parts = append(parts, fmt.Sprintf("%s (%d)", untrusted(v.GetValue()), v.GetCount()))
	}
	return strings.Join(parts, "; ")
}

func toolStickiness(ctx context.Context, deps *Deps, cid uuid.UUID, days int, a toolArgs) string {
	event := strings.TrimSpace(a.Event)
	if event == "" {
		return "Specify an 'event' (see the snapshot's top events)."
	}
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "stickiness")
	}
	st, err := deps.Signals.Synapse.GetStickiness(authed, &signalsv1.GetStickinessRequest{
		ChannelId: cid.String(), Event: event, SinceDays: int32(days), WindowDays: int32(days),
	})
	if err != nil {
		return signalsUnavailable(ctx, err, "stickiness")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q engagement: WAU %d, MAU %d. Active now %d, prior window %d, returning %d (return rate %.1f%%).\n",
		event, st.GetWau(), st.GetMau(), st.GetActiveNow(), st.GetPriorActive(), st.GetReturning(), st.GetReturnRate())
	if len(st.GetDays()) > 0 {
		b.WriteString("Daily active:\n")
		for _, d := range st.GetDays() {
			fmt.Fprintf(&b, "%s: %d\n", d.GetDay(), d.GetCount())
		}
	}
	return b.String()
}

func toolUserFlow(ctx context.Context, deps *Deps, cid uuid.UUID, days int, a toolArgs) string {
	focus := strings.TrimSpace(a.FocusEvent)
	if focus == "" {
		return "Specify a 'focus_event'."
	}
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "the flow")
	}
	fr, err := deps.Signals.Synapse.GetFlow(authed, &signalsv1.GetFlowRequest{
		ChannelId: cid.String(), Event: focus, SinceDays: int32(days),
	})
	if err != nil {
		return signalsUnavailable(ctx, err, "the flow")
	}
	if fr.GetCohort() == 0 {
		return fmt.Sprintf("No one did %q in this window.", focus)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "After %q: %d people, %d continued (avg %.1f next steps).\n", focus, fr.GetCohort(), fr.GetContinued(), fr.GetAvgSteps())
	if len(fr.GetTopPaths()) > 0 {
		b.WriteString("Top paths:\n")
		for _, p := range fr.GetTopPaths() {
			fmt.Fprintf(&b, "- %s (%d)\n", quotedJoin(p.GetEvents(), " → "), p.GetCount())
		}
	}
	return b.String()
}

func toolListAffinityAttrs(ctx context.Context, deps *Deps) string {
	attrs := signalsclient.AffinityAttributes(ctx, deps.Attributes)
	if len(attrs) == 0 {
		return "No tracked affinity attributes."
	}
	var b strings.Builder
	b.WriteString("Tracked attributes (slug - name):\n")
	for _, a := range attrs {
		fmt.Fprintf(&b, "%s - %s\n", a.GetSlug(), a.GetName())
	}
	return b.String()
}

func toolAffinityBreakdown(ctx context.Context, deps *Deps, cid uuid.UUID, days int, a toolArgs) string {
	slug := strings.TrimSpace(a.Attribute)
	if slug == "" {
		return "Provide an 'attribute' slug (use list_affinity_attributes)."
	}
	limit := clampLimit(a.Limit, 15, 50)
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "the affinity breakdown")
	}
	res, err := deps.Signals.Synapse.ListAffinityValues(authed, &signalsv1.ListAffinityValuesRequest{
		ChannelId: cid.String(), Attribute: slug, SinceDays: int32(days), Limit: int32(limit),
	})
	if err != nil {
		return signalsUnavailable(ctx, err, "the affinity breakdown")
	}
	if len(res.GetValues()) == 0 {
		return fmt.Sprintf("No affinity data for %q (check the slug via list_affinity_attributes).", slug)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Top %q values by distinct-customer reach:\n", slug)
	for i, v := range res.GetValues() {
		fmt.Fprintf(&b, "%d. %s - %d customers\n", i+1, v.GetValue(), v.GetActors())
	}
	return b.String()
}

func toolListActiveCustomers(ctx context.Context, deps *Deps, cid uuid.UUID, days int) string {
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "active customers")
	}
	ov, err := deps.Signals.Synapse.GetSynapseOverview(authed, &signalsv1.GetSynapseOverviewRequest{ChannelId: cid.String(), SinceDays: int32(days)})
	if err != nil {
		return signalsUnavailable(ctx, err, "active customers")
	}
	var b strings.Builder
	if len(ov.GetActiveCustomers()) > 0 {
		// The suite knows a customer by id alone, so the names come from core's own records
		ids := make([]uuid.UUID, 0, len(ov.GetActiveCustomers()))
		for _, c := range ov.GetActiveCustomers() {
			if id, err := uuid.Parse(c.GetCustomerId()); err == nil {
				ids = append(ids, id)
			}
		}
		names, err := deps.Customer.GetByIDs(ctx, ids)
		if err != nil {
			slog.WarnContext(ctx, "tool: customer names unavailable", "err", err)
		}
		b.WriteString("Most active customers (id - name - events - last seen):\n")
		for _, c := range ov.GetActiveCustomers() {
			name := "(unknown)"
			if id, err := uuid.Parse(c.GetCustomerId()); err == nil {
				if cust, ok := names[id]; ok {
					if n := strings.TrimSpace(cust.FirstName + " " + cust.LastName); n != "" {
						name = untrusted(n)
					} else if cust.Email != "" {
						name = untrusted(cust.Email)
					}
				}
			}
			fmt.Fprintf(&b, "%s - %s - %d events - %s\n", c.GetCustomerId(), name, c.GetEvents(), c.GetLastSeen().AsTime().Format("2006-01-02"))
		}
	}
	if len(ov.GetActiveVisitors()) > 0 {
		b.WriteString("Most active anonymous visitors (anon_id - events - last page):\n")
		for i, v := range ov.GetActiveVisitors() {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&b, "%s - %d events - %s\n", v.GetAnonId(), v.GetEvents(), v.GetLastPage())
		}
	}
	if b.Len() == 0 {
		return "No active customers or visitors in this window."
	}
	return b.String()
}

func toolCustomerProfile(ctx context.Context, deps *Deps, cid uuid.UUID, days int, a toolArgs) string {
	custID, err := uuid.Parse(strings.TrimSpace(a.CustomerID))
	if err != nil {
		return "Provide a valid 'customer_id' (use list_active_customers)."
	}
	// A profile is not channel-scoped on either side of the wire, so the pin is enforced against core's own record
	cust, err := deps.Customer.GetByID(ctx, custID)
	if err != nil {
		return toolFailed(ctx, err, "Could not verify the customer.")
	}
	if cust.ChannelID != cid {
		return "That customer isn't active in this channel."
	}
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "the customer profile")
	}
	res, err := deps.Signals.Synapse.GetCustomerProfile(authed, &signalsv1.GetCustomerProfileRequest{
		CustomerId: custID.String(), SinceDays: int32(days), Attributes: signalsclient.AffinityAttributes(ctx, deps.Attributes),
	})
	if err != nil {
		return signalsUnavailable(ctx, err, "the customer profile")
	}
	label := "Customer " + custID.String()
	if n := strings.TrimSpace(cust.FirstName + " " + cust.LastName); n != "" {
		label = untrusted(n)
	} else if cust.Email != "" {
		label = untrusted(cust.Email)
	}
	return formatProfile(label, res.GetProfile())
}

func toolVisitorProfile(ctx context.Context, deps *Deps, cid uuid.UUID, days int, a toolArgs) string {
	anon := strings.TrimSpace(a.AnonID)
	if anon == "" {
		return "Provide an 'anon_id' (use list_active_customers)."
	}
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return signalsUnavailable(ctx, err, "the visitor profile")
	}
	res, err := deps.Signals.Synapse.GetVisitorProfile(authed, &signalsv1.GetVisitorProfileRequest{
		ChannelId: cid.String(), AnonId: anon, SinceDays: int32(days), Attributes: signalsclient.AffinityAttributes(ctx, deps.Attributes),
	})
	if err != nil {
		return signalsUnavailable(ctx, err, "the visitor profile")
	}
	return formatProfile("Anonymous visitor "+anon, res.GetProfile())
}

// formatProfile renders a bounded summary of a customer/visitor profile. It deliberately OMITS raw IPs and the full
// event timeline to limit the PII that reaches the model provider
func formatProfile(label string, p *signalsv1.Profile) string {
	var b strings.Builder
	b.WriteString(label + "\n")
	if p.GetFirstSeen() != nil {
		fmt.Fprintf(&b, "First seen %s. ", p.GetFirstSeen().AsTime().Format("2006-01-02"))
	}
	if p.GetLastSeen() != nil {
		fmt.Fprintf(&b, "Last seen %s.", p.GetLastSeen().AsTime().Format("2006-01-02"))
	}
	fmt.Fprintf(&b, "\n%d events, %d page views, %d conversions, %d sessions.\n",
		p.GetTotalEvents(), p.GetPageViews(), p.GetConversions(), p.GetSessions())
	for _, rev := range p.GetRevenue() {
		fmt.Fprintf(&b, "Revenue: %s across %d orders.\n", money.FormatCode(rev.GetCents(), rev.GetCurrency()), rev.GetOrders())
	}
	if len(p.GetCounts()) > 0 {
		parts := make([]string, 0, 8)
		for i, e := range p.GetCounts() {
			if i >= 8 {
				break
			}
			parts = append(parts, fmt.Sprintf("%s=%d", untrusted(e.GetName()), e.GetCount()))
		}
		fmt.Fprintf(&b, "Top events: %s\n", strings.Join(parts, ", "))
	}
	if len(p.GetTopProducts()) > 0 {
		parts := make([]string, 0, 5)
		for i, pr := range p.GetTopProducts() {
			if i >= 5 {
				break
			}
			parts = append(parts, fmt.Sprintf("%s (%d views)", pr.GetName(), pr.GetViews()))
		}
		fmt.Fprintf(&b, "Viewed products: %s\n", strings.Join(parts, "; "))
	}
	if s := valueCountList(p.GetDevices(), 3); s != "" {
		fmt.Fprintf(&b, "Devices: %s\n", s)
	}
	if s := valueCountList(p.GetReferrers(), 3); s != "" {
		fmt.Fprintf(&b, "Referrers: %s\n", s)
	}
	if s := valueCountList(p.GetCountries(), 3); s != "" {
		fmt.Fprintf(&b, "Countries: %s\n", s)
	}
	if s := valueCountList(p.GetSources(), 3); s != "" {
		fmt.Fprintf(&b, "Sources: %s\n", s)
	}
	if s := valueCountList(p.GetSearches(), 5); s != "" {
		fmt.Fprintf(&b, "Searches: %s\n", s)
	}
	return b.String()
}
