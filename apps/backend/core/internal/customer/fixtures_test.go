package customer

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/authstore"
	"github.com/skeletg/skelet/apps/backend/core/internal/configstore"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	dbsqlc "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// The service wired the way main.go does, except the token stores run on their Postgres backends so
// the suite needs one dependency rather than two - the protocol above them is backend-agnostic
type fixture struct {
	svc     *Service
	cfg     *config.Service
	verify  *auth.VerificationStore
	reset   *auth.PasswordResetStore
	refresh *auth.RefreshStore
	conn    *sql.DB
	ctx     context.Context
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	conn := testdb.Require(t)
	queries, txMgr := dbsqlc.New(conn), txrunner.New(conn)

	tokens := authstore.NewPostgresTokenBackend(queries)
	verify := auth.NewVerificationStore(tokens)
	refresh := auth.NewRefreshStore(authstore.NewPostgresRefreshBackend(queries, "customer"), nil, "customer")
	cfg := config.NewService(configstore.New(queries))

	reset := auth.NewPasswordResetStore(tokens)
	svc := NewService(
		NewRepository(queries, nil, txMgr), // no Signals eraser - erasure of the analytics side is its own suite
		verify,
		reset,
		refresh,
		email.LogSender{},
		stubStores{},
		consent.NewService(consent.NewRepository(queries)),
		events.NewBus(),
	)
	svc.SetConfigService(cfg)
	return &fixture{svc: svc, cfg: cfg, verify: verify, reset: reset, refresh: refresh, conn: conn, ctx: context.Background()}
}

// Only the verification and reset paths read a store, and neither asserts on the rendered link
type stubStores struct{}

func (stubStores) GetByID(_ context.Context, id uuid.UUID) (StoreSnapshot, error) {
	return StoreSnapshot{ID: id, PublicURL: "https://shop.invalid", EmailFromName: "Test Shop"}, nil
}

// Clones an existing channel's currency and locale so the FKs resolve without seeding either catalog
func (f *fixture) newChannel(t *testing.T) uuid.UUID {
	t.Helper()
	slug := "cust-test-" + uuid.NewString()[:8]
	var id uuid.UUID
	if err := f.conn.QueryRow(
		`INSERT INTO channels (slug, name, currency_code, default_locale)
		 SELECT $1, $1, currency_code, default_locale FROM channels ORDER BY created_at LIMIT 1
		 RETURNING id`, slug).Scan(&id); err != nil {
		t.Skipf("no channel to clone currency and locale from: %v", err)
	}
	t.Cleanup(func() {
		_, _ = f.conn.Exec(`DELETE FROM customers WHERE channel_id = $1`, id)
		_, _ = f.conn.Exec(`DELETE FROM channels WHERE id = $1`, id)
	})
	return id
}

func (f *fixture) register(t *testing.T, channelID uuid.UUID, addr, password string) Customer {
	t.Helper()
	c, err := f.svc.Register(f.ctx, channelID, addr, password, "Test", "Person", nil)
	if err != nil {
		t.Fatalf("register %s: %v", addr, err)
	}
	f.svc.FlushMail()
	return c
}

// Redeems a real verification token, since nothing else flips the flag through the service
func (f *fixture) markVerified(t *testing.T, customerID uuid.UUID) {
	t.Helper()
	c, err := f.svc.GetByID(f.ctx, customerID)
	if err != nil {
		t.Fatalf("load customer: %v", err)
	}
	token, err := f.verify.Create(f.ctx, customerID, c.Email)
	if err != nil {
		t.Fatalf("mint verification token: %v", err)
	}
	if err := f.svc.VerifyEmail(f.ctx, token); err != nil {
		t.Fatalf("verify email: %v", err)
	}
}

// A live 30-day session, the thing every password write is supposed to kill
func (f *fixture) newSession(t *testing.T, customerID uuid.UUID) string {
	t.Helper()
	token, _, err := f.refresh.Create(f.ctx, customerID, "127.0.0.1", "go-test")
	if err != nil {
		t.Fatalf("mint refresh token: %v", err)
	}
	t.Cleanup(func() { _ = f.refresh.RevokeAll(context.Background(), customerID) })
	return token
}

func (f *fixture) sessionLive(t *testing.T, token string) bool {
	t.Helper()
	_, err := f.refresh.Validate(f.ctx, token)
	return err == nil
}

func (f *fixture) setVerificationGate(t *testing.T, channelID uuid.UUID, on bool) {
	t.Helper()
	if _, err := f.cfg.Set(f.ctx, pathRequireEmailVerification, strconv.FormatBool(on),
		config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}); err != nil {
		t.Fatalf("set verification gate: %v", err)
	}
}

// Scoped to one subject so a package running alongside others can't miscount
func (f *fixture) authTokens(t *testing.T, subjectID uuid.UUID) int {
	t.Helper()
	var n int
	if err := f.conn.QueryRow(`SELECT count(*) FROM auth_tokens WHERE subject_id = $1`, subjectID).Scan(&n); err != nil {
		t.Fatalf("count auth tokens: %v", err)
	}
	return n
}

func (f *fixture) addAddress(t *testing.T, customerID uuid.UUID) Address {
	t.Helper()
	addr, err := f.svc.AddAddress(f.ctx, customerID, addressInput())
	if err != nil {
		t.Fatalf("add address: %v", err)
	}
	return addr
}

func addressInput() AddressInput {
	return AddressInput{
		FirstName:   "Test",
		LastName:    "Person",
		Street1:     "1 Test Street",
		City:        "Testville",
		PostalCode:  "12345",
		CountryCode: "US",
	}
}

// Enough of an order row for CountOrders - everything else on the table defaults
func (f *fixture) seedOrder(t *testing.T, channelID, customerID uuid.UUID) {
	t.Helper()
	var id uuid.UUID
	if err := f.conn.QueryRow(
		`INSERT INTO orders (channel_id, currency_code, customer_id, email)
		 SELECT id, currency_code, $2, $3 FROM channels WHERE id = $1 RETURNING id`,
		channelID, customerID, "order-"+uuid.NewString()[:8]+"@example.invalid").Scan(&id); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	t.Cleanup(func() { _, _ = f.conn.Exec(`DELETE FROM orders WHERE id = $1`, id) })
}

func (f *fixture) newGroup(t *testing.T) CustomerGroup {
	t.Helper()
	slug := "cust-test-" + uuid.NewString()[:8]
	g, err := f.svc.CreateGroup(f.ctx, CreateCustomerGroupInput{Name: slug, Slug: slug})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	t.Cleanup(func() { _, _ = f.conn.Exec(`DELETE FROM customer_groups WHERE id = $1`, g.ID) })
	return g
}

func randomEmail() string { return "cust-" + uuid.NewString()[:8] + "@example.invalid" }

// Records what went through the outbox and that it rode a transaction
type recordingOutbox struct {
	mu     sync.Mutex
	events []events.Event
}

func (o *recordingOutbox) Enqueue(_ context.Context, sqlTx *sql.Tx, e events.Event) error {
	if sqlTx == nil {
		return errors.New("enqueued outside a transaction")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, e)
	return nil
}

func (o *recordingOutbox) types() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]string, len(o.events))
	for i, e := range o.events {
		out[i] = e.Type()
	}
	return out
}

func (f *fixture) withOutbox() *recordingOutbox {
	o := &recordingOutbox{}
	f.svc.SetOutbox(o)
	return o
}

func (f *fixture) withFallback() *recordingFallback {
	fb := &recordingFallback{}
	f.svc.SetAuthEmailFallback(fb)
	return fb
}

func (f *fixture) resetLink(t *testing.T, customerID uuid.UUID) string {
	t.Helper()
	token, err := f.reset.Create(f.ctx, customerID)
	if err != nil {
		t.Fatalf("mint reset token: %v", err)
	}
	return token
}

func (f *fixture) verifyLink(t *testing.T, c Customer) string {
	t.Helper()
	token, err := f.verify.Create(f.ctx, c.ID, c.Email)
	if err != nil {
		t.Fatalf("mint verification token: %v", err)
	}
	return token
}
