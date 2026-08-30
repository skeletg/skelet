package e2e

import (
	"testing"

	"github.com/google/uuid"
)

// Scenario: cancelling a pending unpaid order must return its held stock - the release is event-driven off the durable
// order.cancelled row, compensates the reservation chain to net zero, and a replayed delivery must not compensate twice
func TestCancelReleasesReservation(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 10, 1500)
	o := h.placeGuestCOD(t, f, 1)
	h.settle(t)
	if got := h.reservedFor(t, f); got != 1 {
		t.Fatalf("reserved = %v before cancel, want 1", got)
	}

	h.cancel(t, o.ID)
	if total, _ := h.outboxRows(t, "order.cancelled", o.ID.String()); total != 1 {
		t.Errorf("order.cancelled outbox rows = %d, want 1 - the tender/stock reversals ride on it", total)
	}
	h.settle(t)

	if got := h.reservedFor(t, f); got != 0 {
		t.Errorf("reserved = %v after cancel settled, want 0", got)
	}
	res, err := h.d.Queries.ListReservationsForOrder(h.ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if err != nil {
		t.Fatalf("reservations: %v", err)
	}
	var sum float64
	comp := 0
	for _, r := range res {
		sum += r.QuantityDelta
		if r.Reason == "order_cancelled" {
			comp++
		}
	}
	if len(res) != 2 || sum != 0 || comp != 1 {
		t.Errorf("reservation chain = %d rows, net %v, %d compensations - want 2/0/1: %+v", len(res), sum, comp, res)
	}

	// Cancel never touches money: still unpaid, ledger still empty
	row, err := h.d.Queries.GetOrderByID(h.ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read order: %v", err)
	}
	if row.Status != "cancelled" || row.PaymentStatus != "unpaid" {
		t.Errorf("status/payment = %q/%q, want cancelled/unpaid", row.Status, row.PaymentStatus)
	}

	// A replayed order.cancelled sees a net-zero chain and must add nothing
	h.rearmOutbox(t, "order.cancelled", o.ID.String(), 1)
	h.settle(t)
	if got := h.reservedFor(t, f); got != 0 {
		t.Errorf("reserved = %v after replay, want 0", got)
	}
	resAfter, err := h.d.Queries.ListReservationsForOrder(h.ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if err != nil {
		t.Fatalf("reservations after replay: %v", err)
	}
	if len(resAfter) != len(res) {
		t.Errorf("replay grew the reservation chain %d -> %d rows", len(res), len(resAfter))
	}
}
