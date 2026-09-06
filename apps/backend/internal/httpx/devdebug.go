package httpx

import (
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"go.opentelemetry.io/otel/trace"

	db "github.com/skeletg/skelet/apps/backend/internal/db"
)

// DEV ONLY, wired behind APP_ENV=development - plants a per-request DB query counter on the context and answers with X-DB-Queries and X-Trace-Id, which the dashboard's dev inspector reads for a one-click jump to the Jaeger trace
// Headers must be set before the first body byte, so the writer wraps WriteHeader and Write and injects lazily - by the time a GraphQL response starts writing, resolution and its queries are done - Queries fired during response streaming are missed, which is acceptable for a debugging aid
func DevDebugMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// WebSocket upgrades need the raw writer, since devDebugWriter doesn't implement http.Hijacker and debug headers are meaningless on a socket - pass upgrades straight through
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			next.ServeHTTP(w, r)
			return
		}
		ctx, counter := db.WithQueryCounter(r.Context())
		dw := &devDebugWriter{ResponseWriter: w, counter: counter}
		if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
			dw.traceID = sc.TraceID().String()
		}
		next.ServeHTTP(dw, r.WithContext(ctx))
	})
}

type devDebugWriter struct {
	http.ResponseWriter
	counter *atomic.Int64
	traceID string
	wrote   bool
}

func (w *devDebugWriter) inject() {
	if w.wrote {
		return
	}
	w.wrote = true
	w.Header().Set("X-DB-Queries", strconv.FormatInt(w.counter.Load(), 10))
	if w.traceID != "" {
		w.Header().Set("X-Trace-Id", w.traceID)
	}
	// The dashboard runs on a different origin in some setups, so expose the debug headers to fetch() readers
	w.Header().Add("Access-Control-Expose-Headers", "X-DB-Queries, X-Trace-Id")
}

func (w *devDebugWriter) WriteHeader(statusCode int) {
	w.inject()
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *devDebugWriter) Write(b []byte) (int, error) {
	w.inject()
	return w.ResponseWriter.Write(b)
}
