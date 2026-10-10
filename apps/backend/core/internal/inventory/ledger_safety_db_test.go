package inventory

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func setItemQuantity(t *testing.T, conn *sql.DB, itemID uuid.UUID, qty float64) {
	t.Helper()
	if _, err := conn.Exec(`UPDATE inventory_items SET quantity = $2 WHERE id = $1`, itemID, qty); err != nil {
		t.Fatal(err)
	}
}

// The outbox replays a whole status change when any subscriber fails - flat mode keeps no ledger, so each replay used to put the cancelled order's stock back again
func TestAReplayedFlatCancelRestoresOnce(t *testing.T) {
	conn := recomputeDB(t)
	itemID, _ := seedLevel(t, conn, 0, 0)
	productID := productOf(t, conn, itemID)
	channelID := someChannel(t, conn)
	orderID := seedOrder(t, conn, channelID)
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM inventory_flat_restores WHERE order_id = $1`, orderID) })
	setItemQuantity(t, conn, itemID, 6) // 10 on hand, 4 taken at placement

	svc := reserveSvc(t, conn, map[uuid.UUID]uuid.UUID{productID: itemID})
	svc.SetFlatModeToggle(toggle(true))
	svc.SetOrderLineLookup(fixedOrderLines{lines: []OrderLine{{ProductID: productID, Quantity: 4}}})
	for range 3 {
		if err := svc.ReleaseForOrder(context.Background(), orderID, ReasonOrderCancelled); err != nil {
			t.Fatal(err)
		}
	}
	if got := itemQuantity(t, conn, itemID); got != 10 {
		t.Fatalf("after three deliveries of one cancel: %g on hand, want 10", got)
	}
}

// Two deliveries of one cancel racing each other both read the same open chain - without the per-order lock both compensated it, and the reserved counter went negative
func TestConcurrentReleasesCompensateOnce(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 10, 0)
	productID := productOf(t, conn, itemID)
	channelID := someChannel(t, conn)
	attachLocation(t, conn, channelID, locID, 1)
	svc := reserveSvc(t, conn, map[uuid.UUID]uuid.UUID{productID: itemID})
	ctx := context.Background()

	for round := range 5 {
		orderID := seedOrder(t, conn, channelID)
		lineID := seedOrderLine(t, conn, orderID, 4)
		if _, err := svc.Reserve(ctx, channelID, orderID, []ReserveLine{{ProductID: productID, OrderLineID: lineID, Quantity: 4}}); err != nil {
			t.Fatal(err)
		}
		race(t, 8, func() error { return svc.ReleaseForOrder(ctx, orderID, ReasonOrderCancelled) })
		if _, net := reservationRows(t, conn, orderID); net != 0 {
			t.Fatalf("round %d: the order's chain nets to %g after racing releases, want 0", round, net)
		}
		if got := readReserved(t, conn, itemID, locID); got != 0 {
			t.Fatalf("round %d: reserved counter = %g, want 0", round, got)
		}
	}
}

// Its levels and ledger rows cascade with it - an open order lost its hold, so shipping it later deducted nothing
func TestALocationHoldingStockOrHoldsCannotBeDeleted(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 5, 0)
	channelID := someChannel(t, conn)
	svc := reserveSvc(t, conn, nil)
	ctx := context.Background()

	if err := svc.DeleteLocation(ctx, locID); !errors.Is(err, ErrLocationInUse) {
		t.Fatalf("delete with stock on the shelf = %v, want ErrLocationInUse", err)
	}
	if _, err := conn.Exec(`UPDATE inventory_levels SET quantity = 0 WHERE location_id = $1`, locID); err != nil {
		t.Fatal(err)
	}
	orderID := seedOrder(t, conn, channelID)
	hold := func(delta float64) {
		if _, err := conn.Exec(`INSERT INTO inventory_reservations (inventory_item_id, location_id, quantity_delta, reason, order_id)
			VALUES ($1, $2, $3, 'order_placed', $4)`, itemID, locID, delta, orderID); err != nil {
			t.Fatal(err)
		}
	}
	hold(-2)
	if err := svc.DeleteLocation(ctx, locID); !errors.Is(err, ErrLocationInUse) {
		t.Fatalf("delete with an open order hold = %v, want ErrLocationInUse", err)
	}
	hold(2)
	if err := svc.DeleteLocation(ctx, locID); err != nil {
		t.Fatalf("delete of an empty, settled location = %v", err)
	}
}

// The audit delta is computed from the locked level - read unlocked, an edit landing mid-adjustment made the ledger disagree with the shelf
func TestAConcurrentAdjustmentAuditsTheRealChange(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 10, 0)
	svc := reserveSvc(t, conn, nil)
	ctx := context.Background()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`SELECT 1 FROM inventory_levels WHERE inventory_item_id = $1 AND location_id = $2 FOR UPDATE`, itemID, locID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := svc.AdjustLevel(ctx, AdjustLevelInput{InventoryItemID: itemID, LocationID: locID, NewQuantity: 15})
		done <- err
	}()
	waitForLockWait(t, conn)
	if _, err := tx.Exec(`UPDATE inventory_levels SET quantity = 12 WHERE inventory_item_id = $1 AND location_id = $2`, itemID, locID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO inventory_reservations (inventory_item_id, location_id, quantity_delta, reason) VALUES ($1, $2, 2, 'manual_adjustment')`, itemID, locID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var audited float64
	if err := conn.QueryRow(`SELECT COALESCE(SUM(quantity_delta), 0) FROM inventory_reservations WHERE inventory_item_id = $1 AND reason = 'manual_adjustment'`, itemID).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if got := readLevelQty(t, conn, itemID, locID); got != 15 || audited != 5 {
		t.Fatalf("shelf %g, audited change %g - want 15 and +5, the ledger adding up to what moved", got, audited)
	}
}

func TestPruneDeletesOnlyOldSettledChains(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 10, 0)
	channelID := someChannel(t, conn)
	svc := reserveSvc(t, conn, nil)
	chain := func(age time.Duration, deltas ...float64) uuid.UUID {
		orderID := seedOrder(t, conn, channelID)
		for _, d := range deltas {
			if _, err := conn.Exec(`INSERT INTO inventory_reservations (inventory_item_id, location_id, quantity_delta, reason, order_id, created_at)
				VALUES ($1, $2, $3, 'order_placed', $4, now() - $5::interval)`, itemID, locID, d, orderID, age.String()); err != nil {
				t.Fatal(err)
			}
		}
		return orderID
	}
	oldSettled := chain(200*24*time.Hour, -3, 3)
	oldOpen := chain(200*24*time.Hour, -3)
	newSettled := chain(time.Hour, -2, 2)

	n, err := svc.PruneSettledReservations(context.Background(), 180*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if left, _ := reservationRows(t, conn, oldSettled); left != 0 {
		t.Errorf("an old settled chain kept %d rows", left)
	}
	if left, _ := reservationRows(t, conn, oldOpen); left != 1 {
		t.Errorf("an open chain lost rows: %d left", left)
	}
	if left, _ := reservationRows(t, conn, newSettled); left != 2 {
		t.Errorf("a recently settled chain lost rows: %d left", left)
	}
	if n < 1 {
		t.Errorf("reported %d pruned, want at least the old settled order", n)
	}
}

func waitForLockWait(t *testing.T, conn *sql.DB) { waitForLockWaiters(t, conn, 1) }

func waitForLockWaiters(t *testing.T, conn *sql.DB, want int) {
	t.Helper()
	for range 300 {
		var n int
		if err := conn.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fewer than %d writers queued behind the open transaction", want)
}

// Holds the cell's row lock while two deliveries start, so both have read the ledger before either can write - without the per-order lock they then both act on the same snapshot
func behindHeldCell(t *testing.T, conn *sql.DB, itemID, locID uuid.UUID, op func() error) {
	t.Helper()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`SELECT 1 FROM inventory_levels WHERE inventory_item_id = $1 AND location_id = $2 FOR UPDATE`, itemID, locID); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- op() }()
	}
	waitForLockWaiters(t, conn, 2)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

func race(t *testing.T, n int, op func() error) {
	t.Helper()
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			<-start
			if err := op(); err != nil {
				t.Error(err)
			}
		})
	}
	close(start)
	wg.Wait()
}

// A redelivered shipment racing the original both saw the hold - the cap per line only holds when they take turns
func TestRacingFulfilmentsShipTheGoodsOnce(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 10, 0)
	productID := productOf(t, conn, itemID)
	channelID := someChannel(t, conn)
	attachLocation(t, conn, channelID, locID, 1)
	svc := reserveSvc(t, conn, map[uuid.UUID]uuid.UUID{productID: itemID})
	ctx := context.Background()
	orderID := seedOrder(t, conn, channelID)
	lineID := seedOrderLine(t, conn, orderID, 4)
	if _, err := svc.Reserve(ctx, channelID, orderID, []ReserveLine{{ProductID: productID, OrderLineID: lineID, Quantity: 4}}); err != nil {
		t.Fatal(err)
	}
	behindHeldCell(t, conn, itemID, locID, func() error {
		return svc.FulfillLines(ctx, orderID, []FulfillLineQty{{OrderLineID: lineID, Quantity: 4}})
	})
	if got := readLevelQty(t, conn, itemID, locID); got != 6 {
		t.Fatalf("on hand after one shipment delivered twice at once: %g, want 6", got)
	}
}

func TestRacingReturnsRestockOnce(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 10, 0)
	productID := productOf(t, conn, itemID)
	channelID := someChannel(t, conn)
	attachLocation(t, conn, channelID, locID, 1)
	svc := reserveSvc(t, conn, map[uuid.UUID]uuid.UUID{productID: itemID})
	ctx := context.Background()
	orderID := seedOrder(t, conn, channelID)
	lineID := seedOrderLine(t, conn, orderID, 4)
	if _, err := svc.Reserve(ctx, channelID, orderID, []ReserveLine{{ProductID: productID, OrderLineID: lineID, Quantity: 4}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.FulfillLines(ctx, orderID, []FulfillLineQty{{OrderLineID: lineID, Quantity: 4}}); err != nil {
		t.Fatal(err)
	}
	returnID := uuid.New()
	behindHeldCell(t, conn, itemID, locID, func() error {
		return svc.RestockReturn(ctx, orderID, returnID, []RestockLineQty{{OrderLineID: lineID, Quantity: 4}})
	})
	if got := readLevelQty(t, conn, itemID, locID); got != 10 {
		t.Fatalf("on hand after one return delivered twice at once: %g, want 10", got)
	}
}
