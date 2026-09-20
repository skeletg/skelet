package observability

import (
	"context"
	"path/filepath"
	"strconv"

	"github.com/getsentry/sentry-go"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// Reporter against the global Sentry SDK, fingerprinting by apperr.Code so dedup groups by failure mode rather than by stack - a nil pointer is a valid no-op
type sentryReporter struct{}

// Forwards a typed *apperr.Error as an event - tags come from the apperr fields, so Sentry's UI can facet by code, class, op and request_id without digging into the payload
func (r *sentryReporter) CaptureAppError(ctx context.Context, ae *apperr.Error) {
	if r == nil || ae == nil {
		return
	}
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		// No per-request hub, so fall back to the global one - this happens for background goroutines with no request-scoped context, like events bus async handlers
		hub = sentry.CurrentHub()
	}
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetFingerprint([]string{firstNonEmpty(ae.Code, "INTERNAL")})
		scope.SetTags(map[string]string{
			"apperr.code":  firstNonEmpty(ae.Code, ""),
			"apperr.class": apperr.ClassOf(ae).String(),
			"apperr.op":    ae.Op,
		})
		if file, line := ae.Source(); line > 0 {
			scope.SetTag("apperr.origin", filepath.Base(file)+":"+strconv.Itoa(line))
		}
		// CaptureException so Sentry parses the wrapped chain, including the stack the underlying SDK extracts - when the cause is itself an *apperr.Error, Sentry sees the full chain through Unwrap
		if ae.Cause != nil {
			hub.CaptureException(ae)
		} else {
			hub.CaptureMessage(ae.Error())
		}
	})
}

func (r *sentryReporter) CapturePanic(ctx context.Context, rec any) {
	if r == nil || rec == nil {
		return
	}
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub()
	}
	// RecoverWithContext returns an eventID we don't bubble, because the graphqlx layer already returns a wrapped *Error that the response surfaces via extensions.request_id
	hub.RecoverWithContext(ctx, rec)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

var _ Reporter = (*sentryReporter)(nil)
