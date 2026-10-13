package country

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/configstore"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// A throwaway two-letter code the seeded catalogue doesn't use
func seedCountry(t *testing.T, conn *sql.DB, svc *Service) string {
	t.Helper()
	for _, code := range []string{"QA", "QB", "QC", "QD", "QE", "QF", "QG", "QH"} {
		if _, err := svc.Create(context.Background(), CreateCountryInput{Code: code, Name: "Test " + code, IsActive: true}); err == nil {
			t.Cleanup(func() {
				_, _ = conn.Exec(`DELETE FROM shipping_zone_countries WHERE country_code = $1`, code)
				_, _ = conn.Exec(`DELETE FROM countries WHERE code = $1`, code)
			})
			return code
		}
	}
	t.Skip("no free test country code")
	return ""
}

func dbService(t *testing.T) (*sql.DB, *Service, *config.Service) {
	t.Helper()
	conn := testdb.Require(t)
	cfg := config.NewService(configstore.New(db.New(conn)))
	svc := NewService(NewRepository(db.New(conn)))
	svc.SetConfigService(cfg)
	return conn, svc, cfg
}

// The shipping and payment coverage lists carry no FK, so the count is the only thing keeping them from pointing at nothing
func TestACountryAShippingZoneCoversCannotBeDeleted(t *testing.T) {
	conn, svc, _ := dbService(t)
	ctx := context.Background()
	code := seedCountry(t, conn, svc)
	var channelID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel: %v", err)
	}
	var zoneID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO shipping_zones (channel_id, code, name) VALUES ($1, $2, 'test') RETURNING id`, channelID, "z-"+uuid.NewString()[:8]).Scan(&zoneID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM shipping_zones WHERE id = $1`, zoneID) })
	if _, err := conn.Exec(`INSERT INTO shipping_zone_countries (zone_id, country_code) VALUES ($1, $2)`, zoneID, code); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, code); !errors.Is(err, ErrCountryInUse) {
		t.Fatalf("deleting a country a zone covers: %v, want ErrCountryInUse", err)
	}
}

// A reference that lands between the count and the delete hits the RESTRICT FK, which must read as in-use rather than an internal error
func TestAnFKClashOnDeleteReadsAsInUse(t *testing.T) {
	conn, svc, _ := dbService(t)
	code := seedCountry(t, conn, svc)
	if _, err := conn.Exec(`UPDATE locales SET default_country_code = $1 WHERE code = (SELECT code FROM locales LIMIT 1)`, code); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`UPDATE locales SET default_country_code = NULL WHERE default_country_code = $1`, code)
	})
	if err := NewRepository(db.New(conn)).Delete(context.Background(), code); !errors.Is(err, ErrCountryInUse) {
		t.Fatalf("repo delete under a live FK: %v, want ErrCountryInUse", err)
	}
}

// A deleted code comes out of every country option it was set in, and an option left empty resets
func TestDeletingACountryTakesItOutOfTheSettings(t *testing.T) {
	conn, svc, cfg := dbService(t)
	ctx := context.Background()
	code := seedCountry(t, conn, svc)
	var channelID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel: %v", err)
	}
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
	t.Cleanup(func() {
		for _, p := range codeSettings {
			_, _ = cfg.Reset(ctx, p, scope)
		}
	})
	if _, err := cfg.Set(ctx, CfgAllowedCountries, "US,"+code, scope); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Set(ctx, CfgEUCountries, code, scope); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, code); err != nil {
		t.Fatal(err)
	}
	if v, _ := cfg.GetString(ctx, CfgAllowedCountries, scope); v != "US" {
		t.Errorf("allowed countries = %q, want the deleted code gone", v)
	}
	if rows, _ := cfg.ListStored(ctx, CfgEUCountries); len(rows) != 0 {
		for _, r := range rows {
			if r.Scope.ChannelID != nil && *r.Scope.ChannelID == channelID {
				t.Errorf("an option holding only the deleted code was kept as %q, want it reset", r.Value)
			}
		}
	}
}
