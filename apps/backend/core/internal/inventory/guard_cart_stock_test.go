package inventory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// The cart-side gate needs a sku→product hop plus the salable read path
type fakeGuardRepo struct {
	Repository
	item      Item
	itemErr   error
	locations []Location
	levels    []Level
}

func (f fakeGuardRepo) GetItemByProductID(context.Context, uuid.UUID) (Item, error) {
	return f.item, f.itemErr
}

func (f fakeGuardRepo) ListChannelLocations(context.Context, uuid.UUID) ([]Location, error) {
	return f.locations, nil
}

func (f fakeGuardRepo) ListLevelsByItems(context.Context, []uuid.UUID) ([]Level, error) {
	return f.levels, nil
}

type stubProducts struct {
	id  uuid.UUID
	err error
}

func (s stubProducts) GetProductIDBySKU(context.Context, string) (uuid.UUID, error) {
	return s.id, s.err
}

func (s stubProducts) KitComponents(context.Context, uuid.UUID) ([]KitComponentRef, error) {
	return nil, nil
}

// guardSvc wires the gate with stock at one location the channel serves
func guardSvc(item Item, onHand, reserved float64) (*Service, uuid.UUID) {
	locA := loc("a")
	repo := fakeGuardRepo{
		item:      item,
		locations: []Location{locA},
		levels:    []Level{{InventoryItemID: item.ID, LocationID: locA.ID, Quantity: onHand, ReservedQuantity: reserved}},
	}
	s := &Service{repo: repo}
	s.SetProductLookup(stubProducts{id: item.ProductID})
	return s, uuid.New()
}

// The one case that refuses: a definitive "want more than is salable" on a tracked, non-backorderable item
func TestGuardCartStockRefusesOnlyADefiniteShortage(t *testing.T) {
	item := tracked(false)
	s, ch := guardSvc(item, 10, 7) // 3 salable

	if err := s.guardCartStock(context.Background(), ch, "sku", 3); err != nil {
		t.Errorf("asking for exactly the salable 3 = %v, want nil", err)
	}
	err := s.guardCartStock(context.Background(), ch, "sku", 4)
	if err == nil {
		t.Fatal("asking for 4 of 3 salable was allowed")
	}
	if code := apperr.CodeOf(err); code != "CART_INSUFFICIENT_STOCK" {
		t.Errorf("error code = %q, want CART_INSUFFICIENT_STOCK", code)
	}
	// the message tells the shopper what's left, which the storefront renders verbatim
	if got := err.Error(); got == "" || !contains2(got, "3") {
		t.Errorf("error = %q, want it to name the 3 remaining", got)
	}
}

func contains2(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Every ambiguity fails OPEN - blocking a legitimate add because a lookup blipped is worse than the reservation refusing later at placement, which is the authoritative gate
func TestGuardCartStockFailsOpenOnEveryAmbiguity(t *testing.T) {
	item := tracked(false)
	ch := uuid.New()

	for name, build := range map[string]func() *Service{
		"blank sku": func() *Service {
			s, _ := guardSvc(item, 0, 0)
			return s
		},
		"no product lookup wired": func() *Service {
			s, _ := guardSvc(item, 0, 0)
			s.SetProductLookup(nil)
			return s
		},
		"sku not in the catalog": func() *Service {
			s, _ := guardSvc(item, 0, 0)
			s.SetProductLookup(stubProducts{err: errors.New("no such sku")})
			return s
		},
		"no inventory item": func() *Service {
			s := &Service{repo: fakeGuardRepo{itemErr: ErrItemNotFound}}
			s.SetProductLookup(stubProducts{id: uuid.New()})
			return s
		},
		"item lookup blip": func() *Service {
			s := &Service{repo: fakeGuardRepo{itemErr: errors.New("db down")}}
			s.SetProductLookup(stubProducts{id: uuid.New()})
			return s
		},
		"untracked item": func() *Service {
			s, _ := guardSvc(Item{ID: uuid.New(), ProductID: uuid.New(), Tracked: false}, 0, 0)
			return s
		},
		"backorderable item": func() *Service {
			s, _ := guardSvc(tracked(true), 0, 0)
			return s
		},
		"ledger disabled": func() *Service {
			s, _ := guardSvc(item, 0, 0)
			s.SetLedgerToggle(toggle(false))
			return s
		},
	} {
		s := build()
		sku := "sku"
		if name == "blank sku" {
			sku = ""
		}
		if err := s.guardCartStock(context.Background(), ch, sku, 99); err != nil {
			t.Errorf("%s: guardCartStock = %v, want nil (fail open)", name, err)
		}
	}
}

// A nil channel or a non-positive quantity is not a stock question at all
func TestGuardCartStockIgnoresNonQuestions(t *testing.T) {
	item := tracked(false)
	s, ch := guardSvc(item, 0, 0) // nothing salable, so a real question would refuse

	if err := s.guardCartStock(context.Background(), uuid.Nil, "sku", 5); err != nil {
		t.Errorf("with no channel = %v, want nil", err)
	}
	for _, qty := range []float64{0, -1} {
		if err := s.guardCartStock(context.Background(), ch, "sku", qty); err != nil {
			t.Errorf("with quantity %v = %v, want nil", qty, err)
		}
	}
}

// The -1 always-in-stock sentinel must read as unlimited, not as "less than anything you asked for"
func TestGuardCartStockTreatsTheSentinelAsUnlimited(t *testing.T) {
	// an untracked item in flat mode yields -1 from Salable
	item := Item{ID: uuid.New(), ProductID: uuid.New(), Tracked: true}
	s, ch := guardSvc(item, 5, 0)
	s.SetFlatModeToggle(stubFlatMode{on: true})
	// flat mode reads item.Quantity, which is 0 here, so a tracked item still gates
	if err := s.guardCartStock(context.Background(), ch, "sku", 1); err == nil {
		t.Error("a tracked flat item with no quantity allowed an add")
	}

	untracked := Item{ID: uuid.New(), ProductID: uuid.New(), Tracked: false}
	s2, ch2 := guardSvc(untracked, 0, 0)
	s2.SetFlatModeToggle(stubFlatMode{on: true})
	if err := s2.guardCartStock(context.Background(), ch2, "sku", 9999); err != nil {
		t.Errorf("an untracked flat item = %v, want nil for the unlimited sentinel", err)
	}
}

// CanFulfillSKU is the same gate as a bool - order placement asks before granting a free gift, and a gift that can't be stocked is dropped rather than failing the paid order
func TestCanFulfillSKUMirrorsTheGuard(t *testing.T) {
	item := tracked(false)
	s, ch := guardSvc(item, 10, 7) // 3 salable

	if !s.CanFulfillSKU(context.Background(), ch, "sku", 3) {
		t.Error("CanFulfillSKU for exactly the salable 3 = false, want true")
	}
	if s.CanFulfillSKU(context.Background(), ch, "sku", 4) {
		t.Error("CanFulfillSKU past the salable 3 = true, want false")
	}
	// and it fails open the same way, so a lookup blip never silently drops a gift
	blip := &Service{repo: fakeGuardRepo{itemErr: errors.New("db down")}}
	blip.SetProductLookup(stubProducts{id: uuid.New()})
	if !blip.CanFulfillSKU(context.Background(), ch, "sku", 99) {
		t.Error("CanFulfillSKU on a lookup blip = false, want true (fail open)")
	}
}

// A cart event without an explicit quantity means one unit - defaulting to zero would skip the gate
func TestQtyOrOne(t *testing.T) {
	if got := qtyOrOne(nil); got != 1 {
		t.Errorf("qtyOrOne(nil) = %v, want 1", got)
	}
	for _, in := range []float64{0, 1, 2.5, -3} {
		v := in
		if got := qtyOrOne(&v); got != in {
			t.Errorf("qtyOrOne(%v) = %v, want it passed through", in, got)
		}
	}
}

// Backorderable products bypass the stock gate, so this flag decides whether a shopper can order what isn't on the shelf
func TestBackorderableByProductIDs(t *testing.T) {
	yes, no := tracked(true), tracked(false)
	repo := fakeItemsRepo{items: map[uuid.UUID]Item{
		yes.ProductID: yes,
		no.ProductID:  no,
	}}
	s := &Service{repo: repo}

	got, err := s.BackorderableByProductIDs(context.Background(), []uuid.UUID{yes.ProductID, no.ProductID})
	if err != nil {
		t.Fatalf("BackorderableByProductIDs: %v", err)
	}
	if !got[yes.ProductID] {
		t.Error("the backorderable product reports false")
	}
	if got[no.ProductID] {
		t.Error("the non-backorderable product reports true")
	}

	// a product with no inventory item is simply absent, which callers read as not-backorderable
	unknown := uuid.New()
	got, err = s.BackorderableByProductIDs(context.Background(), []uuid.UUID{unknown})
	if err != nil {
		t.Fatalf("BackorderableByProductIDs: %v", err)
	}
	if _, present := got[unknown]; present {
		t.Errorf("an itemless product appears in the map as %v, want it absent", got[unknown])
	}

	// the empty case must not query at all, and must return a usable map
	empty, err := s.BackorderableByProductIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("BackorderableByProductIDs(nil): %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("BackorderableByProductIDs(nil) = %v, want an empty non-nil map", empty)
	}
}

func TestBackorderableByProductIDsPropagatesErrors(t *testing.T) {
	boom := errors.New("db down")
	s := &Service{repo: fakeItemsRepo{err: boom}}
	got, err := s.BackorderableByProductIDs(context.Background(), []uuid.UUID{uuid.New()})
	if !errors.Is(err, boom) {
		t.Errorf("BackorderableByProductIDs = (%v, %v), want the error surfaced", got, err)
	}
	if got != nil {
		t.Errorf("returned %v alongside the error, want nil rather than a half map", got)
	}
}

type fakeItemsRepo struct {
	Repository
	items map[uuid.UUID]Item
	err   error
}

func (f fakeItemsRepo) ListItemsByProductIDs(_ context.Context, pids []uuid.UUID) (map[uuid.UUID]Item, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[uuid.UUID]Item{}
	for _, p := range pids {
		if it, ok := f.items[p]; ok {
			out[p] = it
		}
	}
	return out, nil
}

// Location coordinates feed pickup-point rendering, so an out-of-range pair would put a store in the ocean
func TestValidateCoords(t *testing.T) {
	f := func(v float64) *float64 { return &v }

	for name, tc := range map[string]struct{ lat, lng *float64 }{
		"both nil":       {nil, nil},
		"lat only":       {f(51.5), nil},
		"lng only":       {nil, f(-0.12)},
		"both":           {f(51.5), f(-0.12)},
		"boundaries":     {f(90), f(180)},
		"neg boundaries": {f(-90), f(-180)},
		"zero":           {f(0), f(0)},
	} {
		if err := validateCoords(tc.lat, tc.lng); err != nil {
			t.Errorf("%s = %v, want nil - any combination is allowed", name, err)
		}
	}

	for name, tc := range map[string]struct{ lat, lng *float64 }{
		"lat too high":        {f(90.1), nil},
		"lat too low":         {f(-90.1), nil},
		"lng too high":        {nil, f(180.1)},
		"lng too low":         {nil, f(-180.1)},
		"lat swapped for lng": {f(120), nil},
	} {
		if err := validateCoords(tc.lat, tc.lng); err == nil {
			t.Errorf("%s was accepted, want it refused", name)
		}
	}
}

// The slug is a stable handle used in config and URLs, so it has to stay lowercase and separator-safe
func TestValidateSlug(t *testing.T) {
	for _, ok := range []string{"warehouse", "warehouse-1", "wh_east", "a", "0", "main-2026", "  padded  "} {
		if err := validateSlug(ok); err != nil {
			t.Errorf("validateSlug(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "   ", "Warehouse", "wh east", "wh.east", "wh/east", "wärehaus", "wh!"} {
		if err := validateSlug(bad); err == nil {
			t.Errorf("validateSlug(%q) = nil, want it refused", bad)
		}
	}
}

// The restock retry exists so a transient database blip doesn't silently lose returned stock
func TestRetryWithBackoffSucceedsWithoutSleeping(t *testing.T) {
	var calls int
	err := retryWithBackoff(context.Background(), 3, time.Hour, func(context.Context) error {
		calls++
		return nil
	})
	if err != nil {
		t.Errorf("retryWithBackoff = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("op ran %d times, want 1 - a first-try success must not sleep out the backoff", calls)
	}
}

func TestRetryWithBackoffRetriesThenSucceeds(t *testing.T) {
	var calls int
	err := retryWithBackoff(context.Background(), 4, time.Millisecond, func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Errorf("retryWithBackoff = %v, want nil once the op recovered", err)
	}
	if calls != 3 {
		t.Errorf("op ran %d times, want 3", calls)
	}
}

// Exhausting the attempts has to return the LAST error, which is what gets logged for the operator
func TestRetryWithBackoffReturnsTheLastError(t *testing.T) {
	var calls int
	last := errors.New("attempt 3 failed")
	err := retryWithBackoff(context.Background(), 3, time.Millisecond, func(context.Context) error {
		calls++
		if calls == 3 {
			return last
		}
		return errors.New("earlier failure")
	})
	if !errors.Is(err, last) {
		t.Errorf("retryWithBackoff = %v, want the last error %v", err, last)
	}
	if calls != 3 {
		t.Errorf("op ran %d times, want exactly the 3 attempts", calls)
	}
}

// A parked retry must not outlive shutdown - the wait aborts on cancel rather than sleeping it out
func TestRetryWithBackoffAbortsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls int
	done := make(chan error, 1)
	go func() {
		done <- retryWithBackoff(ctx, 5, time.Hour, func(context.Context) error {
			calls++
			if calls == 1 {
				cancel() // cancel while the first backoff is parked
			}
			return errors.New("always fails")
		})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("retryWithBackoff = nil, want the abort error")
		}
		if !contains2(err.Error(), "aborted") {
			t.Errorf("error = %v, want it to report the abort", err)
		}
		if calls != 1 {
			t.Errorf("op ran %d times, want 1 - the cancel must stop further attempts", calls)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("retryWithBackoff slept out the hour-long backoff instead of aborting on cancel")
	}
}
