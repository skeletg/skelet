package front

import (
	"errors"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/country"
	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// An inactive country can't be ordered to, so the storefront never offers one - even when the caller asks for every country
func TestTheStorefrontOffersOnlyActiveCountriesAndRegions(t *testing.T) {
	conn := testdb.Require(t)
	var off, on string
	for _, c := range []struct {
		code   *string
		active bool
	}{{&off, false}, {&on, true}} {
		for _, code := range []string{"QJ", "QK", "QL", "QM", "QN", "QP"} {
			if _, err := conn.Exec(`INSERT INTO countries (code, name, is_active) VALUES ($1, 'vis', $2)`, code, c.active); err == nil {
				*c.code = code
				t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM countries WHERE code = $1`, code) })
				break
			}
		}
		if *c.code == "" {
			t.Skip("no free test country code")
		}
	}
	if _, err := conn.Exec(`INSERT INTO regions (country_code, code, name, is_active) VALUES ($1, 'AA', 'live', TRUE), ($1, 'BB', 'retired', FALSE)`, on); err != nil {
		t.Fatal(err)
	}
	q := db.New(conn)
	r := &queryResolver{&Resolver{CountrySvc: country.NewService(country.NewRepository(q)), CurrencySvc: currency.NewService(currency.NewRepository(q))}}
	ctx := t.Context()

	all := false
	list, err := r.Countries(ctx, &all)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		if c.Code == off {
			t.Errorf("countries(activeOnly:false) offered the inactive %s", off)
		}
	}
	if _, err := r.Country(ctx, off); !errors.Is(err, country.ErrCountryNotFound) {
		t.Errorf("country(%s) = %v, want not found", off, err)
	}
	regions, err := r.Regions(ctx, on)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 1 || regions[0].Code != "AA" {
		t.Errorf("regions = %+v, want only the active one", regions)
	}
}
