package sentryx

import (
	"net/http"
	"strings"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"

	"github.com/skeletg/skelet/apps/backend/internal/httpx"
)

// Gives each request its own Sentry Hub tagged with request_id and surface - returns the identity middleware when disabled, so callers wire it unconditionally
func Middleware(enabled bool) func(http.Handler) http.Handler {
	if !enabled {
		return identity
	}
	wrap := sentryhttp.New(sentryhttp.Options{
		// Repanic so httpx.Recover above us can finish the 500 - without it sentryhttp swallows the panic and the client hangs
		Repanic:         true,
		WaitForDelivery: false,
	}).Handle
	return func(next http.Handler) http.Handler {
		return wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// sentryhttp.Handle pushes a per-request Hub onto the context - configure its scope before delegating downstream, so later CaptureException and Breadcrumb calls carry the request metadata
			if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
				hub.Scope().SetTag("request_id", httpx.RequestIDFromContext(r.Context()))
				hub.Scope().SetTag("surface", surfaceFromPath(r.URL.Path))
			}
			next.ServeHTTP(w, r)
		}))
	}
}

// Classifies a request URL into one of the known surfaces, with anything else falling into "other" - a closed tag set beats forking the index on every random path
func surfaceFromPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/api/admin/"):
		return "admin"
	case strings.HasPrefix(path, "/api/front/"):
		return "front"
	case strings.HasPrefix(path, "/api/v1/"):
		return "rest"
	case strings.HasPrefix(path, "/blog/"), path == "/sitemap.xml":
		return "public"
	case strings.HasPrefix(path, "/playground/"):
		return "playground"
	case path == "/health":
		return "health"
	default:
		return "other"
	}
}

func identity(next http.Handler) http.Handler { return next }
