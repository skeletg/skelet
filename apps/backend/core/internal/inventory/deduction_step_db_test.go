package inventory

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// Deduct-at-placement moves FulfillLines' comp + on-hand decrement into the placement tx, and every read-back (fulfill, cancel, edit, return) must then work off the ledger rather than the toggle - DB-backed because the whole point is the row algebra: chain nets, the reserved counter, and the order_deducted balance that keeps restores from ever running twice

type fixedDeduct bool

func (f fixedDeduct) DeductAtPlacement(context.Context) bool { return bool(f) }

type fixedOrderLines struct{ lines []OrderLine }

func (f fixedOrderLines) GetOrderLines(context.Context, uuid.UUID) ([]OrderLine, error) {
	return f.lines, nil
}

func readLevelQty(t *testing.T, conn *sql.DB, itemID, locID uuid.UUID) float64 {
	t.Helper()
	var got float64
	if err := conn.QueryRow(
		`SELECT quantity FROM inventory_levels WHERE inventory_item_id = $1 AND location_id = $2`,
		itemID, locID).Scan(&got); err != nil {
		t.Fatalf("read level quantity: %v", err)
	}
	return got
}

func readChainNet(t *testing.T, conn *sql.DB, itemID, locID uuid.UUID) float64 {
	t.Helper()
	var got float64
	if err := conn.QueryRow(
		`SELECT COALESCE(SUM(quantity_delta), 0) FROM inventory_reservations
		 WHERE inventory_item_id = $1 AND location_id = $2`,
		itemID, locID).Scan(&got); err != nil {
		t.Fatalf("read chain net: %v", err)
	}
	return got
}

// placementFixture seeds a tracked item with stock at one channel-attached location and returns a
// service in deduct-at-placement mode
func placementFixture(t *testing.T, conn *sql.DB, qty float64) (svc *Service, channelID, orderID, lineID, itemID, locID, productID uuid.UUID) {
	t.Helper()
	itemID, locID = seedLevel(t, conn, qty, 0)
	productID = productOf(t, conn, itemID)
	channelID = someChannel(t, conn)
	attachLocation(t, conn, channelID, locID, 1)
	svc = reserveSvc(t, conn, map[uuid.UUID]uuid.UUID{productID: itemID})
	svc.SetDeductionStepToggle(fixedDeduct(true))
	// A real order + line, since the reservation FK to orders is deferred and fails at commit
	orderID = seedOrder(t, conn, channelID)
	lineID = seedOrderLine(t, conn, orderID, 4)
	return svc, channelID, orderID, lineID, itemID, locID, productID
}

func TestPlacementDeductTakesOnHandInThePlacementTx(t *testing.T) {
	conn := testdb.Require(t)
	svc, channelID, orderID, lineID, itemID, locID, productID := placementFixture(t, conn, 10)

	_, err := svc.Reserve(context.Background(), channelID, orderID,
		[]ReserveLine{{ProductID: productID, OrderLineID: lineID, Quantity: 4}})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if got := readLevelQty(t, conn, itemID, locID); got != 6 {
		t.Errorf("on-hand after placement = %g, want 6 (deducted immediately)", got)
	}
	// The hold and its consumption cancel out, so nothing stays reserved and salable equals on-hand
	if got := readChainNet(t, conn, itemID, locID); got != 0 {
		t.Errorf("chain net = %g, want 0", got)
	}
	if got := readReserved(t, conn, itemID, locID); got != 0 {
		t.Errorf("reserved counter = %g, want 0", got)
	}

	// The shipment event later finds nothing held and must not deduct a second time
	if err := svc.FulfillLines(context.Background(), orderID, []FulfillLineQty{{OrderLineID: lineID, Quantity: 4}}); err != nil {
		t.Fatalf("FulfillLines: %v", err)
	}
	if got := readLevelQty(t, conn, itemID, locID); got != 6 {
		t.Errorf("on-hand after fulfillment = %g, want 6 (already deducted at placement)", got)
	}
}

func TestPlacementDeductCancelRestoresAndStaysIdempotent(t *testing.T) {
	conn := testdb.Require(t)
	svc, channelID, orderID, lineID, itemID, locID, productID := placementFixture(t, conn, 10)
	svc.SetOrderLineLookup(fixedOrderLines{lines: []OrderLine{{
		ProductID: productID, Quantity: 4, LineID: lineID, LineQuantity: 4, LineFulfilled: 0,
	}}})

	if _, err := svc.Reserve(context.Background(), channelID, orderID,
		[]ReserveLine{{ProductID: productID, OrderLineID: lineID, Quantity: 4}}); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if err := svc.ReleaseForOrder(context.Background(), orderID, ReasonOrderCancelled); err != nil {
		t.Fatalf("ReleaseForOrder: %v", err)
	}
	if got := readLevelQty(t, conn, itemID, locID); got != 10 {
		t.Errorf("on-hand after cancel = %g, want 10 (nothing shipped, all restored)", got)
	}
	if got := readChainNet(t, conn, itemID, locID); got != 0 {
		t.Errorf("chain net after cancel = %g, want 0", got)
	}
	// A redelivered cancel event finds the order_deducted balance already spent and restores nothing
	if err := svc.ReleaseForOrder(context.Background(), orderID, ReasonOrderCancelled); err != nil {
		t.Fatalf("ReleaseForOrder replay: %v", err)
	}
	if got := readLevelQty(t, conn, itemID, locID); got != 10 {
		t.Errorf("on-hand after replayed cancel = %g, want 10 - a replay must not inflate stock", got)
	}
}

func TestPlacementDeductCancelKeepsTheShippedShare(t *testing.T) {
	conn := testdb.Require(t)
	svc, channelID, orderID, lineID, itemID, locID, productID := placementFixture(t, conn, 10)
	// 1 of 4 shipped before the cancellation - those units left the building and must not come back
	svc.SetOrderLineLookup(fixedOrderLines{lines: []OrderLine{{
		ProductID: productID, Quantity: 4, FulfilledQuantity: 1,
		LineID: lineID, LineQuantity: 4, LineFulfilled: 1,
	}}})

	if _, err := svc.Reserve(context.Background(), channelID, orderID,
		[]ReserveLine{{ProductID: productID, OrderLineID: lineID, Quantity: 4}}); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if err := svc.ReleaseForOrder(context.Background(), orderID, ReasonOrderCancelled); err != nil {
		t.Fatalf("ReleaseForOrder: %v", err)
	}
	if got := readLevelQty(t, conn, itemID, locID); got != 9 {
		t.Errorf("on-hand after cancel = %g, want 9 (3 unshipped back, 1 shipped stays gone)", got)
	}
}

func TestPlacementDeductReturnRestocksAtTheDeductedCell(t *testing.T) {
	conn := testdb.Require(t)
	svc, channelID, orderID, lineID, itemID, locID, productID := placementFixture(t, conn, 10)

	if _, err := svc.Reserve(context.Background(), channelID, orderID,
		[]ReserveLine{{ProductID: productID, OrderLineID: lineID, Quantity: 4}}); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	returnID := uuid.New()
	if err := svc.RestockReturn(context.Background(), orderID, returnID,
		[]RestockLineQty{{OrderLineID: lineID, Quantity: 2}}); err != nil {
		t.Fatalf("RestockReturn: %v", err)
	}
	if got := readLevelQty(t, conn, itemID, locID); got != 8 {
		t.Errorf("on-hand after return = %g, want 8", got)
	}
	// Replay of the same return no-ops on its marker
	if err := svc.RestockReturn(context.Background(), orderID, returnID,
		[]RestockLineQty{{OrderLineID: lineID, Quantity: 2}}); err != nil {
		t.Fatalf("RestockReturn replay: %v", err)
	}
	if got := readLevelQty(t, conn, itemID, locID); got != 8 {
		t.Errorf("on-hand after replayed return = %g, want 8", got)
	}
}

func TestPlacementDeductSpillsAcrossLocationsInPriorityOrder(t *testing.T) {
	conn := testdb.Require(t)
	svc, channelID, orderID, lineID, itemID, locA, productID := placementFixture(t, conn, 3)
	locB := seedSecondLocation(t, conn, itemID, 5)
	attachLocation(t, conn, channelID, locB, 2)

	if _, err := svc.Reserve(context.Background(), channelID, orderID,
		[]ReserveLine{{ProductID: productID, OrderLineID: lineID, Quantity: 6}}); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if got := readLevelQty(t, conn, itemID, locA); got != 0 {
		t.Errorf("priority location on-hand = %g, want 0 (drained first)", got)
	}
	if got := readLevelQty(t, conn, itemID, locB); got != 2 {
		t.Errorf("spill location on-hand = %g, want 2", got)
	}
}

// An edit re-deducts the new line set, so the old deduction must come back first and shipped units
// must be excluded from the re-deduction - or every edit stacks another full deduction
func TestPlacementDeductEditSwapsTheDeduction(t *testing.T) {
	conn := testdb.Require(t)
	svc, channelID, orderID, lineID, itemID, locID, productID := placementFixture(t, conn, 10)
	sku := "DEDUCT-EDIT-" + uuid.NewString()[:8]
	svc.SetProductLookup(skuProducts{sku: sku, productID: productID})
	// 1 of the original 4 shipped; the edit changes the line to 4 (so 3 still to ship)
	svc.SetOrderLineLookup(fixedOrderLines{lines: []OrderLine{{
		ProductID: productID, Quantity: 4, FulfilledQuantity: 1,
		LineID: lineID, LineQuantity: 4, LineFulfilled: 1,
	}}})

	if _, err := svc.Reserve(context.Background(), channelID, orderID,
		[]ReserveLine{{ProductID: productID, OrderLineID: lineID, Quantity: 4}}); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	edited := cart.Cart{Lines: []cart.CartLine{{ID: lineID, ProductSku: sku, Quantity: 4}}}
	if err := svc.tx.InTransaction(context.Background(), func(t2 *sql.Tx) error {
		return svc.ReReserveForEditTx(context.Background(), t2, channelID, orderID, edited)
	}); err != nil {
		t.Fatalf("ReReserveForEditTx: %v", err)
	}
	// 10 - 4 (placement) + 3 (unshipped restored) - 3 (unshipped share of the new set) = 6:
	// exactly newQty deducted overall, the shipped unit counted once
	if got := readLevelQty(t, conn, itemID, locID); got != 6 {
		t.Errorf("on-hand after edit = %g, want 6", got)
	}
	if got := readChainNet(t, conn, itemID, locID); got != 0 {
		t.Errorf("chain net after edit = %g, want 0", got)
	}
}

// skuProducts resolves one SKU for linesFromCart, unlike mapProducts which never resolves
type skuProducts struct {
	sku       string
	productID uuid.UUID
}

func (s skuProducts) GetProductIDBySKU(_ context.Context, sku string) (uuid.UUID, error) {
	if sku == s.sku {
		return s.productID, nil
	}
	return uuid.Nil, ErrItemNotFound
}

func (s skuProducts) KitComponents(context.Context, uuid.UUID) ([]KitComponentRef, error) {
	return nil, nil
}
