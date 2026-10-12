package payment

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// Fails the second country insert inside whatever transaction it's handed
type secondAddFails struct {
	Repository
	adds *int
}

func (f secondAddFails) WithTx(t *sql.Tx) Repository {
	return secondAddFails{Repository: f.Repository.WithTx(t), adds: f.adds}
}

func (f secondAddFails) AddCountryToPaymentMethod(ctx context.Context, id uuid.UUID, code string) error {
	*f.adds++
	if *f.adds == 2 {
		return errors.New("connection reset")
	}
	return f.Repository.AddCountryToPaymentMethod(ctx, id, code)
}

// The clear and the inserts weren't one transaction - a failure partway left a "specific countries" method with none, gone from checkout everywhere
func TestAFailedCountrySaveKeepsTheOldList(t *testing.T) {
	conn := testdb.Require(t)
	ResetForTest()
	t.Cleanup(ResetForTest)
	RegisterProvider(Registration{Slug: "cty-" + uuid.NewString()[:6], Provider: &fakeProvider{}})
	slug := AllProviders()[0].Slug
	var channelID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel: %v", err)
	}
	real := NewRepository(db.New(conn))
	if _, err := real.UpsertPaymentMethod(context.Background(), channelID, slug, "Fixture"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM payment_methods WHERE provider_slug = $1`, slug) })
	ctx := context.Background()
	if err := NewService(real, txrunner.New(conn), nil).SetPaymentMethodCountries(ctx, channelID, slug, "specific", []string{"RS", "DE"}); err != nil {
		t.Fatal(err)
	}
	adds := 0
	svc := NewService(secondAddFails{Repository: real, adds: &adds}, txrunner.New(conn), nil)
	if err := svc.SetPaymentMethodCountries(ctx, channelID, slug, "specific", []string{"FR", "IT"}); err == nil {
		t.Fatal("the failed save reported success")
	}
	m, err := real.GetPaymentMethod(ctx, channelID, slug)
	if err != nil {
		t.Fatal(err)
	}
	got, err := real.ListCountriesForPaymentMethod(ctx, m.ID)
	slices.Sort(got)
	if err != nil || !slices.Equal(got, []string{"DE", "RS"}) {
		t.Fatalf("countries after the failed save = %v (%v), want the old RS and DE kept", got, err)
	}
}
