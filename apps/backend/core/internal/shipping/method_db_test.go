package shipping

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// A method whose type rate names a product type that doesn't exist is refused whole - it isn't left live at checkout without its rates or countries
func TestSaveMethodIsAllOrNothing(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	var ch uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels ORDER BY created_at LIMIT 1 RETURNING id`, "ship-"+uuid.NewString()[:8]).Scan(&ch); err != nil {
		t.Skipf("no channel to clone: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch) })
	svc := NewService(NewRepository(db.New(conn)))
	svc.SetTx(txrunner.New(conn))
	zone, err := svc.Repo().CreateZone(ctx, Zone{ChannelID: ch, Code: "z", Name: "Z", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}

	m := Method{ZoneID: zone.ID, Code: "std", Name: "Standard", CalcType: CalcFlat, FlatCents: 500, IsActive: true, CountryMode: "specific"}
	ghost := []TypeRate{{ProductTypeID: uuid.New(), PriceCents: 3000}}
	if _, err := svc.SaveMethod(ctx, m, ghost, []string{"US"}); !errors.Is(err, errTypeRateUnknown) {
		t.Fatalf("SaveMethod = %v, want %v", err, errTypeRateUnknown)
	}
	if methods, err := svc.Repo().ListMethodsByZone(ctx, zone.ID); err != nil || len(methods) != 0 {
		t.Fatalf("methods after a refused create = %v, %v - want none", methods, err)
	}

	saved, err := svc.SaveMethod(ctx, m, nil, []string{"US"})
	if err != nil {
		t.Fatal(err)
	}
	saved.FlatCents = 900
	if _, err := svc.SaveMethod(ctx, saved, ghost, []string{"DE"}); !errors.Is(err, errTypeRateUnknown) {
		t.Fatalf("SaveMethod update = %v, want %v", err, errTypeRateUnknown)
	}
	got, err := svc.Repo().GetMethod(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	countries, err := svc.Repo().ListCountriesForMethod(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.FlatCents != 500 || len(countries) != 1 || countries[0] != "US" {
		t.Errorf("after a refused update: flat %d, countries %v - want the 500 and US it had", got.FlatCents, countries)
	}
}
