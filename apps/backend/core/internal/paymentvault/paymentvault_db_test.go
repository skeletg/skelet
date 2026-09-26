package paymentvault

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	dbsqlc "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// This table is a pointer at money: each row is a token the gateway will charge off-session, so the two guarantees are ownership (one customer can never read or delete another's saved card) and that a local row never outlives the vault entry it points at

const testProvider = "vault-fixture"

// One registration per test binary, since the provider registry is global and panics on a duplicate
// slug - tests reconfigure the shared fake instead, and run sequentially
var (
	registerOnce sync.Once
	vaulter      = &fakeVaulter{}
)

// Stands in for a gateway's Vaulter capability - the rest of the Provider surface is never reached
// on these paths, so an embedded nil interface is the right shape
type fakeVaulter struct {
	payment.Provider
	vaulted      payment.VaultedMethod
	vaultErr     error
	deleteErr    error
	sawToken     string
	deletedToken string
	deletes      int
}

func (f *fakeVaulter) VaultPaymentMethod(_ context.Context, req payment.VaultRequest) (payment.VaultedMethod, error) {
	f.sawToken = req.Token
	return f.vaulted, f.vaultErr
}

func (f *fakeVaulter) DeleteVaultedPaymentMethod(_ context.Context, req payment.VaultDeleteRequest) error {
	f.deletes++
	f.deletedToken = req.ProviderToken
	return f.deleteErr
}

// Reports the provider as installed and enabled on the channel, which is all Vault checks
type enabledMethodRepo struct {
	payment.Repository
}

func (enabledMethodRepo) GetPaymentMethod(_ context.Context, channelID uuid.UUID, slug string) (payment.PaymentMethod, error) {
	return payment.PaymentMethod{ChannelID: channelID, ProviderSlug: slug, IsEnabled: true}, nil
}

func newTestService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	conn := testdb.Require(t)
	registerOnce.Do(func() {
		payment.RegisterProvider(payment.Registration{Slug: testProvider, Provider: vaulter, DisplayName: "Vault fixture"})
	})
	*vaulter = fakeVaulter{vaulted: payment.VaultedMethod{
		ProviderToken: "vault_" + uuid.NewString(), Brand: "visa", Last4: "4242", ExpMonth: 12, ExpYear: 2030,
	}}
	payments := payment.NewService(enabledMethodRepo{}, nil, nil)
	return NewService(dbsqlc.New(conn), payments), conn
}

func seedCustomer(t *testing.T, conn *sql.DB) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var groupID, channelID uuid.UUID
	if err := conn.QueryRowContext(ctx, `SELECT id FROM customer_groups LIMIT 1`).Scan(&groupID); err != nil {
		t.Skipf("no customer group to seed against: %v", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel to seed against: %v", err)
	}
	var id uuid.UUID
	email := "vault-" + uuid.NewString() + "@example.invalid"
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO customers (email, password_hash, customer_group_id, channel_id)
		 VALUES ($1, 'x', $2, $3) RETURNING id`, email, groupID, channelID).Scan(&id); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM customers WHERE id = $1`, id) }) // methods cascade
	return id
}

func rowsFor(t *testing.T, conn *sql.DB, id uuid.UUID) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM customer_payment_methods WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func TestSaveRecordsTheGatewaysTokenNotTheClientNonce(t *testing.T) {
	// The client-side nonce is single-use - storing it instead of the vault token means every later
	// renewal charge fails, and the card the shopper thinks they saved was never saved at all
	svc, conn := newTestService(t)
	ctx := context.Background()
	customerID := seedCustomer(t, conn)
	want := vaulter.vaulted

	saved, err := svc.Save(ctx, customerID, "shopper@example.invalid", testProvider, "client-nonce-abc", uuid.New())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if vaulter.sawToken != "client-nonce-abc" {
		t.Errorf("provider got token %q, want the client nonce", vaulter.sawToken)
	}
	if saved.Token != want.ProviderToken {
		t.Errorf("stored token = %q, want the gateway's %q", saved.Token, want.ProviderToken)
	}
	if saved.Brand != want.Brand || saved.Last4 != want.Last4 || saved.ExpMonth != want.ExpMonth || saved.ExpYear != want.ExpYear {
		t.Errorf("display fields = %+v, want the gateway's %+v", saved, want)
	}

	readBack, err := svc.Get(ctx, customerID, saved.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if readBack.Token != want.ProviderToken || readBack.CustomerID != customerID {
		t.Errorf("read back %+v, want the row owned by %v holding the vault token", readBack, customerID)
	}
}

func TestNothingIsRecordedWhenTheGatewayRefusesToVault(t *testing.T) { // a row with no vault entry is a card that can never be charged
	svc, conn := newTestService(t)
	ctx := context.Background()
	customerID := seedCustomer(t, conn)
	vaulter.vaultErr = errors.New("card declined")

	if _, err := svc.Save(ctx, customerID, "shopper@example.invalid", testProvider, "client-nonce-abc", uuid.New()); err == nil {
		t.Fatal("Save succeeded despite the gateway refusing")
	}
	methods, err := svc.List(ctx, customerID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(methods) != 0 {
		t.Errorf("list = %+v, want nothing recorded", methods)
	}
}

func TestOneCustomerCannotSeeAnothersSavedCards(t *testing.T) {
	// Method ids reach the server from the account UI and from checkout, so the id alone must never be
	// enough - a missing row and someone else's row answer identically on purpose
	svc, conn := newTestService(t)
	ctx := context.Background()
	owner, stranger := seedCustomer(t, conn), seedCustomer(t, conn)

	saved, err := svc.Save(ctx, owner, "owner@example.invalid", testProvider, "nonce", uuid.New())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := svc.Get(ctx, stranger, saved.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-customer Get err = %v, want ErrNotFound", err)
	}
	if _, err := svc.Get(ctx, stranger, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown-id Get err = %v, want the same ErrNotFound", err)
	}
	methods, err := svc.List(ctx, stranger)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(methods) != 0 {
		t.Errorf("stranger's list = %+v, want empty", methods)
	}
}

func TestDeleteRefusesAnotherCustomersCardBeforeTouchingTheGateway(t *testing.T) {
	// The ownership check has to come first - reaching the provider on a foreign id would let anyone
	// with a method id revoke a stranger's card even though the local row survives
	svc, conn := newTestService(t)
	ctx := context.Background()
	owner, stranger := seedCustomer(t, conn), seedCustomer(t, conn)

	saved, err := svc.Save(ctx, owner, "owner@example.invalid", testProvider, "nonce", uuid.New())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := svc.Delete(ctx, stranger, saved.ID, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-customer Delete err = %v, want ErrNotFound", err)
	}
	if vaulter.deletes != 0 {
		t.Errorf("the gateway was asked to delete %d times on a foreign id", vaulter.deletes)
	}
	if rowsFor(t, conn, saved.ID) != 1 {
		t.Error("the owner's row is gone after a stranger's delete")
	}
}

func TestDeleteKeepsTheRowWhenTheGatewayRefuses(t *testing.T) {
	// Dropping the row anyway orphans a still-chargeable vault entry: the shopper sees the card gone
	// while the gateway will happily keep billing it, and nothing points at it any more
	svc, conn := newTestService(t)
	ctx := context.Background()
	customerID := seedCustomer(t, conn)

	saved, err := svc.Save(ctx, customerID, "owner@example.invalid", testProvider, "nonce", uuid.New())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	vaulter.deleteErr = errors.New("gateway unavailable")

	if err := svc.Delete(ctx, customerID, saved.ID, uuid.New()); err == nil {
		t.Fatal("Delete reported success while the gateway refused")
	}
	if rowsFor(t, conn, saved.ID) != 1 {
		t.Error("the row was dropped even though the vault entry is still live")
	}
}

func TestDeleteRevokesTheVaultEntryThenDropsTheRow(t *testing.T) {
	svc, conn := newTestService(t)
	ctx := context.Background()
	customerID := seedCustomer(t, conn)
	want := vaulter.vaulted.ProviderToken

	saved, err := svc.Save(ctx, customerID, "owner@example.invalid", testProvider, "nonce", uuid.New())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := svc.Delete(ctx, customerID, saved.ID, uuid.New()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if vaulter.deletedToken != want {
		t.Errorf("gateway asked to revoke %q, want the stored vault token %q", vaulter.deletedToken, want)
	}
	if rowsFor(t, conn, saved.ID) != 0 {
		t.Error("the row survived a successful delete")
	}
	if _, err := svc.Get(ctx, customerID, saved.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete err = %v, want ErrNotFound", err)
	}
}
