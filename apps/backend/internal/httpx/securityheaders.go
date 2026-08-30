package httpx

import (
	"net/http"
	"strings"
)

// Security response headers - the app-layer copy of what the prod edge also sets, so a deployment behind any other proxy, or none, still ships a CSP instead of silently losing it
// GENERATED VALUES: cmd/secheadgen (a bin/codegen step) rewrites the two const lines below from
// ComposeSecurityHeaders plus every compiled-in plugin's Manifest.SecurityOrigins, then mirrors them
// into apps/dashboard/nginx.conf.template and deploy/caddy/Caddyfile - bin/contract-check re-verifies
// the equality without a toolchain. Edit securityorigins.go (core baseline) or a plugin manifest, never
// these lines - apps/channels/*/next.config.ts is deliberately narrower, having no gateway SDK to allow
const ContentSecurityPolicy = "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; font-src 'self' data:; connect-src 'self'; frame-src 'none'; frame-ancestors 'self'; base-uri 'self'; form-action 'self'"

const PermissionsPolicy = `camera=(), microphone=(), geolocation=(), payment=(self)`

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/playground/") {
			h := w.Header()
			h.Set("Content-Security-Policy", ContentSecurityPolicy)
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "SAMEORIGIN")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", PermissionsPolicy)
		}
		next.ServeHTTP(w, r)
	})
}
