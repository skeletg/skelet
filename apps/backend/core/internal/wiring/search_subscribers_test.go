package wiring

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

type fakeIndex struct {
	mu       sync.Mutex
	upserted []uuid.UUID
}

func (f *fakeIndex) Upsert(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserted = append(f.upserted, id)
	return nil
}

func (f *fakeIndex) Delete(context.Context, uuid.UUID) error { return nil }

func (f *fakeIndex) ReindexChannel(context.Context, uuid.UUID) (int, error) { return 0, nil }

func (f *fakeIndex) DeleteChannel(context.Context, uuid.UUID) error { return nil }

func TestAttributeEventsReindexTheProductsCarryingThem(t *testing.T) {
	for name, ev := range map[string]func(ids []uuid.UUID) events.Event{
		"updated": func(ids []uuid.UUID) events.Event {
			return &catalog.AttributeUpdated{AttributeID: uuid.New(), ProductIDs: ids}
		},
		"deleted": func(ids []uuid.UUID) events.Event {
			return &catalog.AttributeDeleted{AttributeID: uuid.New(), ProductIDs: ids}
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			bus := events.NewBus()
			products := &fakeIndex{}
			registerSearchSubscribers(ctx, bus, products, &fakeIndex{})

			a, b := uuid.New(), uuid.New()
			bus.Publish(ctx, ev([]uuid.UUID{a, b}))
			dctx, dcancel := context.WithTimeout(ctx, 2*time.Second)
			defer dcancel()
			if err := bus.Drain(dctx); err != nil {
				t.Fatal(err)
			}
			products.mu.Lock()
			defer products.mu.Unlock()
			if len(products.upserted) != 2 || !slices.Contains(products.upserted, a) || !slices.Contains(products.upserted, b) {
				t.Fatalf("upserted %v, want both %s and %s", products.upserted, a, b)
			}
		})
	}
}
