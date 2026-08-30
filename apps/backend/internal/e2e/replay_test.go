package e2e

import (
	"testing"

	"github.com/google/uuid"
)

// Scenario 3: the outbox contract is at-least-once and every reactor claims replay tolerance - this proves it. Re-arming the delivered
// order.placed row simulates the crash window between dispatch and mark-delivered: a second full delivery must change no money state
func TestOrderPlacedReplayIsIdempotent(t *testing.T) {
	h := newHarness(t)
	f := h.newStore(t, 30, 990)
	o := h.placeGuestCOD(t, f, 1)
	h.settle(t)

	before := h.orderSnapshot(t, f, o.ID)

	h.rearmOutbox(t, "order.placed", o.ID.String(), 1)
	h.settle(t) // full second delivery to every subscriber

	after := h.orderSnapshot(t, f, o.ID)
	if after != before {
		t.Errorf("replay changed state:\n before %+v\n after  %+v", before, after)
	}
	if _, undelivered := h.outboxRows(t, "order.placed", o.ID.String()); undelivered != 0 {
		t.Errorf("%d order.placed rows still undelivered after replay settle", undelivered)
	}
}

// The state a replay must not move - comparable so one != catches any drift
type snapshot struct {
	Status          string
	PaymentStatus   string
	ReservationRows int
	Reserved        float64
	LedgerKinds     int
}

func (h *harness) orderSnapshot(t *testing.T, f *store, orderID uuid.UUID) snapshot {
	t.Helper()
	row, err := h.d.Queries.GetOrderByID(h.ctx, orderID)
	if err != nil {
		t.Fatalf("snapshot order: %v", err)
	}
	res, err := h.d.Queries.ListReservationsForOrder(h.ctx, uuid.NullUUID{UUID: orderID, Valid: true})
	if err != nil {
		t.Fatalf("snapshot reservations: %v", err)
	}
	levels, err := h.d.Queries.ListInventoryLevelsByItem(h.ctx, f.Item.ID)
	if err != nil {
		t.Fatalf("snapshot levels: %v", err)
	}
	var reserved float64
	for _, l := range levels {
		reserved += l.ReservedQuantity
	}
	sums, err := h.d.Queries.SumOrderPaymentByKind(h.ctx, orderID)
	if err != nil {
		t.Fatalf("snapshot ledger: %v", err)
	}
	return snapshot{
		Status:          row.Status,
		PaymentStatus:   row.PaymentStatus,
		ReservationRows: len(res),
		Reserved:        reserved,
		LedgerKinds:     len(sums),
	}
}
