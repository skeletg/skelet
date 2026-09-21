package graphqlx

import (
	"github.com/99designs/gqlgen/graphql"
	"github.com/ravilushqa/otelgqlgen"
)

// Spans each GraphQL op and top-level field - no-op-safe when OTel is off
func OTelMiddleware() graphql.HandlerExtension {
	return otelgqlgen.Middleware(
		otelgqlgen.WithoutVariables(), // variables may carry PII - passwords, refresh tokens
		otelgqlgen.WithCreateSpanFromFields(func(ctx *graphql.FieldContext) bool {
			return len(ctx.Path()) <= 2 // top-level only - per-field spans flood Jaeger on deep queries
		}),
	)
}
