package analytics

import (
	"sort"
	"strings"
)

// PropType is what a planned property's value must be once the browser's JSON is decoded
type PropType string

const (
	PropString PropType = "string"
	PropNumber PropType = "number"
	PropBool   PropType = "bool"
)

// PlannedEvent is one browser event the storefront tracker may send and the properties it may carry
type PlannedEvent struct {
	Name  string
	About string
	Props map[string]PropType
	Attrs bool // also takes the open attr:<slug> string properties, the catalogue attributes a page annotates itself with
}

// CustomEvent is the plan entry every named UI interaction is held to - their names are the merchant's own (a
// data-lens-event value, a toggle's label), so only the reserved $ namespace is closed
const CustomEvent = "custom"

// TrackingPlan is what the Lens beacon accepts from a browser. Anything else is stripped and counted for Data health,
// and docs/tracking-plan.md is generated from it
var TrackingPlan = []PlannedEvent{
	{Name: EventPageView, About: "A page was shown. Product pages describe the product they show.", Attrs: true, Props: map[string]PropType{
		"entity": PropString, "entity_name": PropString, "entity_image": PropString, "entity_price": PropString, "sku": PropString,
		"utm_source": PropString, "utm_medium": PropString, "utm_campaign": PropString, "search_query": PropString,
	}},
	{Name: EventProductFilter, About: "A shopper narrowed a product list.", Attrs: true, Props: map[string]PropType{"filters": PropString}},
	{Name: "$search", About: "An on-site search and how many results it found.", Props: map[string]PropType{"query": PropString, "results": PropNumber}},
	{Name: "$experiment", About: "The variation of an experiment a shopper saw.", Props: map[string]PropType{"experiment": PropString, "variation": PropString}},
	{Name: "$js_error", About: "A script error on the page.", Props: map[string]PropType{"message": PropString, "source": PropString, "line": PropNumber, "sessionId": PropString}},
	{Name: "$rage_click", About: "Three or more clicks in one spot within a second.", Props: map[string]PropType{"x": PropNumber, "y": PropNumber, "selector": PropString, "sessionId": PropString}},
	{Name: "$dead_click", About: "A click that changed nothing on the page.", Props: map[string]PropType{"x": PropNumber, "y": PropNumber, "selector": PropString, "sessionId": PropString}},
	{Name: "$web_vitals", About: "A Core Web Vitals measurement.", Props: map[string]PropType{"metric": PropString, "value": PropNumber, "sessionId": PropString}},
	{Name: EventViewItem, About: "A product was viewed. Older storefronts send it; the server's own events are the source of truth.", Props: map[string]PropType{"sku": PropString}},
	{Name: EventBeginCheckout, About: "Checkout was opened. Older storefronts send it.", Props: map[string]PropType{}},
	{Name: EventPurchase, About: "An order was placed. Older storefronts send it; revenue only ever comes from the server's order events.", Props: map[string]PropType{}},
	{Name: CustomEvent, About: "Any other named interaction: a data-lens-event, a toggle opened or closed, a CMS block seen.", Props: map[string]PropType{"action": PropString, "source": PropString}},
}

// ServerProps are set from the request or by the server, never taken from a browser - a forged is_bot, revenue or
// event name would otherwise reach every report
var ServerProps = []string{
	"is_bot", "bot_name", "bot_category", "machine", "revenue_cents", "total_cents", "currency",
	"ip", "device", "client", "os", "country", "page_key", "viewport", "title", "referrer", "name",
}

// What Data health counts, one rollup dim each
const (
	IssueUnknownEvent = "plan_unknown_event" // a $ name the plan doesn't have - the event is dropped
	IssueUnknownProp  = "plan_unknown_prop"  // a property the event's plan entry doesn't list - the property is dropped
	IssueBadType      = "plan_bad_type"      // a planned property with the wrong type - the property is dropped
	IssueServerProp   = "plan_server_prop"   // a browser sent a property only the server may set - the property is dropped
)

type PlanIssue struct {
	Kind  string
	Event string
	Prop  string // empty for IssueUnknownEvent
}

var (
	plan        = map[string]PlannedEvent{}
	serverProps = map[string]bool{}
)

func init() {
	for _, e := range TrackingPlan {
		plan[e.Name] = e
	}
	for _, p := range ServerProps {
		serverProps[p] = true
	}
}

// ApplyPlan holds one browser event to the plan, deleting what breaks it from props in place. keep is false when the
// whole event must go. name is the event's name as the tracker sent it - a custom event's own name
func ApplyPlan(name string, props map[string]any) (keep bool, issues []PlanIssue) {
	entry, ok := plan[name]
	if !ok {
		if strings.HasPrefix(name, "$") {
			return false, []PlanIssue{{Kind: IssueUnknownEvent, Event: name}}
		}
		entry = plan[CustomEvent]
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys) // issues come out in a stable order
	for _, k := range keys {
		kind := ""
		switch want, planned := entry.Props[k]; {
		case serverProps[k]:
			kind = IssueServerProp
		case planned:
			if !hasType(props[k], want) {
				kind = IssueBadType
			}
		case entry.Attrs && strings.HasPrefix(k, "attr:"):
			if !hasType(props[k], PropString) {
				kind = IssueBadType
			}
		default:
			kind = IssueUnknownProp
		}
		if kind != "" {
			delete(props, k)
			issues = append(issues, PlanIssue{Kind: kind, Event: entry.Name, Prop: k})
		}
	}
	return true, issues
}

func hasType(v any, t PropType) bool {
	switch v.(type) {
	case string:
		return t == PropString
	case float64, int, int64:
		return t == PropNumber
	case bool:
		return t == PropBool
	}
	return false
}
