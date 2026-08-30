// Package locale carries the request-scoped content locale through context.Context, mirroring the channel and auth pattern - HTTP middleware extracts it once per request and resolvers consume it through the package's helpers rather than threading a locale arg through every signature
package locale

import (
	"context"
	"strings"
)

type ctxKey struct{}

func WithLocale(ctx context.Context, tag string) context.Context {
	tag = strings.ToLower(strings.TrimSpace(tag)) // codes are stored lowercase and content lookups match exactly, so an un-normalised "DE" missed every translation and fell back to the store default
	if tag == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, tag)
}

func FromContext(ctx context.Context) string {
	s, _ := ctx.Value(ctxKey{}).(string)
	return s
}
