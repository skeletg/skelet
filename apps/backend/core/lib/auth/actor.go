package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// ErrAdminOnly refuses an integration token on what only a person can do - their own account, inbox and preferences, or managing admins and tokens
var ErrAdminOnly = errors.New("this needs a signed-in admin - integration tokens can't use it")

func init() { apperr.RegisterSentinel(ErrAdminOnly, "ADMIN_ONLY", apperr.ClassForbidden) }

type ActorKind string

const (
	ActorAdmin       ActorKind = "admin"
	ActorIntegration ActorKind = "integration"
)

type Actor struct {
	Kind        ActorKind
	ID          uuid.UUID
	Name        string
	IsSuperuser bool
}

// ActorFromContext resolves the admin first, the same precedence every guard uses, so a request carrying both credentials acts as the admin
func ActorFromContext(ctx context.Context) (Actor, bool) {
	if c, ok := AdminFromContext(ctx); ok && c != nil {
		return Actor{Kind: ActorAdmin, ID: c.AdminID, Name: c.Email, IsSuperuser: c.IsSuperuser}, true
	}
	if t, ok := ApiTokenFromContext(ctx); ok {
		return Actor{Kind: ActorIntegration, ID: t.TokenID, Name: t.Name}, true
	}
	return Actor{}, false
}

func RequireActor(ctx context.Context) (Actor, error) {
	a, ok := ActorFromContext(ctx)
	if !ok {
		return Actor{}, ErrUnauthenticated
	}
	return a, nil
}

func RequireAdmin(ctx context.Context) (*Claims, error) {
	if c, ok := AdminFromContext(ctx); ok {
		return c, nil
	}
	if _, ok := ApiTokenFromContext(ctx); ok {
		return nil, ErrAdminOnly
	}
	return nil, ErrUnauthenticated
}

// AdminID and IntegrationID are zero, not just invalid, for the other kind - callers that take .UUID write it straight into an admin column
func (a Actor) AdminID() uuid.NullUUID { return a.idIf(ActorAdmin) }

func (a Actor) IntegrationID() uuid.NullUUID { return a.idIf(ActorIntegration) }

func (a Actor) idIf(k ActorKind) uuid.NullUUID {
	if a.Kind != k || a.ID == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: a.ID, Valid: true}
}
