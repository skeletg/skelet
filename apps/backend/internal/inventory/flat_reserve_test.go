package inventory

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
)

// Same no-op sql driver trick as the cart merge tests - the fakes ignore the
// *sql.Tx, only the Begin/Commit lifecycle has to exist for tx.Manager
type invNoopDriver struct{}
type invNoopConn struct{}
type invNoopTx struct{}

func (invNoopDriver) Open(string) (driver.Conn, error)  { return invNoopConn{}, nil }
func (invNoopConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (invNoopConn) Close() error                        { return nil }
func (invNoopConn) Begin() (driver.Tx, error)           { return invNoopTx{}, nil }
func (invNoopTx) Commit() error                         { return nil }
func (invNoopTx) Rollback() error                       { return nil }

var registerInvNoopDriver sync.Once

func newInvTxManager(t *testing.T) *tx.Manager {
	t.Helper()
	registerInvNoopDriver.Do(func() { sql.Register("inventory-noop", invNoopDriver{}) })
	db, err := sql.Open("inventory-noop", "")
	if err != nil {
		t.Fatalf("open noop db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return tx.New(db)
}

type toggle bool

func (v toggle) FlatModeEnabled(context.Context) bool { return bool(v) }
func (v toggle) LedgerEnabled(context.Context) bool   { return bool(v) }

// flatRepo backs the flat-mode item decrement path over a panicking base
type flatRepo struct {
	Repository
	items map[uuid.UUID]*Item // keyed by product id
}

func (f *flatRepo) WithTx(*sql.Tx) Repository { return f }

func (f *flatRepo) GetItemByProductID(_ context.Context, pid uuid.UUID) (Item, error) {
	if it, ok := f.items[pid]; ok {
		return *it, nil
	}
	return Item{}, ErrItemNotFound
}

func (f *flatRepo) GetItemByProductIDForUpdate(ctx context.Context, pid uuid.UUID) (Item, error) {
	return f.GetItemByProductID(ctx, pid)
}

func (f *flatRepo) AdjustItemQuantity(_ context.Context, itemID uuid.UUID, delta float64) (Item, error) {
	for _, it := range f.items {
		if it.ID == itemID {
			it.Quantity += delta
			return *it, nil
		}
	}
	return Item{}, ErrItemNotFound
}

type fakeOrderLines struct{ lines []OrderLine }

func (f fakeOrderLines) GetOrderLines(context.Context, uuid.UUID) ([]OrderLine, error) {
	return f.lines, nil
}

func flatItem(qty float64, backorderable bool) *Item {
	return &Item{ID: uuid.New(), ProductID: uuid.New(), Tracked: true, Backorderable: backorderable, Quantity: qty}
}

func TestReserveTxFlatMode(t *testing.T) {
	newSvc := func(repo *flatRepo, flat, ledger toggle) *Service {
		s := NewService(repo, newInvTxManager(t))
		s.SetFlatModeToggle(flat)
		s.SetLedgerToggle(ledger)
		return s
	}

	t.Run("placement decrements tracked items", func(t *testing.T) {
		it := flatItem(10, false)
		repo := &flatRepo{items: map[uuid.UUID]*Item{it.ProductID: it}}
		svc := newSvc(repo, true, true)
		_, err := svc.ReserveTx(context.Background(), nil, uuid.New(), uuid.New(),
			[]ReserveLine{{ProductID: it.ProductID, OrderLineID: uuid.New(), Quantity: 3}})
		if err != nil {
			t.Fatalf("ReserveTx: %v", err)
		}
		if it.Quantity != 7 {
			t.Fatalf("quantity = %g, want 7", it.Quantity)
		}
	})

	t.Run("shortage rejects the order without decrementing", func(t *testing.T) {
		it := flatItem(2, false)
		repo := &flatRepo{items: map[uuid.UUID]*Item{it.ProductID: it}}
		svc := newSvc(repo, true, true)
		_, err := svc.ReserveTx(context.Background(), nil, uuid.New(), uuid.New(),
			[]ReserveLine{{ProductID: it.ProductID, OrderLineID: uuid.New(), Quantity: 3}})
		if !errors.Is(err, ErrInsufficientStock) {
			t.Fatalf("expected ErrInsufficientStock, got %v", err)
		}
		if it.Quantity != 2 {
			t.Fatalf("quantity moved on a rejected order: %g", it.Quantity)
		}
	})

	t.Run("backorderable oversells past zero", func(t *testing.T) {
		it := flatItem(2, true)
		repo := &flatRepo{items: map[uuid.UUID]*Item{it.ProductID: it}}
		svc := newSvc(repo, true, true)
		if _, err := svc.ReserveTx(context.Background(), nil, uuid.New(), uuid.New(),
			[]ReserveLine{{ProductID: it.ProductID, OrderLineID: uuid.New(), Quantity: 5}}); err != nil {
			t.Fatalf("ReserveTx: %v", err)
		}
		if it.Quantity != -3 {
			t.Fatalf("quantity = %g, want -3 (backorder debt)", it.Quantity)
		}
	})

	t.Run("untracked and itemless products pass through", func(t *testing.T) {
		it := flatItem(5, false)
		it.Tracked = false
		repo := &flatRepo{items: map[uuid.UUID]*Item{it.ProductID: it}}
		svc := newSvc(repo, true, true)
		if _, err := svc.ReserveTx(context.Background(), nil, uuid.New(), uuid.New(), []ReserveLine{
			{ProductID: it.ProductID, OrderLineID: uuid.New(), Quantity: 3},
			{ProductID: uuid.New(), OrderLineID: uuid.New(), Quantity: 99},
		}); err != nil {
			t.Fatalf("ReserveTx: %v", err)
		}
		if it.Quantity != 5 {
			t.Fatalf("untracked item moved: %g", it.Quantity)
		}
	})

	// The ledger toggle is the cutover kill switch for BOTH modes
	t.Run("ledger disabled means no flat writes either", func(t *testing.T) {
		it := flatItem(10, false)
		repo := &flatRepo{items: map[uuid.UUID]*Item{it.ProductID: it}}
		svc := newSvc(repo, true, false)
		if _, err := svc.ReserveTx(context.Background(), nil, uuid.New(), uuid.New(),
			[]ReserveLine{{ProductID: it.ProductID, OrderLineID: uuid.New(), Quantity: 3}}); err != nil {
			t.Fatalf("ReserveTx: %v", err)
		}
		if it.Quantity != 10 {
			t.Fatalf("kill switch ignored, quantity = %g", it.Quantity)
		}
	})
}

func TestReleaseForOrderFlatMode(t *testing.T) {
	it := flatItem(7, false)
	repo := &flatRepo{items: map[uuid.UUID]*Item{it.ProductID: it}}
	svc := NewService(repo, newInvTxManager(t))
	svc.SetFlatModeToggle(toggle(true))
	svc.SetLedgerToggle(toggle(true))
	svc.SetOrderLineLookup(fakeOrderLines{lines: []OrderLine{{ProductID: it.ProductID, Quantity: 3}}})

	if err := svc.ReleaseForOrder(context.Background(), uuid.New(), ReasonOrderCancelled); err != nil {
		t.Fatalf("ReleaseForOrder: %v", err)
	}
	if it.Quantity != 10 {
		t.Fatalf("quantity = %g, want 10 (cancellation restores the line set)", it.Quantity)
	}
}

// partially_fulfilled -> cancelled is a legal transition, and restoring the whole line would put
// the shipped units back on the shelf and oversell them a second time
func TestReleaseForOrderFlatModeSkipsShippedQuantity(t *testing.T) {
	for _, tc := range []struct {
		name             string
		ordered, shipped float64
		wantQuantity     float64
	}{
		{"nothing shipped restores in full", 3, 0, 10},
		{"partially shipped restores the remainder", 3, 2, 8},
		{"fully shipped restores nothing", 3, 3, 7},
		{"over-shipped never subtracts", 3, 5, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			it := flatItem(7, false)
			repo := &flatRepo{items: map[uuid.UUID]*Item{it.ProductID: it}}
			svc := NewService(repo, newInvTxManager(t))
			svc.SetFlatModeToggle(toggle(true))
			svc.SetLedgerToggle(toggle(true))
			svc.SetOrderLineLookup(fakeOrderLines{lines: []OrderLine{
				{ProductID: it.ProductID, Quantity: tc.ordered, FulfilledQuantity: tc.shipped},
			}})

			if err := svc.ReleaseForOrder(context.Background(), uuid.New(), ReasonOrderCancelled); err != nil {
				t.Fatalf("ReleaseForOrder: %v", err)
			}
			if it.Quantity != tc.wantQuantity {
				t.Fatalf("quantity = %g, want %g", it.Quantity, tc.wantQuantity)
			}
		})
	}
}
