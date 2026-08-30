package locale

import (
	"net/http"
	"strings"
)

// The HTTP header carrying the request's preferred content locale - a single BCP-47 tag, with no weighted lists or fallback chains in the value
const HeaderName = "Locale"

func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if tag := strings.TrimSpace(r.Header.Get(HeaderName)); tag != "" && LooksLikeCode(tag) { // a malformed tag only misses every lookup, and carrying it puts junk in cache keys and term filters
				ctx = WithLocale(ctx, tag)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
