package inventory

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// fakeTrackingRepo embeds Repository (nil) and implements only the three methods SyncProductTracking touches - It
// records writes so tests can assert what was (and wasn't) persisted
type fakeTrackingRepo struct {
	Repository
	item      *Item // nil = no inventory_items row for the product
	created   *bool // tracked flag passed to CreateItem, nil if not called
	updatedTo *bool // tracked flag passed to UpdateItem, nil if not called
}

func (f *fakeTrackingRepo) GetItemByProductID(_ context.Context, _ uuid.UUID) (Item, error) {
	if f.item == nil {
		return Item{}, ErrItemNotFound
	}
	return *f.item, nil
}

func (f *fakeTrackingRepo) CreateItem(_ context.Context, productID uuid.UUID, tracked, _ bool, _ float64) (Item, error) {
	f.created = &tracked
	return Item{ID: uuid.New(), ProductID: productID, Tracked: tracked}, nil
}

func (f *fakeTrackingRepo) UpdateItem(_ context.Context, id uuid.UUID, tracked, backorderable bool, lowStock float64) (Item, error) {
	f.updatedTo = &tracked
	return Item{ID: id, Tracked: tracked, Backorderable: backorderable, LowStockThreshold: lowStock}, nil
}

func TestSyncProductTracking_NoRow_TurnOff_IsNoOp(t *testing.T) {
	// The crux of fix C: turning tracking OFF on a product that has no inventory row must NOT create one - An absent row is the "always in
	// stock" sentinel; a tracked=false row would instead report salable=0 (out of stock), silently flipping the product's storefront state
	repo := &fakeTrackingRepo{item: nil}
	s := &Service{repo: repo}
	if _, err := s.SyncProductTracking(context.Background(), uuid.New(), false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.created != nil {
		t.Errorf("turning tracking off with no row must not CreateItem (created tracked=%v)", *repo.created)
	}
	if repo.updatedTo != nil {
		t.Errorf("turning tracking off with no row must not UpdateItem")
	}
}

func TestSyncProductTracking_NoRow_TurnOn_CreatesTracked(t *testing.T) {
	repo := &fakeTrackingRepo{item: nil}
	s := &Service{repo: repo}
	if _, err := s.SyncProductTracking(context.Background(), uuid.New(), true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.created == nil || *repo.created != true {
		t.Errorf("turning tracking on with no row must CreateItem(tracked=true), got created=%v", repo.created)
	}
}

func TestSyncProductTracking_ExistingRow_FlipsOn(t *testing.T) {
	repo := &fakeTrackingRepo{item: &Item{ID: uuid.New(), Tracked: false}}
	s := &Service{repo: repo}
	if _, err := s.SyncProductTracking(context.Background(), uuid.New(), true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedTo == nil || *repo.updatedTo != true {
		t.Errorf("flipping an existing row on must UpdateItem(tracked=true), got %v", repo.updatedTo)
	}
	if repo.created != nil {
		t.Errorf("existing row must be updated, not created")
	}
}

func TestSyncProductTracking_AlreadyMatches_IsNoOp(t *testing.T) {
	repo := &fakeTrackingRepo{item: &Item{ID: uuid.New(), Tracked: true}}
	s := &Service{repo: repo}
	if _, err := s.SyncProductTracking(context.Background(), uuid.New(), true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedTo != nil || repo.created != nil {
		t.Errorf("matching flag must be a no-op (no write), updated=%v created=%v", repo.updatedTo, repo.created)
	}
}
