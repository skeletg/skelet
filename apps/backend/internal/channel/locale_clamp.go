package channel

import (
	"net/http"
	"strings"

	"github.com/skeletg/skelet/internal/locale"
)

// Snaps the request's content locale to the active channel's supported set - runs AFTER both Middleware (channel resolution) and locale.Middleware (header extraction)
func LocaleClampMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if sv, ok := FromContext(ctx); ok && sv != nil {
				requested := locale.FromContext(ctx)
				if clamped := ClampLocale(*sv, requested); clamped != "" && clamped != requested {
					ctx = locale.WithLocale(ctx, clamped)
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// The effective locale for one channel, in order: the requested code when it matches a supported one, case-insensitively, returning the canonical stored code so cache keys and index term filters stay uniform
func ClampLocale(sv Channel, requested string) string {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		for _, l := range sv.Locales {
			// Globally deactivated locales aren't servable, except the channel default, which always resolves so the store can't go dark from a locale deactivation
			if !l.IsActive && !strings.EqualFold(l.Code, sv.DefaultLocale) {
				continue
			}
			if strings.EqualFold(l.Code, requested) {
				return l.Code
			}
		}
		if len(sv.Locales) == 0 && strings.EqualFold(sv.DefaultLocale, requested) {
			return sv.DefaultLocale
		}
	}
	if sv.DefaultLocale != "" {
		return sv.DefaultLocale
	}
	return requested
}
