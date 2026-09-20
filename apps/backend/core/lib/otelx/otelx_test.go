package otelx_test

import (
	"context"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/otelx"
)

func TestInitNoEnvReturnsNoOp(t *testing.T) {
	for _, k := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_TRACES_EXPORTER",
		"OTEL_METRICS_EXPORTER",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
	} {
		t.Setenv(k, "")
	}
	shutdown, err := otelx.Init(context.Background(), "test")
	if err != nil {
		t.Fatalf("Init with no env should not error, got %v", err)
	}
	if shutdown == nil {
		t.Fatal("Init must always return a non-nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown returned %v", err)
	}
}
