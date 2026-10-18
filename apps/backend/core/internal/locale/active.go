package locale

import (
	"context"
	"strings"
)

type DefaultLocaleProvider interface {
	DefaultLocale(ctx context.Context) (string, error)
}

// The effective locale for a read request, in order: the header from request context, then the system-wide default from `provider`, then "" - which services treat as no preference and resolve per-entity via their populate*Locale helpers
// channel.FromContext is deliberately NOT folded in, since channel imports this package for locale-code validation and the reverse would cycle - a resolver wanting the store default first can do `if tag == "" { tag = sv.DefaultLocale }` itself, and in practice the two match
func Active(ctx context.Context, provider DefaultLocaleProvider) string {
	if tag := FromContext(ctx); tag != "" {
		return tag
	}
	if provider != nil {
		code, err := provider.DefaultLocale(ctx)
		if err == nil {
			return strings.TrimSpace(code)
		}
	}
	return ""
}
