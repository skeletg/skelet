package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestActorFromContext(t *testing.T) {
	if _, err := RequireActor(context.Background()); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("anonymous caller: got %v, want ErrUnauthenticated", err)
	}

	tokID := uuid.New()
	ctx := WithApiToken(context.Background(), &ApiTokenClaims{TokenID: tokID, Name: "ERP sync"})
	a, err := RequireActor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kind != ActorIntegration || a.Name != "ERP sync" || a.AdminID() != (uuid.NullUUID{}) || a.IntegrationID().UUID != tokID {
		t.Errorf("token actor = %+v", a)
	}

	adminID := uuid.New()
	ctx = WithAdmin(ctx, &Claims{AdminID: adminID, Email: "a@x.test"})
	a, _ = RequireActor(ctx)
	if a.Kind != ActorAdmin || a.AdminID().UUID != adminID || a.IntegrationID() != (uuid.NullUUID{}) {
		t.Errorf("admin actor = %+v", a)
	}
}
