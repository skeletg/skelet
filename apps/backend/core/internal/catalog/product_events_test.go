package catalog_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

// Backs the mutations that only had to start announcing themselves - every write is a no-op, the assertions are all about the event
type eventRepo struct {
	catalog.ProductRepository
	catalog.CategoryRepository
	catalog.ListingRepository
	catalog.ProductContentRepository

	product catalog.Product
	primary map[uuid.UUID]*catalog.Category
}

func (r *eventRepo) GetProductByID(context.Context, uuid.UUID) (catalog.Product, error) {
	return r.product, nil
}

func (r *eventRepo) GetProductWithAttributes(context.Context, uuid.UUID) (catalog.Product, error) {
	return r.product, nil
}

func (r *eventRepo) GetCategoryByID(_ context.Context, id uuid.UUID) (catalog.Category, error) {
	return catalog.Category{ID: id}, nil
}

func (r *eventRepo) AddProductToCategory(context.Context, uuid.UUID, uuid.UUID, int32, bool) error {
	return nil
}

func (r *eventRepo) RemoveProductFromCategory(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (r *eventRepo) ListPrimaryCategoryByProductIDs(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]*catalog.Category, error) {
	out := make(map[uuid.UUID]*catalog.Category, len(ids))
	for _, id := range ids {
		out[id] = r.primary[id]
	}
	return out, nil
}

// Dropping or moving the primary recomputes the url path, which lands here with no rewriter wired
func (r *eventRepo) UpdateProductURL(context.Context, uuid.UUID, string) error { return nil }

func (r *eventRepo) ListCategoriesByProduct(_ context.Context, id uuid.UUID) ([]catalog.Category, error) {
	if c := r.primary[id]; c != nil {
		return []catalog.Category{*c}, nil
	}
	return nil, nil
}

func (r *eventRepo) SetPrimaryProductCategory(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (r *eventRepo) UpsertProductContent(_ context.Context, _ uuid.UUID, in catalog.ProductContentInput) (catalog.ProductContent, error) {
	return catalog.ProductContent{Locale: in.Locale, Name: in.Name}, nil
}

// Collects catalog product.updated events, draining the bus so assertions don't race the async dispatch
type updateSink struct {
	bus  *events.Bus
	mu   sync.Mutex
	seen []*catalog.ProductUpdated
}

func newUpdateSink() *updateSink {
	s := &updateSink{bus: events.NewBus()}
	s.bus.Subscribe(catalog.EventProductUpdated, func(_ context.Context, e events.Event) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.seen = append(s.seen, e.(*catalog.ProductUpdated))
		return nil
	})
	return s
}

func (s *updateSink) drain(t *testing.T) []*catalog.ProductUpdated {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.bus.Drain(ctx); err != nil {
		t.Fatalf("drain bus: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen
}

// The search doc carries category_ids, name and description, so these mutations have to emit the same ProductUpdated the other product writes do
func TestProductUpdatedOnIndexRelevantMutations(t *testing.T) {
	product := catalog.Product{ID: uuid.New(), ProductTypeID: uuid.New(), SKU: "SKU-1"}
	categoryID := uuid.New()

	tests := []struct {
		name    string
		primary map[uuid.UUID]*catalog.Category
		run     func(*testing.T, *catalog.Service)
		want    []string
	}{
		{
			name: "adding to a category",
			run: func(t *testing.T, svc *catalog.Service) {
				if _, err := svc.AddProductToCategory(context.Background(), product.ID, categoryID, 0, false); err != nil {
					t.Fatalf("AddProductToCategory: %v", err)
				}
			},
			want: []string{"categories"},
		},
		{
			name: "removing from a non-primary category",
			run: func(t *testing.T, svc *catalog.Service) {
				if _, err := svc.RemoveProductFromCategory(context.Background(), product.ID, categoryID); err != nil {
					t.Fatalf("RemoveProductFromCategory: %v", err)
				}
			},
			want: []string{"categories"},
		},
		{
			// dropping the primary also moves the url path, so the subscriber gets told both
			name:    "removing the primary category",
			primary: map[uuid.UUID]*catalog.Category{product.ID: {ID: categoryID}},
			run: func(t *testing.T, svc *catalog.Service) {
				if _, err := svc.RemoveProductFromCategory(context.Background(), product.ID, categoryID); err != nil {
					t.Fatalf("RemoveProductFromCategory: %v", err)
				}
			},
			want: []string{"categories", "primaryCategory"},
		},
		{
			name:    "promoting a category to primary",
			primary: map[uuid.UUID]*catalog.Category{product.ID: {ID: categoryID}},
			run: func(t *testing.T, svc *catalog.Service) {
				if _, err := svc.SetProductPrimaryCategory(context.Background(), product.ID, categoryID); err != nil {
					t.Fatalf("SetProductPrimaryCategory: %v", err)
				}
			},
			want: []string{"primaryCategory"},
		},
		{
			name: "renaming through product content",
			run: func(t *testing.T, svc *catalog.Service) {
				_, err := svc.UpsertProductContent(context.Background(), product.ID, catalog.ProductContentInput{
					Locale: "en", Name: "New name",
				})
				if err != nil {
					t.Fatalf("UpsertProductContent: %v", err)
				}
			},
			want: []string{"content"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &eventRepo{product: product, primary: tc.primary}
			sink := newUpdateSink()
			svc := svcWith(repo, func(d *catalog.Deps) { d.Bus = sink.bus })

			tc.run(t, svc)

			got := sink.drain(t)
			if len(got) != 1 {
				t.Fatalf("published %d events, want 1", len(got))
			}
			ev := got[0]
			if ev.ProductID != product.ID || ev.ProductTypeID != product.ProductTypeID || ev.SKU != product.SKU {
				t.Fatalf("event mirrors the wrong product: %+v", ev)
			}
			if len(ev.ChangedFields) != len(tc.want) {
				t.Fatalf("changed fields = %v, want %v", ev.ChangedFields, tc.want)
			}
			for i, f := range tc.want {
				if ev.ChangedFields[i] != f {
					t.Fatalf("changed fields = %v, want %v", ev.ChangedFields, tc.want)
				}
			}
		})
	}
}
