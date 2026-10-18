package subscription

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func newLifecycleDB(t *testing.T) (*Service, *sql.DB, func(status string, qty float64, nextBilling time.Time) uuid.UUID) {
	t.Helper()
	conn := testdb.Require(t)
	svc := NewService(NewRepository(db.New(conn)), txrunner.New(conn))
	insert := func(status string, qty float64, next time.Time) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO subscriptions (product_id, status, interval_unit, price, currency_code, quantity, current_period_start, current_period_end, next_billing_at)
			VALUES ($1, $2, 'month', 1250, 'USD', $3, $4::timestamptz - interval '1 month', $4, $4) RETURNING id`, uuid.New(), status, qty, next).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM subscriptions WHERE id = $1`, id) })
		return id
	}
	return svc, conn, insert
}

// A subscription to three bags is billed for three, every period
func TestARenewalBillsTheSubscribedQuantity(t *testing.T) {
	svc, _, insert := newLifecycleDB(t)
	ctx := context.Background()
	now := time.Now()
	id := insert(StatusActive, 3, now.Add(-time.Minute))
	if err := svc.billOne(ctx, id, now); err != nil {
		t.Fatal(err)
	}
	invs, err := svc.Invoices(ctx, id)
	if err != nil || len(invs) != 1 || invs[0].Amount != 3750 {
		t.Fatalf("invoices = %+v, %v - want one of 3750", invs, err)
	}
}

func TestCatchUpIsCappedPerRun(t *testing.T) {
	svc, _, insert := newLifecycleDB(t)
	ctx := context.Background()
	now := time.Now()
	id := insert(StatusActive, 1, now.AddDate(-40, 0, 0))
	if err := svc.billOne(ctx, id, now); err != nil {
		t.Fatal(err)
	}
	if invs, _ := svc.Invoices(ctx, id); len(invs) != maxCatchUpPeriods {
		t.Fatalf("first run minted %d invoices, want the cap of %d", len(invs), maxCatchUpPeriods)
	}
	if err := svc.billOne(ctx, id, now); err != nil {
		t.Fatal(err)
	}
	if invs, _ := svc.Invoices(ctx, id); len(invs) != 40*12+1 {
		t.Fatalf("after the second run %d invoices, want all %d periods", len(invs), 40*12+1)
	}
}

func TestOnlyAPausedSubscriptionResumes(t *testing.T) {
	svc, _, insert := newLifecycleDB(t)
	ctx := context.Background()
	for _, status := range []string{StatusCanceled, StatusExpired} {
		id := insert(status, 1, time.Now().AddDate(0, 1, 0))
		if _, err := svc.Pause(ctx, id); !errors.Is(err, ErrNotActive) {
			t.Errorf("Pause(%s) = %v, want ErrNotActive", status, err)
		}
		if _, err := svc.Resume(ctx, id); !errors.Is(err, ErrNotActive) {
			t.Errorf("Resume(%s) = %v, want ErrNotActive", status, err)
		}
		if got, _ := svc.Get(ctx, id); got.Status != status {
			t.Errorf("a %s subscription became %s", status, got.Status)
		}
	}
	if _, err := svc.Resume(ctx, insert(StatusActive, 1, time.Now().AddDate(0, 1, 0))); !errors.Is(err, ErrNotActive) {
		t.Errorf("Resume(active) = %v, want ErrNotActive", err)
	}
}

// Three months paused is never three invoices on resume
func TestResumingDoesNotBillThePausedMonths(t *testing.T) {
	svc, _, insert := newLifecycleDB(t)
	ctx := context.Background()
	id := insert(StatusActive, 1, time.Now().AddDate(0, 1, 0))
	if _, err := svc.Pause(ctx, id); err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return time.Now().AddDate(0, 4, 0) }
	if _, err := svc.Resume(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := svc.billOne(ctx, id, svc.now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if invs, _ := svc.Invoices(ctx, id); len(invs) != 1 {
		t.Fatalf("resume billed %d periods, want one fresh period", len(invs))
	}
}

func TestAnotherCustomersSubscriptionReadsAsMissing(t *testing.T) {
	svc, _, insert := newLifecycleDB(t)
	if _, err := svc.SetPaymentMethodForCustomer(context.Background(), uuid.New(), insert(StatusActive, 1, time.Now()), nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
