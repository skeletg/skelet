package inventory

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	dbsqlc "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// ReserveTx is the one write that stands between a placement and an oversell, and its subtle parts - the in-call `planned` accumulator, the routing guard, the ledger kill switch - only exist against a real transaction, since the whole point is what a re-entrant read inside one tx can and can't see  The fixture helpers (seedLevel, seedSecondLocation, readReserved) come from reserved_recompute_db_test.go - same package, same disposable-database gate

// reserveSvc builds a service over the live connection with the product lookup the reservation needs
func reserveSvc(t *testing.T, conn *sql.DB, productByID map[uuid.UUID]uuid.UUID) *Service {
	t.Helper()
	q := dbsqlc.New(conn)
	txMgr := txrunner.New(conn)
	s := NewService(NewRepository(q, txMgr), txMgr)
	s.SetProductLookup(mapProducts{byProduct: productByID})
	return s
}

type mapProducts struct{ byProduct map[uuid.UUID]uuid.UUID }

func (m mapProducts) GetProductIDBySKU(context.Context, string) (uuid.UUID, error) {
	return uuid.Nil, errors.New("not used")
}

func (m mapProducts) KitComponents(context.Context, uuid.UUID) ([]KitComponentRef, error) {
	return nil, nil
}

// attachLocation makes a location visible to a channel, which is what ReserveTx allocates over
func attachLocation(t *testing.T, conn *sql.DB, channelID, locID uuid.UUID, priority int32) {
	t.Helper()
	if _, err := conn.Exec(
		`INSERT INTO inventory_channel_locations (channel_id, location_id, priority) VALUES ($1, $2, $3)`,
		channelID, locID, priority); err != nil {
		t.Fatalf("attach location to channel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM inventory_channel_locations WHERE channel_id = $1 AND location_id = $2`, channelID, locID)
	})
}

// someChannel reuses an existing channel row so the fixture stays valid as channel requirements change
func someChannel(t *testing.T, conn *sql.DB) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&id); err != nil {
		t.Skipf("no channel to scope reservations to: %v", err)
	}
	return id
}

// productOf resolves the fixture item's product id, which ReserveLine is keyed on
func productOf(t *testing.T, conn *sql.DB, itemID uuid.UUID) uuid.UUID {
	t.Helper()
	var pid uuid.UUID
	if err := conn.QueryRow(`SELECT product_id FROM inventory_items WHERE id = $1`, itemID).Scan(&pid); err != nil {
		t.Fatalf("read item product: %v", err)
	}
	return pid
}

// seedOrder inserts a minimal order row - inventory_reservations.order_id FKs to orders (deferred, so a fabricated id only fails at commit), and the reservation is meaningless without one anyway
func seedOrder(t *testing.T, conn *sql.DB, channelID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO orders (channel_id, currency_code, email) VALUES ($1, 'USD', $2) RETURNING id`,
		channelID, "reserve-"+uuid.NewString()[:8]+"@e2e.test").Scan(&id); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM inventory_reservations WHERE order_id = $1`, id)
		_, _ = conn.Exec(`DELETE FROM orders WHERE id = $1`, id)
	})
	return id
}

// seedOrderLine inserts a real order line - inventory_reservations.order_line_id FKs to it, and the reservation is what links a hold back to the line it was taken for
func seedOrderLine(t *testing.T, conn *sql.DB, orderID uuid.UUID, qty float64) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO order_lines (order_id, product_name, product_sku, unit_price, quantity, total_price)
		 VALUES ($1, 'fixture', $2, 100, $3, 100) RETURNING id`,
		orderID, "RSV-"+uuid.NewString()[:8], qty).Scan(&id); err != nil {
		t.Fatalf("seed order line: %v", err)
	}
	return id
}

func reservationRows(t *testing.T, conn *sql.DB, orderID uuid.UUID) (count int, totalDelta float64) {
	t.Helper()
	if err := conn.QueryRow(
		`SELECT count(*), coalesce(sum(quantity_delta), 0) FROM inventory_reservations WHERE order_id = $1`,
		orderID).Scan(&count, &totalDelta); err != nil {
		t.Fatalf("read reservations: %v", err)
	}
	return count, totalDelta
}

// The documented oversell guard: two lines for the SAME product in one call must not each read the same availability. The FOR UPDATE lock can't catch it, because within one transaction the second read is re-entrant and still cannot see the first line's uninserted reservation
func TestReserveTxAccumulatesPlannedStockAcrossLinesForOneProduct(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 5, 0)
	pid := productOf(t, conn, itemID)
	ch := someChannel(t, conn)
	attachLocation(t, conn, ch, locID, 0)

	s := reserveSvc(t, conn, nil)
	orderID := seedOrder(t, conn, ch)

	// two lines, 3 + 3, against 5 on hand - each alone fits, together they don't
	_, err := s.Reserve(context.Background(), ch, orderID, []ReserveLine{
		{ProductID: pid, OrderLineID: seedOrderLine(t, conn, orderID, 3), Quantity: 3},
		{ProductID: pid, OrderLineID: seedOrderLine(t, conn, orderID, 3), Quantity: 3},
	})
	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("two lines totalling 6 of 5 on hand = %v, want ErrInsufficientStock", err)
	}
	if n, _ := reservationRows(t, conn, orderID); n != 0 {
		t.Errorf("%d reservation rows after the refused call, want 0 - the tx must roll back", n)
	}
	if got := readReserved(t, conn, itemID, locID); got != 0 {
		t.Errorf("reserved = %v after the refused call, want 0", got)
	}

	// and the same two lines DO fit when the total is within stock
	ok := seedOrder(t, conn, ch)
	if _, err := s.Reserve(context.Background(), ch, ok, []ReserveLine{
		{ProductID: pid, OrderLineID: seedOrderLine(t, conn, ok, 3), Quantity: 3},
		{ProductID: pid, OrderLineID: seedOrderLine(t, conn, ok, 2), Quantity: 2},
	}); err != nil {
		t.Fatalf("two lines totalling exactly the 5 on hand: %v", err)
	}
	n, delta := reservationRows(t, conn, ok)
	if n != 2 {
		t.Errorf("%d reservation rows, want one per line", n)
	}
	if delta != -5 {
		t.Errorf("reservation deltas sum to %v, want -5 (holds are negative)", delta)
	}
}

// A hold is a NEGATIVE ledger delta carrying its order and line, which is how release and fulfilment find it later - a positive delta would read as a restock
func TestReserveTxWritesNegativeHoldsLinkedToTheOrderLine(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 10, 0)
	pid := productOf(t, conn, itemID)
	ch := someChannel(t, conn)
	attachLocation(t, conn, ch, locID, 0)

	s := reserveSvc(t, conn, nil)
	orderID := seedOrder(t, conn, ch)
	lineID := seedOrderLine(t, conn, orderID, 4)

	res, err := s.Reserve(context.Background(), ch, orderID, []ReserveLine{
		{ProductID: pid, OrderLineID: lineID, Quantity: 4},
	})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if len(res.Allocations) != 1 || res.Allocations[0].Quantity != 4 {
		t.Fatalf("allocations = %+v, want one for 4", res.Allocations)
	}

	var delta float64
	var reason string
	var gotLine uuid.UUID
	if err := conn.QueryRow(
		`SELECT quantity_delta, reason, order_line_id FROM inventory_reservations WHERE order_id = $1`,
		orderID).Scan(&delta, &reason, &gotLine); err != nil {
		t.Fatalf("read reservation: %v", err)
	}
	if delta != -4 {
		t.Errorf("quantity_delta = %v, want -4", delta)
	}
	if reason != string(ReasonOrderPlaced) {
		t.Errorf("reason = %q, want %q", reason, ReasonOrderPlaced)
	}
	if gotLine != lineID {
		t.Errorf("order_line_id = %v, want the reserved line %v", gotLine, lineID)
	}
	// the denormalized counter Salable reads has to move with the ledger
	if got := readReserved(t, conn, itemID, locID); got != 4 {
		t.Errorf("reserved counter = %v, want 4", got)
	}
}

// A shortage on the SECOND line has to roll back the first line's allocation - a partial reserve would hold stock for an order that never placed
func TestReserveTxRollsBackEarlierLinesOnAShortage(t *testing.T) {
	conn := recomputeDB(t)
	itemA, locID := seedLevel(t, conn, 10, 0)
	pidA := productOf(t, conn, itemA)
	itemB, locB := seedLevel(t, conn, 1, 0)
	pidB := productOf(t, conn, itemB)
	ch := someChannel(t, conn)
	attachLocation(t, conn, ch, locID, 0)
	attachLocation(t, conn, ch, locB, 1)

	s := reserveSvc(t, conn, nil)
	orderID := seedOrder(t, conn, ch)

	_, err := s.Reserve(context.Background(), ch, orderID, []ReserveLine{
		{ProductID: pidA, OrderLineID: seedOrderLine(t, conn, orderID, 2), Quantity: 2}, // fits
		{ProductID: pidB, OrderLineID: seedOrderLine(t, conn, orderID, 9), Quantity: 9}, // does not
	})
	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("Reserve = %v, want ErrInsufficientStock", err)
	}
	if n, _ := reservationRows(t, conn, orderID); n != 0 {
		t.Errorf("%d reservation rows, want 0 - the first line must roll back with the second", n)
	}
	if got := readReserved(t, conn, itemA, locID); got != 0 {
		t.Errorf("the first product's reserved counter = %v, want 0", got)
	}
}

// A backorderable item is allowed past its stock, which is the whole point of the flag
func TestReserveTxAllowsABackorderableShortage(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 1, 0)
	if _, err := conn.Exec(`UPDATE inventory_items SET backorderable = true WHERE id = $1`, itemID); err != nil {
		t.Fatalf("mark backorderable: %v", err)
	}
	pid := productOf(t, conn, itemID)
	ch := someChannel(t, conn)
	attachLocation(t, conn, ch, locID, 0)

	s := reserveSvc(t, conn, nil)
	orderID := seedOrder(t, conn, ch)

	res, err := s.Reserve(context.Background(), ch, orderID, []ReserveLine{
		{ProductID: pid, OrderLineID: seedOrderLine(t, conn, orderID, 7), Quantity: 7},
	})
	if err != nil {
		t.Fatalf("reserving 7 of a backorderable item with 1 on hand: %v", err)
	}
	// the allocator records what came off the shelf separately from what is backordered, so the row count is its own business - the whole 7 has to be held either way
	var allocated float64
	for _, a := range res.Allocations {
		allocated += a.Quantity
	}
	if allocated != 7 {
		t.Errorf("allocated %v across %d allocations, want the full 7", allocated, len(res.Allocations))
	}
	if _, delta := reservationRows(t, conn, orderID); delta != -7 {
		t.Errorf("reservation deltas sum to %v, want -7", delta)
	}
	// and the counter goes past the 1 on hand, which is what backordering means
	if got := readReserved(t, conn, itemID, locID); got != 7 {
		t.Errorf("reserved counter = %v, want 7 held against 1 on hand", got)
	}
}

// The ledger toggle is the cutover kill switch - with another platform owning inventory truth, a placement must write NOTHING here rather than half-tracking it
func TestReserveTxWritesNothingWithTheLedgerDisabled(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 10, 0)
	pid := productOf(t, conn, itemID)
	ch := someChannel(t, conn)
	attachLocation(t, conn, ch, locID, 0)

	s := reserveSvc(t, conn, nil)
	s.SetLedgerToggle(toggle(false))
	orderID := seedOrder(t, conn, ch)

	res, err := s.Reserve(context.Background(), ch, orderID, []ReserveLine{
		{ProductID: pid, OrderLineID: seedOrderLine(t, conn, orderID, 99), Quantity: 99}, // well past stock
	})
	if err != nil {
		t.Fatalf("Reserve with the ledger off = %v, want it to pass through", err)
	}
	if len(res.Allocations) != 0 {
		t.Errorf("allocations = %+v, want none", res.Allocations)
	}
	if n, _ := reservationRows(t, conn, orderID); n != 0 {
		t.Errorf("%d reservation rows written with the ledger off, want 0", n)
	}
	if got := readReserved(t, conn, itemID, locID); got != 0 {
		t.Errorf("reserved counter = %v, want it untouched", got)
	}
}

// A channel with no locations has nothing to allocate from, and it's treated as ungated rather than as a shortage - otherwise a store mid-setup can't take orders at all
func TestReserveTxIsUngatedForAChannelWithNoLocations(t *testing.T) {
	conn := recomputeDB(t)
	itemID, _ := seedLevel(t, conn, 10, 0)
	pid := productOf(t, conn, itemID)
	ch := someChannel(t, conn)
	// deliberately NOT attaching any location to the channel
	if _, err := conn.Exec(`DELETE FROM inventory_channel_locations WHERE channel_id = $1`, ch); err != nil {
		t.Fatalf("clear channel locations: %v", err)
	}

	s := reserveSvc(t, conn, nil)
	orderID := seedOrder(t, conn, ch)

	res, err := s.Reserve(context.Background(), ch, orderID, []ReserveLine{
		{ProductID: pid, OrderLineID: seedOrderLine(t, conn, orderID, 99), Quantity: 99},
	})
	if err != nil {
		t.Fatalf("Reserve with no channel locations = %v, want it ungated", err)
	}
	if len(res.Allocations) != 0 {
		t.Errorf("allocations = %+v, want none", res.Allocations)
	}
	if n, _ := reservationRows(t, conn, orderID); n != 0 {
		t.Errorf("%d reservation rows, want 0", n)
	}
}

// Stock spread across two locations has to be drawn from both rather than failing because neither alone can cover the line
func TestReserveTxSplitsALineAcrossLocations(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locA := seedLevel(t, conn, 3, 0)
	locB := seedSecondLocation(t, conn, itemID, 4)
	pid := productOf(t, conn, itemID)
	ch := someChannel(t, conn)
	attachLocation(t, conn, ch, locA, 0)
	attachLocation(t, conn, ch, locB, 1)

	s := reserveSvc(t, conn, nil)
	orderID := seedOrder(t, conn, ch)

	res, err := s.Reserve(context.Background(), ch, orderID, []ReserveLine{
		{ProductID: pid, OrderLineID: seedOrderLine(t, conn, orderID, 6), Quantity: 6}, // 3 here + 3 there
	})
	if err != nil {
		t.Fatalf("Reserve across two locations: %v", err)
	}
	if len(res.Allocations) != 2 {
		t.Fatalf("%d allocations, want the line split across both locations: %+v", len(res.Allocations), res.Allocations)
	}
	var total float64
	for _, a := range res.Allocations {
		total += a.Quantity
	}
	if total != 6 {
		t.Errorf("allocated %v across locations, want 6", total)
	}
	if got := readReserved(t, conn, itemID, locA); got != 3 {
		t.Errorf("first location reserved = %v, want its full 3", got)
	}
	if got := readReserved(t, conn, itemID, locB); got != 3 {
		t.Errorf("second location reserved = %v, want the remaining 3", got)
	}
}

// ReconcileReserved rebuilds the denormalized counter from the ledger, which is what repairs drift - Salable reads the counter, so a broken recompute makes every availability answer wrong silently
func TestReconcileReservedRepairsADriftedCounter(t *testing.T) {
	conn := recomputeDB(t)
	itemID, locID := seedLevel(t, conn, 100, 42) // 42 is pure drift, no ledger behind it
	addReservation(t, conn, itemID, locID, -7, ReasonOrderPlaced)

	s := reserveSvc(t, conn, nil)
	if err := s.ReconcileReserved(context.Background()); err != nil {
		t.Fatalf("ReconcileReserved: %v", err)
	}
	if got := readReserved(t, conn, itemID, locID); got != 7 {
		t.Errorf("reserved after reconcile = %v, want the ledger's 7 rather than the drifted 42", got)
	}

	// and it's idempotent - a second sweep must not double or zero it
	if err := s.ReconcileReserved(context.Background()); err != nil {
		t.Fatalf("second ReconcileReserved: %v", err)
	}
	if got := readReserved(t, conn, itemID, locID); got != 7 {
		t.Errorf("reserved after a second reconcile = %v, want still 7", got)
	}
}
