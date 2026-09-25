package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

func TestRecomputeKeepsRecencyCurrent(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	qs := signalsdb.New(conn)
	customerID, channelID := uuid.New(), uuid.New()
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM customer_traits WHERE channel_id = $1`, channelID)
		_, _ = conn.Exec(`DELETE FROM customer_order_agg WHERE channel_id = $1`, channelID)
		_, _ = conn.Exec(`DELETE FROM customer_roster WHERE channel_id = $1`, channelID)
	})
	if err := NewRoster(qs).UpsertRosterEntry(ctx, RosterEntry{CustomerID: customerID, ChannelID: channelID, Email: "gate@test", IsActive: true}); err != nil {
		t.Fatalf("roster: %v", err)
	}
	ordered := time.Now().Add(-10 * 24 * time.Hour)
	if _, err := conn.Exec(`INSERT INTO customer_order_agg (customer_id, channel_id, order_count, monetary_cents, first_order_at, last_order_at) VALUES ($1, $2, 1, 5000, $3, $3)`,
		customerID, channelID, ordered); err != nil {
		t.Fatalf("seed orders: %v", err)
	}
	recompute := func(asOf time.Time) int32 {
		if _, err := qs.RecomputeChannelTraits(ctx, signalsdb.RecomputeChannelTraitsParams{
			ChannelID: channelID, NewDays: 30, ActiveDays: 90, AtRiskDays: 180, AsOf: asOf,
		}); err != nil {
			t.Fatalf("recompute: %v", err)
		}
		tr, err := qs.GetCustomerTrait(ctx, customerID)
		if err != nil {
			t.Fatalf("read trait: %v", err)
		}
		return tr.RecencyDays
	}
	now := time.Now()
	first := recompute(now)
	if later := recompute(now.Add(3 * 24 * time.Hour)); later != first+3 {
		t.Errorf("recency_days three days on = %d, want %d - the gate skipped a row whose age changed", later, first+3)
	}
}
