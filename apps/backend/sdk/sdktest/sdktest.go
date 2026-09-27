// Package sdktest is the in-memory Host for plugin tests - a plugin registers through the real
// SDK, then the test drives events, config, the kill switch and permissions against this fake
// instead of booting the app
//
// Fidelity over convenience where the two conflict: duplicate slugs error like boot does, config
// reads resolve registered defaults, and event delivery skips disabled plugins the way the host's
// gating wrappers do
package sdktest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Permission is one RegisterPermission call, kept flat for assertions
type Permission struct {
	PluginSlug        string
	Group, Slug, Name string
}

// Job is one RegisterScheduledJob call - Name is unqualified, the real host prefixes the slug
type Job struct {
	PluginSlug, Name, Schedule, Description string
	Handler                                 sdk.ScheduledJobHandler
}

// Probe is a health or readiness check with its owning slug
type Probe struct {
	PluginSlug, Name string
	Check            func(ctx context.Context) error
}

// AdminTranslation is one RegisterAdminTranslations call, recorded verbatim
type AdminTranslation struct {
	PluginSlug string
	Reg        sdk.AdminTranslationsRegistration
}

// Route is one RegisterRoute call, pattern still relative to the plugin's namespace
type Route struct {
	PluginSlug, Pattern string
	Handler             sdk.RouteHandler
}

// GraphResolver is one RegisterGraphResolver call
type GraphResolver struct {
	PluginSlug string
	Surface    sdk.GraphSurface
	Impl       any
}

type asyncSub struct {
	slug string
	h    sdk.AsyncHandler
}

type syncSub struct {
	slug     string
	priority int
	order    int
	always   bool // SubscribeSyncAlways - runs past the kill switch, like the host's
	h        sdk.SyncHandler
}

// Host records every registration and answers the runtime surface from in-memory state
// Registration fields are exported for direct assertion; mutate runtime state through the helpers
type Host struct {
	mu sync.Mutex

	Manifests    []sdk.Manifest
	Providers    map[string]sdk.ProviderRegistration
	Carriers     map[string]sdk.CarrierRegistration
	Permissions  []Permission
	ConfigFields map[string]sdk.ConfigField
	Events       map[string]sdk.EventKind
	Migrations   []sdk.MigrationsRegistration
	DataPatches  []sdk.DataPatchRegistration
	// owner slugs per registration key - what attribution records, so tests can assert it
	ConfigFieldOwners   map[string]string
	EventOwners         map[string]string
	MigrationOwners     map[string]string
	DataPatchOwners     map[string]string
	PromotionConditions map[string]sdk.PromotionCondition
	PromotionActions    map[string]sdk.PromotionAction
	PromotionHooks      map[string]sdk.PromotionRedemptionHook
	DataLoaders         []sdk.DataLoaderRegistration
	HealthChecks        []Probe
	ReadinessChecks     []Probe
	Jobs                []Job
	ShutdownFns         map[string][]sdk.ShutdownFunc
	Routes              []Route
	GraphResolvers      []GraphResolver
	PriceResolvers      []PriceResolver
	DraftOrders         []DraftOrder
	Carts               map[uuid.UUID]sdk.Cart
	// Per-permission channel scope - an absent slug bypasses, matching a global grant
	ChannelScope map[string][]uuid.UUID
	// What sdk.AdminID reports - nil is the integration-token and unauthenticated case
	ActingAdmin *uuid.UUID
	// What sdk.CustomerID and sdk.RequestChannelID report on the storefront surface
	RequestCustomer *uuid.UUID
	RequestChannel  uuid.UUID
	// What CreateDraftOrder returns - nil makes it fail like an unwired host
	DraftOrderFunc func(context.Context, sdk.DraftOrderRequest) (sdk.DraftOrderResult, error)
	// The handle sdk.Database hands back - nil until SetDatabase wires one
	DB                sdk.DB
	SearchIndexes     map[string]sdk.SearchIndexRegistration
	SearchDocs        map[string]map[string]any
	AdminTranslations []AdminTranslation

	asyncSubs map[string][]asyncSub
	syncSubs  map[string][]syncSub
	syncSeq   int

	values   map[string]string
	disabled map[string]bool
	denied   map[string]bool
	claimed  map[string]bool
}

// Install wires a fresh fake host into the SDK and returns it
// sdk.Install replays init()-queued registrations only into the FIRST host of the process, exactly
// like the real boot - so plugins registering from init() install once (TestMain), while tests
// registering through sdk.NewPluginForTest can take a fresh host per case
func Install() *Host {
	h := New()
	sdk.ResetForTest()
	sdk.Install(h)
	return h
}

// New builds the fake without installing it, for tests that hand it to sdk.Install themselves
func New() *Host {
	return &Host{
		SearchIndexes:       map[string]sdk.SearchIndexRegistration{},
		SearchDocs:          map[string]map[string]any{},
		Providers:           map[string]sdk.ProviderRegistration{},
		Carriers:            map[string]sdk.CarrierRegistration{},
		ConfigFields:        map[string]sdk.ConfigField{},
		ConfigFieldOwners:   map[string]string{},
		EventOwners:         map[string]string{},
		MigrationOwners:     map[string]string{},
		DataPatchOwners:     map[string]string{},
		Events:              map[string]sdk.EventKind{},
		PromotionConditions: map[string]sdk.PromotionCondition{},
		PromotionActions:    map[string]sdk.PromotionAction{},
		PromotionHooks:      map[string]sdk.PromotionRedemptionHook{},
		ShutdownFns:         map[string][]sdk.ShutdownFunc{},
		asyncSubs:           map[string][]asyncSub{},
		syncSubs:            map[string][]syncSub{},
		values:              map[string]string{},
		disabled:            map[string]bool{},
		denied:              map[string]bool{},
		claimed:             map[string]bool{},
	}
}

// ---- test-side helpers ----

// SetConfig stores a value the ReadConfig* methods resolve before falling back to the field default
func (h *Host) SetConfig(path, value string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.values[path] = value
}

// Disable flips the plugin's kill switch: IsPluginEnabled answers false and delivery skips its handlers
func (h *Host) Disable(slug string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.disabled[slug] = true
}

func (h *Host) Enable(slug string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.disabled, slug)
}

// DenyPermission makes RequireAdminPermission fail for one permission slug - default is allow-all
func (h *Host) DenyPermission(slug string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.denied[slug] = true
}

// ResetState clears config values, kill switches and permission denials between cases - registrations stay, mirroring one boot serving many requests
func (h *Host) ResetState() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.values = map[string]string{}
	h.disabled = map[string]bool{}
	h.denied = map[string]bool{}
	h.claimed = map[string]bool{}
}

// Deliver marshals payload and runs the async subscribers for eventType, skipping disabled plugins like the host's
// gating wrapper - errors are returned joined instead of logged and dropped, so a test can assert on them
func (h *Host) Deliver(ctx context.Context, eventType string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("sdktest: marshal %s payload: %w", eventType, err)
	}
	h.mu.Lock()
	subs := append([]asyncSub(nil), h.asyncSubs[eventType]...)
	h.mu.Unlock()
	var errs []error
	for _, s := range subs {
		if h.pluginDisabled(s.slug) {
			continue
		}
		if err := s.h(ctx, raw); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// DeliverSync runs the sync subscribers in priority order (ties keep registration order), passing the live value so handlers mutate it -
// the first error aborts, and disabled plugins are skipped without aborting unless they subscribed with SubscribeSyncAlways, all mirroring the host
func (h *Host) DeliverSync(ctx context.Context, eventType string, e any) error {
	h.mu.Lock()
	subs := append([]syncSub(nil), h.syncSubs[eventType]...)
	h.mu.Unlock()
	sort.SliceStable(subs, func(i, j int) bool {
		if subs[i].priority != subs[j].priority {
			return subs[i].priority < subs[j].priority
		}
		return subs[i].order < subs[j].order
	})
	for _, s := range subs {
		if !s.always && h.pluginDisabled(s.slug) {
			continue
		}
		if err := s.h(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// Shutdown runs every registered hook like pluginhost.Shutdown, returning the joined errors
func (h *Host) Shutdown(ctx context.Context) error {
	h.mu.Lock()
	var fns []sdk.ShutdownFunc
	for _, hooks := range h.ShutdownFns {
		fns = append(fns, hooks...)
	}
	h.mu.Unlock()
	var errs []error
	for _, fn := range fns {
		if err := fn(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (h *Host) pluginDisabled(slug string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.disabled[slug]
}

// ---- sdk.Host ----

func (h *Host) RegisterPlugin(m sdk.Manifest) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, existing := range h.Manifests {
		if existing.Slug == m.Slug {
			return fmt.Errorf("sdktest: duplicate plugin slug %q", m.Slug)
		}
	}
	h.Manifests = append(h.Manifests, m)
	return nil
}

func (h *Host) RegisterPaymentProvider(reg sdk.ProviderRegistration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, dup := h.Providers[reg.Slug]; dup {
		return fmt.Errorf("sdktest: duplicate payment provider %q", reg.Slug)
	}
	h.Providers[reg.Slug] = reg
	return nil
}

func (h *Host) RegisterShippingCarrier(reg sdk.CarrierRegistration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, dup := h.Carriers[reg.Slug]; dup {
		return fmt.Errorf("sdktest: duplicate shipping carrier %q", reg.Slug)
	}
	h.Carriers[reg.Slug] = reg
	return nil
}

func (h *Host) RegisterPermission(pluginSlug, group, slug, name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Permissions = append(h.Permissions, Permission{PluginSlug: pluginSlug, Group: group, Slug: slug, Name: name})
	return nil
}

func (h *Host) RegisterConfigField(pluginSlug, path string, field sdk.ConfigField) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if owner, dup := h.ConfigFieldOwners[path]; dup && owner != pluginSlug {
		return fmt.Errorf("sdktest: config field %q already registered by plugin %s", path, owner)
	}
	if _, dup := h.ConfigFields[path]; dup {
		return fmt.Errorf("sdktest: duplicate config field %q", path)
	}
	h.ConfigFields[path] = field
	h.ConfigFieldOwners[path] = pluginSlug
	return nil
}

func (h *Host) RegisterEvent(pluginSlug, name string, kind sdk.EventKind) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Events[name] = kind
	h.EventOwners[name] = pluginSlug
	return nil
}

func (h *Host) RegisterMigrations(pluginSlug string, reg sdk.MigrationsRegistration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Migrations = append(h.Migrations, reg)
	h.MigrationOwners[reg.Namespace] = pluginSlug
	return nil
}

func (h *Host) RegisterDataPatch(pluginSlug string, reg sdk.DataPatchRegistration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.DataPatches = append(h.DataPatches, reg)
	h.DataPatchOwners[reg.ID] = pluginSlug
	return nil
}

func (h *Host) SubscribeAsync(pluginSlug, eventType string, handler sdk.AsyncHandler) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.asyncSubs[eventType] = append(h.asyncSubs[eventType], asyncSub{slug: pluginSlug, h: handler})
	return nil
}

// SubscribeAsyncOnce mirrors the host's claim/run/release-on-error semantics with an in-memory
// marker, so a plugin's replay tolerance is testable without Postgres
func (h *Host) SubscribeAsyncOnce(pluginSlug, eventType string, key sdk.DedupKeyFunc, handler sdk.AsyncHandler) error {
	if key == nil {
		return fmt.Errorf("sdktest: SubscribeAsyncOnce requires a key function")
	}
	wrapped := func(ctx context.Context, payload json.RawMessage) error {
		k, err := key(payload)
		if err != nil {
			return err
		}
		ck := pluginSlug + "\x00" + eventType + "\x00" + k
		h.mu.Lock()
		if h.claimed[ck] {
			h.mu.Unlock()
			return nil
		}
		h.claimed[ck] = true
		h.mu.Unlock()
		if err := handler(ctx, payload); err != nil {
			h.mu.Lock()
			delete(h.claimed, ck)
			h.mu.Unlock()
			return err
		}
		return nil
	}
	return h.SubscribeAsync(pluginSlug, eventType, wrapped)
}

func (h *Host) SubscribeSync(pluginSlug, eventType string, priority int, handler sdk.SyncHandler) error {
	return h.subscribeSync(pluginSlug, eventType, priority, handler, false)
}

func (h *Host) SubscribeSyncAlways(pluginSlug, eventType string, priority int, handler sdk.SyncHandler) error {
	return h.subscribeSync(pluginSlug, eventType, priority, handler, true)
}

func (h *Host) subscribeSync(pluginSlug, eventType string, priority int, handler sdk.SyncHandler, always bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.syncSeq++
	h.syncSubs[eventType] = append(h.syncSubs[eventType], syncSub{slug: pluginSlug, priority: priority, order: h.syncSeq, always: always, h: handler})
	return nil
}

func (h *Host) RegisterPromotionCondition(pluginSlug string, c sdk.PromotionCondition) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, dup := h.PromotionConditions[c.Slug()]; dup {
		return fmt.Errorf("sdktest: duplicate promotion condition %q", c.Slug())
	}
	h.PromotionConditions[c.Slug()] = c
	return nil
}

func (h *Host) RegisterPromotionAction(pluginSlug string, a sdk.PromotionAction) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, dup := h.PromotionActions[a.Slug()]; dup {
		return fmt.Errorf("sdktest: duplicate promotion action %q", a.Slug())
	}
	h.PromotionActions[a.Slug()] = a
	return nil
}

func (h *Host) RegisterPromotionRedemptionHook(pluginSlug string, hook sdk.PromotionRedemptionHook) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, dup := h.PromotionHooks[hook.Slug()]; dup {
		return fmt.Errorf("sdktest: duplicate promotion redemption hook %q", hook.Slug())
	}
	h.PromotionHooks[hook.Slug()] = hook
	return nil
}

func (h *Host) RegisterDataLoader(reg sdk.DataLoaderRegistration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.DataLoaders = append(h.DataLoaders, reg)
	return nil
}

func (h *Host) IsPluginEnabled(_ context.Context, slug string, _ uuid.UUID) bool {
	return !h.pluginDisabled(slug)
}

func (h *Host) RegisterHealthCheck(pluginSlug, name string, check sdk.HealthCheck) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.HealthChecks = append(h.HealthChecks, Probe{PluginSlug: pluginSlug, Name: name, Check: check})
	return nil
}

func (h *Host) RegisterReadinessCheck(pluginSlug, name string, check sdk.ReadinessCheck) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ReadinessChecks = append(h.ReadinessChecks, Probe{PluginSlug: pluginSlug, Name: name, Check: check})
	return nil
}

func (h *Host) RegisterAdminTranslations(pluginSlug string, reg sdk.AdminTranslationsRegistration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.AdminTranslations = append(h.AdminTranslations, AdminTranslation{PluginSlug: pluginSlug, Reg: reg})
	return nil
}

func (h *Host) RegisterScheduledJob(pluginSlug, name, defaultSchedule, description string, handler sdk.ScheduledJobHandler) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Jobs = append(h.Jobs, Job{PluginSlug: pluginSlug, Name: name, Schedule: defaultSchedule, Description: description, Handler: handler})
	return nil
}

func (h *Host) RegisterShutdown(pluginSlug string, fn sdk.ShutdownFunc) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ShutdownFns[pluginSlug] = append(h.ShutdownFns[pluginSlug], fn)
	return nil
}

func (h *Host) RegisterRoute(pluginSlug, pattern string, handler sdk.RouteHandler) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.Routes {
		if r.PluginSlug == pluginSlug && r.Pattern == pattern {
			return fmt.Errorf("sdktest: duplicate route %q on plugin %q", pattern, pluginSlug)
		}
	}
	h.Routes = append(h.Routes, Route{PluginSlug: pluginSlug, Pattern: pattern, Handler: handler})
	return nil
}

func (h *Host) RegisterGraphResolver(pluginSlug string, surface sdk.GraphSurface, impl any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.GraphResolvers = append(h.GraphResolvers, GraphResolver{PluginSlug: pluginSlug, Surface: surface, Impl: impl})
	return nil
}

// PriceResolver is one RegisterPriceResolver call with its owning slug
type PriceResolver struct {
	PluginSlug string
	Resolver   sdk.PriceResolver
}

func (h *Host) RegisterPriceResolver(pluginSlug string, r sdk.PriceResolver) error {
	if r == nil {
		return fmt.Errorf("sdktest: plugin %q registered a nil price resolver", pluginSlug)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.PriceResolvers = append(h.PriceResolvers, PriceResolver{PluginSlug: pluginSlug, Resolver: r})
	return nil
}

// ResolvePrice runs the registered resolvers the way core does - first non-fall-through wins, a
// disabled plugin is skipped, and an error reads as a fall-through rather than propagating
func (h *Host) ResolvePrice(ctx context.Context, req sdk.PriceRequest) (int64, bool) {
	h.mu.Lock()
	regs := slices.Clone(h.PriceResolvers)
	h.mu.Unlock()
	for _, reg := range regs {
		if h.pluginDisabled(reg.PluginSlug) {
			continue
		}
		cents, ok, err := reg.Resolver.ResolveUnitPrice(ctx, req)
		if err == nil && ok {
			return cents, true
		}
	}
	return 0, false
}

// Database hands back whatever SetDatabase was given - nil until a test wires one, which is the
// same ErrDBUnavailable a plugin resolving its handle too early sees in production
func (h *Host) Database() (sdk.DB, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.DB == nil {
		return nil, sdk.ErrDBUnavailable
	}
	return h.DB, nil
}

// SetDatabase wires the handle sdk.Database returns - pass a *sql.DB against a test database, or a
// fake implementing the four methods
func (h *Host) SetDatabase(db sdk.DB) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.DB = db
}

// DraftOrders records every CreateDraftOrder call, and DraftOrderFunc lets a test decide what comes
// back - unset, the call fails the way an unwired host does, so a plugin's error path is reachable
type DraftOrder struct {
	Request sdk.DraftOrderRequest
}

func (h *Host) CreateDraftOrder(ctx context.Context, req sdk.DraftOrderRequest) (sdk.DraftOrderResult, error) {
	h.mu.Lock()
	h.DraftOrders = append(h.DraftOrders, DraftOrder{Request: req})
	fn := h.DraftOrderFunc
	h.mu.Unlock()
	if fn == nil {
		return sdk.DraftOrderResult{}, sdk.ErrDraftOrdersUnavailable
	}
	return fn(ctx, req)
}

// Carts is what ReadCart serves, keyed by id - an unseeded id answers ErrCartNotOnStore, the same
// refusal a foreign cart gets, so a plugin's not-found path is reachable without a database
func (h *Host) ReadCart(_ context.Context, cartID uuid.UUID) (sdk.Cart, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c, ok := h.Carts[cartID]
	if !ok {
		return sdk.Cart{}, sdk.ErrCartNotOnStore
	}
	return c, nil
}

// SeedCart makes a cart readable through sdk.ReadCart
func (h *Host) SeedCart(c sdk.Cart) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Carts == nil {
		h.Carts = make(map[uuid.UUID]sdk.Cart)
	}
	h.Carts[c.ID] = c
}

// Channel scope for tests: DenyPermission still refuses outright, and ChannelScope names the channels
// the caller may see. Unset, the caller bypasses - which is the fake's see-everything default, not a
// claim about production, where an unauthenticated caller never reaches a gated field
func (h *Host) RequireAdminChannelPermission(ctx context.Context, slug string, channelID uuid.UUID) error {
	if err := h.RequireAdminPermission(ctx, slug); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	ids, scoped := h.ChannelScope[slug]
	if !scoped {
		return nil
	}
	if slices.Contains(ids, channelID) {
		return nil
	}
	return fmt.Errorf("sdktest: permission %q is not granted on channel %s", slug, channelID)
}

func (h *Host) PermittedChannels(ctx context.Context, slug string) ([]uuid.UUID, bool, error) {
	if err := h.RequireAdminPermission(ctx, slug); err != nil {
		return nil, false, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	ids, scoped := h.ChannelScope[slug]
	if !scoped {
		return nil, true, nil
	}
	return slices.Clone(ids), false, nil
}

// ScopeToChannels restricts one permission to these channels, so a test can prove a plugin filters
// its listings rather than only that it checks the permission
func (h *Host) ScopeToChannels(slug string, ids ...uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ChannelScope == nil {
		h.ChannelScope = make(map[string][]uuid.UUID)
	}
	h.ChannelScope[slug] = ids
}

// ActingAdmin is what AdminID reports - set it to prove a plugin stamps the right admin on an audit
// row, leave it nil for the integration-token and unauthenticated paths
func (h *Host) ActingAdminID(context.Context) *uuid.UUID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ActingAdmin
}

// RequestCustomer and RequestChannel are what the two request-scope readers report - set them to
// drive a front resolver's guest and signed-in paths without a real request
func (h *Host) RequestCustomerID(context.Context) *uuid.UUID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.RequestCustomer
}

func (h *Host) RequestChannelID(context.Context) uuid.UUID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.RequestChannel
}

// PublishEvent mirrors the host's Emits enforcement, then delivers to async subscribers
// immediately - so plugin-to-plugin event flows are testable in one call
func (h *Host) PublishEvent(ctx context.Context, pluginSlug, eventType string, payload any) error {
	if eventType == "" {
		return fmt.Errorf("sdktest: plugin %q published an empty event type", pluginSlug)
	}
	h.mu.Lock()
	var manifest *sdk.Manifest
	for i := range h.Manifests {
		if h.Manifests[i].Slug == pluginSlug {
			manifest = &h.Manifests[i]
			break
		}
	}
	h.mu.Unlock()
	if manifest == nil {
		return fmt.Errorf("sdktest: plugin %q published before RegisterPlugin", pluginSlug)
	}
	if !slices.Contains(manifest.Emits, eventType) {
		return fmt.Errorf("sdktest: plugin %q publishes %q but its manifest does not declare it in Emits", pluginSlug, eventType)
	}
	return h.Deliver(ctx, eventType, payload)
}

func (h *Host) RequireAdminPermission(_ context.Context, slug string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.denied[slug] {
		return fmt.Errorf("sdktest: permission %q denied", slug)
	}
	return nil
}

// The read trio resolves set values first, then the registered field's default - the same
// resolution order a real config service gives an unset path
func (h *Host) ReadConfigString(_ context.Context, path string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if v, ok := h.values[path]; ok {
		return v, nil
	}
	return h.ConfigFields[path].DefaultValue, nil
}

func (h *Host) ReadConfigBool(ctx context.Context, path string) (bool, error) {
	raw, _ := h.ReadConfigString(ctx, path)
	if raw == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("sdktest: %s is not a bool: %w", path, err)
	}
	return v, nil
}

func (h *Host) ReadConfigInt(ctx context.Context, path string) (int64, error) {
	raw, _ := h.ReadConfigString(ctx, path)
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("sdktest: %s is not an int: %w", path, err)
	}
	return v, nil
}

// ---- plugin search indexes ----

// SearchIndexes records registrations; SearchDocs holds an in-memory doc store per
// "<plugin>/<name>" so a plugin's event-driven indexing is testable without a
// cluster. Search answers from that store with NO query semantics - it returns
// every stored doc, because faking OpenSearch scoring here would test the fake
func (h *Host) RegisterSearchIndex(pluginSlug string, reg sdk.SearchIndexRegistration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := pluginSlug + "/" + reg.Name
	if _, dup := h.SearchIndexes[key]; dup {
		return fmt.Errorf("sdktest: duplicate search index %q", key)
	}
	h.SearchIndexes[key] = reg
	if h.SearchDocs[key] == nil {
		h.SearchDocs[key] = map[string]any{}
	}
	return nil
}

func (h *Host) searchStore(pluginSlug, name string) (map[string]any, error) {
	key := pluginSlug + "/" + name
	if _, ok := h.SearchIndexes[key]; !ok {
		return nil, fmt.Errorf("sdktest: search index %q is not registered", key)
	}
	return h.SearchDocs[key], nil
}

func (h *Host) SearchIndexDoc(_ context.Context, pluginSlug, name, docID string, doc any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	store, err := h.searchStore(pluginSlug, name)
	if err != nil {
		return err
	}
	store[docID] = doc
	return nil
}

func (h *Host) SearchIndexDocs(_ context.Context, pluginSlug, name string, docs []sdk.SearchDocument) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	store, err := h.searchStore(pluginSlug, name)
	if err != nil {
		return err
	}
	for _, d := range docs {
		store[d.ID] = d.Doc
	}
	return nil
}

// The in-memory store is always current, so refresh has nothing to do
func (h *Host) SearchRefresh(_ context.Context, pluginSlug, name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.searchStore(pluginSlug, name)
	return err
}

func (h *Host) SearchDeleteDoc(_ context.Context, pluginSlug, name, docID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	store, err := h.searchStore(pluginSlug, name)
	if err != nil {
		return err
	}
	delete(store, docID)
	return nil
}

// Matches on the doc's marshalled JSON carrying "<field>":"<value>" - enough to
// test "deleting record X clears its docs" without reimplementing term queries
func (h *Host) SearchDeleteByField(_ context.Context, pluginSlug, name, field, value string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	store, err := h.searchStore(pluginSlug, name)
	if err != nil {
		return err
	}
	needle := fmt.Sprintf("%q:%q", field, value)
	for id, doc := range store {
		raw, merr := json.Marshal(doc)
		if merr != nil {
			continue
		}
		if strings.Contains(string(raw), needle) {
			delete(store, id)
		}
	}
	return nil
}

func (h *Host) SearchQuery(_ context.Context, pluginSlug, name string, _ any) (sdk.SearchResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	store, err := h.searchStore(pluginSlug, name)
	if err != nil {
		return sdk.SearchResult{}, err
	}
	out := sdk.SearchResult{Total: int64(len(store))}
	ids := make([]string, 0, len(store))
	for id := range store {
		ids = append(ids, id)
	}
	sort.Strings(ids) // stable order so assertions don't flake
	for _, id := range ids {
		raw, merr := json.Marshal(store[id])
		if merr != nil {
			return sdk.SearchResult{}, merr
		}
		out.Hits = append(out.Hits, sdk.SearchHit{ID: id, Source: raw})
	}
	return out, nil
}

// The fake stands in for the real host on both interfaces a plugin can reach
var (
	_ sdk.Host           = (*Host)(nil)
	_ sdk.AlwaysSyncHost = (*Host)(nil)
)
