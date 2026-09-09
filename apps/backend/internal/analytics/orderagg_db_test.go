package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/testdb"
)

// Gated on a disposable marker - the commerce query filtered cancellations at read time, an append-only spine has to net them out instead, and getting that wrong leaves refunded revenue in a customer's score for good

type spineOrder struct {
	customer  uuid.UUID
	orderID   uuid.UUID
	cents     int64
	at        time.Time
	cancelled bool
}

func seedOrders(t *testing.T, channelID uuid.UUID, orders ...spineOrder) *pgSpine {
	t.Helper()
	conn := testdb.RequireSignals(t)
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM analytics_events WHERE channel_id = $1`, channelID) })

	for _, o := range orders {
		if _, err := conn.Exec(`INSERT INTO analytics_events
			(event_name, occurred_at, received_at, channel_id, customer_id, source, properties, dedupe_key)
			VALUES ('order_placed', $1, $1, $2, $3, 'commerce', $4::jsonb, $5)`,
			o.at, channelID, o.customer,
			`{"order_id":"`+o.orderID.String()+`","total_cents":`+itoa(o.cents)+`}`,
			"act:order_placed:"+o.orderID.String()); err != nil {
			t.Fatalf("seed placed: %v", err)
		}
		if o.cancelled {
			if _, err := conn.Exec(`INSERT INTO analytics_events
				(event_name, occurred_at, received_at, channel_id, customer_id, source, properties, dedupe_key)
				VALUES ('order_cancelled', $1, $1, $2, $3, 'commerce', $4::jsonb, $5)`,
				o.at.Add(time.Hour), channelID, o.customer,
				`{"order_id":"`+o.orderID.String()+`","status":"cancelled"}`,
				"act:order_status:cancelled:"+o.orderID.String()); err != nil {
				t.Fatalf("seed cancelled: %v", err)
			}
		}
	}
	return newPGSpine(signalsdb.New(conn), nil)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestOrderAggregatesNetOutCancellations(t *testing.T) {
	channelID, customerID := uuid.New(), uuid.New()
	// Postgres stores timestamptz to microseconds, so a nanosecond-precision clock never compares equal on read-back
	base := time.Now().UTC().Add(-90 * 24 * time.Hour).Truncate(time.Microsecond)
	spine := seedOrders(t, channelID,
		spineOrder{customer: customerID, orderID: uuid.New(), cents: 5000, at: base},
		spineOrder{customer: customerID, orderID: uuid.New(), cents: 3000, at: base.Add(24 * time.Hour), cancelled: true},
		spineOrder{customer: customerID, orderID: uuid.New(), cents: 2500, at: base.Add(48 * time.Hour)},
	)

	got, err := spine.ChannelOrderAggregates(context.Background(), channelID)
	if err != nil {
		t.Fatalf("ChannelOrderAggregates: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d customers, want 1", len(got))
	}
	a := got[0]
	if a.OrderCount != 2 {
		t.Errorf("order_count = %d, want 2 - the cancelled order still counts", a.OrderCount)
	}
	if a.MonetaryCents != 7500 {
		t.Errorf("monetary_cents = %d, want 7500 - cancelled revenue is still in the score", a.MonetaryCents)
	}
	// last_order_at drives recency, so a cancelled order must not make a lapsed customer look active
	if !a.LastOrderAt.Equal(base.Add(48 * time.Hour).UTC()) {
		t.Errorf("last_order_at = %s, want the third order's time", a.LastOrderAt)
	}
	if !a.FirstOrderAt.Equal(base.UTC()) {
		t.Errorf("first_order_at = %s, want the first order's time", a.FirstOrderAt)
	}
}

func TestACustomerWhoseOnlyOrderWasCancelledDisappears(t *testing.T) {
	channelID, customerID := uuid.New(), uuid.New()
	spine := seedOrders(t, channelID,
		spineOrder{customer: customerID, orderID: uuid.New(), cents: 9900, at: time.Now().UTC().Add(-time.Hour), cancelled: true},
	)

	got, err := spine.ChannelOrderAggregates(context.Background(), channelID)
	if err != nil {
		t.Fatalf("ChannelOrderAggregates: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want nobody - their only order was cancelled, so they are a prospect again", got)
	}
}

func TestOrderAggregatesStayWithinTheirChannel(t *testing.T) {
	mine, theirs := uuid.New(), uuid.New()
	customerID := uuid.New()
	spine := seedOrders(t, mine, spineOrder{customer: customerID, orderID: uuid.New(), cents: 1000, at: time.Now().UTC().Add(-time.Hour)})
	_ = seedOrders(t, theirs, spineOrder{customer: customerID, orderID: uuid.New(), cents: 7777, at: time.Now().UTC().Add(-time.Hour)})

	got, err := spine.ChannelOrderAggregates(context.Background(), mine)
	if err != nil {
		t.Fatalf("ChannelOrderAggregates: %v", err)
	}
	if len(got) != 1 || got[0].MonetaryCents != 1000 {
		t.Fatalf("got %+v, want only this channel's 1000", got)
	}
}
