package auth

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// A service authorizing from a token cannot see admin_sessions, so core publishes the killed id and the service checks that list per request

// Revocations is the shared list of sessions killed since their tokens were minted, and entries only have to outlive a token so an implementation is expected to expire them
type Revocations interface {
	Revoke(ctx context.Context, sessionID uuid.UUID)
	Revoked(ctx context.Context, sessionID uuid.UUID) bool
}

// ServiceSessionGuard drops the caller when their session was killed, running AFTER ServiceMiddleware since it reads the injected claims, and a token with no session id passes through
func ServiceSessionGuard(list Revocations) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if list != nil {
				if claims, ok := AdminFromContext(r.Context()); ok && claims.SessionID != uuid.Nil && list.Revoked(r.Context(), claims.SessionID) {
					ctx := WithGrantChecker(WithAdmin(r.Context(), nil), nil)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
