package admin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/ai"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/signalsclient"
)

// Configured but never dialled - grpc.NewClient connects lazily, so this stands in for a reachable suite
func dialableSignals(t *testing.T) *signalsclient.Client {
	t.Helper()
	c, err := signalsclient.New("127.0.0.1:1", "test-secret", time.Minute, nil)
	if err != nil {
		t.Fatalf("signalsclient.New: %v", err)
	}
	if c == nil {
		t.Fatal("an addressed client should not be nil")
	}
	return c
}

func TestPermittedTools(t *testing.T) {
	got := permittedTools(digestPerms{sales: true, lens: true}, true)
	names := map[string]bool{}
	for _, tl := range got {
		names[tl.Name] = true
	}
	for _, want := range []string{
		"get_sales_summary", "get_top_products", "get_sales_trend", "get_refunds",
		"get_web_traffic", "get_top_pages", "get_bot_traffic", "get_site_search", "get_frustration",
		"get_traffic_trend", "get_page_performance", "get_page_stats",
	} {
		if !names[want] {
			t.Fatalf("expected tool %q to be offered, got %v", want, names)
		}
	}
	for _, forbidden := range []string{
		"get_email_engagement", "get_customer_behaviour", "get_funnel", "get_retention",
		"list_segments", "get_segment_detail", "get_experiments", "get_experiment_results",
		"get_stickiness", "get_user_flow", "list_affinity_attributes", "get_affinity_breakdown",
		"list_active_customers", "get_customer_profile", "get_visitor_profile",
		"get_product", "get_order", "get_customer",
		"search_products", "list_categories", "get_category", "list_collections",
		"get_collection", "list_product_types", "list_attributes",
		"search_orders", "get_stock", "list_low_stock", "list_promotions",
		"get_promotion_performance", "get_product_reviews",
		"list_campaigns", "list_flows", "list_subscriptions", "list_giftcards", "list_channels",
		"check_giftcard", "get_flow_runs", "list_redirects",
		"list_currencies", "list_locales", "list_menus", "list_cms_pages",
		"list_shipping_methods", "list_tax_rules",
	} {
		if names[forbidden] {
			t.Fatalf("offered tool %q the admin can't see: %v", forbidden, names)
		}
	}

	ent := map[string]bool{} // page-aware, catalog and domain tools each answer to their own perm
	for _, tl := range permittedTools(digestPerms{
		catalog: true, orders: true, synapse: true, customers: true,
		inventory: true, promotions: true, reviews: true,
		campaigns: true, flows: true, subscriptions: true, giftcards: true, channels: true,
		urlrewrite: true, currencies: true, locales: true, settings: true, cms: true,
	}, true) {
		ent[tl.Name] = true
	}
	for _, want := range []string{
		"get_product", "get_order", "get_customer",
		"search_products", "list_categories", "get_category", "list_collections",
		"get_collection", "list_product_types", "list_attributes",
		"search_orders", "get_stock", "list_low_stock", "list_promotions",
		"get_promotion_performance", "get_product_reviews",
		"list_campaigns", "list_flows", "list_subscriptions", "list_giftcards", "list_channels",
		"check_giftcard", "get_flow_runs", "list_redirects",
		"list_currencies", "list_locales", "list_menus", "list_cms_pages",
		"list_shipping_methods", "list_tax_rules",
	} {
		if !ent[want] {
			t.Fatalf("tool %q should be offered with its perm, got %v", want, ent)
		}
	}

	pii := map[string]bool{}
	for _, tl := range permittedTools(digestPerms{synapse: true, customers: true}, false) {
		pii[tl.Name] = true
	}
	for _, blocked := range []string{"get_customer_profile", "get_visitor_profile", "list_active_customers", "get_customer"} {
		if pii[blocked] {
			t.Fatalf("PII tool %q should be dropped when customer data is disallowed", blocked)
		}
	}
	if !pii["get_customer_behaviour"] {
		t.Fatalf("non-PII synapse tool should remain when customer data is disallowed")
	}
}

func TestRunSignalToolGating(t *testing.T) {
	r := &Resolver{}
	cid := uuid.New()
	ctx := context.Background()

	res := runSignalTool(ctx, r, digestPerms{sales: true}, cid, 30, ai.ToolCall{ID: "1", Name: "nope"})
	if !res.IsError || res.ID != "1" {
		t.Fatalf("unknown tool should be an error result, got %+v", res)
	}

	// A nil Resolver is the point - the permission check has to return before anything touches a read-model
	res = runSignalTool(ctx, r, digestPerms{}, cid, 30, ai.ToolCall{ID: "2", Name: "get_email_engagement"})
	if !res.IsError || res.ID != "2" {
		t.Fatalf("disallowed tool should be an error result, got %+v", res)
	}

	// Drill-down tools validate their id before any service call, so a bad one comes back as guidance the model can act on
	res = runSignalTool(ctx, r, digestPerms{synapse: true}, cid, 30,
		ai.ToolCall{ID: "3", Name: "get_segment_detail", Input: []byte(`{"segment_id":"not-a-uuid"}`)})
	if res.IsError || !strings.Contains(res.Content, "segment_id") {
		t.Fatalf("bad segment_id should return guidance, got %+v", res)
	}
	res = runSignalTool(ctx, r, digestPerms{prism: true}, cid, 30,
		ai.ToolCall{ID: "4", Name: "get_experiment_results", Input: []byte(`{}`)})
	if res.IsError || !strings.Contains(res.Content, "experiment_id") {
		t.Fatalf("missing experiment_id should return guidance, got %+v", res)
	}
	res = runSignalTool(ctx, r, digestPerms{synapse: true}, cid, 30,
		ai.ToolCall{ID: "5", Name: "get_funnel", Input: []byte(`{"steps":["only_one"]}`)})
	if res.IsError || !strings.Contains(res.Content, "steps") {
		t.Fatalf("funnel with <2 steps should return guidance, got %+v", res)
	}

	for _, tc := range []struct {
		name, tool, input, want string
		perm                    digestPerms
	}{
		{"6", "get_page_stats", `{}`, "page_key", digestPerms{lens: true}},
		{"7", "get_affinity_breakdown", `{}`, "attribute", digestPerms{synapse: true}},
		{"8", "get_user_flow", `{}`, "focus_event", digestPerms{synapse: true}},
		{"9", "get_stickiness", `{}`, "event", digestPerms{synapse: true}},
		{"10", "get_customer_profile", `{"customer_id":"nope"}`, "customer_id", digestPerms{synapse: true}},
		{"11", "get_product", `{}`, "product_id", digestPerms{catalog: true}},
		{"12", "get_order", `{"order_id":"nope"}`, "order_id", digestPerms{orders: true}},
		{"13", "get_customer", `{}`, "customer_id", digestPerms{customers: true}},
		{"14", "get_category", `{}`, "category", digestPerms{catalog: true}},
		{"15", "get_collection", `{"collection_id":"nope"}`, "collection_id", digestPerms{catalog: true}},
		{"16", "get_stock", `{}`, "product_id", digestPerms{inventory: true}},
		{"17", "get_product_reviews", `{"product_id":"nope"}`, "product_id", digestPerms{reviews: true}},
		{"18", "check_giftcard", `{}`, "code", digestPerms{giftcards: true}},
		{"19", "get_flow_runs", `{}`, "flow_id", digestPerms{flows: true}},
	} {
		res = runSignalTool(ctx, r, tc.perm, cid, 30, ai.ToolCall{ID: tc.name, Name: tc.tool, Input: []byte(tc.input)})
		if res.IsError || !strings.Contains(res.Content, tc.want) {
			t.Fatalf("%s with bad arg should return guidance mentioning %q, got %+v", tc.tool, tc.want, res)
		}
	}
}

// SIGNALS_ENABLED=false has to take the suite out of the chat entirely - the model is never offered a tool whose data does not exist, rather than calling one and reading an empty answer as a real one
// A superuser bypasses the grant lookup before the admin service is touched, so a bare Resolver is enough here
func TestSignalsSectionsFollowTheSwitch(t *testing.T) {
	ctx := auth.WithAdmin(context.Background(), &auth.Claims{AdminID: uuid.New(), IsSuperuser: true})
	cid := uuid.New()

	on := digestPermsFor(ctx, &Resolver{SignalsEnabled: true, SignalsAPI: dialableSignals(t)}, cid)
	if !on.lens || !on.synapse || !on.pulse || !on.prism {
		t.Fatalf("a superuser should see every suite section with the switch on, got %+v", on)
	}

	// The switch is not the only way the suite goes away - with no service to dial there is nothing to offer either
	unaddressed := digestPermsFor(ctx, &Resolver{SignalsEnabled: true}, cid)
	if unaddressed.lens || unaddressed.synapse || unaddressed.pulse || unaddressed.prism {
		t.Errorf("suite sections should be off with no Signals address configured, got %+v", unaddressed)
	}

	off := digestPermsFor(ctx, &Resolver{SignalsEnabled: false}, cid)
	if off.lens || off.synapse || off.pulse || off.prism {
		t.Errorf("suite sections should be off with SIGNALS_ENABLED=false, got %+v", off)
	}
	// The commerce halves are not the suite's to take away
	if !off.sales || !off.customers || !off.orders || !off.catalog {
		t.Errorf("commerce sections should survive the switch, got %+v", off)
	}
	if !off.any() {
		t.Error("the chat should stay reachable with the suite off - the record tools are still there")
	}
}

// The tool menu is built from those flags, so nothing Signals-backed may reach the model with the suite off
func TestNoSignalsToolIsOfferedWithTheSuiteOff(t *testing.T) {
	ctx := auth.WithAdmin(context.Background(), &auth.Claims{AdminID: uuid.New(), IsSuperuser: true})
	off := digestPermsFor(ctx, &Resolver{SignalsEnabled: false}, uuid.New())

	offered := map[string]bool{}
	for _, tl := range permittedTools(off, true) {
		offered[tl.Name] = true
	}
	for _, gone := range []string{
		// Synapse
		"get_customer_behaviour", "get_event_trend", "get_funnel", "get_retention", "get_stickiness",
		"get_user_flow", "list_segments", "get_segment_detail", "list_affinity_attributes",
		"get_affinity_breakdown", "list_active_customers", "get_customer_profile", "get_visitor_profile",
		// Lens
		"get_web_traffic", "get_top_pages", "get_bot_traffic", "get_site_search", "get_frustration",
		"get_traffic_trend", "get_page_performance", "get_page_stats",
		// Prism
		"get_experiments", "get_experiment_results",
	} {
		if offered[gone] {
			t.Errorf("%q is offered with SIGNALS_ENABLED=false, but its data lives in the suite", gone)
		}
	}
	// A record tool that only looks like a Signals one
	if !offered["get_customer"] {
		t.Error("get_customer reads the commerce record and should survive the switch")
	}

	// Called anyway, it is refused rather than run
	res := runSignalTool(ctx, &Resolver{}, off, uuid.New(), 30, ai.ToolCall{ID: "x", Name: "get_customer_behaviour"})
	if !res.IsError {
		t.Errorf("a suite tool called with the switch off should be refused, got %+v", res)
	}
}
