package dsar

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/authstore"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/storecredit"
	"github.com/skeletg/skelet/apps/backend/core/internal/wishlist"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type stores struct{}

func (stores) GetByID(_ context.Context, id uuid.UUID) (customer.StoreSnapshot, error) {
	return customer.StoreSnapshot{ID: id}, nil
}

type orders struct {
	list []order.Order
	err  error
}

func (o orders) ListOrdersByCustomer(context.Context, uuid.UUID) ([]order.Order, error) {
	return o.list, o.err
}

type channels struct{}

func (channels) GetByID(_ context.Context, id uuid.UUID) (channel.Channel, error) {
	return channel.Channel{ID: id, CurrencyCode: "USD"}, nil
}

type rig struct {
	conn *sql.DB
	ch   uuid.UUID
	b    *Builder
	c    customer.Customer
}

func newRig(t *testing.T) rig {
	t.Helper()
	conn := testdb.Require(t)
	q := db.New(conn)
	ctx := context.Background()
	tokens := authstore.NewPostgresTokenBackend(q)
	customers := customer.NewService(customer.NewRepository(q, nil, txrunner.New(conn)), auth.NewVerificationStore(tokens),
		auth.NewPasswordResetStore(tokens), auth.NewRefreshStore(authstore.NewPostgresRefreshBackend(q, "customer"), nil, "customer"),
		email.LogSender{}, stores{}, consent.NewService(consent.NewRepository(q)), events.NewBus())

	var ch uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels ORDER BY created_at LIMIT 1 RETURNING id`, "dsar-"+uuid.NewString()[:8]).Scan(&ch); err != nil {
		t.Skipf("no channel to clone: %v", err)
	}
	t.Cleanup(func() {
		customers.FlushMail()
		_, _ = conn.Exec(`DELETE FROM customers WHERE channel_id = $1`, ch)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch)
	})
	c, err := customers.Register(ctx, ch, "dsar-"+uuid.NewString()[:8]+"@example.invalid", "password123", "Ana", "K", nil)
	if err != nil {
		t.Fatal(err)
	}
	b := New(customers, orders{}, consent.NewService(consent.NewRepository(q)), channels{}, storecredit.NewService(q, txrunner.New(conn)), wishlist.NewService(q), q)
	return rig{conn: conn, ch: ch, b: b, c: c}
}

func TestTheBundleCarriesEverySectionWhole(t *testing.T) {
	r := newRig(t)
	conn, ch, c, b, ctx := r.conn, r.ch, r.c, r.b, context.Background()
	// More ledger rows than the old export's 50-row clamp, so a page cap would show
	if _, err := conn.Exec(`INSERT INTO loyalty_point_entries (customer_id, points, reason) SELECT $1, 1, 'earn' FROM generate_series(1, 60)`, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO customer_payment_methods (customer_id, provider_slug, token, brand, last4) VALUES ($1, 'stripe', 'pm_vault_secret', 'visa', '4242')`, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO back_in_stock_requests (channel_id, product_id, sku, email) VALUES ($1, $2, 'sku-1', $3)`, ch, uuid.New(), strings.ToUpper(c.Email)); err != nil {
		t.Fatal(err)
	}
	var orderID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO orders (channel_id, currency_code, customer_id, email) SELECT id, currency_code, $2, $3 FROM channels WHERE id = $1 RETURNING id`, ch, c.ID, c.Email).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM orders WHERE id = $1`, orderID) })
	if _, err := conn.Exec(`INSERT INTO order_shipments (order_id, position, group_key, delivery_kind, ship_to) VALUES ($1, 0, 'all', 'ship_to_address', '{"street1":"7 Parcel Way"}')`, orderID); err != nil {
		t.Fatal(err)
	}

	bundle, err := b.Build(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"reviews", "productQuestions", "subscriptions", "giftCards", "savedPaymentMethods", "carts", "orderShipments",
		"downloadGrants", "backInStockRequests", "signupFormSubmissions", "emailOptIns", "webFormSubmissions", "storeCredit", "loyaltyPoints"} {
		if _, ok := bundle.Extra[key]; !ok {
			t.Errorf("the bundle has no %s section", key)
		}
	}
	var loyalty, methods, bis []map[string]any
	for key, into := range map[string]*[]map[string]any{"loyaltyPoints": &loyalty, "savedPaymentMethods": &methods, "backInStockRequests": &bis} {
		if err := json.Unmarshal(bundle.Extra[key].(json.RawMessage), into); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
	}
	if len(loyalty) != 60 {
		t.Errorf("loyalty entries = %d, want all 60", len(loyalty))
	}
	if len(methods) != 1 || methods[0]["last4"] != "4242" || len(bis) != 1 {
		t.Errorf("payment methods %v, back-in-stock %v - want the saved card and the request made under the address", methods, bis)
	}
	if ships := section(t, bundle, "orderShipments"); len(ships) != 1 || !strings.Contains(fmt.Sprint(ships[0]["shipTo"]), "7 Parcel Way") {
		t.Errorf("order shipments = %v, want the package with the address it went to", ships)
	}
	blob, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), "pm_vault_secret") {
		t.Error("the provider's vault token reached the bundle")
	}

	b.orders = orders{err: errors.New("orders unreadable")}
	if _, err := b.Build(ctx, c.ID); err == nil {
		t.Error("an unreadable order history produced a bundle anyway")
	}
}

func section(t *testing.T, bundle customer.DataExport, key string) []map[string]any {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal(bundle.Extra[key].(json.RawMessage), &rows); err != nil {
		t.Fatalf("%s: %v", key, err)
	}
	return rows
}

// Signing in doesn't prove the address, so a stranger's registration under it must not export what a guest left there
func TestAnUnverifiedAddressExportsNoGuestRows(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	form := uuid.New()
	if _, err := r.conn.Exec(`INSERT INTO web_forms (id, channel_id, slug, name) VALUES ($1, $2, $3, 'Contact')`, form, r.ch, "contact-"+uuid.NewString()[:8]); err != nil {
		t.Fatal(err)
	}
	for _, answers := range []string{
		`{"email": "` + strings.ToUpper(r.c.Email) + `", "message": "call me on 555"}`,
		`{"email": "x` + r.c.Email + `", "message": "a stranger whose address only contains theirs"}`,
		`{"note": "` + r.c.Email + ` mentioned in passing"}`,
	} {
		if _, err := r.conn.Exec(`INSERT INTO web_form_submissions (form_id, channel_id, answers) VALUES ($1, $2, $3)`, form, r.ch, answers); err != nil {
			t.Fatal(err)
		}
	}
	// The second row has no address at all - the unverified export's empty address must not match it either
	for _, addr := range []string{r.c.Email, ""} {
		if _, err := r.conn.Exec(`INSERT INTO back_in_stock_requests (channel_id, product_id, sku, email) VALUES ($1, $2, 'sku-1', $3)`, r.ch, uuid.New(), addr); err != nil {
			t.Fatal(err)
		}
	}

	self, err := r.b.BuildSelfServe(ctx, r.c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(section(t, self, "webFormSubmissions")) + len(section(t, self, "backInStockRequests")); n != 0 {
		t.Fatalf("an unverified account exported %d guest rows under its address", n)
	}

	admin, err := r.b.Build(ctx, r.c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := section(t, admin, "webFormSubmissions"); len(got) != 1 || !strings.Contains(fmt.Sprint(got[0]["answers"]), "555") {
		t.Fatalf("admin export web forms %v, want only the submission whose answer IS the address", got)
	}

	if _, err := r.conn.Exec(`UPDATE customers SET is_email_verified = true WHERE id = $1`, r.c.ID); err != nil {
		t.Fatal(err)
	}
	if self, err = r.b.BuildSelfServe(ctx, r.c.ID); err != nil {
		t.Fatal(err)
	}
	if len(section(t, self, "webFormSubmissions")) != 1 || len(section(t, self, "backInStockRequests")) != 1 {
		t.Fatal("a verified address did not export its guest rows")
	}
}
