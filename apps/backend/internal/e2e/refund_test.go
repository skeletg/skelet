package e2e

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/cart"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// Scenario: every refund is a numbered credit memo drawn against a per-target ceiling. The ceiling CLAMPS rather than errors - the
// shortfall lands on the memo as an adjustment fee so the document still adds up - and only a drained ledger refuses outright
func TestRefundMemoCeilingAndDerivedStatus(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 10, 2500)
	o := h.placeGuestCOD(t, f, 1)
	h.settle(t)
	if _, err := h.d.Order.RecordOfflinePayment(h.ctx, order.RecordOfflinePaymentInput{
		OrderID: o.ID, Method: "cod", Reference: "e2e-cash",
	}); err != nil {
		t.Fatalf("collect in full: %v", err)
	}

	intent, err := h.d.Order.RecordCreditMemoIntent(h.ctx, order.IssueCreditMemoInput{
		OrderID:               o.ID,
		AdjustmentRefundCents: 500,
	})
	if err != nil {
		t.Fatalf("first memo intent: %v", err)
	}
	if intent.AmountCents != 500 {
		t.Errorf("first memo amount = %d, want 500", intent.AmountCents)
	}
	if !strings.HasPrefix(intent.Memo.Number, "CM-") {
		t.Errorf("memo number = %q, want CM- prefix", intent.Memo.Number)
	}
	afterPartial, _, err := h.d.Order.FinalizeCreditMemo(h.ctx, intent.Txn.ID, true, "manual")
	if err != nil {
		t.Fatalf("finalize first memo: %v", err)
	}
	if afterPartial.PaymentStatus != "partially_refunded" {
		t.Errorf("payment_status after partial refund = %q, want partially_refunded", afterPartial.PaymentStatus)
	}

	// Ceiling is captured-minus-refunded per order: 2000 remains, so an absurd ask clamps to it
	// and the shortfall becomes the memo's adjustment fee
	over, err := h.d.Order.RecordCreditMemoIntent(h.ctx, order.IssueCreditMemoInput{
		OrderID:               o.ID,
		AdjustmentRefundCents: 9_999_999,
	})
	if err != nil {
		t.Fatalf("over-ceiling memo intent: %v", err)
	}
	if over.AmountCents != 2000 {
		t.Errorf("clamped amount = %d, want 2000", over.AmountCents)
	}
	if fee := over.Memo.AdjustmentFeeCents; fee != 9_999_999-2000 {
		t.Errorf("adjustment fee = %d, want the %d shortfall", fee, 9_999_999-2000)
	}
	afterFull, _, err := h.d.Order.FinalizeCreditMemo(h.ctx, over.Txn.ID, true, "manual")
	if err != nil {
		t.Fatalf("finalize second memo: %v", err)
	}
	if afterFull.PaymentStatus != "refunded" {
		t.Errorf("payment_status after full refund = %q, want refunded", afterFull.PaymentStatus)
	}

	// Drained ledger refuses a third memo outright
	if _, err := h.d.Order.RecordCreditMemoIntent(h.ctx, order.IssueCreditMemoInput{
		OrderID:               o.ID,
		AdjustmentRefundCents: 1,
	}); err == nil {
		t.Fatal("third memo on a fully refunded order succeeded")
	} else if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "CREDIT_MEMO_NOTHING_TO_REFUND" {
		t.Errorf("third memo error = %v, want code CREDIT_MEMO_NOTHING_TO_REFUND", err)
	}

	// Ledger accounting: capture and refund both at the order total
	sums, err := h.d.Queries.SumOrderPaymentByKind(h.ctx, o.ID)
	if err != nil {
		t.Fatalf("ledger sums: %v", err)
	}
	byKind := map[string]int64{}
	for _, s := range sums {
		byKind[s.Kind] = s.Total
	}
	if byKind[order.PaymentTxnCapture] != 2500 || byKind[order.PaymentTxnRefund] != 2500 {
		t.Errorf("ledger = %+v, want capture 2500 and refund 2500", byKind)
	}

	// Memo numbers are sequence-assigned and strictly increasing
	memos, err := h.d.Queries.ListCreditMemosByOrder(h.ctx, o.ID)
	if err != nil {
		t.Fatalf("list memos: %v", err)
	}
	if len(memos) != 2 || memos[0].Number >= memos[1].Number {
		t.Errorf("memos = %d rows %+v, want 2 with increasing numbers", len(memos), memos)
	}

	// Each finalize enqueued a durable order.refunded - deliver and confirm
	if total, _ := h.outboxRows(t, "order.refunded", o.ID.String()); total != 2 {
		t.Errorf("order.refunded outbox rows = %d, want 2", total)
	}
	h.settle(t)
	if _, undelivered := h.outboxRows(t, "order.refunded", o.ID.String()); undelivered != 0 {
		t.Errorf("%d order.refunded rows undelivered after settle", undelivered)
	}
}

// Scenario: line-level memos guard QUANTITY per line - you can refund the second unit after the
// first, but never more units than were ordered minus already credited
func TestRefundLineQuantityGuard(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 10, 1000)
	o := h.placeGuestCOD(t, f, 2)
	h.settle(t)
	if _, err := h.d.Order.RecordOfflinePayment(h.ctx, order.RecordOfflinePaymentInput{
		OrderID: o.ID, Method: "cod", Reference: "e2e-cash",
	}); err != nil {
		t.Fatalf("collect in full: %v", err)
	}
	lines, err := h.d.Queries.ListOrderLinesByOrder(h.ctx, o.ID)
	if err != nil || len(lines) != 1 {
		t.Fatalf("order lines = %d (%v), want 1", len(lines), err)
	}
	lineID := lines[0].ID

	first, err := h.d.Order.RecordCreditMemoIntent(h.ctx, order.IssueCreditMemoInput{
		OrderID: o.ID,
		Lines:   []order.CreditMemoLineInput{{OrderLineID: lineID, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("refund first unit: %v", err)
	}
	if first.AmountCents != 1000 {
		t.Errorf("one unit refunds %d, want 1000", first.AmountCents)
	}
	if _, _, err := h.d.Order.FinalizeCreditMemo(h.ctx, first.Txn.ID, true, "manual"); err != nil {
		t.Fatalf("finalize first unit: %v", err)
	}

	// Only one unit remains creditable - asking for two must refuse, not clamp: quantity is a
	// document fact, unlike money
	if _, err := h.d.Order.RecordCreditMemoIntent(h.ctx, order.IssueCreditMemoInput{
		OrderID: o.ID,
		Lines:   []order.CreditMemoLineInput{{OrderLineID: lineID, Quantity: 2}},
	}); err == nil {
		t.Fatal("credited 2 units with only 1 remaining")
	} else if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "CREDIT_MEMO_EXCEEDS_ORDERED" {
		t.Errorf("over-quantity error = %v, want code CREDIT_MEMO_EXCEEDS_ORDERED", err)
	}

	second, err := h.d.Order.RecordCreditMemoIntent(h.ctx, order.IssueCreditMemoInput{
		OrderID: o.ID,
		Lines:   []order.CreditMemoLineInput{{OrderLineID: lineID, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("refund second unit: %v", err)
	}
	final, _, err := h.d.Order.FinalizeCreditMemo(h.ctx, second.Txn.ID, true, "manual")
	if err != nil {
		t.Fatalf("finalize second unit: %v", err)
	}
	if final.PaymentStatus != "refunded" {
		t.Errorf("payment_status = %q after both units refunded, want refunded", final.PaymentStatus)
	}
	h.settle(t)
}

// Scenario: a gift-card-target refund measures against the TOTAL TENDERED, not captures - it can return money that was
// never collected through a gateway, which the original-target ceiling correctly refuses
func TestRefundGiftCardTargetCeilingIsTendered(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 5, 2000)
	o := h.placeGuestCOD(t, f, 1) // nothing ever collected
	h.settle(t)

	if _, err := h.d.Order.RecordCreditMemoIntent(h.ctx, order.IssueCreditMemoInput{
		OrderID: o.ID, AdjustmentRefundCents: 100,
	}); err == nil {
		t.Fatal("original-target memo succeeded with zero captured")
	} else if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "CREDIT_MEMO_NOTHING_TO_REFUND" {
		t.Errorf("zero-captured memo error = %v, want CREDIT_MEMO_NOTHING_TO_REFUND", err)
	}

	intent, err := h.d.Order.RecordCreditMemoIntent(h.ctx, order.IssueCreditMemoInput{
		OrderID: o.ID, AdjustmentRefundCents: 999_999, RefundTarget: "gift_card",
	})
	if err != nil {
		t.Fatalf("gift-card-target memo: %v", err)
	}
	if intent.AmountCents != o.TotalAmount {
		t.Errorf("gift-card-target clamped to %d, want the tendered total %d", intent.AmountCents, o.TotalAmount)
	}
	updated, _, err := h.d.Order.FinalizeCreditMemo(h.ctx, intent.Txn.ID, true, "manual")
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if updated.PaymentStatus != "refunded" {
		t.Errorf("payment_status = %q, want refunded - full total returned as a card", updated.PaymentStatus)
	}
	sums := h.ledgerByKind(t, o.ID)
	if sums[order.PaymentTxnRefund] != o.TotalAmount || sums[order.PaymentTxnCapture] != 0 {
		t.Errorf("ledger = %+v, want refund %d against zero captures", sums, o.TotalAmount)
	}
	h.settle(t)
}

// Scenario: line memos stay per-line on a two-line order - draining one line's quantity leaves
// the other line refundable, and the statuses derive across both
func TestLineMemosAcrossTwoLines(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 10, 800)
	second := h.addProduct(t, f, 600, true, 10)
	c, err := h.d.Cart.CreateGuestCart(h.ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("cart: %v", err)
	}
	for _, sku := range []string{f.Product.SKU, second.SKU} {
		if _, err := h.d.Cart.AddToCart(h.ctx, c.ID, nil, cart.AddCartLineInput{ProductSku: sku, Quantity: 1}); err != nil {
			t.Fatalf("add %s: %v", sku, err)
		}
	}
	o, err := h.tryPlace(t, f.Channel, c.ID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	h.settle(t)
	if _, err := h.d.Order.RecordOfflinePayment(h.ctx, order.RecordOfflinePaymentInput{
		OrderID: o.ID, Method: "cod", Reference: "e2e-cash",
	}); err != nil {
		t.Fatalf("collect: %v", err)
	}

	var lineA, lineB uuid.UUID
	for _, l := range h.orderLines(t, o.ID) {
		switch l.ProductSku {
		case f.Product.SKU:
			lineA = l.ID
		case second.SKU:
			lineB = l.ID
		}
	}

	first, err := h.d.Order.RecordCreditMemoIntent(h.ctx, order.IssueCreditMemoInput{
		OrderID: o.ID, Lines: []order.CreditMemoLineInput{{OrderLineID: lineA, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("memo line A: %v", err)
	}
	if first.AmountCents != 800 {
		t.Errorf("line A memo = %d, want its 800", first.AmountCents)
	}
	partial, _, err := h.d.Order.FinalizeCreditMemo(h.ctx, first.Txn.ID, true, "manual")
	if err != nil {
		t.Fatalf("finalize A: %v", err)
	}
	if partial.PaymentStatus != "partially_refunded" {
		t.Errorf("after line A: %q, want partially_refunded", partial.PaymentStatus)
	}

	// Line A is drained - only line B still credits
	if _, err := h.d.Order.RecordCreditMemoIntent(h.ctx, order.IssueCreditMemoInput{
		OrderID: o.ID, Lines: []order.CreditMemoLineInput{{OrderLineID: lineA, Quantity: 1}},
	}); err == nil {
		t.Fatal("credited line A past its quantity")
	} else if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "CREDIT_MEMO_EXCEEDS_ORDERED" {
		t.Errorf("drained line error = %v, want CREDIT_MEMO_EXCEEDS_ORDERED", err)
	}
	last, err := h.d.Order.RecordCreditMemoIntent(h.ctx, order.IssueCreditMemoInput{
		OrderID: o.ID, Lines: []order.CreditMemoLineInput{{OrderLineID: lineB, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("memo line B: %v", err)
	}
	full, _, err := h.d.Order.FinalizeCreditMemo(h.ctx, last.Txn.ID, true, "manual")
	if err != nil {
		t.Fatalf("finalize B: %v", err)
	}
	if full.PaymentStatus != "refunded" {
		t.Errorf("after both lines: %q, want refunded", full.PaymentStatus)
	}
	h.settle(t)
}
