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
	svc, ch := newShippingDB(t)
	ctx := context.Background()
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

func newShippingDB(t *testing.T) (*Service, uuid.UUID) {
	t.Helper()
	conn := testdb.Require(t)
	var ch uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels ORDER BY created_at LIMIT 1 RETURNING id`, "ship-"+uuid.NewString()[:8]).Scan(&ch); err != nil {
		t.Skipf("no channel to clone: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch) })
	svc := NewService(NewRepository(db.New(conn)))
	svc.SetTx(txrunner.New(conn))
	return svc, ch
}

func TestSaveZoneWritesCountriesWithTheZone(t *testing.T) {
	svc, ch := newShippingDB(t)
	ctx := context.Background()
	if _, err := svc.SaveZone(ctx, Zone{ChannelID: ch, Code: "eu", IsActive: true}, []string{"de", "France"}); !errors.Is(err, errCountryInvalid) {
		t.Fatalf("SaveZone(bad country) = %v, want %v", err, errCountryInvalid)
	}
	if zones, _ := svc.Repo().ListZonesByChannel(ctx, ch); len(zones) != 0 {
		t.Fatalf("a refused zone was created: %+v", zones)
	}
	zone, err := svc.SaveZone(ctx, Zone{ChannelID: ch, Code: "eu", IsActive: true}, []string{"de"})
	if err != nil {
		t.Fatal(err)
	}
	m := Method{ZoneID: zone.ID, Code: "std", CalcType: CalcFlat, FlatCents: 500, IsActive: true, CountryMode: "specific"}
	if _, err := svc.SaveMethod(ctx, m, nil, []string{"de "}); err != nil {
		t.Fatal(err)
	}
	quotes, err := svc.Quote(ctx, QuoteInput{ChannelID: ch, CountryCode: "de"})
	if err != nil || len(quotes) != 1 {
		t.Fatalf("Quote(de) = %v, %v - want the specific-country method", quotes, err)
	}

	other, err := svc.SaveZone(ctx, Zone{ChannelID: ch, Code: "us", IsActive: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	other.Code = "eu"
	if _, err := svc.SaveZone(ctx, other, nil); !errors.Is(err, ErrCodeTaken) {
		t.Errorf("renaming onto a taken code = %v, want %v", err, ErrCodeTaken)
	}
	if _, err := svc.SaveZone(ctx, Zone{ID: uuid.New(), Code: "gone"}, nil); !errors.Is(err, ErrZoneNotFound) {
		t.Errorf("updating a missing zone = %v, want %v", err, ErrZoneNotFound)
	}
	if _, err := svc.GetBox(ctx, uuid.New()); !errors.Is(err, ErrBoxNotFound) {
		t.Errorf("GetBox(missing) = %v, want %v", err, ErrBoxNotFound)
	}
}

// A channel created after boot has no carrier rows until SyncCarriers runs again
func TestCarrierSettingsOnAChannelWithNoRow(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	RegisterCarrier(CarrierRegistration{Slug: "dhl", Carrier: rateStub{}})
	svc, ch := newShippingDB(t)
	ctx := context.Background()
	cfg, err := svc.SetCarrierEnabled(ctx, ch, "dhl", true)
	if err != nil || !cfg.IsEnabled {
		t.Fatalf("SetCarrierEnabled = %+v, %v - want it enabled", cfg, err)
	}
	if err := svc.SetCarrierCountries(ctx, ch, "dhl", "specific", []string{"de"}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.CarrierForChannel(ctx, ch, "dhl")
	if err != nil || len(got.Config.AllowedCountries) != 1 || got.Config.AllowedCountries[0] != "DE" {
		t.Fatalf("carrier countries = %+v, %v - want [DE]", got.Config.AllowedCountries, err)
	}
	if err := svc.SetCarrierCountries(ctx, ch, "dhl", "specific", []string{"xx1"}); !errors.Is(err, errCountryInvalid) {
		t.Errorf("SetCarrierCountries(bad) = %v, want %v", err, errCountryInvalid)
	}
	if got, _ = svc.CarrierForChannel(ctx, ch, "dhl"); len(got.Config.AllowedCountries) != 1 {
		t.Errorf("a refused write changed the countries: %v", got.Config.AllowedCountries)
	}
}
