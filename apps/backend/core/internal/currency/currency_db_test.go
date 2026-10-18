package currency

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func dbService(t *testing.T) (*sql.DB, *Service) {
	t.Helper()
	conn := testdb.Require(t)
	return conn, NewService(NewRepository(db.New(conn)))
}

func seedCurrency(t *testing.T, conn *sql.DB, svc *Service) string {
	t.Helper()
	for _, code := range []string{"QXA", "QXB", "QXC", "QXD", "QXE", "QXF", "QXG", "QXH"} {
		if _, err := svc.Create(context.Background(), CreateCurrencyInput{Code: code, Name: "Test " + code, Symbol: "q", Decimals: 2, IsActive: true}); err == nil {
			t.Cleanup(func() {
				_, _ = conn.Exec(`DELETE FROM orders WHERE currency_code = $1`, code)
				_, _ = conn.Exec(`DELETE FROM gift_cards WHERE currency_code = $1`, code)
				_, _ = conn.Exec(`DELETE FROM currencies WHERE code = $1`, code)
			})
			return code
		}
	}
	t.Skip("no free test currency code")
	return ""
}

func firstChannel(t *testing.T, conn *sql.DB) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&id); err != nil {
		t.Skipf("no channel: %v", err)
	}
	return id
}

func borrowChannel(t *testing.T, conn *sql.DB, code string) uuid.UUID {
	t.Helper()
	id := firstChannel(t, conn)
	var prev string
	if err := conn.QueryRow(`SELECT currency_code FROM channels WHERE id = $1`, id).Scan(&prev); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`UPDATE channels SET currency_code = $1 WHERE id = $2`, code, id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`UPDATE channels SET currency_code = $1 WHERE id = $2`, prev, id) })
	return id
}

func decimals(v int16) *int16 { return &v }

func TestDecimalsChangeOnlyWhileNothingIsDenominatedInIt(t *testing.T) {
	conn, svc := dbService(t)
	ctx := context.Background()
	code := seedCurrency(t, conn, svc)

	if _, err := svc.Update(ctx, UpdateCurrencyInput{Code: code, Decimals: decimals(3)}); err != nil {
		t.Fatalf("unreferenced currency: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO gift_cards (code, currency_code, initial_balance, balance) VALUES ($1, $2, 500, 500)`, "T-"+uuid.NewString()[:8], code); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, UpdateCurrencyInput{Code: code, Decimals: decimals(2)}); !errors.Is(err, ErrDecimalsLocked) {
		t.Fatalf("with a gift card balance: %v, want ErrDecimalsLocked", err)
	}
	name := "Renamed"
	if _, err := svc.Update(ctx, UpdateCurrencyInput{Code: code, Decimals: decimals(3), Name: &name}); err != nil {
		t.Fatalf("an unchanged decimals value must pass: %v", err)
	}
}

// Orders are snapshots, kept out of the delete guard, but their amounts are still minor units
func TestAnOrderInTheCurrencyLocksItsDecimals(t *testing.T) {
	conn, svc := dbService(t)
	code := seedCurrency(t, conn, svc)
	if _, err := conn.Exec(`INSERT INTO orders (currency_code, channel_id) VALUES ($1, $2)`, code, firstChannel(t, conn)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(context.Background(), UpdateCurrencyInput{Code: code, Decimals: decimals(0)}); !errors.Is(err, ErrDecimalsLocked) {
		t.Fatalf("with an order: %v, want ErrDecimalsLocked", err)
	}
}

func TestAGiftCardBalanceBlocksDelete(t *testing.T) {
	conn, svc := dbService(t)
	code := seedCurrency(t, conn, svc)
	if _, err := conn.Exec(`INSERT INTO gift_cards (code, currency_code, initial_balance, balance) VALUES ($1, $2, 500, 500)`, "T-"+uuid.NewString()[:8], code); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(context.Background(), code); !errors.Is(err, ErrCurrencyInUse) {
		t.Fatalf("delete under a gift card: %v, want ErrCurrencyInUse", err)
	}
}

// A reference that lands between the count and the delete hits the RESTRICT FK, which must read as in-use
func TestAnFKClashOnDeleteReadsAsInUse(t *testing.T) {
	conn, svc := dbService(t)
	code := seedCurrency(t, conn, svc)
	borrowChannel(t, conn, code)
	if err := NewRepository(db.New(conn)).Delete(context.Background(), code); !errors.Is(err, ErrCurrencyInUse) {
		t.Fatalf("repo delete under a live FK: %v, want ErrCurrencyInUse", err)
	}
}

func TestAChannelsCurrencyCannotBeDeactivated(t *testing.T) {
	conn, svc := dbService(t)
	ctx := context.Background()
	code := seedCurrency(t, conn, svc)
	borrowChannel(t, conn, code)
	off := false
	if _, err := svc.Update(ctx, UpdateCurrencyInput{Code: code, IsActive: &off}); !errors.Is(err, ErrCurrencyUsedByChannel) {
		t.Fatalf("deactivating a channel's currency: %v, want ErrCurrencyUsedByChannel", err)
	}
	if c, _ := svc.GetByCode(ctx, code); !c.IsActive {
		t.Fatal("the currency was deactivated anyway")
	}
}

func TestSetRateOnAnUnknownCode(t *testing.T) {
	_, svc := dbService(t)
	if err := svc.SetRate(context.Background(), "QZZ", 1.5); !errors.Is(err, ErrCurrencyNotFound) {
		t.Fatalf("SetRate(QZZ) = %v, want ErrCurrencyNotFound", err)
	}
}
