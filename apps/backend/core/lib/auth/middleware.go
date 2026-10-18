package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// AdminMiddleware reads an admin access token for the admin surface - non-blocking, so a missing or invalid token is silently ignored and resolvers enforce authorization, and a customer token never counts here
func AdminMiddleware(keys AdminKeys) func(http.Handler) http.Handler {
	return bearer(func(r *http.Request, token string) *http.Request {
		if c, err := parseAdmin(token, keys); err == nil {
			return r.WithContext(WithAdmin(r.Context(), &Claims{AdminID: c.AdminID, SessionID: c.SessionID, Email: c.Email, IsSuperuser: c.IsSuperuser}))
		}
		return r
	})
}

// CustomerMiddleware is the storefront's half - only a customer token signs anyone in, so an admin token on a storefront route reads as anonymous
func CustomerMiddleware(v CustomerVerifier) func(http.Handler) http.Handler {
	return bearer(func(r *http.Request, token string) *http.Request {
		if c, err := parseCustomer(token, v); err == nil {
			return r.WithContext(WithCustomer(r.Context(), &CustomerClaims{CustomerID: c.CustomerID, ChannelID: c.ChannelID, Email: c.Email}))
		}
		return r
	})
}

func bearer(identify func(r *http.Request, token string) *http.Request) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(WithRequest(r.Context(), r))
			r = r.WithContext(WithRequestCache(r.Context())) // memoizes permission and scope lookups, so a gated resolver and every per-row channel filter resolve the admin's grants once rather than once per check
			if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
				r = identify(r, token)
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
