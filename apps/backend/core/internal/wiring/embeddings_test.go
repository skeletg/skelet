package wiring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

type recordingQueue struct {
	err                   error
	failFor               int
	products              []uuid.UUID
	categories, typesSeen []uuid.UUID
}

func (q *recordingQueue) Enqueue(_ context.Context, _ string, ids ...uuid.UUID) error {
	if q.failFor > 0 {
		q.failFor--
		return errors.New("db blip")
	}
	q.products = append(q.products, ids...)
	return q.err
}
func (q *recordingQueue) EnqueueCategory(_ context.Context, id uuid.UUID) error {
	q.categories = append(q.categories, id)
	return q.err
}
func (q *recordingQueue) EnqueueProductType(_ context.Context, id uuid.UUID) error {
	q.typesSeen = append(q.typesSeen, id)
	return q.err
}

func TestARenamedCategoryOrTypeRequeuesItsProductsAndABlipIsRetried(t *testing.T) {
	ctx := context.Background()
	enqueueRetries = []time.Duration{time.Millisecond, time.Millisecond}
	bus := events.NewBus()
	q := &recordingQueue{}
	registerEmbeddingSubscribers(bus, q)
	cat, typ, prod := uuid.New(), uuid.New(), uuid.New()
	for _, e := range []events.Event{&catalog.CategoryUpdated{CategoryID: cat, ChangedFields: []string{"content"}}, &catalog.ProductTypeRenamed{ProductTypeID: typ}, &catalog.ProductUpdated{ProductID: prod}} {
		if err := bus.DispatchAsyncNow(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if len(q.categories) != 1 || q.categories[0] != cat || len(q.typesSeen) != 1 || q.typesSeen[0] != typ || len(q.products) != 1 {
		t.Errorf("queued %+v", q)
	}
	q.failFor = 2
	if err := bus.DispatchAsyncNow(ctx, &catalog.ProductUpdated{ProductID: prod}); err != nil || len(q.products) != 2 {
		t.Errorf("a queue down for two tries = %v, %d queued - want the third to land", err, len(q.products))
	}
	q.err = errors.New("db down")
	if err := bus.DispatchAsyncNow(ctx, &catalog.ProductUpdated{ProductID: prod}); err == nil {
		t.Error("a queue that stays down was reported as queued")
	}
}
