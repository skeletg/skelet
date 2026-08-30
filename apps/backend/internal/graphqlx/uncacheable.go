package graphqlx

import "context"

// A response can be a clean HTTP 200 with no GraphQL errors and still be junk -
// the storefront's search paths soft-fail to an EMPTY result when OpenSearch is
// down, which the response cache would then serve for its whole TTL, keeping the
// catalogue empty for a minute after the cluster recovered.
// Resolvers that degrade rather than error mark the request here, and the cache
// middleware declines to store it.
type uncacheableKey struct{}

type uncacheable struct{ set bool }

// WithUncacheableFlag installs the flag holder - the caching middleware calls this before serving
func WithUncacheableFlag(ctx context.Context) context.Context {
	return context.WithValue(ctx, uncacheableKey{}, &uncacheable{})
}

// MarkUncacheable records that this response is degraded and must not be cached - safe to call when no holder is installed
func MarkUncacheable(ctx context.Context) {
	if u, ok := ctx.Value(uncacheableKey{}).(*uncacheable); ok {
		u.set = true
	}
}

// IsUncacheable reports whether a resolver degraded during this request
func IsUncacheable(ctx context.Context) bool {
	u, ok := ctx.Value(uncacheableKey{}).(*uncacheable)
	return ok && u.set
}
