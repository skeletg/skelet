package inventory

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/cart"
)

// Editing a placed order releases what it still holds and re-reserves the new basket, inside the
// edit's own transaction
//
// The arithmetic is the whole risk. Release too much and the store oversells against stock that was
// never freed; release too little and units stay held for an order that no longer wants them. The
// dangerous case is a chain that ALREADY settled - compensating it a second time invents stock

type rereserveRepo struct {
	Repository
	rows     []Reservation // the order's existing ledger
	inserted []Reservation
}

func (r *rereserveRepo) WithTx(*sql.Tx) Repository { return r }

func (r *rereserveRepo) ListReservationsForOrder(context.Context, uuid.UUID) ([]Reservation, error) {
	return r.rows, nil
}

func (r *rereserveRepo) InsertReservation(_ context.Context, res Reservation) (Reservation, error) {
	r.inserted = append(r.inserted, res)
	return res, nil
}

// products stays nil so the re-reserve half short-circuits and these assert the RELEASE arithmetic
// on its own - the reserve half has its own coverage in allocate_test.go
func newReleaseOnly(t *testing.T, rows []Reservation) (*Service, *rereserveRepo) {
	t.Helper()
	repo := &rereserveRepo{rows: rows}
	svc := NewService(repo, newInvTxManager(t))
	svc.SetFlatModeToggle(toggle(false))
	svc.SetLedgerToggle(toggle(true))
	return svc, repo
}

func held(itemID, locID uuid.UUID, lineID *uuid.UUID, delta float64, reason Reason) Reservation {
	oid := uuid.New()
	return Reservation{
		InventoryItemID: itemID, LocationID: locID, OrderLineID: lineID,
		QuantityDelta: delta, Reason: reason, OrderID: &oid,
	}
}

// A still-held line is compensated back to exactly zero
func TestReReserveReleasesTheHeldRemainder(t *testing.T) {
	item, loc := uuid.New(), uuid.New()
	line := uuid.New()
	svc, repo := newReleaseOnly(t, []Reservation{
		held(item, loc, &line, -3, ReasonOrderPlaced),
	})

	if err := svc.ReReserveForEditTx(context.Background(), nil, uuid.New(), uuid.New(), cart.Cart{}); err != nil {
		t.Fatalf("ReReserveForEditTx: %v", err)
	}
	if len(repo.inserted) != 1 {
		t.Fatalf("wrote %d compensations, want 1", len(repo.inserted))
	}
	got := repo.inserted[0]
	if got.QuantityDelta != 3 {
		t.Errorf("compensation = %g, want 3 (the exact held amount)", got.QuantityDelta)
	}
	if got.Reason != ReasonOrderEdited {
		t.Errorf("reason = %q, want order_edited - the recompute reads reason to tell holds apart", got.Reason)
	}
	if got.OrderLineID == nil || *got.OrderLineID != line {
		t.Errorf("compensation lost its line id, so the chain no longer nets to zero per line")
	}
}

// The dangerous one: a chain that already netted to zero is settled, and compensating it again
// would create stock from nothing
func TestReReserveIgnoresSettledChains(t *testing.T) {
	item, loc := uuid.New(), uuid.New()
	line := uuid.New()
	svc, repo := newReleaseOnly(t, []Reservation{
		held(item, loc, &line, -5, ReasonOrderPlaced),
		held(item, loc, &line, 5, ReasonOrderCancelled),
	})

	if err := svc.ReReserveForEditTx(context.Background(), nil, uuid.New(), uuid.New(), cart.Cart{}); err != nil {
		t.Fatalf("ReReserveForEditTx: %v", err)
	}
	if len(repo.inserted) != 0 {
		t.Fatalf("compensated a settled chain with %+v - that invents stock", repo.inserted)
	}
}

// Partial fulfillment leaves part of the line shipped and part still held - only the held part may
// be released, or the shipped units come back as sellable stock that has physically left
func TestReReserveReleasesOnlyTheUnfulfilledPart(t *testing.T) {
	item, loc := uuid.New(), uuid.New()
	line := uuid.New()
	svc, repo := newReleaseOnly(t, []Reservation{
		held(item, loc, &line, -10, ReasonOrderPlaced),
		held(item, loc, &line, 4, ReasonOrderFulfilled), // 4 shipped
	})

	if err := svc.ReReserveForEditTx(context.Background(), nil, uuid.New(), uuid.New(), cart.Cart{}); err != nil {
		t.Fatalf("ReReserveForEditTx: %v", err)
	}
	if len(repo.inserted) != 1 {
		t.Fatalf("wrote %d compensations, want 1", len(repo.inserted))
	}
	if got := repo.inserted[0].QuantityDelta; got != 6 {
		t.Errorf("compensation = %g, want 6 - releasing the shipped units too would resell goods already gone", got)
	}
}

// Holds are per (item, location, line), so a line split across two locations releases each cell for
// what that cell actually holds
func TestReReserveCompensatesEachCellSeparately(t *testing.T) {
	item, locA, locB := uuid.New(), uuid.New(), uuid.New()
	line := uuid.New()
	svc, repo := newReleaseOnly(t, []Reservation{
		held(item, locA, &line, -2, ReasonOrderPlaced),
		held(item, locB, &line, -7, ReasonOrderPlaced),
	})

	if err := svc.ReReserveForEditTx(context.Background(), nil, uuid.New(), uuid.New(), cart.Cart{}); err != nil {
		t.Fatalf("ReReserveForEditTx: %v", err)
	}
	if len(repo.inserted) != 2 {
		t.Fatalf("wrote %d compensations, want one per location", len(repo.inserted))
	}
	byLoc := map[uuid.UUID]float64{}
	for _, ins := range repo.inserted {
		byLoc[ins.LocationID] = ins.QuantityDelta
	}
	if byLoc[locA] != 2 || byLoc[locB] != 7 {
		t.Errorf("compensations = %v, want %g at A and %g at B", byLoc, 2.0, 7.0)
	}
}

// Two lines of the same product at the same location are separate chains - merging them would
// release one line's stock against the other's hold
func TestReReserveKeepsLinesApart(t *testing.T) {
	item, loc := uuid.New(), uuid.New()
	lineA, lineB := uuid.New(), uuid.New()
	svc, repo := newReleaseOnly(t, []Reservation{
		held(item, loc, &lineA, -3, ReasonOrderPlaced),
		held(item, loc, &lineB, -8, ReasonOrderPlaced),
	})

	if err := svc.ReReserveForEditTx(context.Background(), nil, uuid.New(), uuid.New(), cart.Cart{}); err != nil {
		t.Fatalf("ReReserveForEditTx: %v", err)
	}
	if len(repo.inserted) != 2 {
		t.Fatalf("wrote %d compensations, want one per line", len(repo.inserted))
	}
	byLine := map[uuid.UUID]float64{}
	for _, ins := range repo.inserted {
		byLine[*ins.OrderLineID] = ins.QuantityDelta
	}
	if byLine[lineA] != 3 || byLine[lineB] != 8 {
		t.Errorf("compensations = %v, want 3 for line A and 8 for line B", byLine)
	}
}

// An over-released chain nets positive - compensating it would push it further, so it is skipped
func TestReReserveSkipsNetPositiveChains(t *testing.T) {
	item, loc := uuid.New(), uuid.New()
	line := uuid.New()
	svc, repo := newReleaseOnly(t, []Reservation{
		held(item, loc, &line, -2, ReasonOrderPlaced),
		held(item, loc, &line, 9, ReasonOrderCancelled),
	})

	if err := svc.ReReserveForEditTx(context.Background(), nil, uuid.New(), uuid.New(), cart.Cart{}); err != nil {
		t.Fatalf("ReReserveForEditTx: %v", err)
	}
	if len(repo.inserted) != 0 {
		t.Fatalf("compensated an already over-released chain: %+v", repo.inserted)
	}
}

// An order with nothing held releases nothing
func TestReReserveWithNoReservations(t *testing.T) {
	svc, repo := newReleaseOnly(t, nil)
	if err := svc.ReReserveForEditTx(context.Background(), nil, uuid.New(), uuid.New(), cart.Cart{}); err != nil {
		t.Fatalf("ReReserveForEditTx: %v", err)
	}
	if len(repo.inserted) != 0 {
		t.Errorf("wrote %d rows for an order holding nothing", len(repo.inserted))
	}
}

// Both switches make this a no-op - flat mode has no per-location ledger to compensate, and a
// disabled ledger means another system owns inventory truth and must not be written to
func TestReReserveRespectsTheModeSwitches(t *testing.T) {
	rows := []Reservation{held(uuid.New(), uuid.New(), nil, -4, ReasonOrderPlaced)}

	t.Run("flat mode", func(t *testing.T) {
		svc, repo := newReleaseOnly(t, rows)
		svc.SetFlatModeToggle(toggle(true))
		if err := svc.ReReserveForEditTx(context.Background(), nil, uuid.New(), uuid.New(), cart.Cart{}); err != nil {
			t.Fatalf("ReReserveForEditTx: %v", err)
		}
		if len(repo.inserted) != 0 {
			t.Errorf("flat mode wrote %d ledger rows", len(repo.inserted))
		}
	})

	t.Run("ledger disabled", func(t *testing.T) {
		svc, repo := newReleaseOnly(t, rows)
		svc.SetLedgerToggle(toggle(false))
		if err := svc.ReReserveForEditTx(context.Background(), nil, uuid.New(), uuid.New(), cart.Cart{}); err != nil {
			t.Fatalf("ReReserveForEditTx: %v", err)
		}
		if len(repo.inserted) != 0 {
			t.Errorf("a disabled ledger wrote %d rows", len(repo.inserted))
		}
	})
}

// A line whose hold has no line id still nets and releases - order-level holds exist for
// adjustments made outside a specific line
func TestReReserveHandlesLineLessHolds(t *testing.T) {
	item, loc := uuid.New(), uuid.New()
	svc, repo := newReleaseOnly(t, []Reservation{
		held(item, loc, nil, -5, ReasonOrderPlaced),
	})

	if err := svc.ReReserveForEditTx(context.Background(), nil, uuid.New(), uuid.New(), cart.Cart{}); err != nil {
		t.Fatalf("ReReserveForEditTx: %v", err)
	}
	if len(repo.inserted) != 1 || repo.inserted[0].QuantityDelta != 5 {
		t.Fatalf("compensations = %+v, want one of 5", repo.inserted)
	}
	if repo.inserted[0].OrderLineID != nil {
		t.Errorf("a line-less hold was compensated against a line")
	}
}

// The compensation is stamped with the order being edited, so the chain still nets to zero per
// order and the retention sweep can later see it settled
func TestReReserveStampsTheEditedOrder(t *testing.T) {
	item, loc := uuid.New(), uuid.New()
	line := uuid.New()
	svc, repo := newReleaseOnly(t, []Reservation{held(item, loc, &line, -1, ReasonOrderPlaced)})
	orderID := uuid.New()

	if err := svc.ReReserveForEditTx(context.Background(), nil, uuid.New(), orderID, cart.Cart{}); err != nil {
		t.Fatalf("ReReserveForEditTx: %v", err)
	}
	if repo.inserted[0].OrderID == nil || *repo.inserted[0].OrderID != orderID {
		t.Errorf("compensation stamped order %v, want %v", repo.inserted[0].OrderID, orderID)
	}
}
