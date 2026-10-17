package auth

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ServiceUnaryInterceptor puts the caller a service token names on the context - audience-scoped, read off the authorization metadata
// Non-blocking: a bad or revoked token injects nothing and the handler's own permission check refuses - a nil revoked list skips that check
func ServiceUnaryInterceptor(audience string, keys ServiceKeys, revoked Revocations) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(serviceContext(ctx, audience, keys, revoked), req)
	}
}

// The streaming half - a wrapped stream is the only way to hand a handler a different context
func ServiceStreamInterceptor(audience string, keys ServiceKeys, revoked Revocations) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, contextStream{ServerStream: ss, ctx: serviceContext(ss.Context(), audience, keys, revoked)})
	}
}

type contextStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s contextStream) Context() context.Context { return s.ctx }

func serviceContext(ctx context.Context, audience string, keys ServiceKeys, revoked Revocations) context.Context {
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
		if c, err := ValidateServiceToken(after, audience, keys); err == nil {
			if c.System { // carries no admin and no grants, so only a handler asking for it by name lets it through
				return WithSystemCaller(ctx)
			}
			if c.CustomerID != uuid.Nil { // authority over one shopper's own data, nothing else
				return WithCustomerCaller(ctx, c.CustomerID)
			}
			if revoked != nil && c.SessionID != uuid.Nil && revoked.Revoked(ctx, c.SessionID) {
				return ctx // logged out since the token was minted - it still verifies, so this is the only place that can refuse it
			}
			return WithGrantChecker(WithAdmin(ctx, c.Admin()), c.Checker())
		}
	}
	return ctx
}
