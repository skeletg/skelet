package front

import (
	"context"
	"database/sql"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type limiterFunc func() bool

func (f limiterFunc) Allow(context.Context, string) bool { return f() }

// The lookup only reads through Repo; the other seams are wired so NewService accepts them
func newTrackOrderResolver(t *testing.T, conn *sql.DB, allow bool) *queryResolver {
	t.Helper()
	svc, err := order.NewService(order.Deps{
		Repo: order.NewRepository(db.New(conn)), CartRepo: struct{ cart.Repository }{}, Customer: &customer.Service{},
		Tx: txrunner.New(conn), Bus: events.NewBus(), Repricer: struct{ order.CartRepricer }{},
		Outbox: struct{ order.OutboxEnqueuer }{}, PaymentAuthorizer: struct{ order.PaymentAuthorizer }{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &queryResolver{&Resolver{OrderSvc: svc, AuthLimiter: limiterFunc(func() bool { return allow })}}
}

func TestTrackOrderFindsAnOrderByNumberEmailAndPostcode(t *testing.T) {
	conn := testdb.Require(t)
	tag := strings.ToUpper(uuid.NewString()[:8]) // order numbers are stored upper-case, as the default ORD- sequence makes them
	chans := make([]uuid.UUID, 2)
	for i := range chans {
		if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'track', 'USD') RETURNING id`,
			"track-"+strings.ToLower(tag)+string(rune('a'+i))).Scan(&chans[i]); err != nil {
			t.Fatal(err)
		}
	}
	var groupID, customerID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM customer_groups LIMIT 1`).Scan(&groupID); err != nil {
		t.Fatalf("no customer group to seed against: %v", err)
	}
	if err := conn.QueryRow(`INSERT INTO customers (email, password_hash, customer_group_id, channel_id) VALUES ($1, 'x', $2, $3) RETURNING id`,
		"member-"+tag+"@example.invalid", groupID, chans[0]).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	seed := func(number, email, shipPostcode, billPostcode string, ch uuid.UUID, customer *uuid.UUID) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO orders (channel_id, currency_code, number, email, shipping_postal_code, billing_postal_code, customer_id)
			VALUES ($1, 'USD', $2, $3, $4, $5, $6) RETURNING id`, ch, number, email, shipPostcode, billPostcode, customer).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	guest := seed("TRK-"+tag+"-G", "Guest@Example.invalid", "SW1A1AA", "", chans[0], nil)
	member := seed("TRK-"+tag+"-M", "member-"+tag+"@example.invalid", "11000", "", chans[0], &customerID)
	digital := seed("TRK-"+tag+"-D", "digital@example.invalid", "", "10115", chans[0], nil)
	seed("TRK-"+tag+"-O", "other@example.invalid", "75001", "", chans[1], nil)
	addressless := "TRK-" + tag + "-N" // both postcodes stored as '', which an empty input would equal in SQL
	seed(addressless, "noaddr@example.invalid", "", "", chans[0], nil)
	t.Cleanup(func() {
		for _, c := range chans {
			_, _ = conn.Exec(`DELETE FROM orders WHERE channel_id = $1`, c)
		}
		_, _ = conn.Exec(`DELETE FROM customers WHERE id = $1`, customerID)
		for _, c := range chans {
			_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, c)
		}
	})

	req := httptest.NewRequest("POST", "/api/front/graphql", nil)
	ctx := channel.WithChannel(auth.WithRequest(context.Background(), req), &channel.Channel{ID: chans[0]})
	r := newTrackOrderResolver(t, conn, true)

	for _, tc := range []struct {
		name, number, email, postcode string
		want                          *model.TrackOrderResult
	}{
		{"guest order", "TRK-" + tag + "-G", "Guest@Example.invalid", "SW1A1AA",
			&model.TrackOrderResult{OrderID: guest.String(), Email: "Guest@Example.invalid"}},
		{"retyped the way shoppers do", "  trk-" + tag + "-g ", "guest@example.INVALID ", "sw1a 1aa",
			&model.TrackOrderResult{OrderID: guest.String(), Email: "Guest@Example.invalid"}},
		{"customer order needs a sign-in", "TRK-" + tag + "-M", "member-" + tag + "@example.invalid", "11000",
			&model.TrackOrderResult{OrderID: member.String(), RequiresLogin: true}},
		{"no shipping address falls back to billing", "TRK-" + tag + "-D", "digital@example.invalid", "10115",
			&model.TrackOrderResult{OrderID: digital.String(), Email: "digital@example.invalid"}},
		{"wrong email", "TRK-" + tag + "-G", "someone@example.invalid", "SW1A1AA", nil},
		{"wrong postcode", "TRK-" + tag + "-G", "Guest@Example.invalid", "SW1A1AB", nil},
		{"empty postcode never matches an order without one", "TRK-" + tag + "-G", "Guest@Example.invalid", "", nil},
		{"empty postcode never opens an order stored without one", addressless, "noaddr@example.invalid", "", nil},
		{"blank postcode never opens an order stored without one", addressless, "noaddr@example.invalid", "  ", nil},
		{"unknown number", "TRK-" + tag + "-X", "Guest@Example.invalid", "SW1A1AA", nil},
		{"another store's order", "TRK-" + tag + "-O", "other@example.invalid", "75001", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.TrackOrder(ctx, tc.number, tc.email, tc.postcode)
			if err != nil {
				t.Fatal(err)
			}
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("trackOrder = %+v, want %+v", got, tc.want)
			}
		})
	}

	t.Run("rate limited", func(t *testing.T) {
		_, err := newTrackOrderResolver(t, conn, false).TrackOrder(ctx, "TRK-"+tag+"-G", "Guest@Example.invalid", "SW1A1AA")
		if !errors.Is(err, ErrRateLimited) {
			t.Fatalf("err = %v, want ErrRateLimited", err)
		}
	})
}
