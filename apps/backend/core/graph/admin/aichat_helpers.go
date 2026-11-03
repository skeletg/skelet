package admin // helpers for aichat.resolvers.go - free funcs kept out of the resolver file so gqlgen doesn't evict them

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
)

// The raw provider error can carry the provider host and status, so it goes to the log and the dashboard gets a generic one
func aiProviderError(ctx context.Context, err error) error {
	slog.ErrorContext(ctx, "chat: AI provider error", "err", err)
	return errors.New("the AI service had a problem - please try again in a moment")
}

const compactSnapshotChars = 2500

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
