package graphqlx

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// Forwards errors and panics to an external aggregator - declared here so graphqlx stays independent of observability, whose Sentry reporter satisfies it structurally - Must be concurrent-safe, and a nil Reporter passes through
type Reporter interface {
	CaptureAppError(ctx context.Context, ae *apperr.Error)
	CapturePanic(ctx context.Context, rec any)
}

type options struct {
	reporter   Reporter
	devDetails bool
}

type Option func(*options)

func WithReporter(r Reporter) Option {
	return func(o *options) { o.reporter = r }
}

// Enables dev-only error extensions, like the denied permission slug on FORBIDDEN - pass APP_ENV=="development", and never enable it in production
func WithDevDetails(enabled bool) Option {
	return func(o *options) { o.devDetails = enabled }
}

func resolveOptions(opts []Option) options {
	var o options
	for _, fn := range opts {
		fn(&o)
	}
	return o
}
