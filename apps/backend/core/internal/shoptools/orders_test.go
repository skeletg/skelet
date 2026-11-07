package shoptools

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
)

var (
	olderOrder, newerOrder, pendingOrder = uuid.NewString(), uuid.NewString(), uuid.NewString()
	shirtLine, mugLine                   = uuid.NewString(), uuid.NewString()
	signedIn                             = uuid.New()
)

func orderSummaryJSON(id, number, status, at string) map[string]any {
	return map[string]any{"id": id, "number": number, "status": status, "paymentStatus": "PAID", "total": 4500, "currencyCode": "EUR",
		"createdAt": at, "carrier": "DHL", "trackingNumber": "JD" + number, "lines": []any{map[string]any{"quantity": 2}, map[string]any{"quantity": 1}}}
}

// orderRig answers as the front API would for a shopper with three orders: 1002 shipped with two shirts still returnable,
// 1003 not shipped yet. Sent returns land in sent
func orderRig(model *fakeModel, sent *[]map[string]any) (*Service, *fakeGraphQL) {
	svc := assistantRig(model, nil)
	gql := svc.deps.GraphQL.(*fakeGraphQL)
	gql.answers["AgentMyOrders"] = func(map[string]any) (any, error) {
		return map[string]any{"myOrders": []any{
			orderSummaryJSON(olderOrder, "1001", "FULFILLED", "2026-10-01T10:00:00Z"),
			orderSummaryJSON(pendingOrder, "1003", "PROCESSING", "2026-11-03T10:00:00Z"),
			orderSummaryJSON(newerOrder, "1002", "FULFILLED", "2026-11-01T10:00:00Z"),
		}}, nil
	}
	gql.answers["AgentMyOrder"] = func(vars map[string]any) (any, error) {
		status, returnable := "FULFILLED", []any{map[string]any{"orderLineId": shirtLine, "quantity": 2}}
		if vars["id"] == pendingOrder {
			status, returnable = "PROCESSING", []any{}
		}
		return map[string]any{"myOrder": map[string]any{
			"id": vars["id"], "number": map[any]string{newerOrder: "1002", pendingOrder: "1003", olderOrder: "1001"}[vars["id"]], "status": status, "paymentStatus": "PAID",
			"total": 4500, "subtotal": 4000, "shippingAmount": 500, "discountAmount": 0, "taxAmount": 0, "amountDue": 0, "currencyCode": "EUR",
			"createdAt": "2026-11-01T10:00:00Z", "carrier": "DHL", "trackingNumber": "JD1002", "trackingUrl": "https://track.example/JD1002",
			"shippingAddress": map[string]any{"city": "Berlin", "countryCode": "DE"},
			"lines": []any{
				map[string]any{"id": shirtLine, "name": "Blue shirt", "sku": "SHIRT-M", "quantity": 2, "totalPrice": 3000, "isGift": false},
				map[string]any{"id": mugLine, "name": "Mug", "sku": "MUG", "quantity": 1, "totalPrice": 1000, "isGift": false},
			},
			"events":          []any{map[string]any{"type": "shipped", "message": "Handed to DHL", "createdAt": "2026-11-02T09:00:00Z"}},
			"returns":         returnsOf(vars["id"], *sent),
			"returnableLines": returnable,
		}}, nil
	}
	gql.answers["AgentRequestReturn"] = func(vars map[string]any) (any, error) {
		*sent = append(*sent, vars["input"].(map[string]any))
		return map[string]any{"requestReturn": map[string]any{"rmaNumber": "RMA-7", "status": "requested", "resolution": "refund"}}, nil
	}
	return svc, gql
}

// RMA-1 for the mug, and the returns the assistant sent since, as the store would list them
func returnsOf(order any, sent []map[string]any) []any {
	out := []any{map[string]any{"rmaNumber": "RMA-1", "status": "requested", "resolution": "refund", "createdAt": "2026-11-03T09:00:00Z", "lines": []any{map[string]any{"orderLineId": mugLine, "quantity": 1}}}}
	for _, in := range sent {
		if in["orderId"] == order {
			var lines []any
			for _, l := range in["lines"].([]map[string]any) {
				lines = append(lines, map[string]any{"orderLineId": l["orderLineId"], "quantity": l["quantity"]})
			}
			out = append(out, map[string]any{"rmaNumber": "RMA-7", "status": "requested", "resolution": in["resolution"], "createdAt": time.Now().UTC().Format(time.RFC3339), "lines": lines})
		}
	}
	return out
}

func member() Shopper { return Shopper{Address: "1.2.3.4", Customer: &signedIn} }

func TestOrderToolsAnswerOnlyASignedInShopper(t *testing.T) {
	model := &fakeModel{calls: []ai.ToolCall{toolCall("get_my_orders", map[string]any{}), toolCall("get_my_order", map[string]any{"order": "1002"})}}
	var sent []map[string]any
	svc, gql := orderRig(model, &sent)
	if _, err := svc.Assist(context.Background(), shop, guest(nil), ask, "", Notify{}); err != nil {
		t.Fatal(err)
	}
	for _, r := range model.results {
		if !r.IsError || r.Content != notSignedIn {
			t.Errorf("a guest got %+v - want to be asked to sign in", r)
		}
	}
	if slices.Contains(gql.ops(), "AgentMyOrders") {
		t.Error("asked the store for a guest's orders")
	}
}

func TestTheAssistantReadsTheShoppersOrders(t *testing.T) {
	var steps []Step
	model := &fakeModel{calls: []ai.ToolCall{toolCall("get_my_orders", map[string]any{}), toolCall("get_my_order", map[string]any{"order": "#1002"})}}
	var sent []map[string]any
	svc, _ := orderRig(model, &sent)
	turn, err := svc.Assist(context.Background(), shop, member(), ask, "", Notify{Step: func(s Step) { steps = append(steps, s) }})
	if err != nil {
		t.Fatal(err)
	}
	list, detail := model.results[0].Content, model.results[1].Content
	if i, j, k := strings.Index(list, "Order 1003"), strings.Index(list, "Order 1002"), strings.Index(list, "Order 1001"); i < 0 || i > j || j > k {
		t.Errorf("want newest first:\n%s", list)
	}
	for _, want := range []string{"shipped, paid", "2 × Blue shirt [sku SHIRT-M, line " + shirtLine + "]", "Tracking: DHL JD1002 - https://track.example/JD1002",
		"Going to Berlin, DE", "Handed to DHL", "Return RMA-1", "1 × Mug", "up to 2 × Blue shirt [line " + shirtLine + "]"} {
		if !strings.Contains(detail, want) {
			t.Errorf("order detail lacks %q:\n%s", want, detail)
		}
	}
	if len(turn.Links) != 1 || turn.Links[0].Path != orderPagePath+newerOrder || turn.Links[0].Label != "#1002" {
		t.Errorf("links %+v - want a button to the order's page", turn.Links)
	}
	if len(steps) != 2 || steps[1] != (Step{Tool: "get_my_order", Detail: "1002"}) {
		t.Errorf("steps %+v", steps)
	}

	model.calls = []ai.ToolCall{toolCall("get_my_order", map[string]any{"order": "999"})}
	if _, err := svc.Assist(context.Background(), shop, member(), ask, "", Notify{}); err != nil || !model.results[0].IsError || !strings.Contains(model.results[0].Content, "no order \"999\"") {
		t.Errorf("an order that isn't theirs: %+v %v", model.results, err)
	}
}

var shirtBack = map[string]any{"order": "1002", "lines": []map[string]any{{"line_id": shirtLine, "quantity": 1}}, "reason": "Too small"}

func withConfirmed(in map[string]any) map[string]any {
	out := maps.Clone(in)
	out["confirmed"] = true
	return out
}

func TestAReturnIsSentOnlyOnceTheShopperSaidYes(t *testing.T) {
	ctx := context.Background()
	var sent []map[string]any
	model := &fakeModel{}
	svc, _ := orderRig(model, &sent)
	turnWith := func(history []Message, calls ...ai.ToolCall) Turn {
		t.Helper()
		model.calls = calls
		turn, err := svc.Assist(ctx, shop, member(), history, "", Notify{})
		if err != nil {
			t.Fatal(err)
		}
		return turn
	}

	turnWith(ask, toolCall("start_return", withConfirmed(shirtBack)))
	if len(sent) != 0 || !strings.Contains(model.results[0].Content, "hasn't confirmed") {
		t.Fatalf("confirmed straight away: %+v, sent %v", model.results, sent)
	}

	summary := turnWith(ask, toolCall("start_return", shirtBack), toolCall("start_return", withConfirmed(shirtBack)))
	if !strings.Contains(model.results[0].Content, "nothing is sent yet") || !strings.Contains(model.results[0].Content, "1 × Blue shirt (sku SHIRT-M)") {
		t.Errorf("summary:\n%s", model.results[0].Content)
	}
	if len(sent) != 0 || !model.results[1].IsError {
		t.Fatalf("confirmed in the turn that summarised it - the shopper hasn't answered yet: %v", sent)
	}
	if !strings.HasPrefix(summary.Recap, "Return R") || !strings.Contains(summary.Recap, "summarised for order 1002") {
		t.Fatalf("recap %q", summary.Recap)
	}

	agreed := []Message{ask[0], {Role: "assistant", Content: "Shall I send it?", Recap: summary.Recap}, {Role: "user", Content: "Yes please"}}
	changed := withConfirmed(shirtBack)
	changed["lines"] = []map[string]any{{"line_id": shirtLine, "quantity": 2}}
	turnWith(agreed, toolCall("start_return", changed))
	if len(sent) != 0 {
		t.Fatal("sent a return that differs from the one the shopper agreed to")
	}

	done := turnWith(agreed, toolCall("start_return", withConfirmed(shirtBack)), toolCall("start_return", withConfirmed(shirtBack)))
	if len(sent) != 1 || !strings.Contains(model.results[0].Content, "Return RMA-7 is in for order 1002") || !model.results[1].IsError {
		t.Fatalf("sent %v, results %+v - want it sent once", sent, model.results)
	}
	got := sent[0]
	if got["orderId"] != newerOrder || got["resolution"] != "refund" || got["reason"] != "Too small" {
		t.Errorf("sent %v", got)
	}
	if lines := got["lines"].([]map[string]any); len(lines) != 1 || lines[0]["orderLineId"] != shirtLine || lines[0]["quantity"] != 1.0 {
		t.Errorf("lines %v", lines)
	}
	if !strings.Contains(done.Recap, "sent as RMA-7") || len(done.Links) != 1 {
		t.Errorf("recap %q, links %+v", done.Recap, done.Links)
	}

	turnWith(agreed, toolCall("start_return", withConfirmed(shirtBack)))
	if len(sent) != 1 || !strings.Contains(model.results[0].Content, "already in as RMA-7") {
		t.Errorf("the reply saying it was sent got lost and the shopper said yes again: sent %d, %+v", len(sent), model.results)
	}
	both := maps.Clone(shirtBack)
	both["lines"] = []map[string]any{{"line_id": shirtLine, "quantity": 2}}
	older := turnWith(ask, toolCall("start_return", both))
	stale := []Message{ask[0], {Role: "assistant", Content: "Shall I?", Recap: older.Recap}, {Role: "user", Content: "Which size runs big?"},
		{Role: "assistant", Content: "The M.", Recap: "Showed product x"}, {Role: "user", Content: "Yes"}}
	turnWith(stale, toolCall("start_return", withConfirmed(both)))
	if len(sent) != 1 || !strings.Contains(model.results[0].Content, "hasn't confirmed") {
		t.Errorf("a yes confirmed a summary from replies ago: sent %d, %+v", len(sent), model.results)
	}

	later := append(slices.Clone(agreed), Message{Role: "assistant", Content: "Done.", Recap: done.Recap}, Message{Role: "user", Content: "Thanks"})
	turnWith(later, toolCall("start_return", withConfirmed(shirtBack)))
	if len(sent) != 1 || !strings.Contains(model.results[0].Content, "already sent") {
		t.Errorf("a later turn sent it again: %v", sent)
	}
}

func TestAReturnSticksToWhatCanGoBack(t *testing.T) {
	var sent []map[string]any
	model := &fakeModel{}
	svc, _ := orderRig(model, &sent)
	for name, tc := range map[string]struct {
		in   map[string]any
		want string
	}{
		"more than shipped": {map[string]any{"order": "1002", "lines": []map[string]any{{"line_id": shirtLine, "quantity": 3}}}, "At most 2 × Blue shirt"},
		"already returned":  {map[string]any{"order": "1002", "lines": []map[string]any{{"line_id": mugLine, "quantity": 1}}}, "Mug can't be sent back"},
		"not on the order":  {map[string]any{"order": "1002", "lines": []map[string]any{{"line_id": "nope", "quantity": 1}}}, "has no line"},
		"twice":             {map[string]any{"order": "1002", "lines": []map[string]any{{"line_id": shirtLine, "quantity": 1}, {"line_id": shirtLine, "quantity": 1}}}, "goes in once"},
		"not shipped":       {map[string]any{"order": "1003", "lines": []map[string]any{{"line_id": shirtLine, "quantity": 1}}}, "until it ships"},
		"odd resolution":    {map[string]any{"order": "1002", "resolution": "store credit", "lines": []map[string]any{{"line_id": shirtLine, "quantity": 1}}}, "refund or exchange"},
	} {
		model.calls = []ai.ToolCall{toolCall("start_return", tc.in)}
		if _, err := svc.Assist(context.Background(), shop, member(), ask, "", Notify{}); err != nil {
			t.Fatal(err)
		}
		if r := model.results[0]; !r.IsError || !strings.Contains(r.Content, tc.want) {
			t.Errorf("%s: %+v, want %q", name, r, tc.want)
		}
	}
}

func TestTheReturnKeyNamesOneExactReturn(t *testing.T) {
	one, two := 1.0, 2.0
	a := []lineArg{{LineID: "a", Quantity: &one}, {LineID: "b", Quantity: &two, Reason: "too  big"}}
	b := []lineArg{{LineID: "b", Quantity: &two, Reason: "too big"}, {LineID: "a", Quantity: &one}}
	if returnKey("o", a, "refund", "x") != returnKey("o", b, "refund", " x ") {
		t.Error("the same return in another order or spacing should keep its key")
	}
	for name, k := range map[string]string{
		"quantity": returnKey("o", []lineArg{{LineID: "a", Quantity: &two}, {LineID: "b", Quantity: &two, Reason: "too big"}}, "refund", "x"),
		"exchange": returnKey("o", a, "exchange", "x"), "reason": returnKey("o", a, "refund", "y"), "order": returnKey("p", a, "refund", "x"),
	} {
		if k == returnKey("o", a, "refund", "x") {
			t.Errorf("a different %s kept the key", name)
		}
	}
}
