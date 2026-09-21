package graphqlx_test

import (
	"context"
	"errors"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/graphqlx"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

type fakeChecker struct {
	grant map[string]bool
	err   error
	asked []string
}

func (f *fakeChecker) HasPermission(_ context.Context, _ uuid.UUID, slug string) (bool, error) {
	f.asked = append(f.asked, slug)
	if f.err != nil {
		return false, f.err
	}
	return f.grant[slug], nil
}

// The executor defaults DisableIntrospection to true per operation, so "denied" is the state we
// leave alone and "allowed" is the only thing that writes
func run(t *testing.T, ctx context.Context, ext graphqlx.PermittedIntrospection) bool {
	t.Helper()
	opCtx := &graphql.OperationContext{DisableIntrospection: true}
	if gqlErr := ext.MutateOperationContext(ctx, opCtx); gqlErr != nil {
		t.Fatalf("mutator returned %v, want nil - a denial must not reject the whole operation", gqlErr)
	}
	return !opCtx.DisableIntrospection
}

func adminCtx(superuser bool) context.Context {
	return auth.WithAdmin(context.Background(), &auth.Claims{AdminID: uuid.New(), IsSuperuser: superuser})
}

// The whole point: the privileged schema must stay invisible to callers with no admin claims,
// which is what the admin surface gets from its non-blocking auth middleware
func TestPermittedIntrospectionDeniesAnonymous(t *testing.T) {
	checker := &fakeChecker{grant: map[string]bool{"developer.playground": true}}
	ext := graphqlx.PermittedIntrospection{Checker: checker, Slug: "developer.playground"}

	if run(t, context.Background(), ext) {
		t.Error("introspection enabled for a caller with no admin claims")
	}
	if len(checker.asked) != 0 {
		t.Errorf("checker consulted %v for an anonymous caller - the claims check must short-circuit", checker.asked)
	}
}

func TestPermittedIntrospectionAllowsThePermission(t *testing.T) {
	checker := &fakeChecker{grant: map[string]bool{"developer.playground": true}}
	ext := graphqlx.PermittedIntrospection{Checker: checker, Slug: "developer.playground"}

	if !run(t, adminCtx(false), ext) {
		t.Error("introspection denied to an admin holding developer.playground")
	}
	if len(checker.asked) != 1 || checker.asked[0] != "developer.playground" {
		t.Errorf("checker asked for %v, want developer.playground once", checker.asked)
	}
}

func TestPermittedIntrospectionDeniesAdminWithoutThePermission(t *testing.T) {
	ext := graphqlx.PermittedIntrospection{Checker: &fakeChecker{}, Slug: "developer.playground"}

	if run(t, adminCtx(false), ext) {
		t.Error("introspection enabled for an admin lacking developer.playground")
	}
}

func TestPermittedIntrospectionAllowsSuperuser(t *testing.T) {
	// Superusers bypass inside auth.RequirePermission, so the checker is never consulted
	checker := &fakeChecker{}
	ext := graphqlx.PermittedIntrospection{Checker: checker, Slug: "developer.playground"}

	if !run(t, adminCtx(true), ext) {
		t.Error("introspection denied to a superuser")
	}
	if len(checker.asked) != 0 {
		t.Errorf("checker consulted %v for a superuser", checker.asked)
	}
}

// A checker that's down must not become an introspection bypass
func TestPermittedIntrospectionFailsClosedOnCheckerError(t *testing.T) {
	ext := graphqlx.PermittedIntrospection{
		Checker: &fakeChecker{err: errors.New("db down")},
		Slug:    "developer.playground",
	}

	if run(t, adminCtx(false), ext) {
		t.Error("introspection enabled after the permission lookup failed - it has to fail closed")
	}
}

// Zero value is inert rather than wide open, so a misconfigured wiring can't hand out the schema
func TestPermittedIntrospectionUnconfiguredDenies(t *testing.T) {
	if run(t, adminCtx(true), graphqlx.PermittedIntrospection{}) {
		t.Error("introspection enabled with no checker or slug configured")
	}
}
