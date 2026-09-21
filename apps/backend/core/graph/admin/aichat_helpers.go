package admin // helpers for aichat.resolvers.go - free funcs kept out of the resolver file so gqlgen doesn't evict them

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/sales"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// The raw provider error can carry the provider host and status, so it goes to the log and the dashboard gets a generic one
func aiProviderError(ctx context.Context, err error) error {
	slog.ErrorContext(ctx, "chat: AI provider error", "err", err)
	return errors.New("the AI service had a problem - please try again in a moment")
}

func digestPermsFor(ctx context.Context, r *Resolver, cid uuid.UUID) digestPerms {
	// Fails closed on its own: LoadPermittedChannels reports Bypass=true with no admin in ctx, which would hand an anonymous caller every section
	if _, ok := auth.AdminFromContext(ctx); !ok {
		return digestPerms{}
	}
	allows := func(perm string) bool {
		pc, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, perm)
		return err == nil && (pc.Bypass() || pc.Allows(cid))
	}
	// With the suite switched off or unaddressed the model is never offered a tool whose data does not exist
	signals := func(perm string) bool { return r.SignalsEnabled && r.SignalsAPI != nil && allows(perm) }
	return digestPerms{
		sales:         allows("reports.view"),
		lens:          signals("lens.view"),
		synapse:       signals("customers.view"),
		pulse:         signals("emails.view"),
		prism:         signals("experiments.view"),
		customers:     allows("customers.view"),
		catalog:       allows("catalog.view"),
		orders:        allows("orders.view"),
		inventory:     allows("inventory.view"),
		promotions:    allows("promotions.view"),
		reviews:       allows("reviews.view"),
		campaigns:     signals("campaigns.view"), // Pulse is the suite's, so its tools follow the switch too
		flows:         signals("flows.view"),
		subscriptions: allows("subscriptions.view"),
		giftcards:     allows("giftcards.view"),
		channels:      allows("channels.view"),
		urlrewrite:    allows("urlrewrite.view"),
		currencies:    allows("currencies.view"),
		locales:       allows("locales.view"),
		settings:      allows("settings.view"),
		cms:           allows("cms.view"),
	}
}

// catalog/orders gate the page-aware entity tools and are deliberately outside any() - the chat's entry gate stays Signals-only, these just unlock extra tools for an admin who already got in
type digestPerms struct {
	sales, lens, synapse, pulse, prism         bool
	customers                                  bool // customers.view on its own - the commerce record, which outlives the suite
	catalog, orders                            bool
	inventory, promotions, reviews             bool
	campaigns, flows, subscriptions, giftcards bool
	channels, urlrewrite                       bool
	currencies, locales, settings, cms         bool
}

// customers rides along so an admin who reached the chat through customers.view keeps it with the suite off, when the record tools are all that is left
func (p digestPerms) any() bool {
	return p.sales || p.lens || p.synapse || p.pulse || p.prism || p.customers
}

const digestMaxChars = 12000

func fmtMoney(cents int64, code string) string {
	return fmt.Sprintf("%s %.2f", code, float64(cents)/100)
}

// Plain consts rather than fmt.Sprintf format strings, so a literal "%" in the prompt needs no escaping
const signalsPersona = "You are the analytics assistant inside the Skelet admin dashboard. Your main job is " +
	"analyzing this store's Signals data (web traffic, customer behaviour, email engagement, A/B " +
	"experiments, and sales). You can ALSO answer questions about the specific admin record the operator " +
	"is currently viewing - a product, order, or customer - by looking it up with the matching tool. " +
	"Answer using ONLY the data available to you (the snapshot below plus any tool results) - if the " +
	"answer isn't there, say so plainly rather than guessing. Be concise and concrete: cite the numbers, " +
	"point out trends, anomalies, and actionable suggestions. Politely decline requests unrelated to this " +
	"store's data and operations (general knowledge, coding, etc.). Write plainly and professionally like " +
	"a data analyst - no roleplay, catchphrases, mascots, or themed exclamations (the name is just a " +
	"label, not a character to play).\n\n"

const signalsWidgetDSL = "RICH OUTPUT: when figures read better as a visual, embed a widget by writing its JSON object " +
	"on its own line (no code fence needed). Keep surrounding prose short, use a widget only when it " +
	"genuinely helps, and put ONLY real values in it (never invent data). Aim for at most a few focused " +
	"widgets per reply. Available widgets:\n" +
	"- Headline metrics: {\"type\":\"stat\",\"items\":[{\"label\":\"Page views\",\"value\":\"12,400\",\"delta\":\"+8% vs prev\",\"trend\":\"up\"}]} " +
	"(trend is up|down|flat; value and delta are already-formatted strings)\n" +
	"- A single record's fields (a product/order/customer's details): {\"type\":\"kv\",\"title\":\"P-Zero 344/55 R16\",\"items\":[{\"label\":\"SKU\",\"value\":\"PZERO-344\"},{\"label\":\"Default price\",\"value\":\"$99.00\"}]}\n" +
	"- Rankings / comparisons: {\"type\":\"bar\",\"title\":\"Top pages\",\"data\":[{\"label\":\"/home\",\"value\":4200}],\"unit\":\"$\"} " +
	"(value is a raw number; unit optional, e.g. \"$\"). Use bars for rankings - never pie charts.\n" +
	"- Trends over time: {\"type\":\"line\",\"title\":\"Daily revenue\",\"series\":[{\"label\":\"Revenue\",\"data\":[{\"x\":\"Jun 1\",\"y\":1200}]}],\"unit\":\"$\"} " +
	"(1-2 series; use for ANYTHING over time - sales/event trends, retention curves)\n" +
	"- Tabular data (multiple rows × columns): {\"type\":\"table\",\"columns\":[\"Product\",\"Revenue\"],\"rows\":[[\"Widget\",\"$1,200\"]]}\n" +
	"NEVER format data as a markdown table (lines of | … |) - always use the kv widget (one record's fields) or the " +
	"table widget (multiple rows) instead. Plain prose is fine when a visual would not add anything.\n" +
	"FOLLOW-UPS: end your reply with a follow-ups widget of 2-3 SHORT, specific next questions the operator might ask " +
	"next given this answer + the current page, phrased in their voice as they'd type them: " +
	"{\"type\":\"followups\",\"questions\":[\"How did that compare last month?\",\"Which channel drove it?\"]}. " +
	"Skip it only for refusals or trivial replies.\n" +
	"ACTIONS: when the operator asks you to DO something you have an action for, do NOT claim you did it - " +
	"propose it with an action widget; it runs only after they click confirm: " +
	"{\"type\":\"action\",\"action\":\"reply_to_review\",\"label\":\"Post this reply to the review\",\"args\":{\"review_id\":\"…\",\"body\":\"…\"}}. " +
	"Available actions: reply_to_review(review_id, body) - draft the body yourself from the review. If there's " +
	"no matching action for the request, say you can't do that yet.\n\n"

func signalsSystemPrompt(days int, digest string, withTools bool, pageLine string) string {
	toolsHint := ""
	if withTools {
		toolsHint = "TOOLS: you can fetch focused or different-window Signals data on demand by calling the provided " +
			"tools. The snapshot below is the selected window's overview; call a tool when the user wants a metric in " +
			"more detail, a different time range (e.g. last 7 vs 30 days), more rows than the overview lists, or data " +
			"not in the snapshot. Skip tools for questions the snapshot already answers. Some tools are " +
			"discovery-then-detail: list_segments→get_segment_detail, get_experiments→get_experiment_results, " +
			"list_affinity_attributes→get_affinity_breakdown, and list_active_customers→get_customer_profile/" +
			"get_visitor_profile.\n\n"
	}
	return signalsPersona + signalsWidgetDSL + toolsHint + pageLine +
		fmt.Sprintf("SIGNALS SNAPSHOT (last %d days):\n%s", days, digest)
}

// Lets chat resolve "this page" / "this order" and reach for the matching entity tool - empty when the client sent no page context
func pageContextLine(page *model.SignalsChatPage) string {
	if page == nil || strings.TrimSpace(page.Path) == "" {
		return ""
	}
	label := page.Path
	if page.Label != nil && strings.TrimSpace(*page.Label) != "" {
		label = strings.TrimSpace(*page.Label)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "CURRENT PAGE: the operator is viewing the %q page (%s)", label, page.Path)
	if page.EntityType != nil && page.EntityID != nil && strings.TrimSpace(*page.EntityID) != "" {
		fmt.Fprintf(&b, ", which shows %s id %s. When they say \"this page\", \"this %s\", or \"here\", they mean it - "+
			"use the get_%s tool with that id to look it up.", strings.TrimSpace(*page.EntityType), strings.TrimSpace(*page.EntityID),
			strings.TrimSpace(*page.EntityType), strings.TrimSpace(*page.EntityType))
	} else {
		b.WriteString(". Use it to interpret \"this page\" / \"here\".")
	}
	b.WriteString("\n\n")
	return b.String()
}

// Every section is best-effort - a read failure drops that section rather than failing the chat
func buildSignalsDigest(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, days int, p digestPerms, maxChars int) string {
	var b strings.Builder
	if p.sales {
		writeSalesDigest(ctx, r, cid, since, &b)
	}
	if p.lens {
		writeLensDigest(ctx, r, cid, days, &b)
	}
	if p.synapse {
		writeSynapseDigest(ctx, r, cid, days, &b)
	}
	if p.pulse {
		writePulseDigest(ctx, r, cid, days, &b)
	}
	if p.prism {
		writePrismDigest(ctx, r, cid, days, &b)
	}
	out := strings.TrimSpace(b.String())
	if maxChars <= 0 {
		maxChars = digestMaxChars
	}
	if rs := []rune(out); len(rs) > maxChars {
		out = string(rs[:maxChars]) + "\n…(truncated - call a tool for the full detail)"
	}
	if out == "" {
		return "(no Signals data available for this channel and window)"
	}
	return out
}

func writeSalesDigest(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, b *strings.Builder) {
	f := sales.Filter{Start: since, End: time.Now(), ChannelIDs: []uuid.UUID{cid}}
	sum, err := r.SalesSvc.Summary(ctx, f)
	if err != nil || len(sum) == 0 {
		return
	}
	b.WriteString("## Sales\n")
	for _, c := range sum {
		var aov int64
		if c.OrderCount > 0 {
			aov = c.GrossRevenue / int64(c.OrderCount)
		}
		fmt.Fprintf(b, "- %s: %d orders, gross %s, AOV %s, %d unique customers\n",
			c.CurrencyCode, c.OrderCount, fmtMoney(c.GrossRevenue, c.CurrencyCode), fmtMoney(aov, c.CurrencyCode), c.UniqueCustomers)
	}
	if tp, err := r.SalesSvc.TopProducts(ctx, f, 5); err == nil && len(tp) > 0 {
		parts := make([]string, 0, len(tp))
		for _, pr := range tp {
			name := pr.Name
			if name == "" {
				name = pr.SKU
			}
			parts = append(parts, fmt.Sprintf("%s (%d units)", name, pr.UnitsSold))
		}
		fmt.Fprintf(b, "Top products: %s\n", strings.Join(parts, "; "))
	}
	if st, err := r.SalesSvc.ByStatus(ctx, f); err == nil && len(st) > 0 {
		parts := make([]string, 0, len(st))
		for _, s := range st {
			parts = append(parts, fmt.Sprintf("%s=%d", s.Status, s.OrderCount))
		}
		fmt.Fprintf(b, "Orders by status: %s\n", strings.Join(parts, ", "))
	}
	b.WriteString("\n")
}

func writeLensDigest(ctx context.Context, r *Resolver, cid uuid.UUID, days int, b *strings.Builder) {
	authed, err := signalsCall(ctx, r)
	if err != nil {
		return
	}
	ov, err := r.SignalsAPI.Lens.GetOverview(authed, &signalsv1.GetOverviewRequest{ChannelId: cid.String(), SinceDays: int32(days)})
	if err != nil {
		return
	}
	b.WriteString("## Lens (web traffic)\n")
	fmt.Fprintf(b, "- %d page views, %d unique visitors\n", ov.GetViews(), ov.GetVisitors())
	if len(ov.GetTopPages()) > 0 {
		parts := make([]string, 0, 5)
		for i, pg := range ov.GetTopPages() {
			if i >= 5 {
				break
			}
			parts = append(parts, fmt.Sprintf("%s (%d views)", pg.GetPageKey(), pg.GetViews()))
		}
		fmt.Fprintf(b, "Top pages: %s\n", strings.Join(parts, "; "))
	}
	if len(ov.GetDevices()) > 0 {
		parts := make([]string, 0, len(ov.GetDevices()))
		for _, d := range ov.GetDevices() {
			parts = append(parts, fmt.Sprintf("%s=%d", d.GetLabel(), d.GetViews()))
		}
		fmt.Fprintf(b, "Devices: %s\n", strings.Join(parts, ", "))
	}
	if len(ov.GetReferrers()) > 0 {
		parts := make([]string, 0, 5)
		for i, rf := range ov.GetReferrers() {
			if i >= 5 {
				break
			}
			parts = append(parts, fmt.Sprintf("%s (%d)", rf.GetLabel(), rf.GetViews()))
		}
		fmt.Fprintf(b, "Top referrers: %s\n", strings.Join(parts, "; "))
	}
	if vs, err := r.SignalsAPI.Lens.GetVitalsSummary(authed, &signalsv1.GetVitalsSummaryRequest{ChannelId: cid.String(), SinceDays: int32(days)}); err == nil && len(vs.GetVitals()) > 0 {
		parts := make([]string, 0, len(vs.GetVitals()))
		for _, v := range vs.GetVitals() {
			parts = append(parts, fmt.Sprintf("%s p75=%.0f", v.GetMetric(), v.GetP75()))
		}
		fmt.Fprintf(b, "Web vitals: %s\n", strings.Join(parts, ", "))
	}
	b.WriteString("\n")
}

func writeSynapseDigest(ctx context.Context, r *Resolver, cid uuid.UUID, days int, b *strings.Builder) {
	authed, err := signalsCall(ctx, r)
	if err != nil {
		return
	}
	ov, err := r.SignalsAPI.Synapse.GetSynapseOverview(authed, &signalsv1.GetSynapseOverviewRequest{ChannelId: cid.String(), SinceDays: int32(days)})
	if err != nil {
		return
	}
	b.WriteString("## Synapse (customer behaviour)\n")
	fmt.Fprintf(b, "- %d events, %d page views, %d conversions, %d identified customers, %d anonymous visitors\n",
		ov.GetTotalEvents(), ov.GetPageViews(), ov.GetConversions(), ov.GetIdentifiedCustomers(), ov.GetAnonymousVisitors())
	for _, rev := range ov.GetRevenue() {
		fmt.Fprintf(b, "Tracked revenue: %s across %d orders\n", fmtMoney(rev.GetCents(), rev.GetCurrency()), rev.GetOrders())
	}
	if len(ov.GetTopEvents()) > 0 {
		parts := make([]string, 0, 8)
		for i, e := range ov.GetTopEvents() {
			if i >= 8 {
				break
			}
			parts = append(parts, fmt.Sprintf("%s=%d", e.GetName(), e.GetCount()))
		}
		fmt.Fprintf(b, "Top events: %s\n", strings.Join(parts, ", "))
	}
	if segs, err := r.SignalsAPI.Segments.ListSegments(authed, &signalsv1.ListSegmentsRequest{PageSize: 50}); err == nil && len(segs.GetSegments()) > 0 {
		parts := make([]string, 0, len(segs.GetSegments()))
		for _, sg := range segs.GetSegments() {
			parts = append(parts, fmt.Sprintf("%s (%d members)", sg.GetName(), sg.GetMemberCount()))
		}
		fmt.Fprintf(b, "Segments: %s\n", strings.Join(parts, "; "))
	}
	b.WriteString("\n")
}

func writePulseDigest(ctx context.Context, r *Resolver, cid uuid.UUID, days int, b *strings.Builder) {
	authed, err := signalsCall(ctx, r)
	if err != nil {
		return
	}
	e, err := r.SignalsAPI.Pulse.GetEngagement(authed, &signalsv1.GetEngagementRequest{ChannelId: cid.String(), SinceDays: int32(days)})
	if err != nil {
		return
	}
	b.WriteString("## Pulse (email engagement)\n")
	fmt.Fprintf(b, "- %d opens (human), %d clicks, %d conversions\n", e.GetOpensHuman(), e.GetClicks(), e.GetConversions())
	for _, rev := range e.GetRevenue() {
		fmt.Fprintf(b, "Attributed revenue: %s across %d orders\n", fmtMoney(rev.GetCents(), rev.GetCurrency()), rev.GetOrders())
	}
	if len(e.GetTopLinks()) > 0 {
		parts := make([]string, 0, 5)
		for i, l := range e.GetTopLinks() {
			if i >= 5 {
				break
			}
			parts = append(parts, fmt.Sprintf("%s (%d clicks)", l.GetUrl(), l.GetClicks()))
		}
		fmt.Fprintf(b, "Top links: %s\n", strings.Join(parts, "; "))
	}
	b.WriteString("\n")
}

func writePrismDigest(ctx context.Context, r *Resolver, cid uuid.UUID, days int, b *strings.Builder) {
	authed, err := signalsCall(ctx, r)
	if err != nil {
		return
	}
	res, err := r.SignalsAPI.Prism.GetExperimentsOverview(authed, &signalsv1.GetExperimentsOverviewRequest{ChannelId: cid.String(), SinceDays: int32(days)})
	if err != nil {
		return
	}
	ov := res
	b.WriteString("## Prism (A/B experiments)\n")
	fmt.Fprintf(b, "- %d running, %d with a decided winner, %d total exposed\n", ov.GetRunning(), ov.GetDecided(), ov.GetTotalExposed())
	for _, s := range ov.GetExperiments() {
		winner := ""
		if s.GetWinner() != "" {
			winner = fmt.Sprintf(", winner: %s", s.GetWinner())
		}
		fmt.Fprintf(b, "- %s (id %s): leader %q (uplift %.1f%%, chance-to-win %.0f%%)%s\n",
			s.GetName(), s.GetId(), s.GetLeader(), s.GetLeaderUplift()*100, s.GetLeaderChanceToWin()*100, winner)
	}
	b.WriteString("\n")
}
