package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// What a core delegation stub declares privately - the impl below never imports it, so this is the
// structural coupling the whole design rests on
type demoAdmin interface {
	Connected(ctx context.Context) (bool, error)
}

type demoImpl struct{ panics bool }

func (d *demoImpl) Connected(context.Context) (bool, error) {
	if d.panics {
		panic("resolver exploded")
	}
	return true, nil
}

// Satisfies nothing demoAdmin asks for - stands in for a plugin whose SDL and Go methods drifted
type driftedImpl struct{}

func (driftedImpl) SomethingElse(context.Context) (bool, error) { return false, nil }

func resetGraph(t *testing.T) *Host {
	t.Helper()
	clear := func() {
		graphRegistry.mu.Lock()
		graphRegistry.impls = nil
		graphRegistry.mu.Unlock()
	}
	clear()
	t.Cleanup(clear)
	return &Host{}
}

func callDemo(ctx context.Context) (bool, error) {
	return Do(ctx, sdk.GraphAdmin, "demo", "connected",
		func(ctx context.Context, impl demoAdmin) (bool, error) { return impl.Connected(ctx) })
}

func TestGraphResolverResolves(t *testing.T) {
	h := resetGraph(t)
	if err := h.RegisterGraphResolver("demo", sdk.GraphAdmin, &demoImpl{}); err != nil {
		t.Fatal(err)
	}
	got, err := callDemo(context.Background())
	if err != nil || !got {
		t.Fatalf("Do returned (%v, %v), want (true, nil)", got, err)
	}
}

// A plugin that isn't installed must not look like a broken one
func TestMissingResolverIsTyped(t *testing.T) {
	resetGraph(t)
	_, err := callDemo(context.Background())
	if !errors.Is(err, ErrNoGraphResolver) {
		t.Errorf("missing resolver gave %v, want ErrNoGraphResolver", err)
	}
}

// The whole point of structural matching is that drift is caught, not silently mis-resolved
func TestDriftedResolverIsRejected(t *testing.T) {
	h := resetGraph(t)
	if err := h.RegisterGraphResolver("demo", sdk.GraphAdmin, driftedImpl{}); err != nil {
		t.Fatal(err)
	}
	_, err := callDemo(context.Background())
	if err == nil || !strings.Contains(err.Error(), "does not implement") {
		t.Errorf("drifted resolver gave %v, want a does-not-implement error", err)
	}
}

// Boot-time is where drift should surface - a deploy failing beats the first query failing
func TestRequireGraphResolverCatchesDrift(t *testing.T) {
	h := resetGraph(t)
	if err := h.RegisterGraphResolver("demo", sdk.GraphAdmin, driftedImpl{}); err != nil {
		t.Fatal(err)
	}
	if err := RequireGraphResolver[demoAdmin](sdk.GraphAdmin, "demo"); err == nil {
		t.Error("boot check passed a resolver that doesn't implement the stub's interface")
	}
}

func TestRequireGraphResolverPassesWhenCorrect(t *testing.T) {
	h := resetGraph(t)
	if err := h.RegisterGraphResolver("demo", sdk.GraphAdmin, &demoImpl{}); err != nil {
		t.Fatal(err)
	}
	if err := RequireGraphResolver[demoAdmin](sdk.GraphAdmin, "demo"); err != nil {
		t.Errorf("boot check rejected a correct resolver: %v", err)
	}
}

// An uninstalled plugin has no SDL in the build either, so there's nothing to satisfy
func TestRequireGraphResolverSkipsUninstalled(t *testing.T) {
	resetGraph(t)
	if err := RequireGraphResolver[demoAdmin](sdk.GraphAdmin, "demo"); err != nil {
		t.Errorf("boot check failed on a plugin that isn't installed: %v", err)
	}
}

// Plugin GraphQL has to inherit the kill switch every other plugin surface has
func TestDisabledPluginResolverIsGated(t *testing.T) {
	h := resetGraph(t)
	if err := h.RegisterGraphResolver("demo", sdk.GraphAdmin, &demoImpl{}); err != nil {
		t.Fatal(err)
	}
	disableAll(t)
	_, err := callDemo(context.Background())
	if !errors.Is(err, ErrPluginDisabled) {
		t.Errorf("disabled plugin's resolver gave %v, want ErrPluginDisabled", err)
	}
}

// A resolver that dies fails its own field rather than taking the GraphQL request down
func TestPanickingResolverBecomesError(t *testing.T) {
	h := resetGraph(t)
	if err := h.RegisterGraphResolver("demo", sdk.GraphAdmin, &demoImpl{panics: true}); err != nil {
		t.Fatal(err)
	}
	got, err := callDemo(context.Background())
	if !errors.Is(err, ErrPluginPanicked) {
		t.Errorf("panicking resolver gave %v, want ErrPluginPanicked", err)
	}
	if got {
		t.Error("panicking resolver returned a usable value")
	}
}

// The instrumented ctx has to reach the resolver, or the timeout, span and tagged logger are all
// silently dropped - closing over the caller's ctx instead is what this guards against
func TestResolverReceivesInstrumentedContext(t *testing.T) {
	h := resetGraph(t)
	if err := h.RegisterGraphResolver("demo", sdk.GraphAdmin, &demoImpl{}); err != nil {
		t.Fatal(err)
	}
	outer := context.Background()
	var inner context.Context
	_, err := Do(outer, sdk.GraphAdmin, "demo", "connected",
		func(ctx context.Context, impl demoAdmin) (bool, error) {
			inner = ctx
			return impl.Connected(ctx)
		})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := inner.Deadline(); !ok {
		t.Error("resolver ran without Instrument's hook timeout - the ctx was not threaded through")
	}
}

func TestResolverGatesOnRequestChannel(t *testing.T) {
	h := resetGraph(t)
	if err := h.RegisterGraphResolver("demo", sdk.GraphAdmin, &demoImpl{}); err != nil {
		t.Fatal(err)
	}
	want := uuid.New()
	var got uuid.UUID
	orig := isEnabledFromContext
	t.Cleanup(func() { isEnabledFromContext = orig })
	isEnabledFromContext = func(_ context.Context, _ string, id uuid.UUID) bool { got = id; return true }

	ctx := channel.WithChannel(context.Background(), &channel.Channel{ID: want})
	if _, err := callDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("kill switch consulted channel %v, want the request's %v", got, want)
	}
}

func TestDuplicateRegistrationRejected(t *testing.T) {
	h := resetGraph(t)
	if err := h.RegisterGraphResolver("demo", sdk.GraphAdmin, &demoImpl{}); err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterGraphResolver("demo", sdk.GraphAdmin, &demoImpl{}); err == nil {
		t.Error("second registration for the same plugin+surface was accepted")
	}
	// the other surface is a separate slot
	if err := h.RegisterGraphResolver("demo", sdk.GraphFront, &demoImpl{}); err != nil {
		t.Errorf("front registration collided with admin: %v", err)
	}
}

// A plugin can't import apperr, so without translation every resolver error reaches the
// storefront scrubbed to "internal error"
func TestUserErrorBecomesClassifiedApperr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want apperr.Class
	}{
		{"invalid", sdk.Invalid("amountCents must be positive"), apperr.ClassValidation},
		{"unavailable", sdk.Unavailable("gateway down"), apperr.ClassUnavailable},
		{"coded conflict", sdk.UserErrorWithCode(sdk.ErrorConflict, "ALREADY_CAPTURED", "already captured"), apperr.ClassConflict},
		{"not found", sdk.UserErrorWithCode(sdk.ErrorNotFound, "", "no such session"), apperr.ClassNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := resetGraph(t)
			if err := h.RegisterGraphResolver("demo", sdk.GraphAdmin, &erroringImpl{err: tc.err}); err != nil {
				t.Fatal(err)
			}
			_, err := callDemo(context.Background())
			if got := apperr.ClassOf(err); got != tc.want {
				t.Errorf("class %v, want %v", got, tc.want)
			}
			// the message is what a shopper reads, so the slug attribution must not be prefixed onto it
			if strings.Contains(err.Error(), "plugin demo") {
				t.Errorf("user-facing message carries plugin attribution: %q", err.Error())
			}
		})
	}
}

// An unclassified error IS a server fault, so it keeps the attribution that makes it diagnosable
func TestPlainErrorKeepsPluginAttribution(t *testing.T) {
	h := resetGraph(t)
	if err := h.RegisterGraphResolver("demo", sdk.GraphAdmin, &erroringImpl{err: errors.New("upstream exploded")}); err != nil {
		t.Fatal(err)
	}
	_, err := callDemo(context.Background())
	if !strings.Contains(err.Error(), "plugin demo") {
		t.Errorf("server error lost its attribution: %q", err.Error())
	}
}

type erroringImpl struct{ err error }

func (e *erroringImpl) Connected(context.Context) (bool, error) { return false, e.err }

type demoProduct struct{ ID string }

type demoBatchAdmin interface {
	ProductWarranty(ctx context.Context, objs []*demoProduct) ([]*string, error)
}

type demoBatchImpl struct {
	calls int
	short bool
}

func (d *demoBatchImpl) ProductWarranty(_ context.Context, objs []*demoProduct) ([]*string, error) {
	d.calls++
	out := make([]*string, len(objs))
	for i, o := range objs {
		w := "2y for " + o.ID
		out[i] = &w
	}
	if d.short {
		return out[:len(out)-1], nil
	}
	return out, nil
}

func callBatch(ctx context.Context, objs []*demoProduct) ([]*string, error) {
	return DoBatch(ctx, sdk.GraphAdmin, "demo", "Product.warranty", len(objs),
		func(ctx context.Context, impl demoBatchAdmin) ([]*string, error) {
			return impl.ProductWarranty(ctx, objs)
		})
}

// A field on a core type answers a whole list in one call, in the parents' order, and a plugin that's off on the
// channel answers null for each without failing the core query around it
func TestBatchFieldAnswersAListInOneCall(t *testing.T) {
	h := resetGraph(t)
	impl := &demoBatchImpl{}
	if err := h.RegisterGraphResolver("demo", sdk.GraphAdmin, impl); err != nil {
		t.Fatal(err)
	}
	objs := []*demoProduct{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	got, err := callBatch(context.Background(), objs)
	if err != nil || len(got) != 3 || *got[1] != "2y for b" || impl.calls != 1 {
		t.Fatalf("batch = %v, %v after %d calls", got, err, impl.calls)
	}

	off := uuid.New()
	orig := isEnabledFromContext
	t.Cleanup(func() { isEnabledFromContext = orig })
	isEnabledFromContext = func(_ context.Context, _ string, ch uuid.UUID) bool { return ch != off }
	ctx := channel.WithChannel(context.Background(), &channel.Channel{ID: off})
	got, err = callBatch(ctx, objs)
	if err != nil || len(got) != 3 || got[0] != nil || impl.calls != 1 {
		t.Errorf("plugin off on the channel: %v, %v after %d calls - want three nulls and no call", got, err, impl.calls)
	}

	// A failure is reported against the first parent alone, where a plain error would be repeated for every row
	impl.short = true
	got, err = callBatch(context.Background(), objs)
	var perParent graphql.BatchErrors
	if !errors.As(err, &perParent) || len(got) != 3 || len(perParent.Errors()) != 3 {
		t.Fatalf("an answer short of the parents: %v, %v", got, err)
	}
	if errs := perParent.Errors(); errs[0] == nil || !strings.Contains(errs[0].Error(), "2 values for 3 parents") || errs[1] != nil || errs[2] != nil {
		t.Errorf("per-parent errors %v - want the one, on the first", errs)
	}
}

// The field rides core's own listings, so it never spends the plugin's per-minute allowance - a spent one would turn
// every product page into errors
func TestBatchFieldIsUnmetered(t *testing.T) {
	h := resetGraph(t)
	both := &struct {
		demoImpl
		demoBatchImpl
	}{}
	if err := h.RegisterGraphResolver("demo", sdk.GraphAdmin, both); err != nil {
		t.Fatal(err)
	}
	orig := checkRateLimitFn
	t.Cleanup(func() { checkRateLimitFn = orig })
	checkRateLimitFn = func(context.Context, string) error { return ErrPluginRateLimited }
	if got, err := callBatch(context.Background(), []*demoProduct{{ID: "a"}}); err != nil || got[0] == nil {
		t.Errorf("a spent allowance reached the field: %v, %v", got, err)
	}
	if _, err := callDemo(context.Background()); !errors.Is(err, ErrPluginRateLimited) {
		t.Errorf("a plugin's own query is still metered: %v", err)
	}
}
