package sdk

// Plugin init() runs before main(), so registrations queue until the host calls Install and then replay in order - that's what lets a plugin register without core importing it first
// A panic here aborts boot, which is what a duplicate slug or malformed path deserves

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sync"

	"github.com/google/uuid"
)

// What internal/plugin/host.go satisfies - plugins never see it
// Runtime-surface methods take the owning manifest slug explicitly, because inferring it from registration order was a source of silent cross-plugin bleed
type Host interface {
	RegisterPaymentProvider(reg ProviderRegistration) error
	RegisterShippingCarrier(reg CarrierRegistration) error
	RegisterPermission(pluginSlug, group, slug, name string) error
	RegisterConfigField(pluginSlug, path string, field ConfigField) error
	RegisterEvent(pluginSlug, name string, kind EventKind) error
	RegisterMigrations(pluginSlug string, reg MigrationsRegistration) error
	RegisterDataPatch(pluginSlug string, reg DataPatchRegistration) error
	SubscribeAsync(pluginSlug, eventType string, handler AsyncHandler) error
	SubscribeAsyncOnce(pluginSlug, eventType string, key DedupKeyFunc, handler AsyncHandler) error
	SubscribeSync(pluginSlug, eventType string, priority int, handler SyncHandler) error
	PublishEvent(ctx context.Context, pluginSlug, eventType string, payload any) error
	RegisterPromotionCondition(pluginSlug string, c PromotionCondition) error
	RegisterPromotionAction(pluginSlug string, a PromotionAction) error
	RegisterPromotionRedemptionHook(pluginSlug string, h PromotionRedemptionHook) error
	RegisterDataLoader(reg DataLoaderRegistration) error
	RegisterPlugin(m Manifest) error
	IsPluginEnabled(ctx context.Context, slug string, channelID uuid.UUID) bool
	RegisterHealthCheck(pluginSlug, name string, check HealthCheck) error
	RegisterReadinessCheck(pluginSlug, name string, check ReadinessCheck) error
	RegisterSearchIndex(pluginSlug string, reg SearchIndexRegistration) error
	SearchIndexDoc(ctx context.Context, pluginSlug, name, docID string, doc any) error
	SearchIndexDocs(ctx context.Context, pluginSlug, name string, docs []SearchDocument) error
	SearchRefresh(ctx context.Context, pluginSlug, name string) error
	SearchDeleteDoc(ctx context.Context, pluginSlug, name, docID string) error
	SearchDeleteByField(ctx context.Context, pluginSlug, name, field, value string) error
	SearchQuery(ctx context.Context, pluginSlug, name string, body any) (SearchResult, error)
	RegisterScheduledJob(pluginSlug, name, defaultSchedule, description string, handler ScheduledJobHandler) error
	RegisterShutdown(pluginSlug string, fn ShutdownFunc) error
	RegisterRoute(pluginSlug, pattern string, h RouteHandler) error
	RegisterGraphResolver(pluginSlug string, surface GraphSurface, impl any) error
	RegisterPriceResolver(pluginSlug string, r PriceResolver) error
	Database() (DB, error)
	CreateDraftOrder(ctx context.Context, req DraftOrderRequest) (DraftOrderResult, error)
	ReadCart(ctx context.Context, cartID uuid.UUID) (Cart, error)
	RequireAdminChannelPermission(ctx context.Context, slug string, channelID uuid.UUID) error
	PermittedChannels(ctx context.Context, slug string) ([]uuid.UUID, bool, error)
	ActingAdminID(ctx context.Context) *uuid.UUID
	RequestCustomerID(ctx context.Context) *uuid.UUID
	RequestChannelID(ctx context.Context) uuid.UUID
	RegisterAdminTranslations(pluginSlug string, reg AdminTranslationsRegistration) error
	RequireAdminPermission(ctx context.Context, slug string) error
	ReadConfigString(ctx context.Context, path string) (string, error)
	ReadConfigBool(ctx context.Context, path string) (bool, error)
	ReadConfigInt(ctx context.Context, path string) (int64, error)
}

// Namespace is the key in the plugin_migrations tracking table and must be unique FS holds <NNNN>_<name>.up.sql / .down.sql pairs numbered from 1 with no gaps, applied in version order, each in its own transaction - a failure leaves the namespace at the previous version and the next run retries Core tables always exist by the time these run, but referencing ANOTHER plugin's tables is unsafe - cross-plugin ordering is undefined
type MigrationsRegistration struct {
	Namespace string
	FS        fs.FS
	Root      string // directory inside FS to scan, defaults to "migrations"
}

var (
	hostMu     sync.Mutex
	host       Host
	pendingOps []func(Host) error // replayed when host arrives
)

// Called once at boot, replaying whatever plugin init() queued before the host existed
func Install(h Host) {
	hostMu.Lock()
	defer hostMu.Unlock()
	if host != nil {
		panic("sdk: Install called twice")
	}
	host = h
	for _, op := range pendingOps {
		if err := op(h); err != nil {
			// only a few shapes reach here - duplicate slug, bad path - and all beat a half-loaded server
			panic(fmt.Sprintf("sdk: deferred registration failed: %v", err))
		}
	}
	pendingOps = nil
}

// A handle without RegisterPlugin's side effects - it also registers config fields, and config panics on a duplicate path, so a per-case manifest blows up the second test reusing a slug
// Never called in production, where identity comes from the manifest
func NewPluginForTest(slug string) *Plugin {
	return &Plugin{slug: slug}
}

// Between-scenario reset so global state doesn't leak across cases
func ResetForTest() {
	hostMu.Lock()
	defer hostMu.Unlock()
	host = nil
	pendingOps = nil
	manifestMu.Lock()
	recordedManifests = nil
	manifestMu.Unlock()
}

func withHost(op func(Host) error) {
	hostMu.Lock()
	defer hostMu.Unlock()
	if host != nil {
		if err := op(host); err != nil {
			panic(fmt.Sprintf("sdk: registration failed: %v", err))
		}
		return
	}
	pendingOps = append(pendingOps, op)
}

// For read-side helpers like IsEnabled that must answer now rather than queue - they run on the request path, long after init
func hostNow() Host {
	hostMu.Lock()
	defer hostMu.Unlock()
	return host
}

// providerSlug keys config paths (payment/<slug>/*), the webhook route /api/v1/payments/webhook/<slug> and the checkout payload's providerSlug - independent of the manifest slug, so one plugin can ship several providers while the kill switch and metrics stay keyed on the manifest
// Panics on a duplicate provider slug
func (pl *Plugin) RegisterPaymentProvider(providerSlug string, p PaymentProvider, opts ...ProviderOption) {
	reg := ProviderRegistration{PluginSlug: pl.slug, Slug: providerSlug, Provider: p}
	for _, opt := range opts {
		opt(&reg)
	}
	withHost(func(h Host) error { return h.RegisterPaymentProvider(reg) })
}

// Admin-facing slugs use dot notation (catalog.view), app-facing scopes a verb prefix (read:products) - both land in the same table, and duplicates panic A method rather than a package function so every registration carries its owner - attribution is what lets uninstall find a plugin's footprint and the host refuse cross-plugin claims
func (pl *Plugin) RegisterPermission(group, slug, name string) {
	withHost(func(h Host) error { return h.RegisterPermission(pl.slug, group, slug, name) })
}

// Path is "section/group/field" - convention is domain as section, provider slug as group, e.g. "payment/stripe/api_key" - and duplicates panic
func (pl *Plugin) RegisterConfigField(path string, field ConfigField) {
	withHost(func(h Host) error { return h.RegisterConfigField(pl.slug, path, field) })
}

// Makes the name selectable by OUTBOUND webhook subscribers - it does NOT gate SubscribeAsync, which attaches to a bus topic by string either way
func (pl *Plugin) RegisterEvent(name string, kind EventKind) {
	withHost(func(h Host) error { return h.RegisterEvent(pl.slug, name, kind) })
}

// Apply with `bin/cli plugin migrate up <namespace>` - duplicates panic
// Takes an embed.FS of migrations/*.sql under a namespace of the plugin's own
func (pl *Plugin) RegisterMigrations(reg MigrationsRegistration) {
	if reg.Root == "" {
		reg.Root = "migrations"
	}
	if reg.FS == nil {
		panic("sdk.RegisterMigrations: FS is nil")
	}
	if reg.Namespace == "" {
		panic("sdk.RegisterMigrations: Namespace is empty")
	}
	withHost(func(h Host) error { return h.RegisterMigrations(pl.slug, reg) })
}

// Run-once Go code rather than RegisterMigrations' SQL files - read an embedded CSV, call an API, transform rows - against the patch's own *sql.Tx, applied by `bin/cli data-patch up` after schema migrations
type DataPatchRegistration struct {
	Namespace string
	ID        string   // the run-once marker, globally unique and stable - convention "<namespace>:<name>"
	DependsOn []string // patch IDs that must run first
	Apply     func(ctx context.Context, tx *sql.Tx) error
}

func (pl *Plugin) RegisterDataPatch(reg DataPatchRegistration) {
	if reg.ID == "" {
		panic("sdk.RegisterDataPatch: ID is empty")
	}
	if reg.Apply == nil {
		panic("sdk.RegisterDataPatch: Apply is nil")
	}
	withHost(func(h Host) error { return h.RegisterDataPatch(pl.slug, reg) })
}
