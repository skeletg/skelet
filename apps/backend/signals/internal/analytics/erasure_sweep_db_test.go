package analytics

import (
	"context"
	"slices"
	"testing"

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

	if err := NewRecorder(qs, busySpine{}).Maintain(ctx); err != nil {
		t.Fatalf("maintain = %v, want a quiet wait for the running detach", err)
	}
	queued, err := qs.QueuedIdentityErasures(ctx, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(queued, cust) {
		t.Fatal("the erasure left the queue although its detach was never issued")
	}
}
