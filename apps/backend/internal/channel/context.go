package channel

import "context"

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
