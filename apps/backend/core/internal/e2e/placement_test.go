package e2e

import (
	"testing"

	"github.com/google/uuid"
)

// Scenario 1: a guest places a COD order. The committed transaction must hold the money-path invariants on its own - unpaid status, zero ledger rows, a reservation holding the stock - and the durable order.placed row must exist before any relay runs, then deliver cleanly on drain
func TestGuestCODPlacement(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 100, 1990)
	o := h.PlaceGuestCOD(t, f, 1)

	// Committed state, before any async reaction has run
	row, err := h.D.Queries.GetOrderByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("read order back: %v", err)
	}
	if row.PaymentStatus != "unpaid" {
		t.Errorf("payment_status = %q, want unpaid - COD authorizes nothing at placement", row.PaymentStatus)
	}

	sums, err := h.D.Queries.SumOrderPaymentByKind(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("ledger sums: %v", err)
	}
	if len(sums) != 0 {
		t.Errorf("ledger has %d kinds for an unpaid COD order, want none: %+v", len(sums), sums)
	}

	res, err := h.D.Queries.ListReservationsForOrder(h.Ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if err != nil {
		t.Fatalf("reservations: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d reservation rows, want 1: %+v", len(res), res)
	}
	if res[0].QuantityDelta != -1 {
		t.Errorf("reservation delta = %v, want -1 (a hold)", res[0].QuantityDelta)
	}

	levels, err := h.D.Queries.ListInventoryLevelsByItem(h.Ctx, f.Item.ID)
	if err != nil {
		t.Fatalf("levels: %v", err)
	}
	if len(levels) != 1 {
		t.Fatalf("got %d levels, want 1", len(levels))
	}
	if levels[0].Quantity != 100 || levels[0].ReservedQuantity != 1 {
		t.Errorf("level quantity/reserved = %v/%v, want 100/1 - placement reserves, it must not deduct",
			levels[0].Quantity, levels[0].ReservedQuantity)
	}

	if total, _ := h.OutboxRows(t, "order.placed", o.ID.String()); total != 1 {
		t.Errorf("order.placed outbox rows = %d, want exactly 1", total)
	}

	h.Settle(t)

	// Every durable row for this order delivered, and settling changed no money state
	if _, undelivered := h.OutboxRows(t, "order.placed", o.ID.String()); undelivered != 0 {
		t.Errorf("%d order.placed rows still undelivered after settle", undelivered)
	}
	after, err := h.D.Queries.GetOrderByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read order: %v", err)
	}
	if after.PaymentStatus != row.PaymentStatus || after.Status != row.Status {
		t.Errorf("settle changed order state: status %q->%q payment %q->%q",
			row.Status, after.Status, row.PaymentStatus, after.PaymentStatus)
	}
	levelsAfter, err := h.D.Queries.ListInventoryLevelsByItem(h.Ctx, f.Item.ID)
	if err != nil {
		t.Fatalf("levels after settle: %v", err)
	}
	if levelsAfter[0].ReservedQuantity != 1 {
		t.Errorf("reserved = %v after settle, want still 1 - nothing captured or cancelled yet", levelsAfter[0].ReservedQuantity)
	}
}
