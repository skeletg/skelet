package tracking_test

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/tracking"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Pins today's delivery contract through the public API only, so the emit/engine split must keep it: a placed order is
// one purchase per destination however often the outbox replays it, denied consent reaches no analytics or ads
// destination, and a kind the merchant made necessary is the one that still gets it
func TestAPlacedOrderIsOneDeliveryPerDestination(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	var ch uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'trk', 'EUR') RETURNING id`, "trk-"+uuid.NewString()[:8]).Scan(&ch); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch) })

	const necessary = "general/privacy/fake_affiliate_necessary"
	send := func(context.Context, *http.Client, sdk.DestinationConfig, sdk.ShopEvent) error { return nil }
	tracking.RegisterPluginKinds([]sdk.RegisteredDestination{
		{PluginSlug: "fake", Destination: sdk.Destination{Kind: "fake-analytics", Purposes: []sdk.DestinationPurpose{sdk.PurposeAnalytics}, Send: send}},
		{PluginSlug: "fake", Destination: sdk.Destination{Kind: "fake-ads", Purposes: []sdk.DestinationPurpose{sdk.PurposeAds}, DefaultEvents: []string{tracking.Purchase}, Send: send}},
		{PluginSlug: "fake", Destination: sdk.Destination{Kind: "fake-affiliate", Purposes: []sdk.DestinationPurpose{sdk.PurposeAds}, NecessarySetting: necessary, Send: send}},
	}, func(context.Context, string, uuid.UUID) bool { return true })

	settings := switches{}
	svc := tracking.NewService(db.New(conn), nil, settings)
	for _, kind := range []string{"fake-analytics", "fake-ads", "fake-affiliate"} {
		if _, err := svc.Create(ctx, tracking.Destination{ChannelID: ch, Kind: kind, Name: kind, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}

	orders := ordersByID{}
	bus := events.NewBus()
	svc.Subscribe(bus, tracking.Readers{Orders: orders, Channels: channelOf{"EUR"}})
	place := func(c auth.Consent) uuid.UUID {
		t.Helper()
		o := order.Order{ID: uuid.New(), Number: "ORD-" + uuid.NewString()[:6], ChannelID: ch, CurrencyCode: "EUR", SubtotalAmount: 1500, TotalAmount: 1500,
			Lines:   []order.OrderLine{{ID: uuid.New(), ProductSku: "SKU-1", ProductName: "Mug", UnitPrice: 1500, Quantity: 1}},
			Shopper: &auth.Shopper{Consent: c, AnonID: "visitor-1"}}
		orders[o.ID] = o
		for range 3 { // placed, then two outbox replays
			if err := bus.DispatchAsyncNow(ctx, &order.OrderPlaced{OrderID: o.ID}); err != nil {
				t.Fatal(err)
			}
		}
		return o.ID
	}
	deliveries := func(orderID uuid.UUID) map[string]int {
		t.Helper()
		rows, err := conn.Query(`SELECT t.kind, count(*) FROM tracking_deliveries d
			JOIN tracking_destinations t ON t.id = d.destination_id JOIN tracking_events e ON e.id = d.tracking_event_id
			WHERE t.channel_id = $1 AND e.event_name = 'purchase' AND e.event_id = $2 GROUP BY t.kind`, ch, orderID.String())
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		got := map[string]int{}
		for rows.Next() {
			var kind string
			var n int
			if err := rows.Scan(&kind, &n); err != nil {
				t.Fatal(err)
			}
			got[kind] = n
		}
		return got
	}
	same := func(name string, got, want map[string]int) {
		t.Helper()
		if len(got) != len(want) {
			t.Errorf("%s: deliveries %v, want %v", name, got, want)
			return
		}
		for k, n := range want {
			if got[k] != n {
				t.Errorf("%s: deliveries %v, want %v", name, got, want)
				return
			}
		}
	}

	granted := place(auth.Consent{Analytics: true, Ads: true})
	same("consent granted", deliveries(granted), map[string]int{"fake-analytics": 1, "fake-ads": 1, "fake-affiliate": 1})

	denied := place(auth.Consent{})
	same("consent denied", deliveries(denied), map[string]int{})

	settings[necessary] = true
	deniedNecessary := place(auth.Consent{})
	same("consent denied, affiliate made necessary", deliveries(deniedNecessary), map[string]int{"fake-affiliate": 1})

	var stored int
	if err := conn.QueryRow(`SELECT count(*) FROM tracking_events WHERE channel_id = $1`, ch).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 2 {
		t.Errorf("%d events stored, want the two purchases a destination took - one routed to nobody is never stored", stored)
	}
}

type switches map[string]bool

func (s switches) GetBool(_ context.Context, path string, _ config.Scope) (bool, error) {
	return s[path], nil
}

type ordersByID map[uuid.UUID]order.Order

func (o ordersByID) GetByID(_ context.Context, id uuid.UUID) (order.Order, error) {
	if v, ok := o[id]; ok {
		return v, nil
	}
	return order.Order{}, sql.ErrNoRows
}
func (ordersByID) ListCreditMemos(context.Context, uuid.UUID) ([]order.CreditMemo, error) {
	return nil, nil
}

type channelOf struct{ currency string }

func (c channelOf) GetByID(_ context.Context, id uuid.UUID) (channel.Channel, error) {
	return channel.Channel{ID: id, CurrencyCode: c.currency}, nil
}
