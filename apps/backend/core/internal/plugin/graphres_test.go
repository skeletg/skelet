package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"

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
