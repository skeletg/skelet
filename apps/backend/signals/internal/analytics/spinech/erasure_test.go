package spinech

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

func TestAFullSweepPageOfErasuresFitsClickHousesQueryLimit(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	cust := uuid.New()
	anon := "anon-" + uuid.NewString()
	if err := st.UpsertIdentity(ctx, anon, cust, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if err := st.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	ids := make([]uuid.UUID, 10000)
	for i := range ids {
		ids[i] = uuid.New()
	}
	ids[len(ids)-1] = cust
	if err := st.DeleteCustomerIdentities(ctx, ids); err != nil {
		t.Fatalf("erasing a 10000-id page: %v - a backlog this size would fail every sweep", err)
	}
	if left, err := st.AnonIDsForCustomer(ctx, cust); err != nil || len(left) != 0 {
		t.Fatalf("the last id in the page kept %v, %v", left, err)
	}
}

func TestADetachStillRunningIsNotIssuedTwice(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	cust, ch := uuid.New(), uuid.New()
	if _, err := st.AppendEvent(ctx, analytics.SpineEvent{
		EventName: "order_placed", OccurredAt: time.Now().UTC(), ChannelID: &ch, CustomerID: &cust,
		Source: "commerce", Props: map[string]any{}, DedupeKey: "order:" + uuid.NewString(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.conn.Exec(ctx, "SYSTEM STOP MERGES analytics_events"); err != nil {
		t.Fatal(err)
	}
	restarted := false
	restart := func() {
		if !restarted {
			restarted = true
			_ = st.conn.Exec(context.Background(), "SYSTEM START MERGES analytics_events")
		}
	}
	defer restart()
	if err := st.conn.Exec(ctx, "ALTER TABLE analytics_events UPDATE customer_id = toUUID('00000000-0000-0000-0000-000000000000') WHERE customer_id = ?", cust); err != nil {
		t.Fatal(err)
	}

	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := st.DeleteCustomerIdentities(bounded, []uuid.UUID{cust}); !errors.Is(err, analytics.ErrErasureInFlight) {
		t.Fatalf("with a detach still running, erase = %v, want ErrErasureInFlight so the queue keeps its rows", err)
	}
	restart()
	deadline := time.Now().Add(20 * time.Second)
	for {
		err := st.DeleteCustomerIdentities(ctx, []uuid.UUID{cust})
		if err == nil {
			break
		}
		if !errors.Is(err, analytics.ErrErasureInFlight) || time.Now().After(deadline) {
			t.Fatalf("once the detach finished, erase = %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
