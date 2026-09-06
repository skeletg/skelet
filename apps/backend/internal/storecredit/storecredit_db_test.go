package storecredit

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	dbsqlc "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/testdb"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
)

// Store credit is spendable money, so the guarantees here are the ledger's: a balance is the SUM of
// entries, nothing may drive it below zero, and concurrent spenders must not both pass the same
// balance check
//
// DB-backed on purpose - the overdraw guard is SELECT ... FOR UPDATE on the customer row plus a sum
// inside the same transaction, and a fake would only prove the fake serializes

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Require(t)
}

func newService(conn *sql.DB) *Service {
	return NewService(dbsqlc.New(conn), tx.New(conn))
}

// A customer to hang a ledger off - reuses whatever group and channel exist rather than seeding a
// whole customer graph
func seedCustomer(t *testing.T, conn *sql.DB) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var groupID, channelID uuid.UUID
	if err := conn.QueryRowContext(ctx, `SELECT id FROM customer_groups LIMIT 1`).Scan(&groupID); err != nil {
		t.Skipf("no customer group to seed against: %v", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT id FROM channels LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel to seed against: %v", err)
	}
	var id uuid.UUID
	email := "credit-" + uuid.NewString() + "@example.invalid"
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO customers (email, password_hash, customer_group_id, channel_id)
		 VALUES ($1, 'x', $2, $3) RETURNING id`, email, groupID, channelID).Scan(&id); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM store_credit_entries WHERE customer_id = $1`, id)
		_, _ = conn.Exec(`DELETE FROM customers WHERE id = $1`, id)
	})
	return id
}

// store_credit_entries.order_id FKs to orders, so a redemption needs a real one - the ledger row is
// what links a spend to the order it paid for, and that link is the audit trail
func seedOrder(t *testing.T, conn *sql.DB, currency string) uuid.UUID {
	t.Helper()
	var channelID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel to seed an order against: %v", err)
	}
	var id uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO orders (channel_id, currency_code) VALUES ($1, $2) RETURNING id`,
		channelID, currency).Scan(&id); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM store_credit_entries WHERE order_id = $1`, id)
		_, _ = conn.Exec(`DELETE FROM orders WHERE id = $1`, id)
	})
	return id
}

func grant(t *testing.T, conn *sql.DB, svc *Service, customerID uuid.UUID, cents int64, currency string) {
	t.Helper()
	sqlTx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	if err := svc.GrantTx(context.Background(), sqlTx, customerID, cents, currency, "test grant"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := sqlTx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func balance(t *testing.T, svc *Service, customerID uuid.UUID, currency string) int64 {
	t.Helper()
	b, err := svc.Balance(context.Background(), customerID, currency)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	return b
}

// The headline invariant, exercised deterministically rather than by racing goroutines and hoping
// they overlap: tx1 takes the customer lock and spends the balance WITHOUT committing, then tx2
// tries the same spend
//
// With the lock, tx2 blocks until tx1 commits and then reads a balance of 0, so it applies nothing.
// Without it, tx2 reads the pre-commit balance of 1000 and applies it again - two orders paid from
// one balance, which is the overdraw the lock exists to prevent
func TestConcurrentRedeemCannotOverdraw(t *testing.T) {
	conn := testDB(t)
	svc := newService(conn)
	customerID := seedCustomer(t, conn)
	grant(t, conn, svc, customerID, 1000, "USD")
	orderA, orderB := seedOrder(t, conn, "USD"), seedOrder(t, conn, "USD")
	ctx := context.Background()

	tx1, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx1.Rollback() }()
	appliedA, err := svc.RedeemUpToTx(ctx, tx1, customerID, 1000, "USD", orderA)
	if err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	if appliedA != 1000 {
		t.Fatalf("first redeem applied %d, want 1000", appliedA)
	}

	// tx2 starts while tx1 still holds the lock, so it must wait rather than read a stale balance
	var appliedB int64
	var redeemErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		tx2, err := conn.Begin()
		if err != nil {
			redeemErr = err
			return
		}
		defer func() { _ = tx2.Rollback() }()
		appliedB, redeemErr = svc.RedeemUpToTx(ctx, tx2, customerID, 1000, "USD", orderB)
		if redeemErr == nil {
			redeemErr = tx2.Commit()
		}
	}()

	// let tx2 reach the lock and block on it before tx1 releases
	select {
	case <-done:
		t.Fatal("the second redemption completed while the first still held the customer lock - it read an uncommitted balance")
	case <-time.After(300 * time.Millisecond):
	}

	if err := tx1.Commit(); err != nil {
		t.Fatalf("commit first: %v", err)
	}
	<-done
	if redeemErr != nil {
		t.Fatalf("second redeem: %v", redeemErr)
	}

	if appliedB != 0 {
		t.Errorf("second redemption applied %d cents against an already-spent balance, want 0", appliedB)
	}
	if got := balance(t, svc, customerID, "USD"); got != 0 {
		t.Errorf("balance = %d, want 0 - the ledger went negative", got)
	}
}

// Spending is capped at whichever is smaller, so a big order takes only what exists and a small one
// leaves the rest
func TestRedeemUpToAppliesTheSmallerOfBalanceAndDue(t *testing.T) {
	conn := testDB(t)
	svc := newService(conn)

	t.Run("balance smaller than due", func(t *testing.T) {
		id := seedCustomer(t, conn)
		grant(t, conn, svc, id, 300, "USD")
		if got := redeem(t, conn, svc, id, 1000, "USD"); got != 300 {
			t.Errorf("applied %d, want 300", got)
		}
		if got := balance(t, svc, id, "USD"); got != 0 {
			t.Errorf("balance = %d, want 0", got)
		}
	})

	t.Run("due smaller than balance", func(t *testing.T) {
		id := seedCustomer(t, conn)
		grant(t, conn, svc, id, 1000, "USD")
		if got := redeem(t, conn, svc, id, 250, "USD"); got != 250 {
			t.Errorf("applied %d, want 250", got)
		}
		if got := balance(t, svc, id, "USD"); got != 750 {
			t.Errorf("balance = %d, want 750", got)
		}
	})

	t.Run("no balance applies nothing", func(t *testing.T) {
		id := seedCustomer(t, conn)
		if got := redeem(t, conn, svc, id, 500, "USD"); got != 0 {
			t.Errorf("applied %d against an empty balance, want 0", got)
		}
	})

	t.Run("nothing due applies nothing", func(t *testing.T) {
		id := seedCustomer(t, conn)
		grant(t, conn, svc, id, 500, "USD")
		if got := redeem(t, conn, svc, id, 0, "USD"); got != 0 {
			t.Errorf("applied %d with nothing due, want 0", got)
		}
		if got := balance(t, svc, id, "USD"); got != 500 {
			t.Errorf("balance = %d, want 500 - an untouched balance was spent", got)
		}
	})
}

func redeem(t *testing.T, conn *sql.DB, svc *Service, customerID uuid.UUID, due int64, currency string) int64 {
	t.Helper()
	sqlTx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	got, err := svc.RedeemUpToTx(context.Background(), sqlTx, customerID, due, currency, seedOrder(t, conn, currency))
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if err := sqlTx.Commit(); err != nil {
		t.Fatal(err)
	}
	return got
}

// Credit is per currency - EUR credit paying a USD order would be an invented exchange rate
func TestBalanceIsCurrencyScoped(t *testing.T) {
	conn := testDB(t)
	svc := newService(conn)
	id := seedCustomer(t, conn)

	grant(t, conn, svc, id, 500, "USD")
	grant(t, conn, svc, id, 900, "EUR")

	if got := balance(t, svc, id, "USD"); got != 500 {
		t.Errorf("USD balance = %d, want 500", got)
	}
	if got := balance(t, svc, id, "EUR"); got != 900 {
		t.Errorf("EUR balance = %d, want 900", got)
	}
	if got := redeem(t, conn, svc, id, 900, "USD"); got != 500 {
		t.Errorf("a USD order spent %d - it must not reach the EUR balance", got)
	}
	if got := balance(t, svc, id, "EUR"); got != 900 {
		t.Errorf("EUR balance = %d after a USD order, want 900", got)
	}
}

// An admin deduction may not push a customer into debt they can't see
func TestAdjustGuards(t *testing.T) {
	conn := testDB(t)
	svc := newService(conn)
	ctx := context.Background()

	t.Run("zero is rejected", func(t *testing.T) {
		id := seedCustomer(t, conn)
		if _, err := svc.Adjust(ctx, id, 0, "USD", "noop", nil); !errors.Is(err, ErrInvalidAmount) {
			t.Fatalf("got %v, want ErrInvalidAmount", err)
		}
	})

	t.Run("deduction beyond balance is refused", func(t *testing.T) {
		id := seedCustomer(t, conn)
		grant(t, conn, svc, id, 100, "USD")
		if _, err := svc.Adjust(ctx, id, -500, "USD", "correction", nil); !errors.Is(err, ErrInsufficientBalance) {
			t.Fatalf("got %v, want ErrInsufficientBalance", err)
		}
		if got := balance(t, svc, id, "USD"); got != 100 {
			t.Errorf("balance = %d after a refused deduction, want 100", got)
		}
	})

	t.Run("deduction to exactly zero is allowed", func(t *testing.T) {
		id := seedCustomer(t, conn)
		grant(t, conn, svc, id, 100, "USD")
		if _, err := svc.Adjust(ctx, id, -100, "USD", "correction", nil); err != nil {
			t.Fatalf("adjust: %v", err)
		}
		if got := balance(t, svc, id, "USD"); got != 0 {
			t.Errorf("balance = %d, want 0", got)
		}
	})
}

// Grant is the seam sibling programs mint credit through, so a non-positive amount must be refused
// rather than quietly writing a debit under a "grant" reason
func TestGrantRejectsNonPositive(t *testing.T) {
	conn := testDB(t)
	svc := newService(conn)
	id := seedCustomer(t, conn)

	sqlTx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlTx.Rollback() }()

	for _, bad := range []int64{0, -100} {
		if err := svc.GrantTx(context.Background(), sqlTx, id, bad, "USD", "bad grant"); !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("GrantTx(%d) = %v, want ErrInvalidAmount", bad, err)
		}
	}
}

// Clawback takes back what it can and reports the shortfall rather than creating debt - the customer
// may already have spent the credit that is being reversed
func TestClawbackClampsAtBalance(t *testing.T) {
	conn := testDB(t)
	svc := newService(conn)

	t.Run("takes the full amount when it is there", func(t *testing.T) {
		id := seedCustomer(t, conn)
		grant(t, conn, svc, id, 500, "USD")
		if got := clawback(t, conn, svc, id, 200, "USD"); got != 200 {
			t.Errorf("clawed back %d, want 200", got)
		}
		if got := balance(t, svc, id, "USD"); got != 300 {
			t.Errorf("balance = %d, want 300", got)
		}
	})

	t.Run("takes only what remains when already spent", func(t *testing.T) {
		id := seedCustomer(t, conn)
		grant(t, conn, svc, id, 100, "USD")
		if got := clawback(t, conn, svc, id, 500, "USD"); got != 100 {
			t.Errorf("clawed back %d, want 100 - the rest was already spent", got)
		}
		if got := balance(t, svc, id, "USD"); got != 0 {
			t.Errorf("balance = %d, want 0 - clawback drove the ledger negative", got)
		}
	})

	t.Run("non-positive is a no-op", func(t *testing.T) {
		id := seedCustomer(t, conn)
		grant(t, conn, svc, id, 100, "USD")
		if got := clawback(t, conn, svc, id, 0, "USD"); got != 0 {
			t.Errorf("clawed back %d, want 0", got)
		}
		if got := balance(t, svc, id, "USD"); got != 100 {
			t.Errorf("balance = %d, want 100", got)
		}
	})
}

func clawback(t *testing.T, conn *sql.DB, svc *Service, customerID uuid.UUID, cents int64, currency string) int64 {
	t.Helper()
	sqlTx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	got, err := svc.ClawbackTx(context.Background(), sqlTx, customerID, cents, currency, "reversal")
	if err != nil {
		t.Fatalf("clawback: %v", err)
	}
	if err := sqlTx.Commit(); err != nil {
		t.Fatal(err)
	}
	return got
}

// A redemption that rolls back must leave the balance untouched - the tender is applied inside the
// caller's placement transaction, so a failed placement must not silently spend credit
func TestRedeemRollsBackWithTheCallersTransaction(t *testing.T) {
	conn := testDB(t)
	svc := newService(conn)
	id := seedCustomer(t, conn)
	grant(t, conn, svc, id, 400, "USD")

	sqlTx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	orderID := seedOrder(t, conn, "USD")
	if _, err := svc.RedeemUpToTx(context.Background(), sqlTx, id, 400, "USD", orderID); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if err := sqlTx.Rollback(); err != nil {
		t.Fatal(err)
	}

	if got := balance(t, svc, id, "USD"); got != 400 {
		t.Fatalf("balance = %d after a rolled-back placement, want 400 - credit was spent on an order that never existed", got)
	}
}
