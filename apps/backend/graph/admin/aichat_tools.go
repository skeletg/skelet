package admin // the model picks a name + JSON args off this fixed menu and never writes a query, and the channel is pinned server-side - so a tool can only return what the requesting admin may already see

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/ai"
)

type signalTool struct {
	def  ai.Tool
	perm func(digestPerms) bool
	run  func(ctx context.Context, r *Resolver, cid uuid.UUID, since time.Time, days int, a toolArgs) string
}

// One bag for every tool, each reading the fields it needs - channel is NEVER in here, it's pinned to the request's
type toolArgs struct {
	WindowDays   int      `json:"window_days"`
	Limit        int      `json:"limit"`
	Bucket       string   `json:"bucket"`        // day|week (sales trend)
	Event        string   `json:"event"`         // event trend
	Breakdown    string   `json:"breakdown"`     // event-property key
	Measure      string   `json:"measure"`       // count|unique
	Unit         string   `json:"unit"`          // day|week (trend / retention)
	Steps        []string `json:"steps"`         // funnel
	StartEvent   string   `json:"start_event"`   // retention
	ReturnEvent  string   `json:"return_event"`  // retention
	Periods      int      `json:"periods"`       // retention
	SegmentID    string   `json:"segment_id"`    // segment detail
	ExperimentID string   `json:"experiment_id"` // experiment results
	Bot          string   `json:"bot"`           // bot traffic filter
	PageKey      string   `json:"page_key"`      // page stats
	FocusEvent   string   `json:"focus_event"`   // user flow
	Attribute    string   `json:"attribute"`     // affinity breakdown
	CustomerID   string   `json:"customer_id"`   // customer profile / record
	AnonID       string   `json:"anon_id"`       // visitor profile
	ProductID    string   `json:"product_id"`    // product record (page-aware)
	OrderID      string   `json:"order_id"`      // order record (page-aware)
	Query        string   `json:"query"`         // product / order search
	ParentID     string   `json:"parent_id"`     // category children
	Category     string   `json:"category"`      // category detail (id or slug)
	CollectionID string   `json:"collection_id"` // collection detail
	Status       string   `json:"status"`        // order / subscription status filter
	Code         string   `json:"code"`          // gift-card lookup
	FlowID       string   `json:"flow_id"`       // flow runs
}

func objSchema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func windowProp() map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": "Days back to look (1-365). Omit to use the panel's selected window.",
		"minimum":     1,
		"maximum":     365,
	}
}

func limitProp(desc string, max int) map[string]any {
	return map[string]any{"type": "integer", "description": desc, "minimum": 1, "maximum": max}
}

func enumProp(desc string, vals ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": vals}
}

func windowSchema(extra map[string]any, required ...string) map[string]any {
	props := map[string]any{"window_days": windowProp()}
	maps.Copy(props, extra)
	return objSchema(props, required...)
}

func bucketOf(s string) string {
	if s == "week" {
		return "week"
	}
	return "day"
}

func unitOf(s string) string {
	if s == "week" {
		return "week"
	}
	return "day"
}

func clampLimit(n, def, max int) int {
	if n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

func cleanSteps(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Concatenation order is the order the tools are offered to the model
func signalTools() []signalTool {
	out := make([]signalTool, 0, 64)
	out = append(out, salesSignalTools()...)
	out = append(out, lensSignalTools()...)
	out = append(out, synapseSignalTools()...)
	out = append(out, prismSignalTools()...)
	out = append(out, entityLookupSignalTools()...)
	out = append(out, catalogSignalTools()...)
	out = append(out, opsSignalTools()...)
	out = append(out, marketingSignalTools()...)
	out = append(out, storeConfigSignalTools()...)
	return out
}

// Runs a digest section writer into a fresh builder so tool output and the snapshot share one formatting
func reuseDigest(write func(*strings.Builder), empty string) string {
	var b strings.Builder
	write(&b)
	out := strings.TrimSpace(b.String())
	if out == "" {
		return empty
	}
	return out
}

// Dropped from the menu when ai/llm/allow_customer_data is off - that switch exists to keep personal data away from the LLM provider
var piiTools = map[string]bool{
	"get_customer_profile":  true,
	"get_visitor_profile":   true,
	"list_active_customers": true,
	"get_customer":          true,
}

func permittedTools(p digestPerms, allowCustomerData bool) []ai.Tool {
	all := signalTools()
	out := make([]ai.Tool, 0, len(all))
	for _, t := range all {
		if !t.perm(p) {
			continue
		}
		if !allowCustomerData && piiTools[t.def.Name] {
			continue
		}
		out = append(out, t.def)
	}
	return out
}

// Re-checks the section permission and clamps the args on every call - a bad or disallowed one comes back as an error RESULT so the model can recover, rather than failing the chat
func runSignalTool(ctx context.Context, r *Resolver, p digestPerms, cid uuid.UUID, defaultDays int, call ai.ToolCall) ai.ToolResult {
	res := ai.ToolResult{ID: call.ID}
	var tool *signalTool
	tools := signalTools()
	for i := range tools {
		if tools[i].def.Name == call.Name {
			tool = &tools[i]
			break
		}
	}
	if tool == nil {
		res.IsError = true
		res.Content = fmt.Sprintf("Unknown tool %q.", call.Name)
		return res
	}
	if !tool.perm(p) {
		res.IsError = true
		res.Content = "You don't have permission to access that data."
		return res
	}
	var a toolArgs
	if len(call.Input) > 0 {
		_ = json.Unmarshal(call.Input, &a)
	}
	days := a.WindowDays
	if days <= 0 || days > 365 {
		days = defaultDays
	}
	since := time.Now().AddDate(0, 0, -days)
	out := strings.TrimSpace(tool.run(ctx, r, cid, since, days, a))
	if out == "" {
		out = "No data."
	}
	res.Content = out
	return res
}
