package otelx

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Adds OTel server instrumentation - no-op-safe when OTel is uninitialised, so call it unconditionally
func WrapHTTP(handler http.Handler, route string) http.Handler {
	return otelhttp.NewHandler(handler, route,
		otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
	)
}
