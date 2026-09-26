package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// The segment panels read the same projection the RFM scores come off, so these cover per-member totals, the rollup, and a member who has never ordered

type aggRow struct {
	customer uuid.UUID
	orders   int32
	cents    int64
	last     *time.Time
}

func seedAgg(t *testing.T, channelID uuid.UUID, rows ...aggRow) *signalsdb.Queries {
	t.Helper()
	conn := testdb.RequireSignals(t)
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM customer_order_agg WHERE channel_id = $1`, channelID) })
	for _, r := range rows {
		if _, err := conn.Exec(`INSERT INTO customer_order_agg
			(customer_id, channel_id, order_count, monetary_cents, first_order_at, last_order_at)
			VALUES ($1,$2,$3,$4,$5,$5)`, r.customer, channelID, r.orders, r.cents, r.last); err != nil {
			t.Fatalf("seed agg: %v", err)
		}
	}
	return signalsdb.New(conn)
}

func TestSegmentMemberOrderStatsReadTheProjection(t *testing.T) {
	channelID := uuid.New()
	a, b, outsider := uuid.New(), uuid.New(), uuid.New()
	at := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
	q := seedAgg(t, channelID,
		aggRow{customer: a, orders: 3, cents: 12000, last: &at},
		aggRow{customer: b, orders: 1, cents: 4500, last: &at},
		aggRow{customer: outsider, orders: 9, cents: 990000, last: &at},
	)

	got, err := q.SegmentMemberOrderStats(context.Background(), []uuid.UUID{a, b})
	if err != nil {
		t.Fatalf("SegmentMemberOrderStats: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2 - a customer outside the set leaked in", len(got))
	}
	by := map[uuid.UUID]signalsdb.SegmentMemberOrderStatsRow{}
	for _, r := range got {
		by[r.CustomerID] = r
	}
	if by[a].Orders != 3 || by[a].SpendCents != 12000 {
		t.Errorf("a = %d orders / %d cents, want 3 / 12000", by[a].Orders, by[a].SpendCents)
	}
	if !by[a].LastOrderAt.Valid || !by[a].LastOrderAt.Time.Equal(at) {
		t.Errorf("last_order_at = %v, want %s", by[a].LastOrderAt, at)
	}
}

// order_count is already distinct orders per customer, so the rollup sums rather than counting again
func TestSegmentOrderRollupSumsOnlyTheSet(t *testing.T) {
	channelID := uuid.New()
	a, b, outsider := uuid.New(), uuid.New(), uuid.New()
	at := time.Now().UTC().Truncate(time.Microsecond)
	q := seedAgg(t, channelID,
		aggRow{customer: a, orders: 3, cents: 12000, last: &at},
		aggRow{customer: b, orders: 1, cents: 4500, last: &at},
		aggRow{customer: outsider, orders: 9, cents: 990000, last: &at},
	)

	row, err := q.SegmentOrderRollup(context.Background(), []uuid.UUID{a, b})
	if err != nil {
		t.Fatalf("SegmentOrderRollup: %v", err)
	}
	if row.Orders != 4 {
		t.Errorf("orders = %d, want 4", row.Orders)
	}
	if row.RevenueCents != 16500 {
		t.Errorf("revenue = %d, want 16500 - the outsider's 990000 must not be in it", row.RevenueCents)
	}
}

// An empty set has to answer zero rather than sum the whole table
func TestSegmentOrderRollupOfNobodyIsZero(t *testing.T) {
	channelID := uuid.New()
	at := time.Now().UTC().Truncate(time.Microsecond)
	q := seedAgg(t, channelID, aggRow{customer: uuid.New(), orders: 5, cents: 50000, last: &at})

	row, err := q.SegmentOrderRollup(context.Background(), nil)
	if err != nil {
		t.Fatalf("SegmentOrderRollup: %v", err)
	}
	if row.Orders != 0 || row.RevenueCents != 0 {
		t.Errorf("empty set gave %d orders / %d cents, want zero", row.Orders, row.RevenueCents)
	}
}

// The projection holds a row per customer, so one who has never ordered has a null date, not no row
func TestAMemberWhoNeverOrderedHasNoDate(t *testing.T) {
	channelID, never := uuid.New(), uuid.New()
	q := seedAgg(t, channelID, aggRow{customer: never, orders: 0, cents: 0, last: nil})

	got, err := q.SegmentMemberOrderStats(context.Background(), []uuid.UUID{never})
	if err != nil {
		t.Fatalf("SegmentMemberOrderStats: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want the one row with no orders", len(got))
	}
	if got[0].LastOrderAt.Valid {
		t.Errorf("last_order_at = %v, want null - a member with no orders would read as ordering at the zero time", got[0].LastOrderAt)
	}
}
