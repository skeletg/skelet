package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// The first half of the reconciliation in docs/design/boards-cases.md: what a product sold and had refunded reaches
// the spine line by line, as many lines as the commerce DB holds. The second half - the board's net sales for the
// product equal to the cent - needs Insights' sum, and is in the engine branch
func TestEveryOrderLineAndRefundedLineReachesTheSpine(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 1000)
	sku := f.Product.SKU
	first := h.PlaceGuestCOD(t, f, 2)
	h.PlaceGuestCOD(t, f, 1)
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
	// The spine's ingest is asynchronous, so wait for it rather than sleep a fixed time
	deadline := time.Now().Add(30 * time.Second)
	for {
		ordered, refunded := countFor("order_line"), countFor("refund_line")
		if ordered == 2 && refunded == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("spine has %d order lines and %d refunded lines for %s, want 2 and 1 - as the commerce DB holds", ordered, refunded, sku)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
