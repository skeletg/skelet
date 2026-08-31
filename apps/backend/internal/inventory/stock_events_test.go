package inventory

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

// Backs the ledger paths that publish StockChanged - levels and rows are mutated in place so the post-commit salable read-back sees the write
type stockEventRepo struct {
	Repository
	items  map[uuid.UUID]Item    // by inventory item id
	levels map[uuid.UUID][]Level // by inventory item id
	rows   []Reservation
}

func (r *stockEventRepo) WithTx(*sql.Tx) Repository { return r }

func (r *stockEventRepo) GetItemByID(_ context.Context, id uuid.UUID) (Item, error) {
	if it, ok := r.items[id]; ok {
		return it, nil
	}
	return Item{}, ErrItemNotFound
}

func (r *stockEventRepo) ListItemsByProductIDs(_ context.Context, productIDs []uuid.UUID) (map[uuid.UUID]Item, error) {
	out := make(map[uuid.UUID]Item)
	for _, pid := range productIDs {
		for _, it := range r.items {
			if it.ProductID == pid {
				out[pid] = it
			}
		}
	}
	return out, nil
}

func (r *stockEventRepo) ListLevelsByItems(_ context.Context, itemIDs []uuid.UUID) ([]Level, error) {
	var out []Level
	for _, id := range itemIDs {
		out = append(out, r.levels[id]...)
	}
	return out, nil
}

func (r *stockEventRepo) ListReservationsForOrder(context.Context, uuid.UUID) ([]Reservation, error) {
	return r.rows, nil
}

func (r *stockEventRepo) InsertReservation(_ context.Context, res Reservation) (Reservation, error) {
	r.rows = append(r.rows, res)
	if res.Reason != ReasonManualAdjustment && res.QuantityDelta != 0 {
		r.bumpLevel(res.InventoryItemID, res.LocationID, 0, -res.QuantityDelta)
	}
	return res, nil
}

func (r *stockEventRepo) AdjustLevelQuantity(_ context.Context, itemID, locationID uuid.UUID, delta float64) (Level, error) {
	lvl := r.bumpLevel(itemID, locationID, delta, 0)
	if lvl == nil {
		return Level{}, ErrLevelNotFound
	}
	return *lvl, nil
}

func (r *stockEventRepo) bumpLevel(itemID, locationID uuid.UUID, qty, reserved float64) *Level {
	for i := range r.levels[itemID] {
		if r.levels[itemID][i].LocationID == locationID {
			r.levels[itemID][i].Quantity += qty
			r.levels[itemID][i].ReservedQuantity += reserved
			return &r.levels[itemID][i]
		}
	}
	return nil
}

// Collects every StockChanged the service emits - Publish dispatches each on its own goroutine, so a service emitting one event per product appends from several at once, and without the mutex one is silently lost (an occasional "published 1 events, want one per product")
type stockSink struct {
	bus  *events.Bus
	mu   sync.Mutex
	seen []*StockChanged
	// StockOut and StockLow are threshold CROSSINGS rather than movements, so they're counted
	// separately - the adjust tests assert on whether one fired at all
	fired map[string]int
}

func newStockSink() *stockSink {
	s := &stockSink{bus: events.NewBus(), fired: map[string]int{}}
	s.bus.Subscribe(EventStockChanged, func(_ context.Context, e events.Event) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.seen = append(s.seen, e.(*StockChanged))
		s.fired[EventStockChanged]++
		return nil
	})
	for _, typ := range []string{EventStockOut, EventStockLow} {
		s.bus.Subscribe(typ, func(_ context.Context, _ events.Event) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.fired[typ]++
			return nil
		})
	}
	return s
}

// How many of one event type arrived - call after drain
func (s *stockSink) counts(eventType string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fired[eventType]
}

// Drain waits for every handler to return, so the copy taken after it is complete
func (s *stockSink) drain(t *testing.T) []*StockChanged {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.bus.Drain(ctx); err != nil {
		t.Fatalf("drain bus: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*StockChanged(nil), s.seen...)
}

// item plus its levels, one row per location
func stockFixture(locs []uuid.UUID, qty, reserved float64) (Item, []Level) {
	item := Item{ID: uuid.New(), ProductID: uuid.New(), Tracked: true}
	levels := make([]Level, len(locs))
	for i, loc := range locs {
		levels[i] = Level{InventoryItemID: item.ID, LocationID: loc, Quantity: qty, ReservedQuantity: reserved}
	}
	return item, levels
}

func newStockEventSvc(t *testing.T, repo *stockEventRepo, sink *stockSink) *Service {
	t.Helper()
	svc := NewService(repo, newInvTxManager(t))
	svc.SetFlatModeToggle(toggle(false))
	svc.SetLedgerToggle(toggle(true))
	svc.SetEventBus(sink.bus)
	return svc
}

func TestReleaseForOrderPublishesStockChanged(t *testing.T) {
	locA, locB := uuid.New(), uuid.New()
	orderID, lineID := uuid.New(), uuid.New()

	tests := []struct {
		name      string
		rows      func(itemID uuid.UUID) []Reservation
		wantCount int
		wantDelta float64
		wantAfter float64
	}{
		{
			// two cells of one product must collapse into ONE event, not one per ledger row
			name: "held cells fold into a single per-product event",
			rows: func(itemID uuid.UUID) []Reservation {
				return []Reservation{
					{InventoryItemID: itemID, LocationID: locA, QuantityDelta: -2, Reason: ReasonOrderPlaced, OrderID: &orderID, OrderLineID: &lineID},
					{InventoryItemID: itemID, LocationID: locB, QuantityDelta: -3, Reason: ReasonOrderPlaced, OrderID: &orderID, OrderLineID: &lineID},
				}
			},
			wantCount: 1,
			wantDelta: 5,
			wantAfter: 15,
		},
		{
			name: "an already settled chain publishes nothing",
			rows: func(itemID uuid.UUID) []Reservation {
				return []Reservation{
					{InventoryItemID: itemID, LocationID: locA, QuantityDelta: -2, Reason: ReasonOrderPlaced, OrderID: &orderID, OrderLineID: &lineID},
					{InventoryItemID: itemID, LocationID: locA, QuantityDelta: 2, Reason: ReasonOrderCancelled, OrderID: &orderID, OrderLineID: &lineID},
				}
			},
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item, levels := stockFixture([]uuid.UUID{locA, locB}, 10, 5)
			repo := &stockEventRepo{
				items:  map[uuid.UUID]Item{item.ID: item},
				levels: map[uuid.UUID][]Level{item.ID: levels},
				rows:   tc.rows(item.ID),
			}
			sink := newStockSink()
			svc := newStockEventSvc(t, repo, sink)

			if err := svc.ReleaseForOrder(context.Background(), orderID, ReasonOrderCancelled); err != nil {
				t.Fatalf("ReleaseForOrder: %v", err)
			}
			got := sink.drain(t)
			if len(got) != tc.wantCount {
				t.Fatalf("published %d events, want %d", len(got), tc.wantCount)
			}
			if tc.wantCount == 0 {
				return
			}
			ev := got[0]
			if ev.ProductID != item.ProductID || ev.InventoryItemID != item.ID {
				t.Fatalf("event identifies %s/%s, want %s/%s", ev.ProductID, ev.InventoryItemID, item.ProductID, item.ID)
			}
			if ev.Delta != tc.wantDelta {
				t.Fatalf("delta = %g, want %g", ev.Delta, tc.wantDelta)
			}
			if ev.After != tc.wantAfter || ev.Before != tc.wantAfter-tc.wantDelta {
				t.Fatalf("before/after = %g/%g, want %g/%g", ev.Before, ev.After, tc.wantAfter-tc.wantDelta, tc.wantAfter)
			}
			if ev.Reason != ReasonOrderCancelled {
				t.Fatalf("reason = %q", ev.Reason)
			}
		})
	}
}

func TestFulfillLinesStockEvents(t *testing.T) {
	loc := uuid.New()
	orderID, lineID := uuid.New(), uuid.New()

	t.Run("shipping nets to zero salable so nothing is published", func(t *testing.T) {
		item, levels := stockFixture([]uuid.UUID{loc}, 10, 4)
		repo := &stockEventRepo{
			items:  map[uuid.UUID]Item{item.ID: item},
			levels: map[uuid.UUID][]Level{item.ID: levels},
			rows: []Reservation{
				{InventoryItemID: item.ID, LocationID: loc, QuantityDelta: -4, Reason: ReasonOrderPlaced, OrderID: &orderID, OrderLineID: &lineID},
			},
		}
		sink := newStockSink()
		svc := newStockEventSvc(t, repo, sink)

		if err := svc.FulfillLines(context.Background(), orderID, []FulfillLineQty{{OrderLineID: lineID, Quantity: 4}}); err != nil {
			t.Fatalf("FulfillLines: %v", err)
		}
		if got := sink.drain(t); len(got) != 0 {
			t.Fatalf("published %d events, want 0 - the hold and the physical stock move by the same amount", len(got))
		}
	})

	// The comp lands but the cell is gone, so the hold is freed with no matching decrement - salable really does rise
	t.Run("a missing level frees the hold and publishes", func(t *testing.T) {
		item, levels := stockFixture([]uuid.UUID{loc}, 10, 4)
		repo := &stockEventRepo{
			items:  map[uuid.UUID]Item{item.ID: item},
			levels: map[uuid.UUID][]Level{item.ID: levels},
			rows: []Reservation{
				{InventoryItemID: item.ID, LocationID: uuid.New(), QuantityDelta: -4, Reason: ReasonOrderPlaced, OrderID: &orderID, OrderLineID: &lineID},
			},
		}
		sink := newStockSink()
		svc := newStockEventSvc(t, repo, sink)

		if err := svc.FulfillLines(context.Background(), orderID, []FulfillLineQty{{OrderLineID: lineID, Quantity: 4}}); err != nil {
			t.Fatalf("FulfillLines: %v", err)
		}
		got := sink.drain(t)
		if len(got) != 1 || got[0].Delta != 4 {
			t.Fatalf("got %d events (%+v), want one with delta 4", len(got), got)
		}
	})
}

func TestRestockReturnPublishesStockChanged(t *testing.T) {
	loc := uuid.New()
	orderID, returnID, lineID := uuid.New(), uuid.New(), uuid.New()
	item, levels := stockFixture([]uuid.UUID{loc}, 6, 0)
	repo := &stockEventRepo{
		items:  map[uuid.UUID]Item{item.ID: item},
		levels: map[uuid.UUID][]Level{item.ID: levels},
		rows: []Reservation{
			{InventoryItemID: item.ID, LocationID: loc, QuantityDelta: 3, Reason: ReasonOrderFulfilled, OrderID: &orderID, OrderLineID: &lineID},
		},
	}
	sink := newStockSink()
	svc := newStockEventSvc(t, repo, sink)

	if err := svc.RestockReturn(context.Background(), orderID, returnID, []RestockLineQty{{OrderLineID: lineID, Quantity: 2}}); err != nil {
		t.Fatalf("RestockReturn: %v", err)
	}
	got := sink.drain(t)
	if len(got) != 1 {
		t.Fatalf("published %d events, want 1", len(got))
	}
	if got[0].Delta != 2 || got[0].After != 8 || got[0].Reason != ReasonOrderReturned {
		t.Fatalf("unexpected event %+v", got[0])
	}
}

func TestOnOrderPlacedFoldsPlacementHolds(t *testing.T) {
	locA, locB := uuid.New(), uuid.New()
	orderID, lineID := uuid.New(), uuid.New()
	item, levels := stockFixture([]uuid.UUID{locA, locB}, 10, 3)
	other, otherLevels := stockFixture([]uuid.UUID{locA}, 4, 1)
	repo := &stockEventRepo{
		items:  map[uuid.UUID]Item{item.ID: item, other.ID: other},
		levels: map[uuid.UUID][]Level{item.ID: levels, other.ID: otherLevels},
		rows: []Reservation{
			{InventoryItemID: item.ID, LocationID: locA, QuantityDelta: -2, Reason: ReasonOrderPlaced, OrderID: &orderID, OrderLineID: &lineID},
			{InventoryItemID: item.ID, LocationID: locB, QuantityDelta: -1, Reason: ReasonOrderPlaced, OrderID: &orderID, OrderLineID: &lineID},
			{InventoryItemID: other.ID, LocationID: locA, QuantityDelta: -1, Reason: ReasonOrderPlaced, OrderID: &orderID, OrderLineID: &lineID},
			// a fulfillment racing the relay must not be folded into the placement delta
			{InventoryItemID: item.ID, LocationID: locA, QuantityDelta: 2, Reason: ReasonOrderFulfilled, OrderID: &orderID, OrderLineID: &lineID},
		},
	}
	sink := newStockSink()
	svc := newStockEventSvc(t, repo, sink)

	_ = svc.onOrderPlaced(context.Background(), &order.OrderPlaced{OrderID: orderID})
	got := sink.drain(t)
	if len(got) != 2 {
		t.Fatalf("published %d events, want one per product", len(got))
	}
	byProduct := map[uuid.UUID]float64{}
	for _, ev := range got {
		byProduct[ev.ProductID] = ev.Delta
		if ev.Reason != ReasonOrderPlaced {
			t.Fatalf("reason = %q", ev.Reason)
		}
	}
	if byProduct[item.ProductID] != -3 {
		t.Fatalf("folded delta = %g, want -3", byProduct[item.ProductID])
	}
	if byProduct[other.ProductID] != -1 {
		t.Fatalf("second product delta = %g, want -1", byProduct[other.ProductID])
	}
}
