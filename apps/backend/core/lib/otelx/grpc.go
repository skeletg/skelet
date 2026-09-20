package otelx

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

// The two ends of a gRPC hop - the client writes traceparent into the call's metadata, the server reads it back
// Empty options when OTel is off, so both sides wire them unconditionally

func GRPCClient() grpc.DialOption {
	if !enabled() {
		return grpc.EmptyDialOption{}
	}
	return grpc.WithStatsHandler(otelgrpc.NewClientHandler())
}

func GRPCServer() grpc.ServerOption {
	if !enabled() {
		return grpc.EmptyServerOption{}
	}
	return grpc.StatsHandler(otelgrpc.NewServerHandler())
}
