package httpx

import (
	"net/http"
	"strings"
)

// Caps each request body to maxBytes - when a handler reads past it, http.MaxBytesReader fails the read and responds 413, so an oversized request can't exhaust memory
// LimitBody caps every request body except the paths in exempt, which carry deliberate bulk uploads and enforce
// their own limit - a global cap sized for JSON payloads would otherwise reject a file the feature exists to accept
func LimitBody(maxBytes int64, next http.Handler, exempt ...string) http.Handler {
	if maxBytes <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, p := range exempt {
			if strings.HasPrefix(r.URL.Path, p) {
				next.ServeHTTP(w, r)
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next.ServeHTTP(w, r)
	})
}
