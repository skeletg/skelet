package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// Scenario: the full RMA arc - request against shipped quantity only, approve, receive with restock, refund through the return's own memo -
// with every idempotency layer replayed: the restock marker, the return-keyed refund intent, and the one-memo-per-return index
func TestRMAFullCycle(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2500)
	o := h.PlaceGuestCOD(t, f, 2)
	h.Settle(t)
	if _, err := h.D.Order.RecordOfflinePayment(h.Ctx, order.RecordOfflinePaymentInput{
		OrderID: o.ID, Method: "cod", Reference: "e2e-cash",
	}); err != nil {
		t.Fatalf("collect: %v", err)
	}

	// Returns only exist for shipped goods
	lines := h.OrderLines(t, o.ID)
	if _, err := h.D.Order.RequestReturn(h.Ctx, order.RequestReturnInput{
		OrderID: o.ID,
		Lines:   []order.ReturnLineInput{{OrderLineID: lines[0].ID, Quantity: 1}},
	}); err == nil {
		t.Fatal("return accepted on an unshipped order")
	} else if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "RETURN_ORDER_NOT_SHIPPED" {
		t.Errorf("unshipped return error = %v, want RETURN_ORDER_NOT_SHIPPED", err)
	}

	for _, status := range []string{"confirmed", "processing"} {
		if _, err := h.D.Order.UpdateStatus(h.Ctx, o.ID, status); err != nil {
			t.Fatalf("advance to %s: %v", status, err)
		}
	}
	if _, err := h.D.Order.CreateFulfillment(h.Ctx, order.CreateFulfillmentInput{
		OrderID: o.ID,
		Lines:   []order.FulfillmentLineInput{{OrderLineID: lines[0].ID, Quantity: 2}},
		Carrier: "ups", TrackingNumber: "1Z-e2e",
	}); err != nil {
		t.Fatalf("fulfill: %v", err)
	}
	h.Settle(t) // inventory settles the order_fulfilled rows RestockReturn draws on

	levels, err := h.D.Queries.ListInventoryLevelsByItem(h.Ctx, f.Item.ID)
	if err != nil || len(levels) != 1 {
		t.Fatalf("levels: %v", err)
	}
	shipped := levels[0].Quantity // the post-fulfillment baseline restock is measured against

	// More than shipped must refuse - the ceiling is shipped minus already returned
	if _, err := h.D.Order.RequestReturn(h.Ctx, order.RequestReturnInput{
		OrderID: o.ID,
		Lines:   []order.ReturnLineInput{{OrderLineID: lines[0].ID, Quantity: 3}},
	}); err == nil {
		t.Fatal("returned 3 of 2 shipped units")
	} else if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "RETURN_EXCEEDS_SHIPPED" {
		t.Errorf("over-return error = %v, want RETURN_EXCEEDS_SHIPPED", err)
	}

	ret, err := h.D.Order.RequestReturn(h.Ctx, order.RequestReturnInput{
		OrderID: o.ID,
		Lines:   []order.ReturnLineInput{{OrderLineID: lines[0].ID, Quantity: 1, Reason: "damaged"}},
		Restock: true, // what makes receipt durable and the stock come home
	})
	if err != nil {
		t.Fatalf("request return: %v", err)
	}

	// Receiving skips no steps
	if _, err := h.D.Order.ReceiveReturn(h.Ctx, ret.ID); err == nil {
		t.Fatal("received a return that was never approved")
	} else if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "RETURN_INVALID_TRANSITION" {
		t.Errorf("receive-before-approve error = %v, want RETURN_INVALID_TRANSITION", err)
	}
	if _, err := h.D.Order.ApproveReturn(h.Ctx, ret.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := h.D.Order.ReceiveReturn(h.Ctx, ret.ID); err != nil {
		t.Fatalf("receive: %v", err)
	}
	if total, _ := h.OutboxRows(t, "order.return_received", ret.ID.String()); total != 1 {
		t.Errorf("return_received outbox rows = %d, want 1 - the restock rides on it", total)
	}
	h.Settle(t)

	after, err := h.D.Queries.ListInventoryLevelsByItem(h.Ctx, f.Item.ID)
	if err != nil {
		t.Fatalf("levels after restock: %v", err)
	}
	if got := after[0].Quantity - shipped; got != 1 {
		t.Errorf("restock moved stock by %v, want +1", got)
	}

	// A replayed receipt must find the marker row and restock nothing
	h.RearmOutbox(t, "order.return_received", ret.ID.String(), 1)
	h.Settle(t)
	replayed, _ := h.D.Queries.ListInventoryLevelsByItem(h.Ctx, f.Item.ID)
	if replayed[0].Quantity != after[0].Quantity {
		t.Errorf("replayed receipt restocked again: %v -> %v", after[0].Quantity, replayed[0].Quantity)
	}

	// Refund through the return: one unit at its snapshot price
	// The refund basis is the return line's discount-allocated unit snapshot, not the list price
	rlines, err := h.D.Queries.ListOrderReturnLinesByReturnIDs(h.Ctx, []uuid.UUID{ret.ID})
	if err != nil || len(rlines) != 1 {
		t.Fatalf("return lines: %v", err)
	}
	unit := rlines[0].RefundUnitCents
	intent, err := h.D.Order.RecordReturnRefundIntent(h.Ctx, ret.ID, "rma refund")
	if err != nil {
		t.Fatalf("refund intent: %v", err)
	}
	if intent.AmountCents != unit {
		t.Errorf("refund intent = %d, want the %d unit snapshot", intent.AmountCents, unit)
	}
	updated, refunded, err := h.D.Order.FinalizeReturnRefund(h.Ctx, intent.Txn.ID, ret.ID, true, "manual", nil)
	if err != nil {
		t.Fatalf("finalize refund: %v", err)
	}
	if refunded.Status != "refunded" || updated.PaymentStatus != "partially_refunded" {
		t.Errorf("return/payment = %q/%q, want refunded/partially_refunded", refunded.Status, updated.PaymentStatus)
	}

	memo, err := h.D.Queries.GetCreditMemoByReturn(h.Ctx, uuid.NullUUID{UUID: ret.ID, Valid: true})
	if err != nil {
		t.Fatalf("return memo: %v", err)
	}

	// Replaying the finalize must land on the same memo, not a second one
	if _, _, err := h.D.Order.FinalizeReturnRefund(h.Ctx, intent.Txn.ID, ret.ID, true, "manual", nil); err != nil {
		t.Fatalf("replayed finalize: %v", err)
	}
	memos, err := h.D.Queries.ListCreditMemosByOrder(h.Ctx, o.ID)
	if err != nil || len(memos) != 1 || memos[0].Number != memo.Number {
		t.Errorf("memos after replayed finalize = %d, want exactly the one %s", len(memos), memo.Number)
	}

	// And a fresh intent on the now-refunded return must refuse
	if _, err := h.D.Order.RecordReturnRefundIntent(h.Ctx, ret.ID, "again"); err == nil {
		t.Fatal("second refund intent on a refunded return succeeded")
	} else if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "RETURN_NOT_RECEIVED" {
		t.Errorf("re-refund error = %v, want RETURN_NOT_RECEIVED", err)
	}

	row, err := h.D.Queries.GetOrderByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read order: %v", err)
	}
	if sums := h.LedgerByKind(t, o.ID); sums[order.PaymentTxnRefund] != unit || sums[order.PaymentTxnCapture] != row.TotalAmount {
		t.Errorf("ledger = %+v, want capture %d / refund %d", sums, row.TotalAmount, unit)
	}
	h.Settle(t)
}

// Scenario: an exchange-resolution return spawns a zero-priced draft exchange order on receipt
// and refuses to become money
func TestRMAExchangeCreatesDraftAndRefusesRefund(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1500)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	if _, err := h.D.Order.RecordOfflinePayment(h.Ctx, order.RecordOfflinePaymentInput{
		OrderID: o.ID, Method: "cod", Reference: "e2e-cash",
	}); err != nil {
		t.Fatalf("collect: %v", err)
	}
	for _, status := range []string{"confirmed", "processing"} {
		if _, err := h.D.Order.UpdateStatus(h.Ctx, o.ID, status); err != nil {
			t.Fatalf("advance to %s: %v", status, err)
		}
	}
	lines := h.OrderLines(t, o.ID)
	if _, err := h.D.Order.CreateFulfillment(h.Ctx, order.CreateFulfillmentInput{
		OrderID: o.ID,
		Lines:   []order.FulfillmentLineInput{{OrderLineID: lines[0].ID, Quantity: 1}},
	}); err != nil {
		t.Fatalf("fulfill: %v", err)
	}
	h.Settle(t)

	ret, err := h.D.Order.RequestReturn(h.Ctx, order.RequestReturnInput{
		OrderID:    o.ID,
		Lines:      []order.ReturnLineInput{{OrderLineID: lines[0].ID, Quantity: 1}},
		Resolution: "exchange",
	})
	if err != nil {
		t.Fatalf("request exchange: %v", err)
	}
	if _, err := h.D.Order.ApproveReturn(h.Ctx, ret.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := h.D.Order.ReceiveReturn(h.Ctx, ret.ID); err != nil {
		t.Fatalf("receive: %v", err)
	}
	h.Settle(t)

	got, err := h.D.Order.GetReturn(h.Ctx, ret.ID)
	if err != nil {
		t.Fatalf("re-read return: %v", err)
	}
	if got.ExchangeOrderID == nil {
		t.Fatal("exchange return has no draft exchange order")
	}
	draft, err := h.D.Queries.GetOrderByID(h.Ctx, *got.ExchangeOrderID)
	if err != nil {
		t.Fatalf("draft order: %v", err)
	}
	if draft.Status != "draft" {
		t.Errorf("exchange order status = %q, want draft", draft.Status)
	}

	if _, err := h.D.Order.RecordReturnRefundIntent(h.Ctx, ret.ID, "no"); err == nil {
		t.Fatal("refund intent succeeded on an exchange return")
	} else if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "RETURN_IS_EXCHANGE" {
		t.Errorf("exchange refund error = %v, want RETURN_IS_EXCHANGE", err)
	}
	h.Settle(t)
}
