package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

func (h *harness) newDraft(t *testing.T, f *store, lines []order.CreateDraftLine, mutate ...func(*order.CreateDraftOrderInput)) order.Order {
	t.Helper()
	in := order.CreateDraftOrderInput{
		ChannelID:       f.Channel.ID,
		Email:           "draft-" + uuid.NewString()[:8] + "@example.invalid",
		BillingAddress:  *e2eAddress(),
		ShippingAddress: *e2eAddress(),
		Lines:           lines,
	}
	for _, m := range mutate {
		m(&in)
	}
	o, err := h.d.Order.CreateDraftOrder(h.ctx, in)
	if err != nil {
		t.Fatalf("CreateDraftOrder: %v", err)
	}
	return o
}

// Scenario: an admin-built draft prices like a real order but holds NO stock - reserving at draft time would let an unfinished quote sit on inventory the storefront could have sold
func TestDraftOrderPricesWithoutHoldingStock(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 20, 2500)

	d := h.newDraft(t, f, []order.CreateDraftLine{{ProductSku: f.Product.SKU, Quantity: 3}})

	if d.Status != order.OrderStatusDraft {
		t.Errorf("status = %q, want draft", d.Status)
	}
	if d.SubtotalAmount != 3*2500 {
		t.Errorf("subtotal = %d, want %d priced from the channel listing", d.SubtotalAmount, 3*2500)
	}
	if d.TotalAmount != d.SubtotalAmount-d.DiscountAmount+d.TaxAmount+d.ShippingAmount {
		t.Errorf("total %d does not equal its own breakdown", d.TotalAmount)
	}
	if got := h.reservedFor(t, f); got != 0 {
		t.Errorf("reserved while still a draft = %v, want 0", got)
	}
	h.settle(t)
}

// Placing is what takes the stock - and it flips draft→pending UNPAID, since manual orders are collected afterwards through the ledger
func TestDraftOrderPlacementReservesAndLandsUnpaid(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 20, 2500)
	d := h.newDraft(t, f, []order.CreateDraftLine{{ProductSku: f.Product.SKU, Quantity: 3}})

	placed, err := h.d.Order.PlaceDraftOrder(h.ctx, d.ID)
	if err != nil {
		t.Fatalf("PlaceDraftOrder: %v", err)
	}
	h.settle(t)

	if placed.Status != order.OrderStatusPending {
		t.Errorf("status = %q, want pending", placed.Status)
	}
	if placed.PaymentStatus == "paid" {
		t.Errorf("payment_status = %q, want an unpaid draft placement", placed.PaymentStatus)
	}
	if got := h.reservedFor(t, f); got != 3 {
		t.Errorf("reserved after placing = %v, want 3", got)
	}
	if got := h.eventTypes(t, d.ID)["order_placed"]; got == 0 {
		t.Error("no order_placed event on the timeline")
	}
	// the placement has to reach subscribers the same way a storefront order does
	if total, _ := h.outboxRows(t, "order.placed", d.ID.String()); total == 0 {
		t.Error("no order.placed outbox row, so downstream side effects never run for manual orders")
	}
}

// A shortage at placement has to leave the draft intact and untouched - the admin fixes stock and retries, rather than finding a half-placed order
func TestDraftOrderPlacementShortageLeavesTheDraftIntact(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 2, 2500)
	d := h.newDraft(t, f, []order.CreateDraftLine{{ProductSku: f.Product.SKU, Quantity: 9}})

	if _, err := h.d.Order.PlaceDraftOrder(h.ctx, d.ID); err == nil {
		t.Fatal("placing a draft past available stock succeeded")
	}

	reread, err := h.d.Order.GetByID(h.ctx, d.ID)
	if err != nil {
		t.Fatalf("re-read draft: %v", err)
	}
	if reread.Status != order.OrderStatusDraft {
		t.Errorf("status = %q after a failed placement, want it still draft", reread.Status)
	}
	if got := h.reservedFor(t, f); got != 0 {
		t.Errorf("reserved after a failed placement = %v, want 0", got)
	}
	if total, _ := h.outboxRows(t, "order.placed", d.ID.String()); total != 0 {
		t.Errorf("%d order.placed outbox rows for a placement that failed, want 0", total)
	}
	h.settle(t)
}

// Placing twice would reserve the stock twice and re-fire OrderPlaced
func TestDraftOrderCannotBePlacedTwice(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 20, 2500)
	d := h.newDraft(t, f, []order.CreateDraftLine{{ProductSku: f.Product.SKU, Quantity: 2}})

	if _, err := h.d.Order.PlaceDraftOrder(h.ctx, d.ID); err != nil {
		t.Fatalf("first placement: %v", err)
	}
	h.settle(t)

	_, err := h.d.Order.PlaceDraftOrder(h.ctx, d.ID)
	if err == nil {
		t.Fatal("a second placement succeeded")
	}
	if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "ORDER_NOT_DRAFT" {
		t.Errorf("error = %v, want code ORDER_NOT_DRAFT", err)
	}
	if got := h.reservedFor(t, f); got != 2 {
		t.Errorf("reserved after the refused second placement = %v, want the original 2", got)
	}
	h.settle(t)
}

// Quote conversion needs a negotiated price, so an explicit unit price overrides the catalog - and it has to flow into the line total and the order subtotal, not just the line row
func TestDraftOrderHonorsAManualUnitPrice(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 20, 2500)
	negotiated := int64(1800)

	d := h.newDraft(t, f, []order.CreateDraftLine{
		{ProductSku: f.Product.SKU, Quantity: 2, UnitPriceCents: &negotiated},
	})

	lines := h.orderLines(t, d.ID)
	if len(lines) != 1 {
		t.Fatalf("%d lines, want 1", len(lines))
	}
	if lines[0].UnitPrice != negotiated {
		t.Errorf("unit price = %d, want the negotiated %d rather than the catalog 2500", lines[0].UnitPrice, negotiated)
	}
	if lines[0].TotalPrice != 2*negotiated {
		t.Errorf("line total = %d, want %d", lines[0].TotalPrice, 2*negotiated)
	}
	if d.SubtotalAmount != 2*negotiated {
		t.Errorf("subtotal = %d, want the manual price to reach it (%d)", d.SubtotalAmount, 2*negotiated)
	}
	h.settle(t)
}

// A manual discount is the admin's stand-in for a promo code, and it has to reduce the total the customer is actually asked for
func TestDraftOrderAppliesAManualDiscount(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 20, 2500)

	d := h.newDraft(t, f, []order.CreateDraftLine{{ProductSku: f.Product.SKU, Quantity: 4}},
		func(in *order.CreateDraftOrderInput) { in.ManualDiscountCents = 1500 })

	if d.DiscountAmount != 1500 {
		t.Errorf("discount = %d, want the manual 1500", d.DiscountAmount)
	}
	if d.SubtotalAmount != 4*2500 {
		t.Errorf("subtotal = %d, want the undiscounted %d", d.SubtotalAmount, 4*2500)
	}
	if d.TotalAmount >= d.SubtotalAmount {
		t.Errorf("total %d is not below the subtotal %d, so the discount never reached it", d.TotalAmount, d.SubtotalAmount)
	}
	if d.TotalAmount != d.SubtotalAmount-d.DiscountAmount+d.TaxAmount+d.ShippingAmount {
		t.Errorf("total %d does not equal its own breakdown", d.TotalAmount)
	}
	h.settle(t)
}

// Editing a draft re-prices but must NOT reserve - a draft holds no stock by definition, and reserving mid-edit would take inventory the admin hasn't committed to yet
func TestEditingADraftRepricesWithoutReserving(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 20, 2500)
	d := h.newDraft(t, f, []order.CreateDraftLine{{ProductSku: f.Product.SKU, Quantity: 2}})
	lineID := h.orderLines(t, d.ID)[0].ID

	edited, err := h.d.Order.EditOrder(h.ctx, order.EditOrderInput{
		OrderID:   d.ID,
		UpdateQty: []order.EditLineQty{{OrderLineID: lineID, Quantity: 5}},
	})
	if err != nil {
		t.Fatalf("edit the draft: %v", err)
	}

	if edited.SubtotalAmount != 5*2500 {
		t.Errorf("subtotal = %d, want the edit re-priced to %d", edited.SubtotalAmount, 5*2500)
	}
	if edited.Status != order.OrderStatusDraft {
		t.Errorf("status = %q, want it still draft after an edit", edited.Status)
	}
	if got := h.reservedFor(t, f); got != 0 {
		t.Errorf("reserved after editing a draft = %v, want 0 - a draft holds no stock", got)
	}

	// and placing afterwards takes the EDITED quantity
	if _, err := h.d.Order.PlaceDraftOrder(h.ctx, d.ID); err != nil {
		t.Fatalf("place the edited draft: %v", err)
	}
	if got := h.reservedFor(t, f); got != 5 {
		t.Errorf("reserved after placing the edited draft = %v, want the edited 5", got)
	}
	h.settle(t)
}

// The guards refuse before anything is written, so a malformed admin form can't leave a partial draft
func TestDraftOrderGuards(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 20, 2500)
	base := func() order.CreateDraftOrderInput {
		return order.CreateDraftOrderInput{
			ChannelID:       f.Channel.ID,
			Email:           "guard@example.invalid",
			BillingAddress:  *e2eAddress(),
			ShippingAddress: *e2eAddress(),
			Lines:           []order.CreateDraftLine{{ProductSku: f.Product.SKU, Quantity: 1}},
		}
	}

	for name, tc := range map[string]struct {
		mutate func(*order.CreateDraftOrderInput)
		code   string
	}{
		"no email":        {func(in *order.CreateDraftOrderInput) { in.Email = "" }, "EMAIL_REQUIRED"},
		"no lines":        {func(in *order.CreateDraftOrderInput) { in.Lines = nil }, "DRAFT_EMPTY"},
		"empty line list": {func(in *order.CreateDraftOrderInput) { in.Lines = []order.CreateDraftLine{} }, "DRAFT_EMPTY"},
	} {
		in := base()
		tc.mutate(&in)
		_, err := h.d.Order.CreateDraftOrder(h.ctx, in)
		if err == nil {
			t.Errorf("%s was accepted, want %s", name, tc.code)
			continue
		}
		if ae, ok := apperr.AsAppError(err); !ok || ae.Code != tc.code {
			t.Errorf("%s: error = %v, want code %s", name, err, tc.code)
		}
	}

	// an unknown sku fails too, and nothing gets reserved by any of the refusals
	in := base()
	in.Lines = []order.CreateDraftLine{{ProductSku: "e2e-no-such-sku", Quantity: 1}}
	if _, err := h.d.Order.CreateDraftOrder(h.ctx, in); err == nil {
		t.Error("a draft with an unknown sku was accepted")
	}
	if got := h.reservedFor(t, f); got != 0 {
		t.Errorf("reserved = %v after the refused drafts, want 0", got)
	}
	h.settle(t)
}

// Only a draft can be placed through this door - a live order arriving here would re-reserve its stock
func TestPlaceDraftRefusesANonDraftOrder(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 20, 2500)
	o := h.placeGuestCOD(t, f, 1)
	h.settle(t)

	_, err := h.d.Order.PlaceDraftOrder(h.ctx, o.ID)
	if err == nil {
		t.Fatal("placing an already-live order succeeded")
	}
	if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "ORDER_NOT_DRAFT" {
		t.Errorf("error = %v, want code ORDER_NOT_DRAFT", err)
	}
	if got := h.reservedFor(t, f); got != 1 {
		t.Errorf("reserved = %v, want the original 1 - the refusal must not re-reserve", got)
	}
	h.settle(t)
}
