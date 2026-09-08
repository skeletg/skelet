package auth

import (
	"net/http"
	"strings"
)

// ServiceMiddleware puts the caller and their carried grants on the request from a token for one audience, non-blocking like Middleware so a bad one injects nothing and the resolvers refuse
func ServiceMiddleware(audience, secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithRequest(r.Context(), r)
			ctx = WithRequestCache(ctx)
			if after, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
				if c, err := ValidateServiceToken(after, audience, secret); err == nil {
					ctx = WithAdmin(ctx, c.Admin())
					ctx = WithGrantChecker(ctx, c.Checker())
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
