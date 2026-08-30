package dataloaders

import (
	"context"
	"net/http"
)

type loadersCtxKey struct{} // a struct{} key can't collide with anyone else putting strings in the ctx

func WithLoaders(ctx context.Context, l *Loaders) context.Context {
	return context.WithValue(ctx, loadersCtxKey{}, l)
}

func For(ctx context.Context) *Loaders { // nil when the middleware wasn't mounted (tests, CLI, internal call paths) - callers MUST fall back to the underlying service method, mirroring render_cache.getRenderCache
	l, _ := ctx.Value(loadersCtxKey{}).(*Loaders)
	return l
}

// Per-request lifetime is the entire point - DataLoader in-memory caches must not bleed across requests, and a long-running process would otherwise pile up stale entries; mount it just outside render_cache.RenderCacheMiddleware so both context values reach every resolver
func Middleware(deps Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithLoaders(r.Context(), New(deps))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
