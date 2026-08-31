package sdktest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// The shape a plugin author's test takes: register through the real SDK, drive through the fake
func TestRegistrationsFlowThroughTheRealSDK(t *testing.T) {
	h := Install()
	p := sdk.NewPluginForTest("t-plug")

	p.RegisterPaymentProvider("t-gateway", nil, sdk.WithDisplayName("T"), sdk.WithOffline())
	p.RegisterRoute("GET ping", func(http.ResponseWriter, *http.Request) {})

	reg, ok := h.Providers["t-gateway"]
	if !ok {
		t.Fatal("provider never reached the fake host")
	}
	if reg.PluginSlug != "t-plug" || !reg.IsOffline || reg.DisplayName != "T" {
		t.Errorf("registration lost fields: %+v", reg)
	}
	if len(h.Routes) != 1 || h.Routes[0].PluginSlug != "t-plug" || h.Routes[0].Pattern != "GET ping" {
		t.Fatalf("route never reached the fake host: %+v", h.Routes)
	}
}

func TestAsyncDeliveryHonorsTheKillSwitch(t *testing.T) {
	h := Install()
	p := sdk.NewPluginForTest("t-async")

	var got []string
	p.SubscribeAsync("t.event", func(_ context.Context, payload json.RawMessage) error {
		var e struct{ Ref string }
		if err := json.Unmarshal(payload, &e); err != nil {
			return err
		}
		got = append(got, e.Ref)
		return nil
	})

	ctx := context.Background()
	if err := h.Deliver(ctx, "t.event", map[string]string{"Ref": "a"}); err != nil {
		t.Fatal(err)
	}
	h.Disable("t-async")
	if err := h.Deliver(ctx, "t.event", map[string]string{"Ref": "b"}); err != nil {
		t.Fatal(err)
	}
	h.Enable("t-async")
	if err := h.Deliver(ctx, "t.event", map[string]string{"Ref": "c"}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("kill switch did not gate delivery: %v", got)
	}
}

// Errors come back joined instead of logged and dropped, so the failure mode is assertable
func TestAsyncErrorsAreReturned(t *testing.T) {
	h := Install()
	p := sdk.NewPluginForTest("t-err")
	sentinel := errors.New("sink unreachable")
	p.SubscribeAsync("t.event", func(context.Context, json.RawMessage) error { return sentinel })

	if err := h.Deliver(context.Background(), "t.event", struct{}{}); !errors.Is(err, sentinel) {
		t.Fatalf("handler error was swallowed: %v", err)
	}
}

type order struct{ steps []string }

func (o *order) Step(s string) { o.steps = append(o.steps, s) }

func TestSyncDeliveryRunsInPriorityOrderAndMutates(t *testing.T) {
	h := Install()
	a := sdk.NewPluginForTest("t-a")
	b := sdk.NewPluginForTest("t-b")

	b.SubscribeSync("t.placing", 200, func(_ context.Context, e any) error {
		e.(*order).Step("late")
		return nil
	})
	a.SubscribeSync("t.placing", 100, func(_ context.Context, e any) error {
		e.(*order).Step("early")
		return nil
	})

	ev := &order{}
	if err := h.DeliverSync(context.Background(), "t.placing", ev); err != nil {
		t.Fatal(err)
	}
	if len(ev.steps) != 2 || ev.steps[0] != "early" || ev.steps[1] != "late" {
		t.Fatalf("priority order broken: %v", ev.steps)
	}

	// a disabled plugin is skipped without aborting the chain, like the host's wrapper
	h.Disable("t-a")
	ev = &order{}
	if err := h.DeliverSync(context.Background(), "t.placing", ev); err != nil {
		t.Fatal(err)
	}
	if len(ev.steps) != 1 || ev.steps[0] != "late" {
		t.Fatalf("disabled plugin still ran or aborted the chain: %v", ev.steps)
	}
}

func TestConfigResolvesSetValuesThenDefaults(t *testing.T) {
	h := Install()
	p := sdk.RegisterPlugin(sdk.Manifest{Slug: "cfg-test", Version: "0.1.0", SDKRange: ">=0.1.0"})
	p.RegisterConfigField("payment/t/mode", sdk.ConfigField{Type: sdk.TypeString, DefaultValue: "test"})
	p.RegisterConfigField("payment/t/max_retries", sdk.ConfigField{Type: sdk.TypeInt, DefaultValue: "3"})
	ctx := context.Background()

	mode, err := sdk.PluginConfig(ctx, "payment/t/mode")
	if err != nil || mode != "test" {
		t.Fatalf("default did not resolve: %q %v", mode, err)
	}
	h.SetConfig("payment/t/mode", "live")
	if mode, _ = sdk.PluginConfig(ctx, "payment/t/mode"); mode != "live" {
		t.Fatalf("set value did not win: %q", mode)
	}
	n, err := sdk.PluginConfigInt(ctx, "payment/t/max_retries")
	if err != nil || n != 3 {
		t.Fatalf("typed default did not resolve: %d %v", n, err)
	}
}

// Claim/skip/release-on-error, in-memory - the same contract the durable store gives production
func TestSubscribeAsyncOnceDedupes(t *testing.T) {
	h := Install()
	p := sdk.NewPluginForTest("t-once")

	var runs int
	fail := true
	p.SubscribeAsyncOnce("t.event",
		func(payload json.RawMessage) (string, error) {
			var e struct{ Key string }
			if err := json.Unmarshal(payload, &e); err != nil {
				return "", err
			}
			return e.Key, nil
		},
		func(context.Context, json.RawMessage) error {
			runs++
			if fail {
				fail = false
				return errors.New("transient")
			}
			return nil
		})

	ctx := context.Background()
	if err := h.Deliver(ctx, "t.event", map[string]string{"Key": "a"}); err == nil {
		t.Fatal("first delivery's handler error was swallowed")
	}
	if err := h.Deliver(ctx, "t.event", map[string]string{"Key": "a"}); err != nil {
		t.Fatalf("retry after release: %v", err) // the error released the claim, so this runs
	}
	if err := h.Deliver(ctx, "t.event", map[string]string{"Key": "a"}); err != nil {
		t.Fatal(err) // processed key - skipped without error
	}
	if err := h.Deliver(ctx, "t.event", map[string]string{"Key": "b"}); err != nil {
		t.Fatal(err)
	}
	if runs != 3 {
		t.Fatalf("want 3 runs (fail, retry, fresh key), got %d", runs)
	}
}

// Emits enforcement mirrors the real host, and delivery is immediate so plugin-to-plugin flows assert in one call
func TestPublishAsyncEnforcesEmits(t *testing.T) {
	Install()
	p := sdk.RegisterPlugin(sdk.Manifest{
		Slug: "t-pub", Name: "T Pub", Version: "0.1.0", SDKRange: ">=0.1.0",
		Emits: []string{"t-pub.done"},
	})

	var got string
	p.SubscribeAsync("t-pub.done", func(_ context.Context, payload json.RawMessage) error {
		var e struct{ Ref string }
		if err := json.Unmarshal(payload, &e); err != nil {
			return err
		}
		got = e.Ref
		return nil
	})

	ctx := context.Background()
	if err := p.PublishAsync(ctx, "t-pub.done", map[string]string{"Ref": "r1"}); err != nil {
		t.Fatalf("declared publish refused: %v", err)
	}
	if got != "r1" {
		t.Fatalf("subscriber never saw the publish: %q", got)
	}
	if err := p.PublishAsync(ctx, "t-pub.undeclared", struct{}{}); err == nil {
		t.Fatal("undeclared publish passed")
	}
}

func TestPermissionDenialAndEnabledDefaults(t *testing.T) {
	h := Install()
	ctx := context.Background()

	if !h.IsPluginEnabled(ctx, "anything", uuid.Nil) {
		t.Error("plugins must default enabled")
	}
	if err := h.RequireAdminPermission(ctx, "t.manage"); err != nil {
		t.Errorf("permissions must default allowed: %v", err)
	}
	h.DenyPermission("t.manage")
	if err := h.RequireAdminPermission(ctx, "t.manage"); err == nil {
		t.Error("denied permission still passed")
	}
	h.ResetState()
	if err := h.RequireAdminPermission(ctx, "t.manage"); err != nil {
		t.Errorf("ResetState did not clear the denial: %v", err)
	}
}
