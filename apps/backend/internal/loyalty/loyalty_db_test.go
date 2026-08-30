package loyalty

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/skeletg/skelet/internal/config"
	dbsqlc "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/storecredit"
	"github.com/skeletg/skelet/internal/testdb"
	"github.com/skeletg/skelet/internal/tx"
)

// Redeem converts points into store credit, which is spendable money - so the guarantees are that
// points can only be spent once, the debit and the grant are one atomic step, and neither side can
// be driven negative
//
// DB-backed because all three rest on SELECT ... FOR UPDATE and transaction atomicity

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Require(t)
}

// The channel the program config is scoped to, plus the two services wired the way main.go does
func newFixture(t *testing.T, conn *sql.DB) (*Service, *storecredit.Service, uuid.UUID, uuid.UUID) {
	t.Helper()
	queries, txMgr := dbsqlc.New(conn), tx.New(conn)
	cfg := config.NewService(config.NewRepository(queries, txMgr))
	credit := storecredit.NewService(queries, txMgr)

	var channelID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel to scope the program to: %v", err)
	}
	return NewService(queries, txMgr, cfg, credit), credit, seedCustomer(t, conn), channelID
}

func seedCustomer(t *testing.T, conn *sql.DB) uuid.UUID {
	t.Helper()
	var groupID, channelID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM customer_groups LIMIT 1`).Scan(&groupID); err != nil {
		t.Skipf("no customer group to seed against: %v", err)
	}
	if err := conn.QueryRow(`SELECT id FROM channels LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel to seed against: %v", err)
	}
	var id uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO customers (email, password_hash, customer_group_id, channel_id)
		 VALUES ($1, 'x', $2, $3) RETURNING id`,
		"loyalty-"+uuid.NewString()+"@example.invalid", groupID, channelID).Scan(&id); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM store_credit_entries WHERE customer_id = $1`, id)
		_, _ = conn.Exec(`DELETE FROM loyalty_entries WHERE customer_id = $1`, id)
		_, _ = conn.Exec(`DELETE FROM customers WHERE id = $1`, id)
	})
	return id
}

// Writes the channel-scoped program config the service reads back
func setProgram(t *testing.T, conn *sql.DB, channelID uuid.UUID, enabled bool, centsPerPoint int) {
	t.Helper()
	for path, value := range map[string]string{
		"sales/loyalty/enabled":                boolStr(enabled),
		"sales/loyalty/redeem_cents_per_point": itoa(centsPerPoint),
	} {
		if _, err := conn.Exec(
			`INSERT INTO config_values (path, scope, channel_id, value) VALUES ($1, 'channel', $2, $3)`,
			path, channelID, value); err != nil {
			t.Fatalf("set %s: %v", path, err)
		}
		p, c := path, channelID
		t.Cleanup(func() {
			_, _ = conn.Exec(`DELETE FROM config_values WHERE path = $1 AND channel_id = $2`, p, c)
		})
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf []byte
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		return "-" + string(buf)
	}
	return string(buf)
}

func award(t *testing.T, svc *Service, customerID uuid.UUID, points int64) {
	t.Helper()
	if _, err := svc.Adjust(context.Background(), customerID, points, "test award"); err != nil {
		t.Fatalf("award points: %v", err)
	}
}

// The whole point of the feature: points become credit at the configured rate, and BOTH sides move
func TestRedeemDebitsPointsAndGrantsCredit(t *testing.T) {
	conn := testDB(t)
	svc, credit, customerID, channelID := newFixture(t, conn)
	setProgram(t, conn, channelID, true, 2) // 2 cents per point
	award(t, svc, customerID, 500)

	cents, err := svc.Redeem(context.Background(), customerID, 200, channelID, "USD")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if cents != 400 {
		t.Errorf("granted %d cents for 200 points at 2c/point, want 400", cents)
	}
	if got, _ := svc.Balance(context.Background(), customerID); got != 300 {
		t.Errorf("points balance = %d, want 300", got)
	}
	if got, _ := credit.Balance(context.Background(), customerID, "USD"); got != 400 {
		t.Errorf("credit balance = %d, want 400", got)
	}
}

// Spending points you don't have is the direct route to free money
func TestRedeemRefusesMoreThanBalance(t *testing.T) {
	conn := testDB(t)
	svc, credit, customerID, channelID := newFixture(t, conn)
	setProgram(t, conn, channelID, true, 1)
	award(t, svc, customerID, 100)

	if _, err := svc.Redeem(context.Background(), customerID, 101, channelID, "USD"); !errors.Is(err, ErrInsufficientPoints) {
		t.Fatalf("got %v, want ErrInsufficientPoints", err)
	}
	if got, _ := svc.Balance(context.Background(), customerID); got != 100 {
		t.Errorf("points balance = %d after a refused redemption, want 100", got)
	}
	if got, _ := credit.Balance(context.Background(), customerID, "USD"); got != 0 {
		t.Errorf("credit balance = %d, want 0 - a refused redemption still minted credit", got)
	}
}

func TestRedeemGuards(t *testing.T) {
	conn := testDB(t)
	svc, credit, customerID, channelID := newFixture(t, conn)
	ctx := context.Background()

	t.Run("non-positive points refused", func(t *testing.T) {
		setProgram(t, conn, channelID, true, 1)
		for _, bad := range []int64{0, -50} {
			if _, err := svc.Redeem(ctx, customerID, bad, channelID, "USD"); !errors.Is(err, ErrInvalidPoints) {
				t.Errorf("Redeem(%d) = %v, want ErrInvalidPoints", bad, err)
			}
		}
	})

	t.Run("disabled program refused", func(t *testing.T) {
		id := seedCustomer(t, conn)
		setProgram(t, conn, channelID, false, 1)
		award(t, svc, id, 100)
		if _, err := svc.Redeem(ctx, id, 50, channelID, "USD"); !errors.Is(err, ErrDisabled) {
			t.Fatalf("got %v, want ErrDisabled", err)
		}
		if got, _ := credit.Balance(ctx, id, "USD"); got != 0 {
			t.Errorf("a disabled program still granted %d cents", got)
		}
	})

	t.Run("zero rate refused", func(t *testing.T) {
		id := seedCustomer(t, conn)
		// a misconfigured rate would otherwise burn points for nothing
		setProgram(t, conn, channelID, true, 0)
		award(t, svc, id, 100)
		if _, err := svc.Redeem(ctx, id, 50, channelID, "USD"); !errors.Is(err, ErrInvalidPoints) {
			t.Fatalf("got %v, want ErrInvalidPoints", err)
		}
		if got, _ := svc.Balance(ctx, id); got != 100 {
			t.Errorf("points balance = %d, want 100 - points were burned at a zero rate", got)
		}
	})
}

// Points must be spendable exactly once. Redeem owns its own transaction, so this can't be staged
// the way the store-credit lock test is - instead many redemptions of the WHOLE balance run at once
// and the ledger is checked afterwards
//
// Without the customer lock each one reads the same balance before any of them writes, every check
// passes, and the balance goes negative - free store credit, which is the shape the prior laundering
// bug had
func TestConcurrentRedeemSpendsPointsOnce(t *testing.T) {
	conn := testDB(t)
	svc, credit, _, channelID := newFixture(t, conn)
	setProgram(t, conn, channelID, true, 1)
	ctx := context.Background()

	// One round only sometimes gets the reads to overlap, so several run with a fresh customer each -
	// a regression test that catches an overdraw one time in three would let the bug back in
	const (
		rounds = 12
		racers = 8
	)
	for round := range rounds {
		customerID := seedCustomer(t, conn)
		award(t, svc, customerID, 100)

		var wg sync.WaitGroup
		start := make(chan struct{})
		granted := make([]int64, racers)
		wg.Add(racers)
		for i := range racers {
			go func() {
				defer wg.Done()
				<-start // release together so the balance reads overlap
				if cents, err := svc.Redeem(ctx, customerID, 100, channelID, "USD"); err == nil {
					granted[i] = cents
				}
			}()
		}
		close(start)
		wg.Wait()

		var succeeded int
		for _, g := range granted {
			if g > 0 {
				succeeded++
			}
		}
		points, _ := svc.Balance(ctx, customerID)
		cents, _ := credit.Balance(ctx, customerID, "USD")

		if succeeded != 1 || points != 0 || cents != 100 {
			t.Fatalf("round %d: %d of %d redemptions succeeded against 100 points, leaving %d points and %d cents - want 1 / 0 / 100",
				round, succeeded, racers, points, cents)
		}
	}
}

// Redeeming everything twice must take the balance to zero and stop, not to minus one round
func TestRedeemTwiceDrainsThenRefuses(t *testing.T) {
	conn := testDB(t)
	svc, credit, customerID, channelID := newFixture(t, conn)
	setProgram(t, conn, channelID, true, 3)
	award(t, svc, customerID, 100)
	ctx := context.Background()

	if _, err := svc.Redeem(ctx, customerID, 100, channelID, "USD"); err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	if _, err := svc.Redeem(ctx, customerID, 100, channelID, "USD"); !errors.Is(err, ErrInsufficientPoints) {
		t.Fatalf("second redeem = %v, want ErrInsufficientPoints", err)
	}
	if got, _ := svc.Balance(ctx, customerID); got != 0 {
		t.Errorf("points balance = %d, want 0", got)
	}
	if got, _ := credit.Balance(ctx, customerID, "USD"); got != 300 {
		t.Errorf("credit balance = %d, want 300 - the second attempt minted credit anyway", got)
	}
}

// Admin corrections: a deduction may not take the balance negative, and zero is not a correction
func TestAdjustGuards(t *testing.T) {
	conn := testDB(t)
	svc, _, customerID, _ := newFixture(t, conn)
	ctx := context.Background()

	if _, err := svc.Adjust(ctx, customerID, 0, "noop"); !errors.Is(err, ErrInvalidPoints) {
		t.Errorf("Adjust(0) = %v, want ErrInvalidPoints", err)
	}
	award(t, svc, customerID, 50)
	if _, err := svc.Adjust(ctx, customerID, -80, "correction"); !errors.Is(err, ErrInsufficientPoints) {
		t.Errorf("over-deduction = %v, want ErrInsufficientPoints", err)
	}
	if got, _ := svc.Balance(ctx, customerID); got != 50 {
		t.Errorf("balance = %d after a refused deduction, want 50", got)
	}
	if _, err := svc.Adjust(ctx, customerID, -50, "correction"); err != nil {
		t.Errorf("deduction to exactly zero: %v", err)
	}
	if got, _ := svc.Balance(ctx, customerID); got != 0 {
		t.Errorf("balance = %d, want 0", got)
	}
}
