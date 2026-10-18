package channel

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// Resolves the current store for an incoming request and injects it into the context Order: the X-Channel-Id header (a uuid, not the guessable slug - siblings sharing one backend stay unenumerable), then the Host header matched against the host of channels.public_url (case-folded, default port dropped) so one domain per subsidiary works without extra headers, then the opt-in default storefront The header short-circuits: present but unresolvable means no channel, never a fallback to a different market - see resolve() Inactive channels resolve as not found on this surface - retired or pre-launch stores must not be reachable by id With no row flagged default, resolution returns ok=false and downstream resolvers see no store context The middleware NEVER fails the request - a resolver that requires a store calls FromContext and returns a domain error on ok=false, which keeps health checks, the dev playground and GraphQL introspection from being store-gated
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
	if raw := strings.TrimSpace(r.Header.Get(HeaderChannelID)); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			slog.Debug("channel: admin scope: malformed channel id header", "value", raw)
			return nil, false
		}
		sv, err := svc.GetByID(ctx, id)
		if err == nil {
			return &sv, true
		}
		logLookup("admin scope by id", raw, err)
		return nil, false
	}
	if slug := strings.TrimSpace(r.Header.Get(HeaderStoreSlug)); slug != "" {
		sv, err := svc.GetBySlug(ctx, slug)
		if err == nil {
			return &sv, true
		}
		logLookup("admin scope by slug", slug, err)
	}
	return nil, false
}

// A miss is the caller's mistake and logs at Debug; anything else is a failed lookup
func logLookup(what, key string, err error) {
	if errors.Is(err, ErrChannelNotFound) {
		slog.Debug("channel: no match", "lookup", what, "key", key)
		return
	}
	slog.Error("channel: lookup failed", "lookup", what, "key", key, "err", err)
}

func resolve(ctx context.Context, svc *Service, r *http.Request) (*Channel, bool) {
	// An explicit X-Channel-Id is a directive, not a hint - the same rule AdminMiddleware already follows. A storefront is pinned to exactly one channel, so a header that names a missing, inactive or malformed one means misconfiguration: a typo, a stale id, a config copied between environments, or a channel an operator deliberately retired. Falling through to Host and then to the default would answer with a DIFFERENT market's catalog, prices and currency while looking like it worked, so resolution stops here and resolvers report STORE_REQUIRED instead
	if raw := strings.TrimSpace(r.Header.Get(HeaderChannelID)); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			slog.Debug("channel: malformed channel id header", "value", raw)
			return nil, false
		}
		sv, err := svc.GetByID(ctx, id)
		if err == nil && sv.IsActive {
			return &sv, true
		}
		if err != nil {
			logLookup("id", raw, err)
		}
		return nil, false
	}
	if host := normalizeHost("", r.Host); host != "" {
		sv, err := svc.GetByStorefrontHost(ctx, host)
		if err == nil && sv.IsActive {
			return &sv, true
		}
		if err != nil {
			logLookup("host", host, err)
		}
		// Fall through to the opt-in default below
	}
	sv, err := svc.GetDefault(ctx)
	if err == nil && sv.IsActive {
		return &sv, true
	}
	if err != nil && !errors.Is(err, ErrChannelNotFound) {
		slog.Error("channel: resolve default", "err", err)
	}
	return nil, false
}
