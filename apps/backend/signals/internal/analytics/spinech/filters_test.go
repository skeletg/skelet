package spinech

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// A small shop with known answers for filtered reads: three shoppers, two products, one refund
//
//	a: views X, buys 2 × X (net 3600), comes back a week later and buys a mug
//	b: views X, adds X to the cart, leaves
//	c: views the mug and buys it (net 900)
//	a's second X is refunded (-1800)
func seedSales(t *testing.T) *engineSpine {
	e := newEngineSpine(t)
	h := time.Hour
	x, mug := map[string]any{"sku": "X"}, map[string]any{"sku": "MUG"}
	line := func(sku string, net int64) map[string]any { return map[string]any{"sku": sku, "net_base_cents": net} }
	e.add("view_item", "a", nil, 200*h, x)
	e.add("order_placed", "a", nil, 199*h, nil)
	e.add("order_line", "a", nil, 199*h, line("X", 3600))
	e.add("refund_line", "a", nil, 100*h, line("X", -1800))
	e.add("order_placed", "a", nil, 30*h, nil) // a returns to buy again
	e.add("order_line", "a", nil, 30*h, line("MUG", 900))
	e.add("view_item", "b", nil, 50*h, x)
	e.add("cart_add", "b", nil, 49*h, x)
	e.add("view_item", "c", nil, 40*h, mug)
	e.add("order_placed", "c", nil, 39*h, nil)
	e.add("order_line", "c", nil, 39*h, line("MUG", 900))
	e.add("page_view", "c", nil, 38*h, map[string]any{"device": "mobile"})
	e.flush()
	return e
}

func (e *engineSpine) insights(q analytics.InsightsQuery) analytics.Insights {
	e.t.Helper()
	ins, err := e.st.Insights(context.Background(), e.ch, q, e.now.Add(-14*24*time.Hour), e.now)
	if err != nil {
		e.t.Fatal(err)
	}
	return ins
}

func eq(k, v string) analytics.PropertyFilter {
	return analytics.PropertyFilter{Key: k, Op: analytics.FilterEq, Values: []string{v}}
}

func TestInsightsHoldsEventsToEachOperator(t *testing.T) {
	e := seedSales(t)
	defer e.st.Close()
	views := []string{"view_item"}
	for name, tc := range map[string]struct {
		f    analytics.PropertyFilter
		want int64
	}{
		"= X":          {eq("sku", "X"), 2},
		"≠ X":          {analytics.PropertyFilter{Key: "sku", Op: analytics.FilterNeq, Values: []string{"X"}}, 1},
		"one of X,MUG": {analytics.PropertyFilter{Key: "sku", Op: analytics.FilterIn, Values: []string{"X", "MUG"}}, 3},
		"= nothing":    {eq("sku", "NONE"), 0},
	} {
		if got := e.insights(analytics.InsightsQuery{Events: views, Measure: "total", Unit: "day", Filters: []analytics.PropertyFilter{tc.f}}).Total; got != tc.want {
			t.Errorf("product views where sku %s = %d, want %d", name, got, tc.want)
		}
	}
	// ≠ lets through an event that doesn't carry the property at all: c's page view has no sku
	if got := e.insights(analytics.InsightsQuery{Events: []string{"page_view"}, Measure: "total", Unit: "day", Filters: []analytics.PropertyFilter{{Key: "sku", Op: analytics.FilterNeq, Values: []string{"X"}}}}).Total; got != 1 {
		t.Errorf("page views without sku X = %d, want 1 - an event lacking the property passes ≠", got)
	}
}

// Net sales for X is the sum over its lines and refunds - the negative refund takes back what it credited
func TestNetSalesIsASumOverOrderAndRefundLines(t *testing.T) {
	e := seedSales(t)
	defer e.st.Close()
	sales := analytics.InsightsQuery{Events: []string{"order_line", "order_line_adjusted", "refund_line"}, Measure: "sum", SumKey: "net_base_cents", Unit: "day"}
	x := sales
	x.Filters = []analytics.PropertyFilter{eq("sku", "X")}
	if got := e.insights(x).Total; got != 1800 {
		t.Fatalf("X's net sales = %d, want 3600 sold - 1800 refunded = 1800", got)
	}
	if got := e.insights(sales).Total; got != 1800+900+900 {
		t.Fatalf("the shop's net sales = %d, want 3600", got)
	}
	if _, err := e.st.Insights(context.Background(), e.ch, analytics.InsightsQuery{Measure: "sum"}, e.now.Add(-time.Hour), e.now); !errors.Is(err, analytics.ErrBadFilter) {
		t.Fatalf("a sum of nothing = %v, want ErrBadFilter", err)
	}
}

func TestStickinessAndRetentionFollowTheirFilters(t *testing.T) {
	e := seedSales(t)
	defer e.st.Close()
	st, err := e.st.Stickiness(context.Background(), e.ch, "view_item", []analytics.PropertyFilter{eq("sku", "X")}, e.now.Add(-14*24*time.Hour), e.now, 30)
	if err != nil || st.ActiveNow != 2 {
		t.Fatalf("people active on X = %d, %v, want a and b", st.ActiveNow, err)
	}
	// Buyers of X who came back to any order: a is the only buyer of X, and came back
	ret, err := e.st.Retention(context.Background(), e.ch, "order_line", []analytics.PropertyFilter{eq("sku", "X")}, "order_placed", nil, e.now.Add(-14*24*time.Hour), e.now, 2, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if ret.Cohort != 1 || ret.Buckets[1].Count != 1 {
		t.Fatalf("buyers of X = %d, back in week 1 = %d - want a, who came back", ret.Cohort, ret.Buckets[1].Count)
	}
}

// Where the people who dropped off went: who viewed X and did not add it to the cart after
func TestFlowFollowsOnlyThePeopleWhoDroppedOff(t *testing.T) {
	e := seedSales(t)
	defer e.st.Close()
	since := e.now.Add(-14 * 24 * time.Hour)
	all, err := e.st.Flow(context.Background(), e.ch, "view_item", []analytics.PropertyFilter{eq("sku", "X")}, nil, since, 3, 3)
	if err != nil || all.Cohort != 2 {
		t.Fatalf("viewers of X = %d, %v, want a and b", all.Cohort, err)
	}
	dropped, err := e.st.Flow(context.Background(), e.ch, "view_item", []analytics.PropertyFilter{eq("sku", "X")}, &analytics.FlowExclusion{Event: "cart_add", Within: 24 * time.Hour}, since, 3, 3)
	if err != nil || dropped.Cohort != 1 {
		t.Fatalf("viewers of X who didn't add it = %d, %v, want a alone - b added it within the day", dropped.Cohort, err)
	}
}

func TestAFunnelStepTakesTheOtherOperators(t *testing.T) {
	e := seedSales(t)
	defer e.st.Close()
	res, err := e.st.Funnel(context.Background(), e.ch, analytics.FunnelQuery{
		Steps:   []string{"view_item", "order_placed"},
		Filters: []analytics.FunnelFilter{{Step: 0, Key: "sku", Op: analytics.FilterIn, Values: []string{"MUG"}}},
	}, e.now.Add(-14*24*time.Hour))
	if err != nil || res.Steps[0].Count != 1 || res.Steps[1].Count != 1 {
		t.Fatalf("mug viewers to an order = %+v, %v, want c through both", res.Steps, err)
	}
	if _, err := e.st.Funnel(context.Background(), e.ch, analytics.FunnelQuery{Steps: []string{"a", "b"}, Filters: []analytics.FunnelFilter{{Step: 0, Key: "sku", Op: "like", Values: []string{"X"}}}}, e.now); !errors.Is(err, analytics.ErrBadFilter) {
		t.Fatalf("an unknown operator = %v, want ErrBadFilter", err)
	}
}
