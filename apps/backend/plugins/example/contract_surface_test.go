package example

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/datapatch"
	"github.com/skeletg/skelet/internal/events"
	"github.com/skeletg/skelet/internal/payment"
	pluginhost "github.com/skeletg/skelet/internal/plugin"
	"github.com/skeletg/skelet/internal/promotion"
	"github.com/skeletg/skelet/sdk"
)

// SetEventBus is single-shot, so every test needing the bus shares this one
var (
	busOnce sync.Once
	testBus *events.Bus
)

func sharedBus() *events.Bus {
	busOnce.Do(func() {
		testBus = events.NewBus()
		pluginhost.SetEventBus(testBus)
	})
	return testBus
}

// Slug keys the plugin's own namespaces, IsEnabled is what its self-spawned goroutines check
func TestHandleExposesSlugAndEnabled(t *testing.T) {
	if pluginHandle.Slug() != Slug {
		t.Errorf("handle slug: got %q, want %q", pluginHandle.Slug(), Slug)
	}
	if !pluginHandle.IsEnabled(context.Background(), uuid.Nil) {
		t.Error("kill switch must default open - a fresh install has no config row")
	}
}

// Through the real host chain: prefix-qualified mount, method restriction, and the wrappers routes_test pins individually
func TestRouteServesThroughTheHostChain(t *testing.T) {
	h := pluginhost.RouteHandler()
	if h == nil {
		t.Fatal("RouteHandler is nil despite a registered route")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, pluginhost.RoutePrefix+Slug+"/ping", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET ping: got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "pong" {
		t.Errorf("body: got %q, want pong", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, pluginhost.RoutePrefix+Slug+"/ping", nil))
	if rec.Code == http.StatusOK {
		t.Error("POST reached a GET-restricted route")
	}
}

func TestOfflineProviderCarriesTheFlag(t *testing.T) {
	reg, err := payment.GetProvider(OfflineProviderSlug)
	if err != nil {
		t.Fatalf("offline provider not registered: %v", err)
	}
	if !reg.IsOffline {
		t.Error("WithOffline did not survive the host translation")
	}
	if reg.DisplayName != "Example Offline" {
		t.Errorf("display name: got %q", reg.DisplayName)
	}
}

// Registration presence, the admin-write validation path, Evaluate/Apply through the full environment and ledger adaptation, and the
// redemption pairing - each piece can be right on its own while the composition is wrong, which is what a contract test exists to hold still
func TestPromotionTypesBridgeToTheEngine(t *testing.T) {
	ctx := context.Background()

	cond, ok := promotion.DefaultRegistry.LookupCondition(Slug + ".min-subtotal")
	if !ok {
		t.Fatal("condition never reached the promotion registry")
	}
	if err := cond.ValidateParams([]byte(`{"min_cents":-1}`)); err == nil {
		t.Error("negative min_cents validated - the admin write path would accept a broken rule")
	}
	match, err := cond.Evaluate(ctx, fakeEnv{subtotal: 5000}, []byte(`{"min_cents":1000}`))
	if err != nil || !match {
		t.Errorf("subtotal 5000 against min 1000: match=%v err=%v, want a match", match, err)
	}
	match, err = cond.Evaluate(ctx, fakeEnv{subtotal: 500}, []byte(`{"min_cents":1000}`))
	if err != nil || match {
		t.Errorf("subtotal 500 against min 1000: match=%v err=%v, want no match", match, err)
	}

	action, ok := promotion.DefaultRegistry.LookupAction(Slug + ".flat-discount")
	if !ok {
		t.Fatal("action never reached the promotion registry")
	}
	ledger := &fakeLedger{}
	if err := action.Apply(ctx, fakeEnv{subtotal: 5000}, []byte(`{"cents":250}`), ledger, promotion.AppliedRuleContext{RuleID: uuid.New()}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if ledger.subtotalDiscount != 250 {
		t.Errorf("discount did not reach the ledger through the adapter: got %d", ledger.subtotalDiscount)
	}

	hook, ok := promotion.DefaultRegistry.LookupHook(Slug + ".flat-discount")
	if !ok {
		t.Fatal("redemption hook never reached the promotion registry")
	}
	if err := hook.Redeem(ctx, nil, fakeEnv{}, ledger, promotion.AppliedRuleContext{RuleID: uuid.New()}); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	select {
	case <-redemptionRan:
	case <-time.After(time.Second):
		t.Fatal("redemption hook never fired")
	}
}

// The plugin publishes its own declared event and both of its subscribers see it - and an
// undeclared type is refused, which is what makes Manifest.Emits a contract instead of a claim
func TestPublishAsyncRoundTripsAndEnforcesEmits(t *testing.T) {
	sharedBus() // the publish lands on the host's wired bus
	ctx := context.Background()

	if err := pluginHandle.PublishAsync(ctx, EventSettled, map[string]string{"Reference": "self-pub-1"}); err != nil {
		t.Fatalf("declared publish refused: %v", err)
	}
drain:
	for {
		select {
		case p := <-settledSeen:
			if strings.Contains(string(p), "self-pub-1") {
				break drain
			}
		case <-time.After(2 * time.Second):
			t.Fatal("self-published event never reached the plugin's own subscriber")
		}
	}
	if err := pluginHandle.PublishAsync(ctx, "example.undeclared", struct{}{}); err == nil {
		t.Fatal("undeclared event type published without error")
	}
}

// Without a wired dedup store the verb degrades to plain delivery - the dedup semantics
// themselves are pinned in internal/plugin and sdktest
func TestSubscribeAsyncOnceDegradesToDelivery(t *testing.T) {
	sharedBus().Publish(context.Background(), settledEvent{Reference: "once-1"})
	for { // drain past other tests' publishes - every settled event fans into this channel
		select {
		case p := <-onceSeen:
			if strings.Contains(string(p), "once-1") {
				return
			}
		case <-time.After(2 * time.Second):
			t.Fatal("SubscribeAsyncOnce subscriber never fired")
		}
	}
}

// The pre-commit seam - the handler mutates the live value, and the emitter reads it back after a nil return
func TestSyncSubscriberMutatesTheEvent(t *testing.T) {
	ev := &settlingEvent{}
	if err := sharedBus().PublishSync(context.Background(), ev); err != nil {
		t.Fatalf("publish sync: %v", err)
	}
	if !ev.settling {
		t.Fatal("sync handler never mutated the event - the init()-time SubscribeSync did not replay")
	}
}

// The typed siblings of PluginConfig resolve registered defaults without hand-parsing - the
// auto-registered kill switch (bool, default true) and rate limit (int, default 0) prove both
func TestTypedConfigReadsResolveDefaults(t *testing.T) {
	withDB(t)
	ctx := context.Background()

	enabled, err := sdk.PluginConfigBool(ctx, "plugin/"+Slug+"/enabled")
	if err != nil {
		t.Fatalf("bool read: %v", err)
	}
	if !enabled {
		t.Error("enabled must default true with no config row")
	}

	limit, err := sdk.PluginConfigInt(ctx, "plugin/"+Slug+"/rate_limit_per_min")
	if err != nil {
		t.Fatalf("int read: %v", err)
	}
	if limit != 0 {
		t.Errorf("rate limit: got %d, want the unlimited default 0", limit)
	}
}

func TestDataPatchRegisteredThroughTheSDK(t *testing.T) {
	for _, p := range datapatch.Registered() {
		if p.ID == Slug+":seed-example-records" {
			if p.Namespace != Slug {
				t.Errorf("patch namespace: got %q, want %q", p.Namespace, Slug)
			}
			return
		}
	}
	t.Fatal("data patch never reached the registry")
}

type settlingEvent struct{ settling bool }

func (*settlingEvent) Type() string    { return EventSettling }
func (e *settlingEvent) MarkSettling() { e.settling = true }

type fakeEnv struct{ subtotal int64 }

func (f fakeEnv) SubtotalCents() int64          { return f.subtotal }
func (fakeEnv) Currency() string                { return "USD" }
func (fakeEnv) CountryCode() string             { return "US" }
func (fakeEnv) ChannelID() *uuid.UUID           { return nil }
func (fakeEnv) ChannelSlug() string             { return "" }
func (fakeEnv) CustomerID() *uuid.UUID          { return nil }
func (fakeEnv) CustomerGroupSlug() string       { return "" }
func (fakeEnv) CustomerEmail() string           { return "" }
func (fakeEnv) CustomerSegmentSlugs() []string  { return nil }
func (fakeEnv) Lines() []promotion.CartLineView { return nil }
func (fakeEnv) AppliedCodes() []string          { return nil }

type fakeLedger struct{ subtotalDiscount int64 }

func (l *fakeLedger) AddSubtotalDiscount(cents int64)    { l.subtotalDiscount += cents }
func (*fakeLedger) AddLineAdjustment(uuid.UUID, int64)   {}
func (*fakeLedger) SetFreeShipping([]string)             {}
func (*fakeLedger) SetShippingOverride(int64)            {}
func (*fakeLedger) AddFreeGift(string, float64)          {}
func (*fakeLedger) CapSubtotalDiscount(int64)            {}
func (l *fakeLedger) SubtotalDiscountCents() int64       { return l.subtotalDiscount }
func (*fakeLedger) LineAdjustments() map[uuid.UUID]int64 { return nil }
func (*fakeLedger) LineAdjustment(uuid.UUID) int64       { return 0 }
func (*fakeLedger) FreeShipping() bool                   { return false }
func (*fakeLedger) FreeShippingMethods() []string        { return nil }
func (*fakeLedger) ShippingOverride() *int64             { return nil }
func (*fakeLedger) FreeGifts() []promotion.FreeGift      { return nil }
