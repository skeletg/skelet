package inventory

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/cart"
)

// Flat mode decremented the FULL ordered quantity at placement, so an edit has to move that
// deduction rather than reserve afresh - and it must never hand back units that already shipped,
// the same rule cancellation follows

func flatEditSvc(t *testing.T, repo *flatRepo, prior []OrderLine, sku string, pid uuid.UUID) *Service {
	t.Helper()
	svc := NewService(repo, newInvTxManager(t))
	svc.SetFlatModeToggle(toggle(true))
	svc.SetLedgerToggle(toggle(true))
	svc.SetOrderLineLookup(fakeOrderLines{lines: prior})
	svc.SetProductLookup(fakeKitLookup{idBySku: map[string]uuid.UUID{sku: pid}})
	return svc
}

func TestReReserveForEditFlatMode(t *testing.T) {
	const sku = "widget"
	for _, tc := range []struct {
		name             string
		startQuantity    float64
		ordered, shipped float64
		editedTo         float64
		backorderable    bool
		wantQuantity     float64
		wantErr          error
	}{
		// placement took 3, the edit wants 5, so two more come off the shelf
		{name: "raising the quantity takes the difference", startQuantity: 10, ordered: 3, editedTo: 5, wantQuantity: 8},
		// placement took 3, the edit wants 1, so two go back
		{name: "lowering the quantity gives the difference back", startQuantity: 10, ordered: 3, editedTo: 1, wantQuantity: 12},
		{name: "no change moves nothing", startQuantity: 10, ordered: 3, editedTo: 3, wantQuantity: 10},
		// dropping the line entirely returns the whole hold
		{name: "removing the line returns the hold", startQuantity: 10, ordered: 3, editedTo: 0, wantQuantity: 13},
		// 2 of the 3 already shipped, so lowering to 1 may only give back the 1 still held
		{name: "never gives back what shipped", startQuantity: 10, ordered: 3, shipped: 2, editedTo: 1, wantQuantity: 11},
		// everything shipped, so there is nothing to return
		{name: "fully shipped returns nothing", startQuantity: 10, ordered: 3, shipped: 3, editedTo: 0, wantQuantity: 10},
		// a shortage must abort so the surrounding edit tx rolls back
		{name: "shortage on an increase aborts the edit", startQuantity: 1, ordered: 3, editedTo: 9, wantQuantity: 1, wantErr: ErrInsufficientStock},
		{name: "backorderable takes the increase past zero", startQuantity: 1, ordered: 3, editedTo: 9, backorderable: true, wantQuantity: -5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			it := flatItem(tc.startQuantity, tc.backorderable)
			repo := &flatRepo{items: map[uuid.UUID]*Item{it.ProductID: it}}
			prior := []OrderLine{{ProductID: it.ProductID, Quantity: tc.ordered, FulfilledQuantity: tc.shipped}}
			svc := flatEditSvc(t, repo, prior, sku, it.ProductID)

			c := cart.Cart{}
			if tc.editedTo > 0 {
				c.Lines = []cart.CartLine{{ID: uuid.New(), ProductSku: sku, Quantity: tc.editedTo}}
			}

			err := svc.ReReserveForEditTx(context.Background(), nil, uuid.New(), uuid.New(), c)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if it.Quantity != tc.wantQuantity {
				t.Fatalf("quantity = %g, want %g", it.Quantity, tc.wantQuantity)
			}
		})
	}
}

// The ledger kill switch outranks the mode, the same ordering ReserveTx uses
func TestReReserveForEditFlatModeRespectsKillSwitch(t *testing.T) {
	const sku = "widget"
	it := flatItem(10, false)
	repo := &flatRepo{items: map[uuid.UUID]*Item{it.ProductID: it}}
	svc := flatEditSvc(t, repo, []OrderLine{{ProductID: it.ProductID, Quantity: 3}}, sku, it.ProductID)
	svc.SetLedgerToggle(toggle(false))

	c := cart.Cart{Lines: []cart.CartLine{{ID: uuid.New(), ProductSku: sku, Quantity: 9}}}
	if err := svc.ReReserveForEditTx(context.Background(), nil, uuid.New(), uuid.New(), c); err != nil {
		t.Fatalf("ReReserveForEditTx: %v", err)
	}
	if it.Quantity != 10 {
		t.Fatalf("quantity = %g, want 10 (kill switch means no writes)", it.Quantity)
	}
}
