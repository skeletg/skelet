package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// The reconciliation in docs/design/boards-cases.md: what a product sold and had refunded reaches the spine line by
// line, as many lines as the commerce DB holds, and a board's net sales for it - Insights' sum of net_base_cents over
// the line events, held to its sku - equals the commerce DB's to the cent
func TestAProductsNetSalesOnTheSpineEqualTheCommerceDB(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 1000)
	sku := f.Product.SKU
	first := h.PlaceGuestCOD(t, f, 2)
	second := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	if _, err := h.D.Order.RecordOfflinePayment(h.Ctx, order.RecordOfflinePaymentInput{OrderID: first.ID, Method: "cod", Reference: "e2e-cash"}); err != nil {
		t.Fatalf("collect: %v", err)
	}
	lines, err := h.D.Queries.ListOrderLinesByOrder(h.Ctx, first.ID)
	if err != nil || len(lines) != 1 {
		t.Fatalf("order lines = %d (%v)", len(lines), err)
	}
	intent, err := h.D.Order.RecordCreditMemoIntent(h.Ctx, order.IssueCreditMemoInput{
		OrderID: first.ID, Lines: []order.CreditMemoLineInput{{OrderLineID: lines[0].ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("refund one: %v", err)
	}
	if _, _, err := h.D.Order.FinalizeCreditMemo(h.Ctx, intent.Txn.ID, true, "manual"); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	h.Settle(t)

	adm, err := h.D.Admin.CreateAdmin(h.Ctx, "sales-"+uuid.NewString()[:8]+"@e2e.test", "sales-e2e-password-1", "Sales", "Admin", []string{"administrator"})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	actx, err := h.D.SignalsClient.AsAdmin(h.Ctx, adm.ID)
	if err != nil {
		t.Fatalf("admin token: %v", err)
	}
	countFor := func(event string) int32 {
		res, err := h.D.SignalsClient.Synapse.GetInsights(actx, &signalsv1.GetInsightsRequest{ChannelId: f.Channel.ID.String(), Event: event, Breakdown: "sku", Measure: "count", Unit: "day", SinceDays: 7})
		if err != nil {
			t.Fatalf("insights %s: %v", event, err)
		}
		for _, s := range res.GetSeries() {
			if s.GetKey() == sku {
				return s.GetTotal()
			}
		}
		return 0
	}
	// The commerce side, through the same allocation the refund itself used: both orders' X lines, less the refunded unit
	var want int64
	for _, id := range []uuid.UUID{first.ID, second.ID} {
		o, err := h.D.Order.GetByID(h.Ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		for lineID, net := range order.LineNet(o) {
			if l := lineOf(o, lineID); l.ProductSku == sku {
				want += net
			}
		}
	}
	paid, err := h.D.Order.GetByID(h.Ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	want -= order.LineNetFor(paid, lines[0].ID, 1)
	if want <= 0 {
		t.Fatalf("expected net sales %d - the fixture sold nothing", want)
	}
	netSales := func() int64 {
		res, err := h.D.SignalsClient.Synapse.GetInsights(actx, &signalsv1.GetInsightsRequest{
			ChannelId: f.Channel.ID.String(), Events: []string{"order_line", "order_line_adjusted", "refund_line"},
			Measure: "sum", SumKey: "net_base_cents", Unit: "day", SinceDays: 7,
			Filters: []*signalsv1.PropertyFilter{{Key: "sku", Op: "eq", Values: []string{sku}}},
		})
		if err != nil {
			t.Fatalf("net sales: %v", err)
		}
		return res.GetTotalAmount()
	}

	// The spine's ingest is asynchronous, so wait for it rather than sleep a fixed time
	deadline := time.Now().Add(30 * time.Second)
	for {
		ordered, refunded := countFor("order_line"), countFor("refund_line")
		if ordered == 2 && refunded == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("spine has %d order lines and %d refunded lines for %s, want 2 and 1 - as the commerce DB holds", ordered, refunded, sku)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if got := netSales(); got != want {
		t.Fatalf("the board's net sales for %s = %d, the commerce DB's = %d - exact, in cents", sku, got, want)
	}
}

func lineOf(o order.Order, id uuid.UUID) order.OrderLine {
	for _, l := range o.Lines {
		if l.ID == id {
			return l
		}
	}
	return order.OrderLine{}
}
