package httpx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/applog"
)

// Catches panics in handlers downstream and converts them into a structured 500 - the full stack is logged, and the client sees only request_id plus a generic message
// It sits OUTSIDE the gqlgen handler so panics in middleware (auth, channel, integration token parsing) are caught too, since gqlgen's own RecoverFunc only fires inside resolver execution
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// track whether downstream already sent headers - WriteHeader(500) after a partial response
		// logs "superfluous WriteHeader" and delivers a truncated body under the ORIGINAL status,
		// which reads as success to the client
		ww := &writeTracker{ResponseWriter: w}
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			err := panicAsError(rec)
			// ctx first: the header is only populated once RequestID (which runs inside this
			// middleware) has executed - a panic in an earlier middleware left it empty
			requestID := RequestIDFromContext(r.Context())
			if requestID == "" {
				requestID = w.Header().Get(HeaderRequestID)
			}
			applog.From(r.Context()).Error(
				"http panic recovered",
				"err", err.Error(),
				"path", r.URL.Path,
				"method", r.Method,
				"stack", string(debug.Stack()),
			)
			if !ww.wrote {
				writeInternalErrorJSON(w, requestID)
			}
		}()
		// A websocket upgrade needs the raw writer hijacked - writeTracker doesn't implement
		// http.Hijacker, same reasoning as devDebugWriter in devdebug.go - the defer above still
		// catches a panic on this path since it closes over w, not ww
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(ww, r)
	})
}

type writeTracker struct {
	http.ResponseWriter
	wrote bool
}

func (t *writeTracker) WriteHeader(code int) {
	t.wrote = true
	t.ResponseWriter.WriteHeader(code)
}

func (t *writeTracker) Write(b []byte) (int, error) {
	t.wrote = true
	return t.ResponseWriter.Write(b)
}

// Flush keeps streaming handlers (feeds, exports) working through the tracker
func (t *writeTracker) Flush() {
	if f, ok := t.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Normalises whatever was passed to panic() into an error the apperr layer can carry - strings and arbitrary values get wrapped, existing errors are reused
func panicAsError(rec any) error {
	switch v := rec.(type) {
	case error:
		return apperr.WrapClass(v, apperr.ClassInternal, "httpx.Recover")
	default:
		return apperr.WrapClass(
			fmt.Errorf("panic: %v", v),
			apperr.ClassInternal,
			"httpx.Recover",
		)
	}
}

// Writes the panic response body, ignoring the original handler's status, since recover always overrides because the downstream code never finished
// The body matches the GraphQL error extension shape, so a dashboard handling a 500 versus a 200-with-errors needs only one parser
func writeInternalErrorJSON(w http.ResponseWriter, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	body := map[string]any{
		"error": map[string]any{
			"code":       "INTERNAL",
			"class":      apperr.ClassInternal.String(),
			"message":    "internal error",
			"request_id": requestID,
		},
	}
	_ = json.NewEncoder(w).Encode(body)
}
