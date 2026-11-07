package analytics

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

type busySpine struct{ SpineStore }

func (busySpine) DeleteCustomerIdentities(context.Context, []uuid.UUID) error {
	return ErrErasureInFlight
}

func TestASweepBehindARunningDetachKeepsItsQueue(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	qs := signalsdb.New(conn)
	cust := uuid.New()
	if err := qs.EnqueueIdentityErasure(ctx, cust); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = qs.DeleteIdentityErasures(context.Background(), []uuid.UUID{cust}) })
	if _, err := conn.Exec(`UPDATE gdpr_identity_erasures SET requested_at = now() - interval '1 hour' WHERE customer_id = $1`, cust); err != nil {
		t.Fatal(err)
	}

	if err := NewRecorder(qs, busySpine{}).Maintain(ctx); err != nil {
		t.Fatalf("maintain = %v, want a quiet wait for the running detach", err)
	}
	queued, err := qs.QueuedIdentityErasures(ctx, signalsdb.QueuedIdentityErasuresParams{OlderThan: time.Now(), Lim: 10000})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(queued, cust) {
		t.Fatal("the erasure left the queue although its detach was never issued")
	}
}

type countingSpine struct {
	SpineStore
	erased []uuid.UUID
}

func (s *countingSpine) DeleteCustomerIdentities(_ context.Context, ids []uuid.UUID) error {
	s.erased = append(s.erased, ids...)
	return nil
}

// A fresh request waits out the grace, so an identify still buffered in any process can't land after the erasure
func TestAFreshErasureWaitsOutTheGrace(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	qs := signalsdb.New(conn)
	fresh, due := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{fresh, due} {
		if err := qs.EnqueueIdentityErasure(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = qs.DeleteIdentityErasures(context.Background(), []uuid.UUID{fresh, due}) })
	if _, err := conn.Exec(`UPDATE gdpr_identity_erasures SET requested_at = now() - interval '1 hour' WHERE customer_id = $1`, due); err != nil {
		t.Fatal(err)
	}
	sp := &countingSpine{}
	if err := NewRecorder(qs, sp).sweepIdentityErasures(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(sp.erased, due) || slices.Contains(sp.erased, fresh) {
		t.Errorf("erased %v - want the old request erased and the fresh one still waiting", sp.erased)
	}
}

// The hook sees each due customer while their identities still stand, and a failure keeps the queue for the next sweep
func TestTheSweepHandsItsCustomersOnBeforeTheirIdentitiesGo(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	qs := signalsdb.New(conn)
	due := uuid.New()
	if err := qs.EnqueueIdentityErasure(ctx, due); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = qs.DeleteIdentityErasures(context.Background(), []uuid.UUID{due}) })
	if _, err := conn.Exec(`UPDATE gdpr_identity_erasures SET requested_at = now() - interval '1 hour' WHERE customer_id = $1`, due); err != nil {
		t.Fatal(err)
	}

	sp := &countingSpine{}
	rec := NewRecorder(qs, sp)
	rec.OnIdentityErasure(func(context.Context, []uuid.UUID) error { return errors.New("conversations unreachable") })
	if err := rec.sweepIdentityErasures(ctx); err == nil || len(sp.erased) != 0 {
		t.Fatalf("a failed hook = %v with %v erased - the identities must stay until it succeeds", err, sp.erased)
	}
	var handed []uuid.UUID
	rec.OnIdentityErasure(func(_ context.Context, ids []uuid.UUID) error {
		if len(sp.erased) != 0 {
			t.Error("the identities went before the hook ran")
		}
		handed = append(handed, ids...)
		return nil
	})
	if err := rec.sweepIdentityErasures(ctx); err != nil || !slices.Contains(handed, due) || !slices.Contains(sp.erased, due) {
		t.Errorf("sweep = %v, handed %v, erased %v", err, handed, sp.erased)
	}
}
