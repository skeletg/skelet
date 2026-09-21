package dataloaders

import (
	"context"
	"testing"

	"github.com/graph-gophers/dataloader/v7"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// The whole plugin loader path without booting the real host - fake registration in, Plugin[K, V] out, and the batch fn had better fire once for two concurrent Loads
func TestPluginAccessorRoundTrip(t *testing.T) {
	calls := 0
	reg := fakePluginReg{
		slug: "test.echo",
		build: func() any {
			return dataloader.NewBatchedLoader(
				func(_ context.Context, keys []int) []*dataloader.Result[string] {
					calls++
					out := make([]*dataloader.Result[string], len(keys))
					for i, k := range keys {
						out[i] = &dataloader.Result[string]{Data: stringFromInt(k)}
					}
					return out
				},
				dataloader.WithWait[int, string](sdk.DataLoaderWait),
				dataloader.WithCache[int, string](dataloader.NewCache[int, string]()),
			)
		},
	}

	loaders := &Loaders{} // only the plugin reg is wired - the core loaders need real services, and the plugin map is independent of them
	loaders.plugin = map[string]any{reg.slug: reg.build()}

	loader, ok := Plugin[int, string](loaders, "test.echo")
	if !ok || loader == nil {
		t.Fatalf("Plugin[int,string] retrieval failed: ok=%v loader=%v", ok, loader)
	}

	ctx := context.Background()
	t1 := loader.Load(ctx, 2) // both Loads should coalesce into one batch fn call - the 250µs wait window dwarfs in-process call latency
	t2 := loader.Load(ctx, 5)
	v1, err := t1()
	if err != nil {
		t.Fatalf("t1: %v", err)
	}
	v2, err := t2()
	if err != nil {
		t.Fatalf("t2: %v", err)
	}
	if v1 != "2" || v2 != "5" {
		t.Errorf("values: got (%q, %q), want (\"2\", \"5\")", v1, v2)
	}
	if calls != 1 {
		t.Errorf("batch fn called %d times, want 1 - batching broken", calls)
	}

	if _, ok := Plugin[string, string](loaders, "test.echo"); ok { // a wrong type assertion returns (nil, false), not a panic
		t.Error("Plugin with wrong K returned ok=true; type-mismatch should fail closed")
	}

	if _, ok := Plugin[int, string](loaders, "test.missing"); ok { // unknown slug is (nil, false) too
		t.Error("Plugin with unknown slug returned ok=true")
	}
}

// Plugin has to tolerate a nil *Loaders and an empty plugin map - same fail-open posture as For(ctx) with the middleware unmounted
func TestPluginAccessorNilSafe(t *testing.T) {
	if _, ok := Plugin[int, string](nil, "x"); ok {
		t.Error("Plugin(nil, ...) returned ok=true")
	}
	if _, ok := Plugin[int, string](&Loaders{}, "x"); ok {
		t.Error("Plugin on bundle with no plugin map returned ok=true")
	}
}

type fakePluginReg struct {
	slug  string
	build func() any
}

func (r fakePluginReg) Slug() string { return r.slug }
func (r fakePluginReg) Build() any   { return r.build() }

func stringFromInt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
