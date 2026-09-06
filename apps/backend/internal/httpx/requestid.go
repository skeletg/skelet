// Package httpx holds reusable net/http middlewares that don't belong to a domain - request-id stamping and panic recovery
package httpx

import (
	"context"
	"net/http"
	"regexp"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/applog"
)

const HeaderRequestID = "X-Request-Id"

type requestIDKey struct{}

// A client-supplied id is only honored when it looks like an id - anything else would let a caller
// inject arbitrary bytes into every log line and the response header
var requestIDShape = regexp.MustCompile(`^[A-Za-z0-9._-]{4,64}$`)

// Assigns each request an id, read from X-Request-Id when present and valid and generated otherwise, echoes it in the response header, and stamps both the raw id and a pre-tagged logger into the context
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if !requestIDShape.MatchString(id) {
			id = uuid.NewString()
		}
		w.Header().Set(HeaderRequestID, id)

		ctx := r.Context()
		ctx = context.WithValue(ctx, requestIDKey{}, id)
		logger := applog.From(ctx).With("request_id", id)
		ctx = applog.Into(ctx, logger)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}
