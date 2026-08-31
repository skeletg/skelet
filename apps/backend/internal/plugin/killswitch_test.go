package plugin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// The integration test for the flow operators rely on - flip plugin/<slug>/enabled=false and the next hook call returns ErrPluginDisabled WITHOUT entering the plugin code
// It stubs sdk.IsEnabled by swapping the package-level isEnabledFromContext seam, the layer Instrument calls into, so no fake host is required
func TestInstrument_KillSwitchOff(t *testing.T) {
	// Capture and restore the original so this test's swap doesn't leak into others in the package
	orig := isEnabledFromContext
	t.Cleanup(func() { isEnabledFromContext = orig })

	isEnabledFromContext = func(ctx context.Context, slug string, channelID uuid.UUID) bool {
		return false
	}

	// Mimic an adapter's call shape - Instrument's third return is the gate error, and non-nil means don't run the plugin code
	var (
		called bool
		err    error
	)
	func() {
		_, finish, gateErr := Instrument(context.Background(), "test-plugin", "DoThing", uuid.Nil)
		defer finish(&err)
		if gateErr != nil {
			err = gateErr
			return
		}
		called = true // would normally call into the plugin here
	}()

	if called {
		t.Error("plugin code ran despite kill switch off")
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrPluginDisabled) {
		t.Errorf("expected ErrPluginDisabled, got %v", err)
	}
	var perr *PluginError
	if !errors.As(err, &perr) {
		t.Fatalf("expected *PluginError wrapping, got %T", err)
	}
	if perr.Slug != "test-plugin" || perr.Hook != "DoThing" {
		t.Errorf("PluginError attribution: got (%s, %s), want (test-plugin, DoThing)", perr.Slug, perr.Hook)
	}
}

// Confirms the happy path - enabled plugins receive the call and the hook returns its own value with no disabled wrapping, so Instrument is transparent on success
func TestInstrument_KillSwitchOn(t *testing.T) {
	orig := isEnabledFromContext
	t.Cleanup(func() { isEnabledFromContext = orig })

	isEnabledFromContext = func(ctx context.Context, slug string, channelID uuid.UUID) bool {
		return true
	}

	var (
		called bool
		err    error
	)
	func() {
		_, finish, gateErr := Instrument(context.Background(), "test-plugin", "DoThing", uuid.Nil)
		defer finish(&err)
		if gateErr != nil {
			err = gateErr
			return
		}
		called = true
	}()

	if !called {
		t.Error("plugin code did not run despite kill switch on")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// Operators set plugin/<slug>/rate_limit_per_min expecting it to bound hook traffic without the plugin cooperating, so the body must not run once the allowance is spent
func TestInstrument_RateLimitGate(t *testing.T) {
	origEnabled := isEnabledFromContext
	origLimit := checkRateLimitFn
	t.Cleanup(func() {
		isEnabledFromContext = origEnabled
		checkRateLimitFn = origLimit
	})
	isEnabledFromContext = func(_ context.Context, _ string, _ uuid.UUID) bool { return true }
	checkRateLimitFn = func(_ context.Context, _ string) error { return ErrPluginRateLimited }

	var (
		called bool
		err    error
	)
	func() {
		_, finish, gateErr := Instrument(context.Background(), "test-plugin", "DoThing", uuid.Nil)
		defer finish(&err)
		if gateErr != nil {
			err = gateErr
			return
		}
		called = true
	}()

	if called {
		t.Error("plugin code ran despite being over its rate limit")
	}
	if !errors.Is(err, ErrPluginRateLimited) {
		t.Errorf("expected ErrPluginRateLimited, got %v", err)
	}
	var perr *PluginError
	if !errors.As(err, &perr) || perr.Slug != "test-plugin" {
		t.Errorf("expected *PluginError attributed to test-plugin, got %v", err)
	}
}

// A disabled plugin must not consume rate-limit budget - otherwise turning a plugin off would still drain the allowance its own hooks share
func TestInstrument_DisabledSkipsRateLimit(t *testing.T) {
	origEnabled := isEnabledFromContext
	origLimit := checkRateLimitFn
	t.Cleanup(func() {
		isEnabledFromContext = origEnabled
		checkRateLimitFn = origLimit
	})
	isEnabledFromContext = func(_ context.Context, _ string, _ uuid.UUID) bool { return false }
	consulted := false
	checkRateLimitFn = func(_ context.Context, _ string) error {
		consulted = true
		return nil
	}

	var err error
	func() {
		_, finish, gateErr := Instrument(context.Background(), "test-plugin", "DoThing", uuid.Nil)
		defer finish(&err)
		err = gateErr
	}()

	if consulted {
		t.Error("rate limiter consulted for a disabled plugin")
	}
	if !errors.Is(err, ErrPluginDisabled) {
		t.Errorf("expected ErrPluginDisabled, got %v", err)
	}
}

// Confirms a panicking plugin returns a typed PluginError wrapping ErrPluginPanicked rather than crashing the process
func TestInstrument_RecoversPanic(t *testing.T) {
	orig := isEnabledFromContext
	t.Cleanup(func() { isEnabledFromContext = orig })
	isEnabledFromContext = func(_ context.Context, _ string, _ uuid.UUID) bool { return true }

	var err error
	func() {
		_, finish, gateErr := Instrument(context.Background(), "test-plugin", "Panicky", uuid.Nil)
		defer finish(&err)
		if gateErr != nil {
			err = gateErr
			return
		}
		panic("intentional panic for test")
	}()

	if err == nil {
		t.Fatal("expected error from panic, got nil")
	}
	if !errors.Is(err, ErrPluginPanicked) {
		t.Errorf("expected ErrPluginPanicked, got %v", err)
	}
}

// Confirms a hook blocking past its configured timeout produces a timeout-wrapped error - the default 5s is too long for a unit test, so hookTimeoutFromContext is stubbed through the same seam pattern
func TestInstrument_RespectsTimeout(t *testing.T) {
	origEnabled := isEnabledFromContext
	origTimeout := hookTimeoutFromContext
	t.Cleanup(func() {
		isEnabledFromContext = origEnabled
		hookTimeoutFromContext = origTimeout
	})
	isEnabledFromContext = func(_ context.Context, _ string, _ uuid.UUID) bool { return true }
	hookTimeoutFromContext = func(_ context.Context, _ string) time.Duration {
		return 20 * time.Millisecond
	}

	var err error
	func() {
		ctx, finish, gateErr := Instrument(context.Background(), "test-plugin", "Slow", uuid.Nil)
		defer finish(&err)
		if gateErr != nil {
			err = gateErr
			return
		}
		// Block until ctx expires - an external API call would do the same shape through ctx-aware HTTP and DB clients
		<-ctx.Done()
		err = ctx.Err()
	}()

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, ErrPluginTimeout) {
		t.Errorf("expected ErrPluginTimeout wrapping, got %v", err)
	}
}

// The policy exists for the case where the kill switch can't be read at all - fail_open keeps a gateway serving through
// a config hiccup, fail_closed refuses, and a plugin that never declared one gets fail_open
func TestFailsClosedComesFromTheManifest(t *testing.T) {
	h := &Host{manifestBySlug: map[string]sdk.Manifest{
		"strict":  {Slug: "strict", FailurePolicy: sdk.FailClosed},
		"relaxed": {Slug: "relaxed"},
	}}
	if !h.failsClosed("strict") {
		t.Error("a fail_closed manifest did not fail closed")
	}
	if h.failsClosed("relaxed") {
		t.Error("an undeclared policy should default open")
	}
	if h.failsClosed("unregistered") {
		t.Error("a slug with no manifest should default open")
	}
}

// The whole point is surviving a config outage, so the answer must not itself need the store -
// a config-stored policy would be unreadable in the same outage and collapse back to fail_open
func TestFailsClosedNeedsNoConfigService(t *testing.T) {
	h := &Host{manifestBySlug: map[string]sdk.Manifest{"strict": {FailurePolicy: sdk.FailClosed}}}
	if h.configSvc.Load() != nil {
		t.Fatal("test assumes no config service is wired")
	}
	if !h.failsClosed("strict") {
		t.Error("policy became unreadable without a config service - it must live in memory")
	}
	if h.IsPluginEnabled(context.Background(), "strict", uuid.Nil) != true {
		t.Log("no config service short-circuits to enabled before the policy is consulted")
	}
}

// ---- promotion adapter gate translation ----
// These pin the OTHER contract Instrument serves: payment hooks propagate the gate error, but a
// disabled or rate-limited PROMOTION plugin must answer neutrally - a condition answers false, an
// action and a redemption hook no-op - because the engine and order placement abort on any error

type gateTestEnv struct{}

func (gateTestEnv) SubtotalCents() int64            { return 1000 }
func (gateTestEnv) Currency() string                { return "USD" }
func (gateTestEnv) CountryCode() string             { return "US" }
func (gateTestEnv) ChannelID() *uuid.UUID           { return nil }
func (gateTestEnv) ChannelSlug() string             { return "web" }
func (gateTestEnv) CustomerID() *uuid.UUID          { return nil }
func (gateTestEnv) CustomerGroupSlug() string       { return "" }
func (gateTestEnv) CustomerEmail() string           { return "" }
func (gateTestEnv) CustomerSegmentSlugs() []string  { return nil }
func (gateTestEnv) Lines() []promotion.CartLineView { return nil }
func (gateTestEnv) AppliedCodes() []string          { return nil }

type gateTestCondition struct{ ran bool }

func (c *gateTestCondition) Slug() string                         { return "gated.cond" }
func (c *gateTestCondition) Label() string                        { return "" }
func (c *gateTestCondition) Description() string                  { return "" }
func (c *gateTestCondition) ParamsSchema() json.RawMessage        { return nil }
func (c *gateTestCondition) ValidateParams(json.RawMessage) error { return nil }
func (c *gateTestCondition) Evaluate(context.Context, sdk.PromotionEnvironment, json.RawMessage) (bool, error) {
	c.ran = true
	return true, nil
}

type gateTestAction struct {
	ran bool
	err error
}

func (a *gateTestAction) Slug() string                         { return "gated.act" }
func (a *gateTestAction) Label() string                        { return "" }
func (a *gateTestAction) Description() string                  { return "" }
func (a *gateTestAction) ParamsSchema() json.RawMessage        { return nil }
func (a *gateTestAction) ValidateParams(json.RawMessage) error { return nil }
func (a *gateTestAction) Apply(context.Context, sdk.PromotionEnvironment, json.RawMessage, sdk.PromotionLedger, sdk.PromotionAppliedContext) error {
	a.ran = true
	if a.err != nil {
		return a.err
	}
	return nil
}

type gateTestHook struct{ ran bool }

func (h *gateTestHook) Slug() string { return "gated.hook" }
func (h *gateTestHook) Redeem(context.Context, *sql.Tx, sdk.PromotionEnvironment, sdk.PromotionLedger, sdk.PromotionAppliedContext) error {
	h.ran = true
	return nil
}

// swapGates points both seams at the given answers for one test
func swapGates(t *testing.T, enabled bool, limitErr error) {
	t.Helper()
	origEnabled, origLimit := isEnabledFromContext, checkRateLimitFn
	t.Cleanup(func() { isEnabledFromContext, checkRateLimitFn = origEnabled, origLimit })
	isEnabledFromContext = func(context.Context, string, uuid.UUID) bool { return enabled }
	checkRateLimitFn = func(context.Context, string) error { return limitErr }
}

func TestPromotionAdapters_GateErrorsAreSwallowed(t *testing.T) {
	for name, gate := range map[string]func(t *testing.T){
		"disabled":     func(t *testing.T) { swapGates(t, false, nil) },
		"rate limited": func(t *testing.T) { swapGates(t, true, ErrPluginRateLimited) },
	} {
		t.Run(name, func(t *testing.T) {
			gate(t)

			cond := &gateTestCondition{}
			match, err := (&promotionConditionAdapter{p: cond, slug: "gated"}).Evaluate(context.Background(), gateTestEnv{}, nil)
			if err != nil || match {
				t.Errorf("condition: got (%v, %v), want (false, nil)", match, err)
			}
			if cond.ran {
				t.Error("condition body ran despite the gate")
			}

			act := &gateTestAction{}
			if err := (&promotionActionAdapter{p: act, slug: "gated"}).Apply(context.Background(), gateTestEnv{}, nil, nil, promotion.AppliedRuleContext{}); err != nil {
				t.Errorf("action: got %v, want nil", err)
			}
			if act.ran {
				t.Error("action body ran despite the gate")
			}

			hook := &gateTestHook{}
			if err := (&promotionHookAdapter{p: hook, slug: "gated"}).Redeem(context.Background(), nil, gateTestEnv{}, nil, promotion.AppliedRuleContext{}); err != nil {
				t.Errorf("hook: got %v, want nil", err)
			}
			if hook.ran {
				t.Error("hook body ran despite the gate")
			}
		})
	}
}

// The swallow is gate-only - a real plugin failure still propagates, and a panic surfaces as ErrPluginPanicked
func TestPromotionAdapters_RealErrorsStillPropagate(t *testing.T) {
	swapGates(t, true, nil)

	wantErr := errors.New("boom")
	act := &gateTestAction{err: wantErr}
	if err := (&promotionActionAdapter{p: act, slug: "gated"}).Apply(context.Background(), gateTestEnv{}, nil, nil, promotion.AppliedRuleContext{}); !errors.Is(err, wantErr) {
		t.Errorf("action error: got %v, want wrapped %v", err, wantErr)
	}
	if !act.ran {
		t.Error("action body never ran while enabled")
	}
}

type panickingCondition struct{ gateTestCondition }

func (p *panickingCondition) Evaluate(context.Context, sdk.PromotionEnvironment, json.RawMessage) (bool, error) {
	panic("kaboom")
}

func TestPromotionAdapters_PanicIsNotSwallowed(t *testing.T) {
	swapGates(t, true, nil)
	_, err := (&promotionConditionAdapter{p: &panickingCondition{}, slug: "gated"}).Evaluate(context.Background(), gateTestEnv{}, nil)
	if !errors.Is(err, ErrPluginPanicked) {
		t.Errorf("got %v, want ErrPluginPanicked", err)
	}
}

// The paypal outage class: a full path through the prefixing reader must fail loudly, not read a
// path that never exists and hand back ""
func TestPluginConfigReaderRefusesFullPaths(t *testing.T) {
	r := &pluginConfigReader{prefix: "payment/paypal/"}
	if _, err := r.Get(context.Background(), "payment/paypal/client_id", uuid.Nil); err == nil || !strings.Contains(err.Error(), "full path") {
		t.Errorf("full path through prefixing reader: got %v, want a loud refusal", err)
	}
}
