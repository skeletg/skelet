package channel

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey struct{}

func WithChannel(ctx context.Context, sv *Channel) context.Context {
	if sv == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, sv)
}

// The current store, with ok=true when one was resolved for this request - storefront-only resolvers should treat ok=false as a hard STORE_REQUIRED error, while admin paths can ignore it
func FromContext(ctx context.Context) (*Channel, bool) {
	sv, ok := ctx.Value(ctxKey{}).(*Channel)
	return sv, ok && sv != nil
}

// IDFromContext is the current store's id, uuid.Nil outside a storefront request
func IDFromContext(ctx context.Context) uuid.UUID {
	if sv, ok := FromContext(ctx); ok {
		return sv.ID
	}
	return uuid.Nil
}
