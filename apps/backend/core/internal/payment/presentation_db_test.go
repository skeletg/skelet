package payment

import (
	"context"
	"testing"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestProviderSyncPreservesCheckoutPresentation(t *testing.T) {
	conn := testdb.Require(t)
	queries := db.New(conn)
	channels, err := queries.ListChannels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) == 0 {
		t.Fatal("disposable fixture has no channel")
	}
	repo := NewRepository(queries)
	id := channels[0].ID
	if _, err := repo.UpsertPaymentMethod(context.Background(), id, "presentation-test", "Gateway default"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdatePaymentMethodPresentation(context.Background(), id, "presentation-test", "Credit/debit card", 7); err != nil {
		t.Fatal(err)
	}
	after, err := repo.UpsertPaymentMethod(context.Background(), id, "presentation-test", "Gateway renamed")
	if err != nil {
		t.Fatal(err)
	}
	if after.DisplayName != "Credit/debit card" || after.SortOrder != 7 {
		t.Fatalf("sync overwrote presentation: %+v", after)
	}
}
