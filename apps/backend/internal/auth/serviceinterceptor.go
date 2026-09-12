package auth

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ServiceUnaryInterceptor is ServiceMiddleware for gRPC - same audience-scoped token, read off the
// authorization metadata instead of the header
// Non-blocking like the HTTP one: a bad token injects nothing and the handler's own permission check
// refuses, so one place decides what each method needs
func ServiceUnaryInterceptor(audience, secret string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(serviceContext(ctx, audience, secret), req)
	}
}

// The streaming half - a wrapped stream is the only way to hand a handler a different context
func ServiceStreamInterceptor(audience, secret string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, contextStream{ServerStream: ss, ctx: serviceContext(ss.Context(), audience, secret)})
	}
}

type contextStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s contextStream) Context() context.Context { return s.ctx }

func serviceContext(ctx context.Context, audience, secret string) context.Context {
	ctx = WithRequestCache(ctx)
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	for _, v := range md.Get("authorization") {
		after, found := strings.CutPrefix(v, "Bearer ")
		if !found {
			continue
		}
		if c, err := ValidateServiceToken(after, audience, secret); err == nil {
			if c.System { // carries no admin and no grants, so only a handler asking for it by name lets it through
				return WithSystemCaller(ctx)
			}
			return WithGrantChecker(WithAdmin(ctx, c.Admin()), c.Checker())
		}
	}
	return ctx
}
