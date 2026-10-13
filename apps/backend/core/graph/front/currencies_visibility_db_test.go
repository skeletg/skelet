package front

import (
	"errors"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestTheStorefrontOffersOnlyActiveCurrencies(t *testing.T) {
	conn := testdb.Require(t)
	var off string
	for _, code := range []string{"QVA", "QVB", "QVC", "QVD"} {
		if _, err := conn.Exec(`INSERT INTO currencies (code, name, symbol, decimals, symbol_position, is_active) VALUES ($1, 'vis', 'q', 2, 'prefix', FALSE)`, code); err == nil {
			off = code
			t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM currencies WHERE code = $1`, code) })
			break
		}
	}
	if off == "" {
		t.Skip("no free test currency code")
	}
	r := &queryResolver{&Resolver{CurrencySvc: currency.NewService(currency.NewRepository(db.New(conn)))}}
	ctx := t.Context()

	all := false
	list, err := r.Currencies(ctx, &all)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		if c.Code == off {
			t.Errorf("currencies(activeOnly:false) offered the inactive %s", off)
		}
	}
	if _, err := r.Currency(ctx, off); !errors.Is(err, currency.ErrCurrencyNotFound) {
		t.Errorf("currency(%s) = %v, want not found", off, err)
	}
}
