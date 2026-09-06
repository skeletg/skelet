package sdk

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
	"github.com/graph-gophers/dataloader/v7"
)

// keeps the v7 generic parameter out of every use site
type dataloaderResult[V any] = dataloader.Result[V]

// Records every Register* call, so the SDK runs end-to-end without booting the internal/* host
type fakeHost struct {
	mu                sync.Mutex
	providers         []ProviderRegistration
	searchIndexes     []string
	carriers          []CarrierRegistration
	permissions       []permRec
	configs           []configRec
	events            []eventRec
	migrations        []MigrationsRegistration
	dataPatches       []DataPatchRegistration
	dataloaders       []DataLoaderRegistration
	pluginManifests   []Manifest
	healthChecks      []attributedRec
	readinessChecks   []attributedRec
	scheduledJobs     []attributedRec
	asyncSubs         []attributedRec
	syncSubs          []attributedRec
	shutdowns         []attributedRec
	routes            []attributedRec
	graphResolvers    []attributedRec
	priceResolvers    []attributedRec
	adminTranslations []attributedRec
}

// PluginSlug is the field these tests exist to pin down
type attributedRec struct {
	PluginSlug string
	Name       string
	Extra      string
}

type permRec struct{ Group, Slug, Name string }
type configRec struct {
	Path  string
	Field ConfigField
}
type eventRec struct {
	Name string
	Kind EventKind
}

func (h *fakeHost) RegisterPaymentProvider(reg ProviderRegistration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.providers = append(h.providers, reg)
	return nil
}
func (h *fakeHost) RegisterShippingCarrier(reg CarrierRegistration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.carriers = append(h.carriers, reg)
	return nil
}
func (h *fakeHost) RegisterPermission(pluginSlug, group, slug, name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.permissions = append(h.permissions, permRec{group, slug, name})
	return nil
}
func (h *fakeHost) RegisterConfigField(pluginSlug, path string, field ConfigField) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.configs = append(h.configs, configRec{path, field})
	return nil
}
func (h *fakeHost) RegisterEvent(pluginSlug, name string, kind EventKind) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, eventRec{name, kind})
	return nil
}
func (h *fakeHost) RegisterMigrations(pluginSlug string, reg MigrationsRegistration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.migrations = append(h.migrations, reg)
	return nil
}
func (h *fakeHost) RegisterDataPatch(pluginSlug string, reg DataPatchRegistration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dataPatches = append(h.dataPatches, reg)
	return nil
}

// Routing is covered in internal/plugin/events_test.go - here we only record attribution
func (h *fakeHost) SubscribeAsync(pluginSlug, eventType string, _ AsyncHandler) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.asyncSubs = append(h.asyncSubs, attributedRec{PluginSlug: pluginSlug, Name: eventType})
	return nil
}

func (h *fakeHost) SubscribeSync(pluginSlug, eventType string, _ int, _ SyncHandler) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.syncSubs = append(h.syncSubs, attributedRec{PluginSlug: pluginSlug, Name: eventType})
	return nil
}

func (h *fakeHost) RegisterScheduledJob(pluginSlug, name, schedule, _ string, _ ScheduledJobHandler) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.scheduledJobs = append(h.scheduledJobs, attributedRec{PluginSlug: pluginSlug, Name: name, Extra: schedule})
	return nil
}

func (h *fakeHost) RegisterPromotionCondition(_ string, _ PromotionCondition) error { return nil }
func (h *fakeHost) RegisterPromotionAction(_ string, _ PromotionAction) error       { return nil }
func (h *fakeHost) RegisterPromotionRedemptionHook(_ string, _ PromotionRedemptionHook) error {
	return nil
}

func (h *fakeHost) RegisterDataLoader(reg DataLoaderRegistration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dataloaders = append(h.dataloaders, reg)
	return nil
}

func (h *fakeHost) RegisterPlugin(m Manifest) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pluginManifests = append(h.pluginManifests, m)
	return nil
}

func (h *fakeHost) IsPluginEnabled(_ context.Context, _ string, _ uuid.UUID) bool {
	// no kill-switch cases here - fail open
	return true
}

func (h *fakeHost) RegisterHealthCheck(pluginSlug, name string, _ HealthCheck) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.healthChecks = append(h.healthChecks, attributedRec{PluginSlug: pluginSlug, Name: name})
	return nil
}

func (h *fakeHost) RegisterReadinessCheck(pluginSlug, name string, _ ReadinessCheck) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.readinessChecks = append(h.readinessChecks, attributedRec{PluginSlug: pluginSlug, Name: name})
	return nil
}

func (h *fakeHost) RegisterShutdown(pluginSlug string, _ ShutdownFunc) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.shutdowns = append(h.shutdowns, attributedRec{PluginSlug: pluginSlug})
	return nil
}

func (h *fakeHost) RegisterRoute(pluginSlug, pattern string, _ RouteHandler) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.routes = append(h.routes, attributedRec{PluginSlug: pluginSlug, Name: pattern})
	return nil
}

func (h *fakeHost) RegisterAdminTranslations(pluginSlug string, reg AdminTranslationsRegistration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.adminTranslations = append(h.adminTranslations, attributedRec{PluginSlug: pluginSlug, Name: reg.Locale})
	return nil
}

func (h *fakeHost) RegisterGraphResolver(pluginSlug string, surface GraphSurface, _ any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.graphResolvers = append(h.graphResolvers, attributedRec{PluginSlug: pluginSlug, Name: string(surface)})
	return nil
}

func (h *fakeHost) RegisterPriceResolver(pluginSlug string, _ PriceResolver) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.priceResolvers = append(h.priceResolvers, attributedRec{PluginSlug: pluginSlug})
	return nil
}

// Unwired in these tests, which never reach the database or the order service - a plugin resolving
// either before boot wired it sees exactly this
func (h *fakeHost) Database() (DB, error) { return nil, ErrDBUnavailable }

func (h *fakeHost) CreateDraftOrder(context.Context, DraftOrderRequest) (DraftOrderResult, error) {
	return DraftOrderResult{}, ErrDraftOrdersUnavailable
}

func (h *fakeHost) ReadCart(context.Context, uuid.UUID) (Cart, error) {
	return Cart{}, ErrCartsUnavailable
}

func (h *fakeHost) RequireAdminChannelPermission(context.Context, string, uuid.UUID) error {
	return ErrPermissionUnavailable
}

func (h *fakeHost) PermittedChannels(context.Context, string) ([]uuid.UUID, bool, error) {
	return nil, false, ErrPermissionUnavailable
}

func (h *fakeHost) ActingAdminID(context.Context) *uuid.UUID     { return nil }
func (h *fakeHost) RequestCustomerID(context.Context) *uuid.UUID { return nil }
func (h *fakeHost) RequestChannelID(context.Context) uuid.UUID   { return uuid.Nil }

func (h *fakeHost) RequireAdminPermission(_ context.Context, _ string) error { return nil }

func (h *fakeHost) ReadConfigString(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (h *fakeHost) ReadConfigBool(_ context.Context, _ string) (bool, error) { return false, nil }
func (h *fakeHost) ReadConfigInt(_ context.Context, _ string) (int64, error) { return 0, nil }

func (h *fakeHost) SubscribeAsyncOnce(pluginSlug, eventType string, _ DedupKeyFunc, handler AsyncHandler) error {
	return h.SubscribeAsync(pluginSlug, eventType, handler)
}

func (h *fakeHost) PublishEvent(_ context.Context, _, _ string, _ any) error { return nil }

// The load-bearing path - init() registrations queue, then replay in order once Install arrives
func TestQueueAndReplay(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	// simulate plugin init() - identity-bearing registrations hang off the handle
	p := RegisterPlugin(Manifest{Slug: "payment-stripe", Version: "0.1.0", SDKRange: "^0.1.0"})
	p.RegisterPermission("payments", "payments.view", "View payments")
	p.RegisterConfigField("payment/stripe/api_key", ConfigField{Type: TypeSecret})
	p.RegisterEvent("payment.captured", EventKindAsync)
	p.RegisterPaymentProvider("stripe", &noopProvider{}, WithDisplayName("Stripe"))

	host := &fakeHost{}

	if len(host.permissions) != 0 || len(host.providers) != 0 {
		t.Fatal("fake host received calls before Install - pending queue not isolated")
	}

	Install(host)

	if len(host.permissions) != 1 {
		t.Errorf("permissions: got %d, want 1", len(host.permissions))
	}
	if len(host.configs) != 1 {
		t.Errorf("configs: got %d, want 1", len(host.configs))
	}
	if len(host.events) != 1 {
		t.Errorf("events: got %d, want 1", len(host.events))
	}
	if len(host.providers) != 1 {
		t.Fatalf("providers: got %d, want 1", len(host.providers))
	}
	if host.providers[0].Slug != "stripe" {
		t.Errorf("provider slug: got %q, want stripe", host.providers[0].Slug)
	}
	if host.providers[0].DisplayName != "Stripe" {
		t.Errorf("WithDisplayName not applied: got %q", host.providers[0].DisplayName)
	}
	if host.providers[0].PluginSlug != "payment-stripe" {
		t.Errorf("provider PluginSlug: got %q, want payment-stripe", host.providers[0].PluginSlug)
	}
}

type noopCarrier struct{}

func (noopCarrier) Quote(context.Context, RateRequest) ([]RatedMethod, error) { return nil, nil }

func TestRegisterShippingCarrier(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	p := RegisterPlugin(Manifest{Slug: "shipping-flatzone", Version: "0.1.0", SDKRange: "^0.1.0"})
	p.RegisterShippingCarrier("flatzone", noopCarrier{}, WithCarrierDisplayName("Flat Zone"))

	host := &fakeHost{}
	Install(host)

	if len(host.carriers) != 1 {
		t.Fatalf("carriers: got %d, want 1", len(host.carriers))
	}
	if host.carriers[0].Slug != "flatzone" || host.carriers[0].DisplayName != "Flat Zone" {
		t.Errorf("carrier registration mangled: %+v", host.carriers[0])
	}
	if host.carriers[0].PluginSlug != "shipping-flatzone" {
		t.Errorf("carrier PluginSlug: got %q, want shipping-flatzone", host.carriers[0].PluginSlug)
	}
}

// Once Install ran, later calls hit the host directly instead of queuing
func TestPostInstallPassThrough(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	host := &fakeHost{}
	Install(host)

	p := RegisterPlugin(Manifest{Slug: "post-install", Version: "0.1.0", SDKRange: "^0.1.0"})
	p.RegisterPermission("payments", "payments.manage", "Manage")
	if len(host.permissions) != 1 {
		t.Fatalf("post-install Register did not hit host: got %d", len(host.permissions))
	}
}

// Install is single-shot
func TestDoubleInstallPanics(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	Install(&fakeHost{})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("second Install did not panic")
		}
		msg, _ := r.(string)
		if !strings.Contains(msg, "Install called twice") {
			t.Errorf("panic message: got %q, want substring %q", msg, "Install called twice")
		}
	}()
	Install(&fakeHost{})
}

// Empty namespace and nil FS both fail at Register* time, before Install runs
func TestRegisterMigrationsValidates(t *testing.T) {
	cases := []struct {
		name string
		reg  MigrationsRegistration
	}{
		{"empty namespace", MigrationsRegistration{FS: fstest.MapFS{}}},
		{"nil FS", MigrationsRegistration{Namespace: "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ResetForTest()
			t.Cleanup(ResetForTest)
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic")
				}
			}()
			RegisterPlugin(Manifest{Slug: "mig-test", Version: "0.1.0", SDKRange: "^0.1.0"}).RegisterMigrations(tc.reg)
		})
	}
}

// Both pre- and post-Install routes reach the host with the slug preserved and a buildable loader
func TestRegisterDataLoader(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	noopBatch := func(_ context.Context, keys []int) []*dataloaderResult[int] {
		out := make([]*dataloaderResult[int], len(keys))
		for i, k := range keys {
			out[i] = &dataloaderResult[int]{Data: k * 2}
		}
		return out
	}

	RegisterDataLoader[int, int]("test.double", noopBatch)

	host := &fakeHost{}
	if len(host.dataloaders) != 0 {
		t.Fatal("dataloaders received before Install")
	}
	Install(host)
	if len(host.dataloaders) != 1 {
		t.Fatalf("after Install: got %d regs, want 1", len(host.dataloaders))
	}
	if got := host.dataloaders[0].Slug(); got != "test.double" {
		t.Errorf("slug: got %q, want test.double", got)
	}
	built := host.dataloaders[0].Build()
	if built == nil {
		t.Error("Build returned nil - expected a *dataloader.Loader")
	}

	RegisterDataLoader[string, string]("test.upper", func(_ context.Context, keys []string) []*dataloaderResult[string] {
		out := make([]*dataloaderResult[string], len(keys))
		for i, k := range keys {
			out[i] = &dataloaderResult[string]{Data: strings.ToUpper(k)}
		}
		return out
	})
	if len(host.dataloaders) != 2 {
		t.Errorf("post-Install: got %d regs, want 2", len(host.dataloaders))
	}
}

// Inferring an owner from "whichever manifest registered most recently" lets a plugin registering in an unlucky order silently inherit another's identity - wrong kill switch, wrong metrics, wrong scheduled-job row key
// B registers after A's manifest and interleaves with it; every registration must still come back on the handle it was made through
func TestAttribution_NoCrossPluginBleed(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	a := RegisterPlugin(Manifest{Slug: "plugin-a", Version: "1.0.0", SDKRange: "^0.1.0"})
	b := RegisterPlugin(Manifest{Slug: "plugin-b", Version: "1.0.0", SDKRange: "^0.1.0"})

	// interleaved on purpose - order must not matter
	b.RegisterPaymentProvider("bpay", &noopProvider{})
	b.RegisterShippingCarrier("bship", noopCarrier{})
	a.RegisterPaymentProvider("apay", &noopProvider{})
	a.RegisterHealthCheck("api", func(context.Context) error { return nil })
	b.RegisterHealthCheck("api", func(context.Context) error { return nil })
	b.RegisterReadinessCheck("config", func(context.Context) error { return nil })
	a.RegisterScheduledJob("reconcile", "@every 1h", "", func(context.Context) error { return nil })
	b.RegisterScheduledJob("reconcile", "@every 1h", "", func(context.Context) error { return nil })
	b.SubscribeAsync("order.placed", func(context.Context, json.RawMessage) error { return nil })
	a.SubscribeSync("order.placing", 100, func(context.Context, any) error { return nil })
	a.RegisterGraphResolver(GraphAdmin, struct{}{})
	b.RegisterGraphResolver(GraphFront, struct{}{})

	host := &fakeHost{}
	Install(host)

	providerOwner := map[string]string{}
	for _, p := range host.providers {
		providerOwner[p.Slug] = p.PluginSlug
	}
	if got := providerOwner["apay"]; got != "plugin-a" {
		t.Errorf("provider apay attributed to %q, want plugin-a", got)
	}
	if got := providerOwner["bpay"]; got != "plugin-b" {
		t.Errorf("provider bpay attributed to %q, want plugin-b", got)
	}
	if len(host.carriers) != 1 || host.carriers[0].PluginSlug != "plugin-b" {
		t.Errorf("carrier bship attributed to %v, want plugin-b", host.carriers)
	}
	// both named their probe "api" and their job "reconcile" - the slug is all that keeps them apart downstream
	wantHealth := map[string]bool{"plugin-a": true, "plugin-b": true}
	gotHealth := map[string]bool{}
	for _, rec := range host.healthChecks {
		if rec.Name != "api" {
			t.Errorf("health check name: got %q, want api", rec.Name)
		}
		gotHealth[rec.PluginSlug] = true
	}
	if len(gotHealth) != len(wantHealth) {
		t.Errorf("health check owners: got %v, want %v", gotHealth, wantHealth)
	}
	gotJobs := map[string]bool{}
	for _, rec := range host.scheduledJobs {
		gotJobs[rec.PluginSlug] = true
	}
	if !gotJobs["plugin-a"] || !gotJobs["plugin-b"] {
		t.Errorf("scheduled job owners: got %v, want both plugins", gotJobs)
	}
	if len(host.readinessChecks) != 1 || host.readinessChecks[0].PluginSlug != "plugin-b" {
		t.Errorf("readiness check attributed to %q, want plugin-b", host.readinessChecks[0].PluginSlug)
	}
	if len(host.asyncSubs) != 1 || host.asyncSubs[0].PluginSlug != "plugin-b" {
		t.Errorf("async sub attributed to %q, want plugin-b", host.asyncSubs[0].PluginSlug)
	}
	if len(host.syncSubs) != 1 || host.syncSubs[0].PluginSlug != "plugin-a" {
		t.Errorf("sync sub attributed to %q, want plugin-a", host.syncSubs[0].PluginSlug)
	}
	// a graph resolver landing on the wrong plugin means the wrong kill switch gates someone's admin fields
	graphOwner := map[string]string{}
	for _, rec := range host.graphResolvers {
		graphOwner[rec.Name] = rec.PluginSlug
	}
	if got := graphOwner[string(GraphAdmin)]; got != "plugin-a" {
		t.Errorf("admin graph resolver attributed to %q, want plugin-a", got)
	}
	if got := graphOwner[string(GraphFront)]; got != "plugin-b" {
		t.Errorf("front graph resolver attributed to %q, want plugin-b", got)
	}
}

// Never invoked - it just gives RegisterPaymentProvider something to record
type noopProvider struct{}

func (noopProvider) Authorize(_ context.Context, _ AuthorizeRequest) (AuthorizeResult, error) {
	return AuthorizeResult{}, nil
}
func (noopProvider) Capture(_ context.Context, _ CaptureRequest) (CaptureResult, error) {
	return CaptureResult{}, nil
}
func (noopProvider) Refund(_ context.Context, _ RefundRequest) (RefundResult, error) {
	return RefundResult{}, nil
}
func (noopProvider) Void(_ context.Context, _ VoidRequest) (VoidResult, error) {
	return VoidResult{}, nil
}
func (noopProvider) HandleWebhook(_ context.Context, _ WebhookRequest) (WebhookResult, error) {
	return WebhookResult{}, nil
}

// keeps imports stable across edits
var (
	_ fs.FS = fstest.MapFS{}
	_ *http.Request
)

func (h *fakeHost) RegisterSearchIndex(pluginSlug string, reg SearchIndexRegistration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.searchIndexes = append(h.searchIndexes, pluginSlug+"/"+reg.Name)
	return nil
}
func (h *fakeHost) SearchIndexDoc(_ context.Context, _, _, _ string, _ any) error { return nil }
func (h *fakeHost) SearchIndexDocs(_ context.Context, _, _ string, _ []SearchDocument) error {
	return nil
}
func (h *fakeHost) SearchRefresh(_ context.Context, _, _ string) error      { return nil }
func (h *fakeHost) SearchDeleteDoc(_ context.Context, _, _, _ string) error { return nil }
func (h *fakeHost) SearchDeleteByField(_ context.Context, _, _, _, _ string) error {
	return nil
}
func (h *fakeHost) SearchQuery(_ context.Context, _, _ string, _ any) (SearchResult, error) {
	return SearchResult{}, nil
}
