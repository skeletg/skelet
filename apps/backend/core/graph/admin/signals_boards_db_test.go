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
	owner       string
	savedTo     int
	transferred []string // the new owner of each transfer relayed, "" for a shop board
}

func (f *fakeBoards) board() *signalsv1.Board {
	return &signalsv1.Board{Id: uuid.NewString(), Name: "Product X", UpdatedAt: timestamppb.Now(), OwnerAdminId: f.owner, Access: "own"}
}

func (f *fakeBoards) GetBoard(context.Context, *signalsv1.GetBoardRequest, ...grpc.CallOption) (*signalsv1.GetBoardResponse, error) {
	return &signalsv1.GetBoardResponse{Board: f.board()}, nil
}

func (f *fakeBoards) TransferBoard(_ context.Context, req *signalsv1.TransferBoardRequest, _ ...grpc.CallOption) (*signalsv1.TransferBoardResponse, error) {
	f.transferred = append(f.transferred, req.GetOwnerAdminId())
	return &signalsv1.TransferBoardResponse{}, nil
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

// A board is only handed to an active admin who can open boards on its shop - Signals can't know who that is, so core
// is where it's held. No new owner makes it a shop board
func TestABoardIsHandedOnlyToSomeoneWhoCanOpenIt(t *testing.T) {
	boards := &fakeBoards{}
	f, r := boardsFixture(t, boards)
	m := &mutationResolver{r}
	boardsHere := f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "boards.view", ChannelID: &f.channel})
	_, owner := f.admin(t, boardsHere)
	colleague, _ := f.admin(t, boardsHere)
	outsider, _ := f.admin(t) // no boards here
	transfer := func(to *string) error {
		_, err := m.TransferBoard(owner, f.channel.String(), uuid.NewString(), to)
		return err
	}
	str := func(s string) *string { return &s }

	for name, to := range map[string]*string{"no boards here": str(outsider.String()), "no such admin": str(uuid.NewString()), "not an id": str("x")} {
		if err := transfer(to); !hasCode(err, "BOARD_OWNER_INVALID") {
			t.Errorf("handing to %s: err %v, want BOARD_OWNER_INVALID", name, err)
		}
	}
	if err := transfer(str(colleague.String())); err != nil {
		t.Fatal(err)
	}
	if err := transfer(nil); err != nil {
		t.Fatal(err)
	}
	if len(boards.transferred) != 2 || boards.transferred[0] != colleague.String() || boards.transferred[1] != "" {
		t.Fatalf("relayed %q, want the colleague and then a shop board", boards.transferred)
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
