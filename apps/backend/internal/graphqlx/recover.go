package graphqlx

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/99designs/gqlgen/graphql"

	"github.com/skeletg/skelet/internal/apperr"
	"github.com/skeletg/skelet/internal/applog"
)

// Converts resolver panics into a ClassInternal *apperr.Error, logging the stack and letting the error flow back through the surface Presenter
// Needed because gqlgen catches resolver panics before httpx.Recover ever sees them - without it gqlgen returns a generic "internal system error" with no structured code
func RecoverFunc(s Surface, opts ...Option) graphql.RecoverFunc {
	cfg := resolveOptions(opts)
	return func(ctx context.Context, rec any) error {
		err := panicAsError(rec)
		applog.From(ctx).Error(
			"graphql resolver panic recovered",
			"surface", s.String(),
			"err", err.Error(),
			"stack", string(debug.Stack()),
		)
		if cfg.reporter != nil {
			// Capture the raw rec rather than the wrapped err - Sentry's own stack-trace extraction handles `any` through reflect.TypeOf and surfaces a richer stack than the already-wrapped *apperr.Error would
			cfg.reporter.CapturePanic(ctx, rec)
		}
		return apperr.WrapClass(err, apperr.ClassInternal, "graphqlx.RecoverFunc")
	}
}

func panicAsError(rec any) error {
	switch v := rec.(type) {
	case error:
		return v
	default:
		return fmt.Errorf("panic: %v", v)
	}
}
