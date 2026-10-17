package signalsclient

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
)

type standingGrants struct {
	superuser, active bool
	grants            []auth.Grant
}

func (s standingGrants) Grants(context.Context, uuid.UUID) ([]auth.Grant, error) {
	return s.grants, nil
}
func (s standingGrants) Standing(context.Context, uuid.UUID) (bool, bool, error) {
	return s.superuser, s.active, nil
}

func minted(t *testing.T, ctx context.Context) *auth.ServiceClaims {
	t.Helper()
	md, _ := metadata.FromOutgoingContext(ctx)
	tok := strings.TrimPrefix(md.Get("authorization")[0], "Bearer ")
	c, err := auth.ValidateServiceToken(tok, signalsapi.Audience, auth.MustServiceKeys("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAsAdminCarriesTheAdminsStanding(t *testing.T) {
	mk := func(g Grants) *Client {
		c, err := New("127.0.0.1:1", auth.MustServiceKeys("test-secret"), time.Minute, g)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	if _, err := mk(standingGrants{active: false}).AsAdmin(context.Background(), uuid.New()); !errors.Is(err, ErrAdminInactive) {
		t.Errorf("a deactivated admin's queued work = %v, want ErrAdminInactive", err)
	}
	ctx, err := mk(standingGrants{superuser: true, active: true}).AsAdmin(context.Background(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if c := minted(t, ctx); !c.IsSuperuser {
		t.Error("a superuser's queued work was minted without the superuser flag, so the suite refuses it")
	}
}
