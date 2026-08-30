package otelx

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Emits a span per outbound request and injects traceparent so downstream services running their own OTel continue the trace - returns base unchanged when OTel is off, so callers wire unconditionally
func HTTPTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if !enabled() {
		return base
	}
	return otelhttp.NewTransport(base)
}
