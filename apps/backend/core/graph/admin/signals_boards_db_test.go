package admin

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// fakeBoards answers as Signals would for one board, and records what core relayed to it
type fakeBoards struct {
	signalsv1.BoardServiceClient
	owner            string
	claimed, savedTo int
}

func (f *fakeBoards) board() *signalsv1.Board {
	return &signalsv1.Board{Id: uuid.NewString(), Name: "Product X", UpdatedAt: timestamppb.Now(), OwnerAdminId: f.owner, Status: "published", Access: "own"}
}

func (f *fakeBoards) GetBoard(context.Context, *signalsv1.GetBoardRequest, ...grpc.CallOption) (*signalsv1.GetBoardResponse, error) {
	return &signalsv1.GetBoardResponse{Board: f.board()}, nil
}

func (f *fakeBoards) ClaimBoard(context.Context, *signalsv1.ClaimBoardRequest, ...grpc.CallOption) (*signalsv1.ClaimBoardResponse, error) {
	f.claimed++
	return &signalsv1.ClaimBoardResponse{Board: f.board()}, nil
}

func (f *fakeBoards) SaveBoard(context.Context, *signalsv1.SaveBoardRequest, ...grpc.CallOption) (*signalsv1.SaveBoardResponse, error) {
	f.savedTo++
	return &signalsv1.SaveBoardResponse{Board: f.board()}, nil
}

func boardsFixture(t *testing.T, boards *fakeBoards) (escalationFixture, *Resolver) {
	t.Helper()
	f := newEscalationFixture(t)
	client := fakeSignalsClient(t)
	client.Boards = boards
	r := &Resolver{AdminSvc: f.svc, SignalsEnabled: true, SignalsAPI: client}
	return f, r
}

// A claim is only for a board whose owner core no longer has - Signals can't know that, so core is where it's held
func TestABoardCanOnlyBeClaimedOnceItsOwnerIsGone(t *testing.T) {
	boards := &fakeBoards{}
	f, r := boardsFixture(t, boards)
	m := &mutationResolver{r}
	ownerID, _ := f.admin(t)
	_, manager := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "boards.manage", ChannelID: &f.channel}))

	boards.owner = ownerID.String()
	if _, err := m.ClaimBoard(manager, f.channel.String(), uuid.NewString()); !hasCode(err, "BOARD_HAS_OWNER") || boards.claimed != 0 {
		t.Fatalf("claiming a board whose owner is here: err %v, %d claims relayed", err, boards.claimed)
	}
	boards.owner = uuid.NewString() // an admin core doesn't have - deleted
	if _, err := m.ClaimBoard(manager, f.channel.String(), uuid.NewString()); err != nil || boards.claimed != 1 {
		t.Fatalf("claiming an orphan: err %v, %d claims relayed", err, boards.claimed)
	}
}

func TestABoardNamesItsOwnerAndSaysWhenTheyreGone(t *testing.T) {
	boards := &fakeBoards{}
	f, r := boardsFixture(t, boards)
	q := &queryResolver{r}
	ownerID, _ := f.admin(t)
	_, viewer := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "boards.view", ChannelID: &f.channel}))

	boards.owner = ownerID.String()
	b, err := q.Board(viewer, f.channel.String(), uuid.NewString())
	if err != nil || b.Owner == nil || b.Owner.Name != "E S" || b.Owner.Removed {
		t.Fatalf("owner = %+v, %v - want the admin's name", b.Owner, err)
	}
	boards.owner = uuid.NewString()
	if b, _ = q.Board(viewer, f.channel.String(), uuid.NewString()); b.Owner == nil || !b.Owner.Removed {
		t.Fatalf("a deleted owner = %+v, want removed", b.Owner)
	}
	boards.owner = ""
	if b, _ = q.Board(viewer, f.channel.String(), uuid.NewString()); b.Owner != nil {
		t.Fatalf("a shop board has owner %+v, want none", b.Owner)
	}
}

// Saving needs boards.view on the shop, not boards.manage: an edit share is what lets someone save, and Signals checks it
func TestAnEditShareSavesWithBoardsViewAlone(t *testing.T) {
	boards := &fakeBoards{}
	f, r := boardsFixture(t, boards)
	m := &mutationResolver{r}
	_, editor := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "boards.view", ChannelID: &f.channel}))
	_, nobody := f.admin(t)

	if _, err := m.SaveBoard(nobody, f.channel.String(), uuid.NewString(), "x", nil, time.Now()); err == nil || boards.savedTo != 0 {
		t.Fatalf("an admin without boards.view reached Signals: err %v, %d saves", err, boards.savedTo)
	}
	if _, err := m.SaveBoard(editor, f.channel.String(), uuid.NewString(), "x", nil, time.Now()); err != nil || boards.savedTo != 1 {
		t.Fatalf("boards.view alone: err %v, %d saves relayed", err, boards.savedTo)
	}
}

// The share dialog warns about someone who'd see the board without its numbers, so the targets say which they'd see
func TestShareTargetsSayWhoWouldSeeTheBoardAndItsNumbers(t *testing.T) {
	f, r := boardsFixture(t, &fakeBoards{})
	q := &queryResolver{r}
	scope := func(slug string) adminpkg.RolePermissionScopeInput {
		return adminpkg.RolePermissionScopeInput{PermissionSlug: slug, ChannelID: &f.channel}
	}
	layoutOnly := f.role(t, scope("boards.view"))
	analysts := f.role(t, scope("boards.view"), scope("customers.view"))
	_, owner := f.admin(t, f.role(t, scope("boards.manage")))
	_, viewer := f.admin(t, layoutOnly)

	if _, err := q.BoardShareTargets(viewer, f.channel.String()); err == nil {
		t.Fatal("someone who can't make boards listed who to share them with")
	}
	targets, err := q.BoardShareTargets(owner, f.channel.String())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][2]bool{}
	for _, tg := range targets {
		if tg.Kind == "role" {
			got[tg.ID] = [2]bool{tg.SeesBoards, tg.SeesData}
		}
	}
	if got[layoutOnly.ID.String()] != [2]bool{true, false} || got[analysts.ID.String()] != [2]bool{true, true} {
		t.Fatalf("roles = %v, want boards-only to see no numbers and analysts to see both", got)
	}
}

func hasCode(err error, code string) bool {
	e, ok := apperr.AsAppError(err)
	return ok && e.Code == code
}
