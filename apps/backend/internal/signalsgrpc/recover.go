package signalsgrpc

import (
	"context"
	"runtime/debug"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/internal/applog"
)

// A panicking handler must not take the whole server down with it - one bad method should answer
// INTERNAL and leave every other call on that connection working, the way the HTTP surfaces already do
// The panic value never reaches the caller, since it can carry anything the handler was holding
func recoverUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if rec := recover(); rec != nil {
				logPanic(ctx, info.FullMethod, rec)
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

func recoverStream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if rec := recover(); rec != nil {
				logPanic(ss.Context(), info.FullMethod, rec)
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(srv, ss)
	}
}

func logPanic(ctx context.Context, method string, rec any) {
	applog.From(ctx).Error("signals grpc handler panic recovered",
		"method", method, "err", rec, "stack", string(debug.Stack()))
}
