package urlrewrite

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

type txFakeRepo struct{ *fakeScopeRepo }

func (f txFakeRepo) WithTx(*sql.Tx) Repository { return f }

func TestADeferredWriteHoldsItsEventUntilFlush(t *testing.T) {
	bus := events.NewBus()
	var seen atomic.Int32
	bus.Subscribe(EventChanged, func(context.Context, events.Event) error {
		seen.Add(1)
		return nil
	})
	s := &Service{repo: txFakeRepo{&fakeScopeRepo{rows: map[string]UrlRewrite{}}}, bus: bus}
	bound, flush := s.WithTxDeferred(nil)
	if err := bound.CreateRedirect(context.Background(), "old", string(EntityTypeCMSPage), uuid.New(), 301); err != nil {
		t.Fatal(err)
	}
	idle := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := bus.WaitIdle(ctx); err != nil {
			t.Fatal(err)
		}
	}
	idle()
	if n := seen.Load(); n != 0 {
		t.Fatalf("%d events before the flush, want none", n)
	}
	flush(context.Background())
	idle()
	if n := seen.Load(); n != 1 {
		t.Errorf("%d events after the flush, want 1", n)
	}
}
