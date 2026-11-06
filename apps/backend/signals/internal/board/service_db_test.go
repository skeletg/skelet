package board

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

func TestBoardsStayInTheirShopAndRefuseAStaleSave(t *testing.T) {
	conn := testdb.RequireSignals(t)
	svc := NewService(conn)
	ctx := context.Background()
	shop, other := uuid.New(), uuid.New()
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM boards WHERE channel_id = ANY($1)`, []uuid.UUID{shop, other}) })

	views := Report{Title: "Views", Definition: Definition{Kind: "insights", Event: "page_view"}}
	b, err := svc.Create(ctx, shop, "  Mine  ", []Report{views})
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "Mine" {
		t.Errorf("name = %q, want it trimmed", b.Name)
	}
	if _, err := svc.Get(ctx, other, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another shop read the board: %v", err)
	}
	if err := svc.Delete(ctx, other, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another shop deleted the board: %v", err)
	}

	got, err := svc.Get(ctx, shop, b.ID)
	if err != nil || len(got.Reports) != 1 || got.Reports[0].Definition.Event != "page_view" {
		t.Fatalf("board = %+v, %v", got, err)
	}
	funnel := Report{Title: "Funnel", Size: "full", Definition: Definition{Kind: "funnel", Steps: []string{"page_view", "order_placed"}}}
	saved, err := svc.Save(ctx, shop, b.ID, "Mine", []Report{funnel, views}, got.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Reports) != 2 || saved.Reports[0].Title != "Funnel" || saved.Reports[0].Size != "full" || saved.Reports[1].Title != "Views" {
		t.Fatalf("saved reports = %+v, want them in the order sent", saved.Reports)
	}
	if _, err := svc.Save(ctx, shop, b.ID, "Mine", nil, got.UpdatedAt); !errors.Is(err, ErrStale) {
		t.Fatalf("a save from an old read: %v, want ErrStale", err)
	}
	if again, _ := svc.Get(ctx, shop, b.ID); len(again.Reports) != 2 {
		t.Fatalf("the stale save changed the board: %+v", again.Reports)
	}
	if _, err := svc.Save(ctx, shop, b.ID, "", nil, saved.UpdatedAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a nameless save: %v", err)
	}
	if err := svc.Delete(ctx, shop, b.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = conn.QueryRow(`SELECT count(*) FROM board_reports WHERE board_id = $1`, b.ID).Scan(&left)
	if left != 0 {
		t.Errorf("%d reports outlived their board", left)
	}
}

func TestStarterBoardsAreAddedOnce(t *testing.T) {
	conn := testdb.RequireSignals(t)
	svc := NewService(conn)
	ctx := context.Background()
	shop := uuid.New()
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM boards WHERE channel_id = $1`, shop) })
	first, err := svc.CreateStarters(ctx, shop)
	if err != nil || len(first) != 4 {
		t.Fatalf("starters = %d, %v", len(first), err)
	}
	again, err := svc.CreateStarters(ctx, shop)
	if err != nil || len(again) != 4 {
		t.Fatalf("a second call left %d boards, %v - want the same four", len(again), err)
	}

	// Two admins clicking at once still get one set
	racing := uuid.New()
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM boards WHERE channel_id = $1`, racing) })
	errs := make(chan error, 2)
	for range 2 {
		go func() { _, err := svc.CreateStarters(ctx, racing); errs <- err }()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if bs, _ := svc.List(ctx, racing); len(bs) != 4 {
		t.Fatalf("concurrent starters left %d boards, want 4", len(bs))
	}
}

func TestARetiredShopsBoardsGo(t *testing.T) {
	conn := testdb.RequireSignals(t)
	svc := NewService(conn)
	ctx := context.Background()
	shop := uuid.New() // never in channel_roster, so retired as far as the prune is concerned
	if _, err := svc.Create(ctx, shop, "Gone", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.New(conn).PruneOrphanedBoards(ctx); err != nil {
		t.Fatal(err)
	}
	if bs, _ := svc.List(ctx, shop); len(bs) != 0 {
		t.Fatalf("%d boards outlived their shop", len(bs))
	}
}
