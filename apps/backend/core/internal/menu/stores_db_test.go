package menu

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// The delete-then-insert used to commit half-way - an insert failing after the delete took the menu off every store
func TestAFailedStoreAssignmentKeepsTheOldOnes(t *testing.T) {
	conn := testdb.Require(t)
	var channelID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel: %v", err)
	}
	svc := NewService(NewRepository(db.New(conn), txrunner.New(conn)))
	ctx := context.Background()
	m, err := svc.CreateMenu(ctx, CreateMenuInput{Slug: "m-" + uuid.NewString()[:8], Name: "Fixture"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM menus WHERE id = $1`, m.ID) })
	if err := svc.SetMenuStores(ctx, m.ID, []uuid.UUID{channelID}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetMenuStores(ctx, m.ID, []uuid.UUID{uuid.New(), channelID}); err == nil {
		t.Fatal("assigning a channel that doesn't exist succeeded")
	}
	got, err := svc.ListMenuChannelAssignments(ctx, m.ID)
	if err != nil || !slices.Equal(got, []uuid.UUID{channelID}) {
		t.Fatalf("assignments after the refused save = %v (%v), want the original store kept", got, err)
	}
}
