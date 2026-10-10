package inventory

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// The manual-adjustment paths - an operator counting a shelf, correcting shrinkage, receiving a delivery - plus the threshold events merchandising leans on

type adjustRepo struct {
	Repository
	items    map[uuid.UUID]Item    // by item id
	byProd   map[uuid.UUID]Item    // by product id
	levels   map[uuid.UUID][]Level // by item id
	written  []Reservation
	setQtyIn []SetItemQuantityInput
}

func (r *adjustRepo) WithTx(*sql.Tx) Repository { return r }

func (r *adjustRepo) GetItemByID(_ context.Context, id uuid.UUID) (Item, error) {
	if it, ok := r.items[id]; ok {
		return it, nil
	}
	return Item{}, ErrItemNotFound
}

func (r *adjustRepo) GetItemByProductID(_ context.Context, pid uuid.UUID) (Item, error) {
	if it, ok := r.byProd[pid]; ok {
		return it, nil
	}
	return Item{}, ErrItemNotFound
}

func (r *adjustRepo) GetLevel(_ context.Context, itemID, locID uuid.UUID) (Level, error) {
	for _, l := range r.levels[itemID] {
		if l.LocationID == locID {
			return l, nil
		}
	}
	return Level{}, ErrLevelNotFound
}

func (r *adjustRepo) EnsureLevel(ctx context.Context, itemID, locID uuid.UUID) error {
	if _, err := r.GetLevel(ctx, itemID, locID); errors.Is(err, ErrLevelNotFound) {
		r.levels[itemID] = append(r.levels[itemID], Level{InventoryItemID: itemID, LocationID: locID})
	}
	return nil
}

func (r *adjustRepo) GetLevelForUpdate(ctx context.Context, itemID, locID uuid.UUID) (Level, error) {
	return r.GetLevel(ctx, itemID, locID)
}

func (r *adjustRepo) UpsertLevel(_ context.Context, itemID, locID uuid.UUID, qty float64) (Level, error) {
	for i := range r.levels[itemID] {
		if r.levels[itemID][i].LocationID == locID {
			r.levels[itemID][i].Quantity = qty
			return r.levels[itemID][i], nil
		}
	}
	lvl := Level{InventoryItemID: itemID, LocationID: locID, Quantity: qty}
	r.levels[itemID] = append(r.levels[itemID], lvl)
	return lvl, nil
}

func (r *adjustRepo) InsertReservation(_ context.Context, res Reservation) (Reservation, error) {
	r.written = append(r.written, res)
	return res, nil
}

func (r *adjustRepo) ListLevelsByItem(_ context.Context, itemID uuid.UUID) ([]Level, error) {
	return r.levels[itemID], nil
}

func (r *adjustRepo) SetItemQuantity(_ context.Context, in SetItemQuantityInput) (Item, error) {
	r.setQtyIn = append(r.setQtyIn, in)
	it := r.byProd[in.ProductID]
	it.Quantity = in.Quantity
	r.byProd[in.ProductID] = it
	return it, nil
}

func newAdjustFixture(t *testing.T, qty, threshold float64) (*Service, *adjustRepo, *stockSink, uuid.UUID, uuid.UUID) {
	t.Helper()
	itemID, locID, prodID := uuid.New(), uuid.New(), uuid.New()
	item := Item{ID: itemID, ProductID: prodID, Tracked: true, LowStockThreshold: threshold, Quantity: qty}
	repo := &adjustRepo{
		items:  map[uuid.UUID]Item{itemID: item},
		byProd: map[uuid.UUID]Item{prodID: item},
		levels: map[uuid.UUID][]Level{itemID: {{InventoryItemID: itemID, LocationID: locID, Quantity: qty}}},
	}
	sink := newStockSink()
	svc := NewService(repo, newInvTxManager(t))
	svc.SetFlatModeToggle(toggle(false))
	svc.SetLedgerToggle(toggle(true))
	svc.SetEventBus(sink.bus)
	return svc, repo, sink, itemID, locID
}

// An adjustment records the DELTA, not the new total - a ledger row carrying the absolute quantity
// would read as an enormous hold the moment anything summed it
func TestAdjustLevelWritesDeltaAuditRow(t *testing.T) {
	svc, repo, _, itemID, locID := newAdjustFixture(t, 10, 0)

	lvl, err := svc.AdjustLevel(context.Background(), AdjustLevelInput{
		InventoryItemID: itemID, LocationID: locID, NewQuantity: 14, Note: "delivery",
	})
	if err != nil {
		t.Fatalf("AdjustLevel: %v", err)
	}
	if lvl.Quantity != 14 {
		t.Errorf("level quantity = %g, want 14", lvl.Quantity)
	}
	if len(repo.written) != 1 {
		t.Fatalf("wrote %d ledger rows, want 1", len(repo.written))
	}
	row := repo.written[0]
	if row.QuantityDelta != 4 {
		t.Errorf("ledger delta = %g, want 4 (14 - 10)", row.QuantityDelta)
	}
	// anything but manual_adjustment here is counted as a hold by the reserved recompute
	if row.Reason != ReasonManualAdjustment {
		t.Errorf("ledger reason = %q, want manual_adjustment", row.Reason)
	}
	if row.Note != "delivery" {
		t.Errorf("note not carried onto the audit row: %q", row.Note)
	}
}

// Shrinkage goes the other way, and the sign has to survive
func TestAdjustLevelNegativeDelta(t *testing.T) {
	svc, repo, _, itemID, locID := newAdjustFixture(t, 10, 0)

	if _, err := svc.AdjustLevel(context.Background(), AdjustLevelInput{
		InventoryItemID: itemID, LocationID: locID, NewQuantity: 6,
	}); err != nil {
		t.Fatalf("AdjustLevel: %v", err)
	}
	if repo.written[0].QuantityDelta != -4 {
		t.Errorf("ledger delta = %g, want -4", repo.written[0].QuantityDelta)
	}
}

// Setting the same number is not a change - an audit row for it is noise that also drags the
// recompute across rows meaning nothing
func TestAdjustLevelNoOpWritesNothing(t *testing.T) {
	svc, repo, _, itemID, locID := newAdjustFixture(t, 10, 0)

	if _, err := svc.AdjustLevel(context.Background(), AdjustLevelInput{
		InventoryItemID: itemID, LocationID: locID, NewQuantity: 10,
	}); err != nil {
		t.Fatalf("AdjustLevel: %v", err)
	}
	if len(repo.written) != 0 {
		t.Errorf("a no-op adjustment wrote %d ledger rows, want 0", len(repo.written))
	}
}

// A location with no level yet starts from zero rather than erroring, so receiving stock into a new
// location is an adjustment like any other
func TestAdjustLevelMissingLevelStartsAtZero(t *testing.T) {
	svc, repo, _, itemID, _ := newAdjustFixture(t, 10, 0)

	fresh := uuid.New()
	lvl, err := svc.AdjustLevel(context.Background(), AdjustLevelInput{
		InventoryItemID: itemID, LocationID: fresh, NewQuantity: 7,
	})
	if err != nil {
		t.Fatalf("AdjustLevel: %v", err)
	}
	if lvl.Quantity != 7 {
		t.Errorf("quantity = %g, want 7", lvl.Quantity)
	}
	if repo.written[0].QuantityDelta != 7 {
		t.Errorf("delta = %g, want 7 (0 -> 7)", repo.written[0].QuantityDelta)
	}
}

func TestAdjustLevelRejectsNegative(t *testing.T) {
	svc, repo, _, itemID, locID := newAdjustFixture(t, 10, 0)

	_, err := svc.AdjustLevel(context.Background(), AdjustLevelInput{
		InventoryItemID: itemID, LocationID: locID, NewQuantity: -1,
	})
	if !errors.Is(err, ErrNegativeQuantity) {
		t.Fatalf("got %v, want ErrNegativeQuantity", err)
	}
	if len(repo.written) != 0 {
		t.Error("a rejected adjustment still wrote to the ledger")
	}
}

// Per-location adjustment is meaningless when the store runs flat, and silently accepting it would
// write a level the flat path never reads
func TestAdjustLevelRefusedInFlatMode(t *testing.T) {
	svc, _, _, itemID, locID := newAdjustFixture(t, 10, 0)
	svc.SetFlatModeToggle(toggle(true))

	_, err := svc.AdjustLevel(context.Background(), AdjustLevelInput{
		InventoryItemID: itemID, LocationID: locID, NewQuantity: 3,
	})
	if !errors.Is(err, ErrFlatModePerLocation) {
		t.Fatalf("got %v, want ErrFlatModePerLocation", err)
	}
}

// The threshold events merchandising and ops subscribe to - each fires on a CROSSING, so a level
// already below threshold must not re-alert on every further adjustment
func TestAdjustLevelThresholdEvents(t *testing.T) {
	cases := []struct {
		name             string
		from, to         float64
		threshold        float64
		wantOut, wantLow bool
	}{
		{name: "crossing to zero raises stock out", from: 5, to: 0, threshold: 0, wantOut: true},
		{name: "crossing below threshold raises stock low", from: 10, to: 2, threshold: 3, wantLow: true},
		{name: "already below threshold does not re-alert", from: 2, to: 1, threshold: 3},
		{name: "already at zero does not re-alert", from: 0, to: 0, threshold: 3},
		{name: "restocking above threshold is silent", from: 1, to: 20, threshold: 3},
		{name: "zero threshold disables the low alert", from: 10, to: 1, threshold: 0},
		// out wins over low: a merchant paged about "low" when it is actually zero acts too slowly
		{name: "hitting zero is out, not low", from: 10, to: 0, threshold: 3, wantOut: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, sink, itemID, locID := newAdjustFixture(t, tc.from, tc.threshold)
			if _, err := svc.AdjustLevel(context.Background(), AdjustLevelInput{
				InventoryItemID: itemID, LocationID: locID, NewQuantity: tc.to,
			}); err != nil {
				t.Fatalf("AdjustLevel: %v", err)
			}
			sink.drain(t)
			if got := sink.counts(EventStockOut); (got > 0) != tc.wantOut {
				t.Errorf("stock_out fired %d times, want fired=%v", got, tc.wantOut)
			}
			if got := sink.counts(EventStockLow); (got > 0) != tc.wantLow {
				t.Errorf("stock_low fired %d times, want fired=%v", got, tc.wantLow)
			}
		})
	}
}

// Flat mode's absolute write - same guard, same events, no per-location ledger
func TestSetItemQuantity(t *testing.T) {
	t.Run("rejects negative", func(t *testing.T) {
		svc, repo, _, _, _ := newAdjustFixture(t, 10, 0)
		var prodID uuid.UUID
		for p := range repo.byProd {
			prodID = p
		}
		_, err := svc.SetItemQuantity(context.Background(), SetItemQuantityInput{ProductID: prodID, Quantity: -5})
		if !errors.Is(err, ErrNegativeQuantity) {
			t.Fatalf("got %v, want ErrNegativeQuantity", err)
		}
		if len(repo.setQtyIn) != 0 {
			t.Error("a rejected set still reached the repository")
		}
	})

	t.Run("writes the absolute quantity and reports the delta", func(t *testing.T) {
		svc, repo, sink, _, _ := newAdjustFixture(t, 10, 0)
		var prodID uuid.UUID
		for p := range repo.byProd {
			prodID = p
		}
		item, err := svc.SetItemQuantity(context.Background(), SetItemQuantityInput{ProductID: prodID, Quantity: 4})
		if err != nil {
			t.Fatalf("SetItemQuantity: %v", err)
		}
		if item.Quantity != 4 {
			t.Errorf("quantity = %g, want 4", item.Quantity)
		}
		got := sink.drain(t)
		if len(got) != 1 {
			t.Fatalf("published %d StockChanged, want 1", len(got))
		}
		// the event carries the movement, not just the new total, so subscribers can act on size
		if got[0].Before != 10 || got[0].After != 4 || got[0].Delta != -6 {
			t.Errorf("before/after/delta = %g/%g/%g, want 10/4/-6", got[0].Before, got[0].After, got[0].Delta)
		}
	})
}

// reserved_quantity is stored positive, but the allocator works in net ledger terms where a hold is
// negative - flipping the sign here would make held stock read as EXTRA available
func TestNetReservationsForItemNegatesTheCounter(t *testing.T) {
	itemID, locA, locB := uuid.New(), uuid.New(), uuid.New()
	repo := &adjustRepo{
		items: map[uuid.UUID]Item{itemID: {ID: itemID}},
		levels: map[uuid.UUID][]Level{itemID: {
			{InventoryItemID: itemID, LocationID: locA, Quantity: 10, ReservedQuantity: 3},
			{InventoryItemID: itemID, LocationID: locB, Quantity: 5, ReservedQuantity: 0},
		}},
	}
	svc := NewService(repo, newInvTxManager(t))

	net, err := svc.NetReservationsForItem(context.Background(), itemID)
	if err != nil {
		t.Fatalf("NetReservationsForItem: %v", err)
	}
	if net[locA] != -3 {
		t.Errorf("location A net = %g, want -3", net[locA])
	}
	if net[locB] != 0 {
		t.Errorf("location B net = %g, want 0", net[locB])
	}
}
