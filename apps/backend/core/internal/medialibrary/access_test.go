package medialibrary

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	authpkg "github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

type grantsStub struct {
	everywhere map[string]bool
	asked      []string
}

func (g *grantsStub) HasGlobalPermission(_ context.Context, _ uuid.UUID, slug string) (bool, error) {
	g.asked = append(g.asked, slug)
	return g.everywhere[slug], nil
}

func (g *grantsStub) HasChannelPermission(context.Context, uuid.UUID, string, uuid.UUID, *string) (bool, error) {
	return true, nil
}

func TestThePrivateHalfIsAskedForOnlyWhenNamedAndOnlyEverywhere(t *testing.T) {
	admin := authpkg.WithAdmin(context.Background(), &authpkg.Claims{AdminID: uuid.New()})
	grants := &grantsStub{everywhere: map[string]bool{PermPrivateView: true}}

	if err := RequireHalf(admin, grants, PermPrivateManage, Public); err != nil || len(grants.asked) != 0 {
		t.Errorf("the public half asked for %v and answered %v, want nothing asked", grants.asked, err)
	}
	if err := RequireHalf(admin, grants, PermPrivateView, Private); err != nil {
		t.Errorf("a caller with the grant everywhere was refused: %v", err)
	}
	for name, halves := range map[string][]Visibility{"the private half": {Private}, "a move out of it": {Private, Public}, "a move into it": {Public, Private}} {
		err := RequireHalf(admin, grants, PermPrivateManage, halves...)
		if denied, ok := errors.AsType[*authpkg.ScopedDenial](err); !ok || denied.Slug != PermPrivateManage {
			t.Errorf("%s with the grant on one channel only: %v, want it refused for %s", name, err, PermPrivateManage)
		}
	}
	if err := RequireHalf(context.Background(), grants, PermPrivateView, Private); !errors.Is(err, authpkg.ErrUnauthenticated) {
		t.Errorf("no one signed in: %v, want unauthenticated", err)
	}
}
