package board

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

func newShop(t *testing.T) (*sql.DB, *Service, uuid.UUID) {
	t.Helper()
	conn := testdb.RequireSignals(t)
	shop := uuid.New()
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM boards WHERE channel_id = $1`, shop) })
	return conn, NewService(conn), shop
}

// publish creates a board as its owner and saves it once, which is what takes it out of draft
func publish(t *testing.T, svc *Service, shop uuid.UUID, owner Actor, name string, reports ...Report) Board {
	t.Helper()
	ctx := context.Background()
	b, err := svc.Create(ctx, shop, owner, name, reports)
	if err != nil {
		t.Fatal(err)
	}
	b, err = svc.Save(ctx, shop, owner, b.ID, name, reports, b.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var views = Report{Title: "Views", Definition: Definition{Kind: "insights", Event: "page_view"}}

func TestBoardsStayInTheirShopAndRefuseAStaleSave(t *testing.T) {
	conn, svc, shop := newShop(t)
	ctx := context.Background()
	other := uuid.New()
	me := Actor{AdminID: uuid.New(), CanManage: true}

	b, err := svc.Create(ctx, shop, me, "  Mine  ", []Report{views})
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "Mine" {
		t.Errorf("name = %q, want it trimmed", b.Name)
	}
	if _, err := svc.Get(ctx, other, me, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another shop read the board: %v", err)
	}
	if err := svc.Delete(ctx, other, me, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another shop deleted the board: %v", err)
	}

	got, err := svc.Get(ctx, shop, me, b.ID)
	if err != nil || len(got.Reports) != 1 || got.Reports[0].Definition.Event != "page_view" {
		t.Fatalf("board = %+v, %v", got, err)
	}
	funnel := Report{Title: "Funnel", Size: "full", Definition: Definition{Kind: "funnel", Steps: []string{"page_view", "order_placed"}}}
	saved, err := svc.Save(ctx, shop, me, b.ID, "Mine", []Report{funnel, views}, got.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Reports) != 2 || saved.Reports[0].Title != "Funnel" || saved.Reports[0].Size != "full" || saved.Reports[1].Title != "Views" {
		t.Fatalf("saved reports = %+v, want them in the order sent", saved.Reports)
	}
	if _, err := svc.Save(ctx, shop, me, b.ID, "Mine", nil, got.UpdatedAt); !errors.Is(err, ErrStale) {
		t.Fatalf("a save from an old read: %v, want ErrStale", err)
	}
	if again, _ := svc.Get(ctx, shop, me, b.ID); len(again.Reports) != 2 {
		t.Fatalf("the stale save changed the board: %+v", again.Reports)
	}
	if _, err := svc.Save(ctx, shop, me, b.ID, "", nil, saved.UpdatedAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a nameless save: %v", err)
	}
	if err := svc.Delete(ctx, shop, me, b.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = conn.QueryRow(`SELECT count(*) FROM board_reports WHERE board_id = $1`, b.ID).Scan(&left)
	if left != 0 {
		t.Errorf("%d reports outlived their board", left)
	}
}

// A new board is its creator's draft; the first Save publishes it, and only then can it be shared
func TestANewBoardIsAPrivateDraftUntilSaved(t *testing.T) {
	_, svc, shop := newShop(t)
	ctx := context.Background()
	me, colleague := Actor{AdminID: uuid.New(), CanManage: true}, Actor{AdminID: uuid.New(), CanManage: true}

	b, err := svc.Create(ctx, shop, me, "Product X", []Report{views})
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != StatusDraft || b.OwnerID != me.AdminID {
		t.Fatalf("a new board = status %q owner %s, want my draft", b.Status, b.OwnerID)
	}
	if _, err := svc.Get(ctx, shop, colleague, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a colleague opened my draft: %v", err)
	}
	if list, _ := svc.List(ctx, shop, colleague, false); len(list) != 0 {
		t.Fatalf("a colleague lists my draft: %+v", list)
	}
	if _, err := svc.SetShares(ctx, shop, me, b.ID, []Share{{AdminID: colleague.AdminID, Level: ShareView}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("sharing a draft: %v, want ErrInvalid", err)
	}
	saved, err := svc.Save(ctx, shop, me, b.ID, "Product X", []Report{views}, b.UpdatedAt)
	if err != nil || saved.Status != StatusPublished {
		t.Fatalf("first save = %q, %v, want published", saved.Status, err)
	}
	if _, err := svc.Get(ctx, shop, colleague, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a saved board isn't shared by being saved: %v", err)
	}
}

// Shared read-only with a role: its members open it and can't change it, and someone outside it can't tell it exists
func TestSharingWithARoleToViewOnly(t *testing.T) {
	_, svc, shop := newShop(t)
	ctx := context.Background()
	marketing := uuid.New()
	me := Actor{AdminID: uuid.New(), CanManage: true}
	member := Actor{AdminID: uuid.New(), RoleIDs: []uuid.UUID{marketing}}
	outsider := Actor{AdminID: uuid.New(), CanManage: true}
	b := publish(t, svc, shop, me, "Product X", views)

	shared, err := svc.SetShares(ctx, shop, me, b.ID, []Share{{RoleID: marketing, Level: ShareView}})
	if err != nil || len(shared.Shares) != 1 {
		t.Fatalf("shared = %+v, %v", shared.Shares, err)
	}
	got, err := svc.Get(ctx, shop, member, b.ID)
	if err != nil || got.Access != View || len(got.Shares) != 0 {
		t.Fatalf("a member's read = access %s, shares %v, %v - want view, and the share list kept for the owner", got.Access, got.Shares, err)
	}
	if _, err := svc.Save(ctx, shop, member, b.ID, "Mine now", nil, got.UpdatedAt); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a viewer saved: %v, want ErrForbidden", err)
	}
	if _, err := svc.SaveDraft(ctx, shop, member, b.ID, []byte(`{}`), got.UpdatedAt); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a viewer kept a draft: %v, want ErrForbidden", err)
	}
	if err := svc.SetFavourite(ctx, shop, member, b.ID, true); err != nil {
		t.Fatalf("a viewer can star what they can see: %v", err)
	}
	if _, err := svc.Get(ctx, shop, outsider, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an outsider = %v, want not found rather than forbidden", err)
	}
	if err := svc.Delete(ctx, shop, outsider, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an outsider deleted it: %v", err)
	}
	if _, err := svc.SetShares(ctx, shop, member, b.ID, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a viewer unshared it: %v", err)
	}
	list, _ := svc.List(ctx, shop, member, false)
	if len(list) != 1 || !list[0].Favourite || list[0].Access != View {
		t.Fatalf("the member's list = %+v, want the board, starred, as a viewer", list)
	}
}

// An edit share changes the board and keeps unsaved edits between visits; saving clears them
func TestAnEditorsDraftIsKeptUntilTheySave(t *testing.T) {
	_, svc, shop := newShop(t)
	ctx := context.Background()
	me, editor := Actor{AdminID: uuid.New(), CanManage: true}, Actor{AdminID: uuid.New()}
	b := publish(t, svc, shop, me, "Product X", views)
	if _, err := svc.SetShares(ctx, shop, me, b.ID, []Share{{AdminID: editor.AdminID, Level: ShareEdit}}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.SaveDraft(ctx, shop, editor, b.ID, []byte(`{"name":"Product X v2"}`), b.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(ctx, shop, editor, b.ID)
	if err != nil || got.Draft == nil || string(got.Draft.Payload) != `{"name": "Product X v2"}` || !got.Draft.BaseUpdatedAt.Equal(b.UpdatedAt) {
		t.Fatalf("the editor's draft = %+v, %v", got.Draft, err)
	}
	if mine, _ := svc.Get(ctx, shop, me, b.ID); mine.Draft != nil {
		t.Fatal("the owner sees the editor's draft - a draft is its admin's alone")
	}
	if list, _ := svc.List(ctx, shop, editor, false); len(list) != 1 || list[0].DraftAt.IsZero() {
		t.Fatalf("the editor's list doesn't mark the draft: %+v", list)
	}
	if _, err := svc.Save(ctx, shop, editor, b.ID, "Product X v2", []Report{views}, got.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if after, _ := svc.Get(ctx, shop, editor, b.ID); after.Draft != nil {
		t.Fatal("the draft outlived the save that applied it")
	}
	if _, err := svc.SaveDraft(ctx, shop, editor, b.ID, []byte(`not json`), b.UpdatedAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a junk draft: %v", err)
	}
}

func TestDraftsNobodyCameBackToAreSwept(t *testing.T) {
	conn, svc, shop := newShop(t)
	ctx := context.Background()
	me := Actor{AdminID: uuid.New(), CanManage: true}
	b := publish(t, svc, shop, me, "Old", views)
	if _, err := svc.SaveDraft(ctx, shop, me, b.ID, []byte(`{}`), b.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`UPDATE board_drafts SET updated_at = now() - interval '31 days' WHERE board_id = $1`, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SweepDrafts(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Get(ctx, shop, me, b.ID); got.Draft != nil {
		t.Fatal("a 31-day-old draft survived the sweep")
	}
	if _, err := svc.Get(ctx, shop, me, b.ID); err != nil {
		t.Fatalf("the sweep took the board too: %v", err)
	}
}

// A shop board is everyone's to see and its managers' to run; making one drops the shares it no longer needs
func TestMakingAShopBoardAndClaimingAnOrphan(t *testing.T) {
	_, svc, shop := newShop(t)
	ctx := context.Background()
	me := Actor{AdminID: uuid.New(), CanManage: true}
	viewer, manager := Actor{AdminID: uuid.New()}, Actor{AdminID: uuid.New(), CanManage: true}
	b := publish(t, svc, shop, me, "Weekly", views)
	if _, err := svc.SetShares(ctx, shop, me, b.ID, []Share{{AdminID: manager.AdminID, Level: ShareView}}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Claim(ctx, shop, viewer, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("someone it isn't shared with claimed it: %v", err)
	}
	claimed, err := svc.Claim(ctx, shop, manager, b.ID) // core checked the owner is gone
	if err != nil || claimed.OwnerID != manager.AdminID || claimed.Access != Own {
		t.Fatalf("claimed = owner %s access %s, %v", claimed.OwnerID, claimed.Access, err)
	}

	shopBoard, err := svc.MakeShopBoard(ctx, shop, manager, b.ID)
	if err != nil || shopBoard.OwnerID != uuid.Nil {
		t.Fatalf("shop board = owner %s, %v", shopBoard.OwnerID, err)
	}
	got, err := svc.Get(ctx, shop, viewer, b.ID)
	if err != nil || got.Access != View {
		t.Fatalf("anyone on the shop = %s, %v, want view", got.Access, err)
	}
	if _, err := svc.Save(ctx, shop, viewer, b.ID, "x", nil, got.UpdatedAt); !errors.Is(err, ErrForbidden) {
		t.Fatalf("boards.view alone edited a shop board: %v", err)
	}
	if mine, _ := svc.Get(ctx, shop, Actor{AdminID: uuid.New(), CanManage: true}, b.ID); mine.Access != Own {
		t.Fatalf("a manager on a shop board = %s, want own", mine.Access)
	}
}

func TestStarterBoardsAreShopBoardsAddedOnce(t *testing.T) {
	conn, svc, shop := newShop(t)
	ctx := context.Background()
	me := Actor{AdminID: uuid.New(), CanManage: true}
	first, err := svc.CreateStarters(ctx, shop, me)
	if err != nil || len(first) != 4 {
		t.Fatalf("starters = %d, %v", len(first), err)
	}
	for _, b := range first {
		if b.OwnerID != uuid.Nil || b.Status != StatusPublished {
			t.Fatalf("starter %q = owner %s status %s, want a published shop board", b.Name, b.OwnerID, b.Status)
		}
	}
	again, err := svc.CreateStarters(ctx, shop, me)
	if err != nil || len(again) != 4 {
		t.Fatalf("a second call left %d boards, %v - want the same four", len(again), err)
	}
	if seen, _ := svc.List(ctx, shop, Actor{AdminID: uuid.New()}, false); len(seen) != 4 {
		t.Fatalf("someone with boards.view sees %d starters, want all 4", len(seen))
	}
	// Two admins clicking at once still get one set
	racing := uuid.New()
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM boards WHERE channel_id = $1`, racing) })
	errs := make(chan error, 2)
	for range 2 {
		go func() { _, err := svc.CreateStarters(ctx, racing, me); errs <- err }()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if bs, _ := svc.List(ctx, racing, me, false); len(bs) != 4 {
		t.Fatalf("concurrent starters left %d boards, want 4", len(bs))
	}
}

func TestASuperuserListsOtherPeoplesBoardsOnlyWhenAskingForAll(t *testing.T) {
	_, svc, shop := newShop(t)
	ctx := context.Background()
	publish(t, svc, shop, Actor{AdminID: uuid.New(), CanManage: true}, "Someone's", views)
	su := Actor{AdminID: uuid.New(), Superuser: true}
	if mine, _ := svc.List(ctx, shop, su, false); len(mine) != 0 {
		t.Fatalf("a superuser's own list holds someone's private board: %+v", mine)
	}
	if all, _ := svc.List(ctx, shop, su, true); len(all) != 1 || all[0].Access != Own {
		t.Fatalf("All boards = %+v, want the private board, which they can act on", all)
	}
}

func TestARetiredShopsBoardsGo(t *testing.T) {
	_, svc, shop := newShop(t) // never in channel_roster, so retired as far as the prune is concerned
	ctx := context.Background()
	me := Actor{AdminID: uuid.New(), CanManage: true}
	if _, err := svc.Create(ctx, shop, me, "Gone", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.New(testdb.RequireSignals(t)).PruneOrphanedBoards(ctx); err != nil {
		t.Fatal(err)
	}
	if bs, _ := svc.List(ctx, shop, me, false); len(bs) != 0 {
		t.Fatalf("%d boards outlived their shop", len(bs))
	}
}
