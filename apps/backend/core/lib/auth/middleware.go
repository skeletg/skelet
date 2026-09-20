package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

func Middleware(secret string) func(http.Handler) http.Handler { // non-blocking, so a missing or invalid token is silently ignored and resolvers enforce authorization
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(WithRequest(r.Context(), r))
			r = r.WithContext(WithRequestCache(r.Context())) // memoizes permission and scope lookups, so a gated resolver and every per-row channel filter resolve the admin's grants once rather than once per check
			header := r.Header.Get("Authorization")
			if after, ok := strings.CutPrefix(header, "Bearer "); ok {
				tokenStr := after
				if c, err := parse(tokenStr, secret); err == nil {
					switch c.Type {
					case "customer":
						r = r.WithContext(WithCustomer(r.Context(), &CustomerClaims{
							CustomerID: c.CustomerID,
							ChannelID:  c.ChannelID,
							Email:      c.Email,
						}))
					case "admin":
						r = r.WithContext(WithAdmin(r.Context(), &Claims{
							AdminID:     c.AdminID,
							SessionID:   c.SessionID,
							Email:       c.Email,
							IsSuperuser: c.IsSuperuser,
						}))
					default: // a type-less or unknown token injects nothing - reads as anonymous instead of falling through to admin
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

type CustomerActiveFunc func(ctx context.Context, customerID uuid.UUID) (bool, error) // lives here so auth doesn't import customer

// Without this a fraud team's "disable" leaves the existing access token working for the rest of its ~15-min TTL, long enough to drain store credit or place orders - runs AFTER Middleware since it reads the injected claims, costs one indexed lookup per AUTHENTICATED request (guests skip it), and a store error fails OPEN rather than logging everyone out on a blip
func CustomerSessionGuard(active CustomerActiveFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if claims, ok := CustomerFromContext(r.Context()); ok && claims.CustomerID != uuid.Nil && active != nil {
				if ok, err := active(r.Context(), claims.CustomerID); err == nil && !ok {
					next.ServeHTTP(w, r.WithContext(WithCustomer(r.Context(), nil)))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Every logout path and password reset deletes the admin_sessions row, so a revoked session's already-issued access token stops authenticating immediately instead of living out its ~15-min JWT TTL - runs AFTER Middleware since it reads the injected claims, tokens with no session id (dev-impersonate, and any path that mints without a session row) pass through unchanged, and a store error fails OPEN rather than locking admins out on a blip
func AdminSessionGuard(store *RefreshStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if claims, ok := AdminFromContext(r.Context()); ok && claims.SessionID != uuid.Nil {
				if active, err := store.SessionActive(r.Context(), claims.AdminID, claims.SessionID); err == nil && !active {
					next.ServeHTTP(w, r.WithContext(WithAdmin(r.Context(), nil)))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
