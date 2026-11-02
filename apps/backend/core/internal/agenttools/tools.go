package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
)

type tool struct {
	def  ai.Tool
	perm func(Perms) bool
	// Both halves of one window - the commerce read-models filter on a timestamp range, the Signals contract takes a day count
	run func(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time, days int, a toolArgs) string
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

const maxWindowDays = 365

func windowProp() map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": fmt.Sprintf("Days back to look (1-%d). Omit for the default window.", maxWindowDays),
		"minimum":     1,
		"maximum":     maxWindowDays,
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

func dayOrWeek(s string) string {
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

var allTools = sync.OnceValue(registry)

// Concatenation order is the order the tools are offered to the model
func registry() []tool {
	out := make([]tool, 0, 64)
	out = append(out, salesTools()...)
	out = append(out, lensTools()...)
	out = append(out, synapseTools()...)
	out = append(out, prismTools()...)
	out = append(out, entityLookupTools()...)
	out = append(out, catalogTools()...)
	out = append(out, opsTools()...)
	out = append(out, marketingTools()...)
	out = append(out, storeConfigTools()...)
	return out
}

// The model has to tell "could not ask" from "asked, nothing there" - a gRPC status from the service is usually a permission refusal worth relaying
func signalsUnavailable(ctx context.Context, err error, what string) string {
	if signalsclient.Unreachable(err) {
		return toolFailed(ctx, err, "The Signals service is not reachable right now, so "+what+" could not be read.")
	}
	return toolFailed(ctx, err, "Could not load "+what+": "+status.Convert(err).Message())
}

type toolCall struct {
	name              string
	failed            bool
	allowCustomerData bool
}

type toolCallKey struct{}

func toolFailed(ctx context.Context, err error, msg string) string {
	if err == nil {
		return msg
	}
	name := ""
	if c, ok := ctx.Value(toolCallKey{}).(*toolCall); ok {
		c.failed, name = true, c.name
	}
	slog.WarnContext(ctx, "tool: read failed", "tool", name, "err", err)
	return msg
}

// Text a shopper or visitor wrote goes out quoted, so a name, search or page address can't pose as the tool's own output
func untrusted(s string) string { return strconv.Quote(s) }

func quotedJoin(items []string, sep string) string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = untrusted(s)
	}
	return strings.Join(out, sep)
}

// Names the model copies back from quoted output arrive bare, whichever way it sends them
func bare(s string) string {
	s = strings.TrimSpace(s)
	if u, err := strconv.Unquote(s); err == nil && strings.HasPrefix(s, `"`) {
		return u
	}
	return s
}

func shopperEmail(ctx context.Context, email string) string {
	if c, ok := ctx.Value(toolCallKey{}).(*toolCall); !ok || !c.allowCustomerData {
		return "(hidden - customer data is off)"
	}
	return untrusted(email)
}

// Every Signals tool starts here - the caller's own token rides along, so the service refuses what they could not read themselves
func signalsCall(ctx context.Context, deps *Deps) (context.Context, error) {
	return deps.Signals.Authed(ctx)
}

// A gRPC status came from the suite and is worth relaying, anything else stays in the log
func readFailed(ctx context.Context, err error, what string) string {
	if _, fromSuite := status.FromError(err); fromSuite || signalsclient.Unreachable(err) {
		return signalsUnavailable(ctx, err, what)
	}
	return toolFailed(ctx, err, "Could not load "+what+".")
}

// Dropped from the menu when ai/llm/allow_customer_data is off - that switch exists to keep personal data away from the LLM provider
var piiTools = map[string]bool{
	"get_customer_profile":  true,
	"get_visitor_profile":   true,
	"list_active_customers": true,
	"get_customer":          true,
}

func offeredTools(p Perms, allowCustomerData bool) []tool {
	all := allTools()
	out := make([]tool, 0, len(all))
	for _, t := range all {
		if !t.perm(p) {
			continue
		}
		if !allowCustomerData && piiTools[t.def.Name] {
			continue
		}
		out = append(out, t)
	}
	return out
}

// Clamps the args and runs one tool - a bad argument or a failed read comes back as an error RESULT so the model can recover, rather than failing the chat
func runTool(ctx context.Context, deps *Deps, t tool, cid uuid.UUID, defaultDays int, allowCustomerData bool, input json.RawMessage) ai.ToolResult {
	var a toolArgs
	if len(input) > 0 {
		if err := json.Unmarshal(input, &a); err != nil {
			return ai.ToolResult{IsError: true, Content: "The arguments don't match the tool's input schema: " + err.Error()}
		}
	}
	a.PageKey, a.Event, a.FocusEvent, a.StartEvent, a.ReturnEvent, a.Breakdown = bare(a.PageKey), bare(a.Event), bare(a.FocusEvent), bare(a.StartEvent), bare(a.ReturnEvent), bare(a.Breakdown)
	for i := range a.Steps {
		a.Steps[i] = bare(a.Steps[i])
	}
	days := clampDays(a.WindowDays, defaultDays)
	since := time.Now().AddDate(0, 0, -days)
	state := &toolCall{name: t.def.Name, allowCustomerData: allowCustomerData}
	out := strings.TrimSpace(t.run(context.WithValue(ctx, toolCallKey{}, state), deps, cid, since, days, a))
	if out == "" {
		out = "No data."
	}
	return ai.ToolResult{Content: out, IsError: state.failed}
}

// Every tool there is, before any caller is in the picture - the MCP server registers these once at boot
func Definitions() []ai.Tool {
	all := allTools()
	out := make([]ai.Tool, 0, len(all))
	for _, t := range all {
		out = append(out, t.def)
	}
	return out
}
