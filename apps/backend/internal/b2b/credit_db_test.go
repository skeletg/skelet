package b2b

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	dbsqlc "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/testdb"
	"github.com/skeletg/skelet/internal/tx"
)

// A credit limit is an authorization boundary - buying on account is spending money the merchant hasn't been paid, so every guard that refuses has to actually refuse AND leave nothing behind
// DB-backed because the overdraw guard is GetCompanyForUpdate, a row lock, followed by a read and a write in the same transaction, which no fake can represent

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Require(t)
}

func newService(conn *sql.DB) *Service {
	svc := NewService(dbsqlc.New(conn), tx.New(conn))
	svc.SetEnabled(true)
	return svc
}

type fixture struct {
	companyID  uuid.UUID
	customerID uuid.UUID
	orderID    uuid.UUID
}

func seed(t *testing.T, conn *sql.DB, limit, used, memberOrderLimit int64, status string) fixture { // a company with a credit line, one member, and an order to charge against it
	t.Helper()
	ctx := context.Background()
	var channelID, groupID uuid.UUID
	if err := conn.QueryRowContext(ctx, `SELECT id FROM channels LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel to seed against: %v", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT id FROM customer_groups LIMIT 1`).Scan(&groupID); err != nil {
		t.Skipf("no customer group to seed against: %v", err)
	}

	var f fixture
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO companies (channel_id, name, status, credit_limit_cents, credit_used_cents, payment_terms_days)
		 VALUES ($1, $2, $3, $4, $5, 30) RETURNING id`,
		channelID, "credit fixture "+uuid.NewString()[:8], status, limit, used).Scan(&f.companyID); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO customers (email, password_hash, customer_group_id, channel_id)
		 VALUES ($1, 'x', $2, $3) RETURNING id`,
		"b2b-"+uuid.NewString()+"@example.invalid", groupID, channelID).Scan(&f.customerID); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO company_members (company_id, customer_id, role, order_limit_cents)
		 VALUES ($1, $2, 'buyer', $3)`, f.companyID, f.customerID, memberOrderLimit); err != nil {
		t.Fatalf("seed member: %v", err)
	}
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO orders (channel_id, currency_code) VALUES ($1, 'USD') RETURNING id`,
		channelID).Scan(&f.orderID); err != nil {
		t.Fatalf("seed order: %v", err)
	}

	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM b2b_invoices WHERE company_id = $1`, f.companyID)
		_, _ = conn.Exec(`DELETE FROM b2b_credit_entries WHERE company_id = $1`, f.companyID)
		_, _ = conn.Exec(`DELETE FROM company_members WHERE company_id = $1`, f.companyID)
		_, _ = conn.Exec(`DELETE FROM orders WHERE id = $1`, f.orderID)
		_, _ = conn.Exec(`DELETE FROM customers WHERE id = $1`, f.customerID)
		_, _ = conn.Exec(`DELETE FROM companies WHERE id = $1`, f.companyID)
	})
	return f
}

func reserve(t *testing.T, conn *sql.DB, svc *Service, f fixture, amount int64) error { // runs ReserveCreditTx in its own committed transaction, the way placement does
	t.Helper()
	sqlTx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	if err := svc.ReserveCreditTx(context.Background(), sqlTx, f.customerID, f.orderID, "ORD-1001", amount); err != nil {
		return err
	}
	return sqlTx.Commit()
}

func creditUsed(t *testing.T, conn *sql.DB, companyID uuid.UUID) int64 {
	t.Helper()
	var used int64
	if err := conn.QueryRow(`SELECT credit_used_cents FROM companies WHERE id = $1`, companyID).Scan(&used); err != nil {
		t.Fatalf("read credit used: %v", err)
	}
	return used
}

func rowCounts(t *testing.T, conn *sql.DB, companyID uuid.UUID) (entries, invoices int) {
	t.Helper()
	if err := conn.QueryRow(`SELECT COUNT(*) FROM b2b_credit_entries WHERE company_id = $1`, companyID).Scan(&entries); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM b2b_invoices WHERE company_id = $1`, companyID).Scan(&invoices); err != nil {
		t.Fatal(err)
	}
	return entries, invoices
}

// The happy path leaves three things behind - the drawn-down limit, the ledger entry, and the receivable the merchant will chase
func TestReserveCreditRecordsDrawdownEntryAndInvoice(t *testing.T) {
	conn := testDB(t)
	svc := newService(conn)
	f := seed(t, conn, 100_000, 0, 0, "active")

	if err := reserve(t, conn, svc, f, 25_000); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if got := creditUsed(t, conn, f.companyID); got != 25_000 {
		t.Errorf("credit used = %d, want 25000", got)
	}
	entries, invoices := rowCounts(t, conn, f.companyID)
	if entries != 1 {
		t.Errorf("credit entries = %d, want 1", entries)
	}
	if invoices != 1 {
		t.Errorf("invoices = %d, want 1 - the merchant has nothing to chase", invoices)
	}
}

// Every refusal must be total - no drawdown, no entry, no invoice
func TestReserveCreditRefusals(t *testing.T) {
	conn := testDB(t)
	svc := newService(conn)

	cases := []struct {
		name                             string
		limit, used, memberLimit, amount int64
		status                           string
		wantErr                          error
	}{
		{name: "over the company limit", limit: 10_000, used: 0, amount: 10_001, status: "active", wantErr: ErrCreditExceeded},
		{name: "over the remaining limit", limit: 10_000, used: 9_000, amount: 1_001, status: "active", wantErr: ErrCreditExceeded},
		{name: "no credit line at all", limit: 0, used: 0, amount: 1, status: "active", wantErr: ErrCreditExceeded},
		{name: "over the member's per-order limit", limit: 100_000, used: 0, memberLimit: 5_000, amount: 5_001, status: "active", wantErr: ErrMemberOrderLimit},
		{name: "suspended company", limit: 100_000, used: 0, amount: 1_000, status: "suspended", wantErr: ErrCompanySuspended},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := seed(t, conn, tc.limit, tc.used, tc.memberLimit, tc.status)
			err := reserve(t, conn, svc, f, tc.amount)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
			if got := creditUsed(t, conn, f.companyID); got != tc.used {
				t.Errorf("credit used = %d after a refusal, want %d", got, tc.used)
			}
			entries, invoices := rowCounts(t, conn, f.companyID)
			if entries != 0 || invoices != 0 {
				t.Errorf("a refused order left %d entries and %d invoices behind", entries, invoices)
			}
		})
	}
}

// Spending to exactly the limit is allowed - an off-by-one here refuses legitimate orders at the boundary, which is the complaint nobody can reproduce
func TestReserveCreditAllowsExactlyTheLimit(t *testing.T) {
	conn := testDB(t)
	svc := newService(conn)

	t.Run("whole limit in one order", func(t *testing.T) {
		f := seed(t, conn, 10_000, 0, 0, "active")
		if err := reserve(t, conn, svc, f, 10_000); err != nil {
			t.Fatalf("spending exactly the limit was refused: %v", err)
		}
		if got := creditUsed(t, conn, f.companyID); got != 10_000 {
			t.Errorf("credit used = %d, want 10000", got)
		}
	})

	t.Run("exactly the remainder", func(t *testing.T) {
		f := seed(t, conn, 10_000, 7_500, 0, "active")
		if err := reserve(t, conn, svc, f, 2_500); err != nil {
			t.Fatalf("spending exactly the remainder was refused: %v", err)
		}
	})

	t.Run("member limit met exactly", func(t *testing.T) {
		f := seed(t, conn, 100_000, 0, 5_000, "active")
		if err := reserve(t, conn, svc, f, 5_000); err != nil {
			t.Fatalf("an order at exactly the member limit was refused: %v", err)
		}
	})
}

// A zero member limit means "no personal cap", not "cannot buy anything"
func TestZeroMemberLimitMeansUnlimited(t *testing.T) {
	conn := testDB(t)
	svc := newService(conn)
	f := seed(t, conn, 100_000, 0, 0, "active")

	if err := reserve(t, conn, svc, f, 90_000); err != nil {
		t.Fatalf("a member with no per-order cap was blocked: %v", err)
	}
}

// A customer with no company cannot buy on account at all
func TestReserveCreditWithoutMembership(t *testing.T) {
	conn := testDB(t)
	svc := newService(conn)
	f := seed(t, conn, 100_000, 0, 0, "active")

	sqlTx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	if err := svc.ReserveCreditTx(context.Background(), sqlTx, uuid.New(), f.orderID, "ORD-1", 100); !errors.Is(err, ErrNoCompany) { // a customer id that belongs to no company
		t.Fatalf("got %v, want ErrNoCompany", err)
	}
}

// The whole feature is gated, and a disabled server must not draw down credit
func TestReserveCreditDisabled(t *testing.T) {
	conn := testDB(t)
	svc := newService(conn)
	svc.SetEnabled(false)
	f := seed(t, conn, 100_000, 0, 0, "active")

	if err := reserve(t, conn, svc, f, 1_000); !errors.Is(err, ErrDisabled) {
		t.Fatalf("got %v, want ErrDisabled", err)
	}
	if got := creditUsed(t, conn, f.companyID); got != 0 {
		t.Errorf("credit used = %d with B2B disabled, want 0", got)
	}
}

// Two buyers from the same company placing at once must not both fit into credit that only covers one - GetCompanyForUpdate is what serializes them, and this stages the overlap deterministically rather than racing goroutines and hoping
func TestConcurrentReserveCannotExceedTheLimit(t *testing.T) {
	conn := testDB(t)
	svc := newService(conn)
	f := seed(t, conn, 10_000, 0, 0, "active")
	ctx := context.Background()

	tx1, err := conn.Begin() // tx1 draws the whole line and holds it open
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx1.Rollback() }()
	if err := svc.ReserveCreditTx(ctx, tx1, f.customerID, f.orderID, "ORD-A", 10_000); err != nil {
		t.Fatalf("first reserve: %v", err)
	}

	second := make(chan error, 1) // tx2 must block on the company row rather than read the pre-drawdown balance
	go func() {
		tx2, err := conn.Begin()
		if err != nil {
			second <- err
			return
		}
		defer func() { _ = tx2.Rollback() }()
		err = svc.ReserveCreditTx(ctx, tx2, f.customerID, f.orderID, "ORD-B", 10_000)
		if err == nil {
			err = tx2.Commit()
		}
		second <- err
	}()

	select {
	case err := <-second:
		t.Fatalf("the second order resolved (err=%v) while the first still held the company row - it read a stale credit balance", err)
	case <-time.After(300 * time.Millisecond):
	}

	if err := tx1.Commit(); err != nil {
		t.Fatalf("commit first: %v", err)
	}
	if err := <-second; !errors.Is(err, ErrCreditExceeded) {
		t.Fatalf("second order = %v, want ErrCreditExceeded - the company overdrew its line", err)
	}
	if got := creditUsed(t, conn, f.companyID); got != 10_000 {
		t.Errorf("credit used = %d, want 10000", got)
	}
}
