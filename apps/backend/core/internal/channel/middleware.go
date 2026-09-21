package channel

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// Resolves the current store for an incoming request and injects it into the context Order: the X-Channel-Id header (a uuid, not the guessable slug - siblings sharing one backend stay unenumerable), then the Host header matched against channels.public_url so one domain per subsidiary works without extra headers, then the opt-in default storefront The header short-circuits: present but unresolvable means no channel, never a fallback to a different market - see resolve() Inactive channels resolve as not found on this surface - retired or pre-launch stores must not be reachable by id With no row flagged default, resolution returns ok=false and downstream resolvers see no store context The middleware NEVER fails the request - a resolver that requires a store calls FromContext and returns a domain error on ok=false, which keeps health checks, the dev playground and GraphQL introspection from being store-gated
func Middleware(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if sv, ok := resolve(ctx, svc, r); ok {
				ctx = WithChannel(ctx, sv)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Resolves the admin's VIEWING SCOPE channel from an explicit request header - unlike Middleware it never falls back to the request host or the default store The admin scope is a deliberate directive from the dashboard's scope selector, so its absence means "no channel scope", i.e. all channels, NOT "the default store" - treat them as one rule and a Global-scope admin silently sees only the default store Order is the X-Channel-Id header, since the dashboard works in channel ids, then X-Store-Slug for tooling parity - A missing, malformed or unmatched header leaves the context without a channel, which viewing resolvers treat as all channels
func AdminMiddleware(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if sv, ok := resolveAdminScope(ctx, svc, r); ok {
				ctx = WithChannel(ctx, sv)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func resolveAdminScope(ctx context.Context, svc *Service, r *http.Request) (*Channel, bool) {
	if raw := strings.TrimSpace(r.Header.Get("X-Channel-Id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			log.Printf("[channel] admin scope: invalid X-Channel-Id %q: %v", raw, err)
			return nil, false
		}
		sv, err := svc.GetByID(ctx, id)
		if err == nil {
			return &sv, true
		}
		if !errors.Is(err, ErrChannelNotFound) {
			log.Printf("[channel] admin scope: resolve id %q: %v", raw, err)
		}
		return nil, false
	}
	if slug := strings.TrimSpace(r.Header.Get("X-Store-Slug")); slug != "" {
		sv, err := svc.GetBySlug(ctx, slug)
		if err == nil {
			return &sv, true
		}
		if !errors.Is(err, ErrChannelNotFound) {
			log.Printf("[channel] admin scope: resolve slug %q: %v", slug, err)
		}
	}
	return nil, false
}

func resolve(ctx context.Context, svc *Service, r *http.Request) (*Channel, bool) {
	// An explicit X-Channel-Id is a directive, not a hint - the same rule AdminMiddleware already follows. A storefront is pinned to exactly one channel, so a header that names a missing, inactive or malformed one means misconfiguration: a typo, a stale id, a config copied between environments, or a channel an operator deliberately retired. Falling through to Host and then to the default would answer with a DIFFERENT market's catalog, prices and currency while looking like it worked, so resolution stops here and resolvers report STORE_REQUIRED instead
	if raw := strings.TrimSpace(r.Header.Get("X-Channel-Id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			log.Printf("[channel] resolve: malformed X-Channel-Id %q", raw)
			return nil, false
		}
		sv, err := svc.GetByID(ctx, id)
		if err == nil && sv.IsActive {
			return &sv, true
		}
		if err != nil && !errors.Is(err, ErrChannelNotFound) {
			log.Printf("[channel] resolve by id %q: %v", raw, err)
		}
		return nil, false
	}
	if host := hostFromRequest(r); host != "" {
		sv, err := svc.GetByStorefrontHost(ctx, host)
		if err == nil && sv.IsActive {
			return &sv, true
		}
		if err != nil && !errors.Is(err, ErrChannelNotFound) {
			log.Printf("[channel] resolve by host %q: %v", host, err)
		}
		// Fall through to the opt-in default below
	}
	sv, err := svc.GetDefault(ctx)
	if err == nil && sv.IsActive {
		return &sv, true
	}
	if err != nil && !errors.Is(err, ErrChannelNotFound) {
		log.Printf("[channel] resolve default: %v", err)
	}
	return nil, false
}

// The host portion of r.Host without the port, so a store registered as "https://wheels.example.com" resolves from a request to either "wheels.example.com:443" or "wheels.example.com"
func hostFromRequest(r *http.Request) string {
	host := r.Host
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	return strings.ToLower(strings.TrimSpace(host))
}
