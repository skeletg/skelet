package board

import (
	"testing"

	"github.com/google/uuid"
)

// The access matrix: every kind of admin against every kind of board. Channel scope and boards.view are checked before
// access is asked (the contract handler and service_db_test), so here every actor already holds boards.view on the shop
func TestAccessMatrix(t *testing.T) {
	owner, viewer, editor, stranger, manager, su := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	viewRole, editRole, otherRole := uuid.New(), uuid.New(), uuid.New()
	shares := []Share{
		{AdminID: viewer, Level: ShareView},
		{AdminID: editor, Level: ShareEdit},
		{RoleID: viewRole, Level: ShareView},
		{RoleID: editRole, Level: ShareEdit},
	}
	actors := []struct {
		name string
		a    Actor
	}{
		{"owner", Actor{AdminID: owner}},
		{"admin share, view", Actor{AdminID: viewer}},
		{"admin share, edit", Actor{AdminID: editor}},
		{"role share, view", Actor{AdminID: uuid.New(), RoleIDs: []uuid.UUID{otherRole, viewRole}}},
		{"role share, edit", Actor{AdminID: uuid.New(), RoleIDs: []uuid.UUID{viewRole, editRole}}}, // the best share wins
		{"left the role", Actor{AdminID: uuid.New(), RoleIDs: []uuid.UUID{otherRole}}},
		{"unrelated, boards.view", Actor{AdminID: stranger}},
		{"unrelated, boards.manage", Actor{AdminID: manager, CanManage: true}},
		{"superuser", Actor{AdminID: su, Superuser: true}},
	}
	boards := []struct {
		name   string
		b      Board
		shares []Share
	}{
		{"private draft", Board{OwnerID: owner, Status: StatusDraft}, shares},
		{"private", Board{OwnerID: owner, Status: StatusPublished}, nil},
		{"shared", Board{OwnerID: owner, Status: StatusPublished}, shares},
		{"shop board", Board{Status: StatusPublished}, nil},
	}
	want := map[string][4]Access{ // private draft, private, shared, shop board
		"owner":                    {Own, Own, Own, View},
		"admin share, view":        {NoAccess, NoAccess, View, View},
		"admin share, edit":        {NoAccess, NoAccess, Edit, View},
		"role share, view":         {NoAccess, NoAccess, View, View},
		"role share, edit":         {NoAccess, NoAccess, Edit, View},
		"left the role":            {NoAccess, NoAccess, NoAccess, View},
		"unrelated, boards.view":   {NoAccess, NoAccess, NoAccess, View},
		"unrelated, boards.manage": {NoAccess, NoAccess, NoAccess, Own},
		"superuser":                {Own, Own, Own, Own},
	}
	for _, ac := range actors {
		for i, bd := range boards {
			if got := access(bd.b, bd.shares, ac.a); got != want[ac.name][i] {
				t.Errorf("%s on a %s board = %s, want %s", ac.name, bd.name, got, want[ac.name][i])
			}
		}
	}
}

// A superuser's own lists show what they'd see as themselves; the bypass is for "All boards" and for acting
func TestASuperusersOwnListLeavesOutOtherPeoplesPrivateBoards(t *testing.T) {
	su := Actor{AdminID: uuid.New(), Superuser: true}
	private := Board{OwnerID: uuid.New(), Status: StatusPublished}
	if access(private, nil, su.asSelf()) != NoAccess || access(private, nil, su) != Own {
		t.Fatal("a superuser lists someone's private board as themselves, or can't act on it")
	}
}
