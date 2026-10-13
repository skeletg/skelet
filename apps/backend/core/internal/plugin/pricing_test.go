package plugin

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

type fixedPrice struct {
	cents int64
	calls int
}

func (f *fixedPrice) ResolveUnitPrice(context.Context, sdk.PriceRequest) (int64, bool, error) {
	f.calls++
	return f.cents, true, nil
}

func withPriceResolver(t *testing.T, r sdk.PriceResolver) {
	t.Helper()
	ResetPriceResolversForTest()
	t.Cleanup(ResetPriceResolversForTest)
	if err := (&Host{}).RegisterPriceResolver("pricer", r); err != nil {
		t.Fatal(err)
	}
}

// A negative price would sell below zero - it reads as no plugin price, and list pricing applies
func TestNegativePluginPriceFallsBack(t *testing.T) {
	swapGates(t, true, nil)
	withPriceResolver(t, &fixedPrice{cents: -500})
	if cents, ok := ResolveUnitPrice(context.Background(), sdk.PriceRequest{ChannelID: uuid.New()}); ok {
		t.Errorf("negative price used: %d", cents)
	}
}

func TestPluginPriceIsUsed(t *testing.T) {
	swapGates(t, true, nil)
	withPriceResolver(t, &fixedPrice{cents: 1250})
	if cents, ok := ResolveUnitPrice(context.Background(), sdk.PriceRequest{ChannelID: uuid.New()}); !ok || cents != 1250 {
		t.Errorf("got %d %v, want 1250 true", cents, ok)
	}
}

// A closed gate still ends the hook's span - leaving it open leaked one span and timer per price read while the plugin was off
func TestGatedPriceResolverEndsItsSpan(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	orig := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	t.Cleanup(func() { otel.SetTracerProvider(orig) })

	swapGates(t, false, nil)
	f := &fixedPrice{cents: 100}
	withPriceResolver(t, f)
	if _, ok := ResolveUnitPrice(context.Background(), sdk.PriceRequest{ChannelID: uuid.New()}); ok {
		t.Error("a disabled resolver priced the line")
	}
	if f.calls != 0 {
		t.Error("a disabled resolver was called")
	}
	if started, ended := len(rec.Started()), len(rec.Ended()); started != 1 || ended != 1 {
		t.Errorf("spans started %d, ended %d - want 1 and 1", started, ended)
	}
}
