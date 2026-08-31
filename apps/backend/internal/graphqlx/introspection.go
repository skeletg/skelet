package graphqlx

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/skeletg/skelet/apps/backend/internal/auth"
)

// Gating introspection on APP_ENV instead left the dashboard playground with no docs or autocomplete everywhere it actually runs
// Introspection resolves outside the domain authz layer and the admin middleware is non-blocking, so the caller check has to happen here
type PermittedIntrospection struct {
	Checker auth.PermissionChecker // nil disables introspection outright - a misconfigured wiring must not hand out the schema
	Slug    string
}

var _ interface {
	graphql.OperationContextMutator
	graphql.HandlerExtension
} = PermittedIntrospection{}

func (PermittedIntrospection) ExtensionName() string { return "PermittedIntrospection" }

func (p PermittedIntrospection) Validate(graphql.ExecutableSchema) error { return nil }

// Silent on denial - a gqlerror would reject the whole operation, where leaving the executor's default true fails only the __schema selection
func (p PermittedIntrospection) MutateOperationContext(ctx context.Context, opCtx *graphql.OperationContext) *gqlerror.Error {
	if p.Checker == nil || p.Slug == "" {
		return nil
	}
	if err := auth.RequirePermission(ctx, p.Checker, p.Slug); err != nil {
		return nil
	}
	opCtx.DisableIntrospection = false
	return nil
}
