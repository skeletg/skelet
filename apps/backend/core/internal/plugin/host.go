// Package plugin bridges the public SDK to the internal registries: plugins call sdk.Register* from init(), the SDK queues them until main.go installs this host, which replays them onto internal/* registries - Keeps the SDK free of internal imports - Registration panics propagate to main.go (a bad plugin fails boot loudly)
package plugin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/datapatch"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
	"github.com/skeletg/skelet/apps/backend/core/internal/pluginmigrate"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/search"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/internal/webhook"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Host implements sdk.Host and carries the runtime deps a plugin needs, scoped to its own namespace - Install() replays the SDK's queued registrations before configSvc exists
// SetConfigService() arrives later, so configSvc is an atomic.Pointer and a config read before it lands returns ErrConfigUnavailable, which flags a boot-order bug
type Host struct {
	configSvc atomic.Pointer[config.Service]

	// wired after adminSvc exists, like configSvc - a pointer to the interface because the concrete
	// checker is chosen by main.go, and the read path stays lock-free
	permChecker atomic.Pointer[auth.PermissionChecker]

	// wired like configSvc - backs sdk.SubscribeAsyncOnce, nil degrades it to plain SubscribeAsync
	eventDedup atomic.Pointer[EventDedupStore]

	// the shared pool behind sdk.Database, wired after main.go opens it - lazy like configSvc, since
	// plugin init() runs long before there is a database to hand out
	db atomic.Pointer[sql.DB]

	// bus is lazy-wired; subscriptions queue until SetEventBus replays them (pendingMu covers both)
	bus          atomic.Pointer[events.Bus]
	pendingMu    sync.Mutex
	pendingAsync []pendingAsyncSub
	pendingSync  []pendingSyncSub

	dataloaderMu    sync.Mutex
	dataloaderRegs  []sdk.DataLoaderRegistration
	dataloaderSlugs map[string]struct{}

	manifestMu     sync.Mutex
	manifests      []sdk.Manifest
	manifestBySlug map[string]sdk.Manifest

	// Plugin search indexes queue like bus subscriptions do - SetSearchTier replays
	// pendingSearch once the tier exists, or drops it when search is unconfigured
	searchMu       sync.Mutex
	searchRegistry *search.IndexRegistry
	searchClient   *search.Client
	searchDisabled bool
	searchIndexes  map[string]sdk.SearchIndexRegistration
	pendingSearch  []pendingSearchIndex
}

type pendingAsyncSub struct {
	slug      string
	eventType string
	handler   sdk.AsyncHandler
	retry     bool // SubscribeAsyncOnce - a failure goes back to the outbox for redelivery
}

type pendingSyncSub struct {
	slug      string
	eventType string
	priority  int
	handler   sdk.SyncHandler
	always    bool // SubscribeSyncAlways - runs past the kill switch
}

// The package-level reference so SetConfigService can reach the Host after Install - one instance, since there's exactly one Host per server process
var installedHost atomic.Pointer[Host]

func init() {
	// the "plugin" config section parents the per-plugin groups RegisterPlugin adds - registered
	// here so it exists before any plugin's init() (which imports internal/plugin transitively)
	config.RegisterSection(config.Section{
		Slug:      "plugin",
		Label:     "Plugins",
		SortOrder: 80,
	})
}

// Registers a fresh Host with the SDK and replays queued plugin Register* calls - call once at startup before Validate(), with configSvc wired later through SetConfigService
func Install() {
	h := &Host{}
	if !installedHost.CompareAndSwap(nil, h) {
		panic("plugin.Install called twice")
	}
	sdk.Install(h)
}

// Wires the runtime config service, after which plugins can read their namespaced config - calling it twice is a programming error, since the second call would silently swap config
func SetConfigService(svc *config.Service) {
	h := installedHost.Load()
	if h == nil {
		panic("plugin.SetConfigService called before plugin.Install")
	}
	if !h.configSvc.CompareAndSwap(nil, svc) {
		panic("plugin.SetConfigService called twice")
	}
}

// Wires the admin permission checker that backs sdk.RequirePermission - called once, after adminSvc
// exists, for the same reason SetConfigService is
func SetPermissionChecker(c auth.PermissionChecker) {
	h := installedHost.Load()
	if h == nil {
		panic("plugin.SetPermissionChecker called before plugin.Install")
	}
	if !h.permChecker.CompareAndSwap(nil, &c) {
		panic("plugin.SetPermissionChecker called twice")
	}
}

// Wires the pool behind sdk.Database, once main.go has opened it - once only, like the two above,
// since swapping a plugin's database under it mid-run is never intentional
func SetDatabase(db *sql.DB) {
	h := installedHost.Load()
	if h == nil {
		panic("plugin.SetDatabase called before plugin.Install")
	}
	if db == nil {
		panic("plugin.SetDatabase called with a nil pool")
	}
	if !h.db.CompareAndSwap(nil, db) {
		panic("plugin.SetDatabase called twice")
	}
}

// The pool behind sdk.Database - an error rather than nil, so a plugin resolving the handle too early
// (in init(), before boot wired it) gets told so instead of panicking on first query
func (h *Host) Database() (sdk.DB, error) {
	db := h.db.Load()
	if db == nil {
		return nil, sdk.ErrDBUnavailable
	}
	return db, nil
}

// Fails CLOSED when unwired: a plugin gating a privileged field must not be told "allowed" because
// boot hadn't reached the auth wiring yet
func (h *Host) RequireAdminPermission(ctx context.Context, slug string) error {
	c := h.permChecker.Load()
	if c == nil {
		return sdk.ErrPermissionUnavailable
	}
	return auth.RequirePermission(ctx, *c, slug)
}

// The channel-scoped check behind sdk.RequireChannelPermission - asserted, not required by the
// signature, so an unscoped checker refuses instead of panicking
func (h *Host) RequireAdminChannelPermission(ctx context.Context, slug string, channelID uuid.UUID) error {
	c := h.permChecker.Load()
	if c == nil {
		return sdk.ErrPermissionUnavailable
	}
	scoped, ok := (*c).(auth.ScopedPermissionChecker)
	if !ok {
		return sdk.ErrPermissionUnavailable // fail CLOSED - an unscoped checker cannot answer this question
	}
	return auth.RequireChannelPermission(ctx, scoped, slug, channelID, nil)
}

// The listing half. The permission check comes FIRST: LoadPermittedChannels hands a caller with no admin claims a bypass, which a listing would read as see-everything, so a plugin that only called this would publish every channel's records to an unauthenticated caller
func (h *Host) PermittedChannels(ctx context.Context, slug string) ([]uuid.UUID, bool, error) {
	c := h.permChecker.Load()
	if c == nil {
		return nil, false, sdk.ErrPermissionUnavailable
	}
	if err := auth.RequirePermission(ctx, *c, slug); err != nil {
		return nil, false, err
	}
	provider, ok := (*c).(auth.ChannelScopeProvider)
	if !ok {
		return nil, false, sdk.ErrPermissionUnavailable
	}
	p, err := auth.LoadPermittedChannels(ctx, provider, slug)
	if err != nil {
		return nil, false, err
	}
	if p.Bypass() {
		return nil, true, nil
	}
	return p.IDs(), false, nil
}

// Who is acting, for audit attribution only - nil for an integration token or an unauthenticated
// caller, both of which are legitimate ways to reach a plugin and neither of which is an admin
func (h *Host) ActingAdminID(ctx context.Context) *uuid.UUID {
	claims, ok := auth.AdminFromContext(ctx)
	if !ok {
		return nil
	}
	id := claims.AdminID
	return &id
}

// The storefront customer behind this request, nil for a guest
func (h *Host) RequestCustomerID(ctx context.Context) *uuid.UUID {
	return channel.OptionalCustomerID(ctx)
}

// The channel this request resolved to - the storefront's channel, the admin's X-Channel-Id scope on the admin
// graph, uuid.Nil in background work
func (h *Host) RequestChannelID(ctx context.Context) uuid.UUID {
	return graphChannelID(ctx)
}

// SetEventBus wires the runtime event bus - Plugin subscriptions queued before this is called are replayed against the bus; new
// subscriptions after this point attach directly - Calling twice panics - see SetConfigService for the rationale
func SetEventBus(bus *events.Bus) {
	h := installedHost.Load()
	if h == nil {
		panic("plugin.SetEventBus called before plugin.Install")
	}
	if !h.bus.CompareAndSwap(nil, bus) {
		panic("plugin.SetEventBus called twice")
	}
	// replay queued subscriptions under the mutex so a concurrent Subscribe* sees the bus and skips the queue
	h.pendingMu.Lock()
	pendingAsync := h.pendingAsync
	pendingSync := h.pendingSync
	h.pendingAsync = nil
	h.pendingSync = nil
	h.pendingMu.Unlock()
	for _, p := range pendingAsync {
		attachAsyncHandler(bus, p.slug, p.eventType, p.handler, p.retry)
	}
	for _, p := range pendingSync {
		attachSyncHandler(bus, p.slug, p.eventType, p.priority, p.handler, p.always)
	}
}

func (h *Host) RegisterPaymentProvider(reg sdk.ProviderRegistration) error {
	// A per-slug config reader so a plugin uses short keys and stays in its own namespace by default - convenience, not containment, since sdk.PluginConfig reads any path - holds *Host so configSvc can wire late
	reader := &pluginConfigReader{
		host:   h,
		prefix: "payment/" + reg.Slug + "/",
	}
	payment.RegisterProvider(payment.Registration{
		Slug: reg.Slug,
		Provider: &providerAdapter{
			p:          reg.Provider,
			config:     reader,
			slug:       attributionSlug(reg.PluginSlug, reg.Slug),
			providerID: reg.Slug,
		},
		DisplayName: reg.DisplayName,
		IsOffline:   reg.IsOffline,
	})
	label := reg.DisplayName
	if label == "" {
		label = reg.Slug
	}
	config.RegisterGroup(config.Group{
		Section: "payment",
		Slug:    reg.Slug,
		Label:   label,
		// pluginConfigReader resolves against the channel in context, so a gateway can hold
		// different credentials per channel
		Scoping: config.ScopingChannel,
	})
	return nil
}

// RegisterShippingCarrier bridges an sdk.Carrier into the carrier registry (shipping mirror of
// RegisterPaymentProvider): per-slug config reader + carrierAdapter (Instrument), Tracker/Labeler only when implemented
func (h *Host) RegisterShippingCarrier(reg sdk.CarrierRegistration) error {
	reader := &pluginConfigReader{
		host:   h,
		prefix: "shipping/" + reg.Slug + "/",
	}
	base := &carrierAdapter{
		c:         reg.Carrier,
		config:    reader,
		slug:      attributionSlug(reg.PluginSlug, reg.Slug),
		carrierID: reg.Slug,
	}
	tr, hasTrack := reg.Carrier.(sdk.Tracker)
	lb, hasLabel := reg.Carrier.(sdk.Labeler)
	var carrier shipping.Carrier = base
	switch {
	case hasTrack && hasLabel:
		carrier = &carrierAdapterFull{carrierAdapter: base, t: tr, l: lb}
	case hasTrack:
		carrier = &carrierAdapterTracker{carrierAdapter: base, t: tr}
	case hasLabel:
		carrier = &carrierAdapterLabeler{carrierAdapter: base, l: lb}
	}
	shipping.RegisterCarrier(shipping.CarrierRegistration{
		Slug:        reg.Slug,
		Carrier:     carrier,
		DisplayName: reg.DisplayName,
	})
	label := reg.DisplayName
	if label == "" {
		label = reg.Slug
	}
	config.RegisterGroup(config.Group{
		Section: "shipping",
		Slug:    reg.Slug,
		Label:   label,
		// pluginConfigReader resolves against the channel in context, so a gateway can hold
		// different credentials per channel
		Scoping: config.ScopingChannel,
	})
	return nil
}

func (h *Host) RegisterPermission(pluginSlug, group, slug, name string) error {
	recordOwnership(&permissionOwners, slug, pluginSlug, "permission")
	// permission.RegisterGroup is internal, so a plugin's group is created here on first reference;
	// SortOrder 80 parks them below every core group so installing one never reorders the screen
	h.ensurePermissionGroup(pluginSlug, group)
	permission.Register(permission.Definition{
		Group: group,
		Slug:  slug,
		Name:  name,
	})
	return nil
}

// Registers the group once, ignoring a name core already owns - a plugin adding permissions to an
// EXISTING core group (customers, orders) is legitimate and must not panic on the duplicate
func (h *Host) ensurePermissionGroup(pluginSlug, group string) {
	if group == "" {
		return
	}
	if _, exists := permission.FindGroup(group); exists {
		return
	}
	label := group
	h.manifestMu.Lock()
	if m, ok := h.manifestBySlug[pluginSlug]; ok && m.Name != "" {
		label = m.Name
	}
	h.manifestMu.Unlock()
	permission.RegisterGroup(permission.Group{Slug: group, Name: label, SortOrder: 80})
}

func (h *Host) RegisterConfigField(pluginSlug, path string, field sdk.ConfigField) error {
	typ, err := translateConfigType(field.Type)
	if err != nil {
		return fmt.Errorf("config field %q: %w", path, err)
	}
	// attribution is what plugin uninstall sweeps by, and the collision check inside is what stops a plugin quietly claiming another's path (or a core one) - config.Register would panic on the dup, but this names BOTH plugins instead of just the path
	recordOwnership(&configOwners, path, pluginSlug, "config path")
	config.Register(config.Definition{
		Path:    path,
		Type:    typ,
		Default: field.DefaultValue,
		Label:   field.Description,
	})
	return nil
}

func (h *Host) RegisterEvent(pluginSlug, name string, kind sdk.EventKind) error {
	recordOwnership(&eventOwners, name, pluginSlug, "event")
	webhook.RegisterEvent(webhook.EventDefinition{
		Type: name,
		Name: name,
		Kind: translateEventKind(kind),
	})
	return nil
}

func (h *Host) RegisterMigrations(pluginSlug string, reg sdk.MigrationsRegistration) error {
	recordOwnership(&migrationOwners, reg.Namespace, pluginSlug, "migration namespace")
	pluginmigrate.Register(pluginmigrate.Registration{
		Namespace: reg.Namespace,
		FS:        reg.FS,
		Root:      reg.Root,
	})
	return nil
}

func (h *Host) RegisterDataPatch(pluginSlug string, reg sdk.DataPatchRegistration) error {
	recordOwnership(&datapatchOwners, reg.ID, pluginSlug, "data patch")
	namespace := reg.Namespace
	if namespace == "" {
		namespace = pluginSlug // left empty, core's registry would file it under "core"
	}
	datapatch.Register(datapatch.Patch{
		ID:        reg.ID,
		Namespace: namespace,
		DependsOn: reg.DependsOn,
		Apply: func(ctx context.Context, pc *datapatch.Context) error {
			return reg.Apply(ctx, pc.Tx)
		},
	})
	return nil
}

// ---- registration attribution ---- Every attributed registration lands in one of these key->slug maps, so uninstall can enumerate a plugin's exact footprint instead of guessing prefixes, and a second plugin claiming the same key fails at boot naming both sides

var (
	ownersMu         sync.Mutex
	configOwners     = map[string]string{}
	permissionOwners = map[string]string{}
	eventOwners      = map[string]string{}
	migrationOwners  = map[string]string{}
	datapatchOwners  = map[string]string{}
	// every event type a plugin subscribed to, so boot can hold the subscriptions to the manifest's Consumes
	subscriptions = map[string]map[string]struct{}{}
)

func recordSubscription(slug, eventType string) {
	if slug == "" {
		return // core-side callers have no manifest to answer to
	}
	ownersMu.Lock()
	defer ownersMu.Unlock()
	if subscriptions[slug] == nil {
		subscriptions[slug] = map[string]struct{}{}
	}
	subscriptions[slug][eventType] = struct{}{}
}

func recordOwnership(m *map[string]string, key, slug, what string) {
	ownersMu.Lock()
	defer ownersMu.Unlock()
	if prev, taken := (*m)[key]; taken && prev != slug {
		panic(fmt.Sprintf("plugin %s: %s %q is already registered by plugin %s", slug, what, key, prev))
	}
	(*m)[key] = slug
}

// ConfigPathsFor lists the exact config paths a plugin registered - what `plugin uninstall` deletes,
// replacing the plugin/<slug>/ prefix guess that left payment credentials behind
func ConfigPathsFor(pluginSlug string) []string {
	ownersMu.Lock()
	defer ownersMu.Unlock()
	var out []string
	for path, slug := range configOwners {
		if slug == pluginSlug {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// SubscribeAsync attaches a plugin handler to a post-commit event (attaches now if the bus is wired, else queues)
func (h *Host) SubscribeAsync(slug, eventType string, handler sdk.AsyncHandler) error {
	return h.subscribeAsync(slug, eventType, handler, false)
}

func (h *Host) subscribeAsync(slug, eventType string, handler sdk.AsyncHandler, retry bool) error {
	recordSubscription(slug, eventType)
	if bus := h.bus.Load(); bus != nil {
		attachAsyncHandler(bus, slug, eventType, handler, retry)
		return nil
	}
	h.pendingMu.Lock()
	defer h.pendingMu.Unlock()
	// re-check under lock - SetEventBus may have run since the Load, else the subscription is lost
	if bus := h.bus.Load(); bus != nil {
		attachAsyncHandler(bus, slug, eventType, handler, retry)
		return nil
	}
	h.pendingAsync = append(h.pendingAsync, pendingAsyncSub{slug: slug, eventType: eventType, handler: handler, retry: retry})
	return nil
}

// SubscribeSync attaches a plugin handler to a pre-commit event at priority - like SubscribeAsync
// but runs in the publisher's goroutine and its error aborts the publish (see sdk.SubscribeSync for the mutation contract)
func (h *Host) SubscribeSync(slug, eventType string, priority int, handler sdk.SyncHandler) error {
	return h.subscribeSync(slug, eventType, priority, handler, false)
}

// SubscribeSyncAlways backs sdk.SubscribeSyncAlways - the handler runs whatever the kill switch says, so a veto holds while the plugin is off
func (h *Host) SubscribeSyncAlways(slug, eventType string, priority int, handler sdk.SyncHandler) error {
	return h.subscribeSync(slug, eventType, priority, handler, true)
}

func (h *Host) subscribeSync(slug, eventType string, priority int, handler sdk.SyncHandler, always bool) error {
	recordSubscription(slug, eventType)
	if bus := h.bus.Load(); bus != nil {
		attachSyncHandler(bus, slug, eventType, priority, handler, always)
		return nil
	}
	h.pendingMu.Lock()
	defer h.pendingMu.Unlock()
	if bus := h.bus.Load(); bus != nil {
		attachSyncHandler(bus, slug, eventType, priority, handler, always)
		return nil
	}
	h.pendingSync = append(h.pendingSync, pendingSyncSub{
		slug:      slug,
		eventType: eventType,
		priority:  priority,
		handler:   handler,
		always:    always,
	})
	return nil
}

// The event's own channel when it names one, so a per-channel kill switch reaches event handlers too - default scope otherwise
type channelScopedEvent interface{ EventChannelID() uuid.UUID }

func eventChannel(e events.Event) uuid.UUID {
	if c, ok := e.(channelScopedEvent); ok {
		return c.EventChannelID()
	}
	return uuid.Nil
}

// A panic is the plugin failing, not the event - recovered here so it lands in the same log, tally and retry decision as a returned error
func callAsync(ctx context.Context, handler sdk.AsyncHandler, payload json.RawMessage) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("%w: %w", ErrPluginPanicked, errFromPanic(rec))
		}
	}()
	return handler(ctx, payload)
}

// attachAsyncHandler wraps a plugin's JSON handler as events.AsyncHandler, gated by the enabled flag + hook timeout (Instrument's guarantees minus the rate limit - async is already off the request path)
// Errors and panics are logged and recorded; only a retry subscription (SubscribeAsyncOnce, whose claim makes its own replay safe) returns one, so the outbox redelivers - a plain subscriber's failure must not replay an event every core subscriber already handled
// The timeout is cooperative - it cancels ctx, so a handler ignoring ctx still runs to completion
func attachAsyncHandler(bus *events.Bus, slug, eventType string, handler sdk.AsyncHandler, retry bool) {
	bus.Subscribe(eventType, func(ctx context.Context, e events.Event) error {
		// a disabled plugin drops the event, retry subscriptions included - switching one off is not a pause with a backlog
		if slug != "" && !isEnabledFromContext(ctx, slug, eventChannel(e)) {
			return nil
		}
		payload, err := json.Marshal(e)
		if err != nil {
			slog.Error("plugin: marshal event for subscriber",
				"event_type", eventType, "err", err)
			return nil
		}
		if slug != "" {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, hookTimeoutFromContext(ctx, slug))
			defer cancel()
		}
		if err := callAsync(ctx, handler, payload); err != nil {
			slog.Error("plugin: async event handler returned error",
				"plugin", slug, "event_type", eventType, "err", err, "redelivered", retry)
			recordAsyncFailure(slug, eventType, err) // the notification scan drains this - a log line alone is invisible
			if retry {
				return &PluginError{Slug: slug, Hook: "event " + eventType, Err: err}
			}
		}
		return nil
	})
}

// attachSyncHandler wraps a plugin's typed handler as events.SyncHandler (pass-through so it can mutate; errors abort the publish) - A disabled plugin is SKIPPED (returns nil, not abort)
// No timeout - sync runs under the request deadline; a mid-mutation cut would be worse - No recover either: the bus's callSyncWithRecover already turns a handler panic into an error that aborts the publish
func attachSyncHandler(bus *events.Bus, slug, eventType string, priority int, handler sdk.SyncHandler, always bool) {
	bus.SubscribeSync(eventType, priority, func(ctx context.Context, e events.Event) error {
		if slug != "" && !always && !isEnabledFromContext(ctx, slug, eventChannel(e)) {
			return nil // disabled - don't participate, don't abort
		}
		// A sync veto is the plugin telling the SHOPPER why their order was refused ("your company credit line is short"), so classify it here for the same reason the graph boundary does - unclassified, the presenter scrubs it to "internal error" and the message is wasted
		return translateUserError(handler(ctx, e))
	})
}

// ErrConfigUnavailable = a plugin read config before SetConfigService ran (a boot-wiring bug)
var ErrConfigUnavailable = errors.New("plugin: config service not yet wired (plugin.SetConfigService not called)")

// pluginConfigReader prefixes every key with the plugin's namespace so a plugin's own
// reads stay scoped by default - Holds *Host so configSvc can wire late (lock-free via atomic.Pointer)
type pluginConfigReader struct {
	host   *Host
	prefix string // e.g. "payment/stripe/"
}

func (r *pluginConfigReader) Get(ctx context.Context, key string, channelID uuid.UUID) (string, error) {
	// a full path through a prefixing reader double-prefixes and reads a path that never exists - the
	// paypal plugin shipped exactly that and was silently inert, so it fails loudly now
	if strings.Contains(key, "/") { // a short key never has one, whichever namespace the reader serves
		return "", fmt.Errorf("plugin config: key %q looks like a full path - this reader already prefixes %q, pass the short key", key, r.prefix)
	}
	if r.host == nil {
		return "", ErrConfigUnavailable
	}
	svc := r.host.configSvc.Load()
	if svc == nil {
		return "", ErrConfigUnavailable
	}
	scope := config.DefaultScope
	if channelID != uuid.Nil {
		id := channelID
		scope = config.Scope{Kind: config.ScopeChannel, ChannelID: &id}
	}
	return svc.GetString(ctx, r.prefix+key, scope)
}

// providerAdapter bridges sdk.PaymentProvider → payment.Provider, injecting the bound
// config reader and routing every call through Instrument (panic isolation, IsEnabled, timeout, observability)
type providerAdapter struct {
	p          sdk.PaymentProvider
	config     sdk.ConfigReader
	slug       string // plugin manifest slug - what Instrument keys metrics/IsEnabled on
	providerID string // payment provider slug - what the payment registry exposes; same as slug when the plugin's manifest slug matches its provider slug
}

func (a *providerAdapter) Authorize(ctx context.Context, req payment.AuthorizeRequest) (_ payment.AuthorizeResult, err error) {
	ctx, finish, err := Instrument(ctx, a.slug, "Authorize", req.ChannelID)
	defer finish(&err)
	if err != nil {
		// IsEnabled returned false - Instrument already pre-set the error
		return payment.AuthorizeResult{}, err
	}
	sdkReq := sdk.AuthorizeRequest{
		OrderID:      req.OrderID,
		ChannelID:    req.ChannelID,
		CurrencyCode: req.CurrencyCode,
		AmountCents:  req.AmountCents,
		CustomerID:   req.CustomerID,
		Email:        req.Email,
		PaymentToken: req.PaymentToken,
		VaultedToken: req.VaultedToken,
		Metadata:     req.Metadata,
		Config:       a.config,
	}
	res, err := a.p.Authorize(ctx, sdkReq)
	if err != nil {
		return payment.AuthorizeResult{}, err
	}
	return payment.AuthorizeResult{
		ProviderReference: res.ProviderReference,
		RawResponse:       res.RawResponse,
		Action: payment.PendingAction{
			Kind:  payment.ActionKind(res.Action.Kind),
			Value: res.Action.Value,
		},
	}, nil
}

// Exposes the plugin's optional sdk.AuthorizationVerifier capability for 3DS and SCA finalize - providers without it report not-supported
func (a *providerAdapter) VerifyAuthorization(ctx context.Context, req payment.VerifyAuthorizationRequest) (_ payment.VerifyAuthorizationResult, err error) {
	verifier, ok := a.p.(sdk.AuthorizationVerifier)
	if !ok {
		return payment.VerifyAuthorizationResult{}, fmt.Errorf("provider %q cannot verify pending authorizations", a.providerID)
	}
	ctx, finish, err := Instrument(ctx, a.slug, "VerifyAuthorization", req.ChannelID)
	defer finish(&err)
	if err != nil {
		return payment.VerifyAuthorizationResult{}, err
	}
	res, err := verifier.VerifyAuthorization(ctx, sdk.VerifyAuthorizationRequest{
		OrderID:           req.OrderID,
		ChannelID:         req.ChannelID,
		ProviderReference: req.ProviderReference,
		AmountCents:       req.AmountCents,
		CurrencyCode:      req.CurrencyCode,
		Config:            a.config,
	})
	if err != nil {
		return payment.VerifyAuthorizationResult{}, err
	}
	return payment.VerifyAuthorizationResult{
		Authorized:        res.Authorized,
		ProviderReference: res.ProviderReference,
		RawResponse:       res.RawResponse,
	}, nil
}

// Exposes the plugin's optional sdk.PaymentSynchronizer so core can ask a gateway where a payment
// stands - providers without it report not-supported, and reconciliation skips them
func (a *providerAdapter) SyncPayment(ctx context.Context, req payment.SyncPaymentRequest) (_ payment.SyncPaymentResult, err error) {
	syncer, ok := a.p.(sdk.PaymentSynchronizer)
	if !ok {
		return payment.SyncPaymentResult{}, fmt.Errorf("provider %q cannot report payment state", a.providerID)
	}
	ctx, finish, err := Instrument(ctx, a.slug, "SyncPayment", req.ChannelID)
	defer finish(&err)
	if err != nil {
		return payment.SyncPaymentResult{}, err
	}
	res, err := syncer.SyncPayment(ctx, sdk.SyncPaymentRequest{
		OrderID:           req.OrderID,
		ChannelID:         req.ChannelID,
		ProviderReference: req.ProviderReference,
		Config:            a.config,
	})
	if err != nil {
		return payment.SyncPaymentResult{}, err
	}
	return payment.SyncPaymentResult{
		State:           payment.PaymentState(res.State),
		AuthorizedCents: res.AuthorizedCents,
		CapturedCents:   res.CapturedCents,
		RefundedCents:   res.RefundedCents,
		RawResponse:     res.RawResponse,
	}, nil
}

// Exposes the plugin's optional sdk.Vaulter capability for saved payment methods - providers without it report not-supported
func (a *providerAdapter) VaultPaymentMethod(ctx context.Context, req payment.VaultRequest) (_ payment.VaultedMethod, err error) {
	vaulter, ok := a.p.(sdk.Vaulter)
	if !ok {
		return payment.VaultedMethod{}, fmt.Errorf("provider %q cannot vault payment methods", a.providerID)
	}
	ctx, finish, err := Instrument(ctx, a.slug, "VaultPaymentMethod", req.ChannelID)
	defer finish(&err)
	if err != nil {
		return payment.VaultedMethod{}, err
	}
	res, err := vaulter.VaultPaymentMethod(ctx, sdk.VaultRequest{
		CustomerID: req.CustomerID,
		Email:      req.Email,
		Token:      req.Token,
		ChannelID:  req.ChannelID,
		Config:     a.config,
	})
	if err != nil {
		return payment.VaultedMethod{}, err
	}
	return payment.VaultedMethod{
		ProviderToken: res.ProviderToken,
		Brand:         res.Brand,
		Last4:         res.Last4,
		ExpMonth:      res.ExpMonth,
		ExpYear:       res.ExpYear,
		RawResponse:   res.RawResponse,
	}, nil
}

func (a *providerAdapter) DeleteVaultedPaymentMethod(ctx context.Context, req payment.VaultDeleteRequest) (err error) {
	vaulter, ok := a.p.(sdk.Vaulter)
	if !ok {
		return fmt.Errorf("provider %q cannot vault payment methods", a.providerID)
	}
	ctx, finish, err := Instrument(ctx, a.slug, "DeleteVaultedPaymentMethod", req.ChannelID)
	defer finish(&err)
	if err != nil {
		return err
	}
	return vaulter.DeleteVaultedPaymentMethod(ctx, sdk.VaultDeleteRequest{
		ProviderToken: req.ProviderToken,
		ChannelID:     req.ChannelID,
		Config:        a.config,
	})
}

func (a *providerAdapter) Capture(ctx context.Context, req payment.CaptureRequest) (_ payment.CaptureResult, err error) {
	ctx, finish, err := Instrument(ctx, a.slug, "Capture", req.ChannelID)
	defer finish(&err)
	if err != nil {
		return payment.CaptureResult{}, err
	}
	sdkReq := sdk.CaptureRequest{
		OrderID:           req.OrderID,
		ChannelID:         req.ChannelID,
		ProviderReference: req.ProviderReference,
		AmountCents:       req.AmountCents,
		CurrencyCode:      req.CurrencyCode,
		IdempotencyKey:    req.IdempotencyKey,
		Config:            a.config,
	}
	res, err := a.p.Capture(ctx, sdkReq)
	if err != nil {
		return payment.CaptureResult{}, err
	}
	return payment.CaptureResult{
		ProviderReference: res.ProviderReference,
		CapturedAt:        res.CapturedAt,
		RawResponse:       res.RawResponse,
	}, nil
}

func (a *providerAdapter) Refund(ctx context.Context, req payment.RefundRequest) (_ payment.RefundResult, err error) {
	ctx, finish, err := Instrument(ctx, a.slug, "Refund", req.ChannelID)
	defer finish(&err)
	if err != nil {
		return payment.RefundResult{}, err
	}
	sdkReq := sdk.RefundRequest{
		OrderID:           req.OrderID,
		ChannelID:         req.ChannelID,
		ProviderReference: req.ProviderReference,
		AmountCents:       req.AmountCents,
		CurrencyCode:      req.CurrencyCode,
		Reason:            req.Reason,
		IdempotencyKey:    req.IdempotencyKey,
		Config:            a.config,
	}
	res, err := a.p.Refund(ctx, sdkReq)
	if err != nil {
		return payment.RefundResult{}, err
	}
	return payment.RefundResult{
		ProviderReference: res.ProviderReference,
		RefundedAt:        res.RefundedAt,
		RawResponse:       res.RawResponse,
	}, nil
}

func (a *providerAdapter) Void(ctx context.Context, req payment.VoidRequest) (_ payment.VoidResult, err error) {
	ctx, finish, err := Instrument(ctx, a.slug, "Void", req.ChannelID)
	defer finish(&err)
	if err != nil {
		return payment.VoidResult{}, err
	}
	sdkReq := sdk.VoidRequest{
		OrderID:           req.OrderID,
		ChannelID:         req.ChannelID,
		ProviderReference: req.ProviderReference,
		Config:            a.config,
	}
	res, err := a.p.Void(ctx, sdkReq)
	if err != nil {
		return payment.VoidResult{}, err
	}
	return payment.VoidResult{
		VoidedAt:    res.VoidedAt,
		RawResponse: res.RawResponse,
	}, nil
}

func (a *providerAdapter) HandleWebhook(ctx context.Context, r *http.Request) (_ payment.WebhookResult, err error) {
	// a webhook URL registered with ?channel=<uuid> scopes the push - config reads and the kill switch then resolve per channel instead of default scope, which is all a merchant with per-channel credentials needs
	channelID := uuid.Nil
	if raw := r.URL.Query().Get("channel"); raw != "" {
		if id, perr := uuid.Parse(raw); perr == nil {
			channelID = id
		}
	}
	ctx, finish, err := Instrument(ctx, a.slug, "HandleWebhook", channelID)
	defer finish(&err)
	if err != nil {
		return payment.WebhookResult{}, err
	}
	res, err := a.p.HandleWebhook(ctx, sdk.WebhookRequest{
		HTTPRequest: r,
		Config:      a.config,
		ChannelID:   channelID,
	})
	if err != nil {
		return payment.WebhookResult{}, err
	}
	return payment.WebhookResult{
		OrderID:           res.OrderID,
		EventName:         res.EventName,
		ProviderReference: res.ProviderReference,
		AmountCents:       res.AmountCents,
		CurrencyCode:      res.CurrencyCode,
	}, nil
}

// carrierAdapter bridges sdk.Carrier → shipping.Carrier via Instrument; Tracker/Labeler wrappers added only when implemented
type carrierAdapter struct {
	c         sdk.Carrier
	config    sdk.ConfigReader
	slug      string // plugin manifest slug - what Instrument keys metrics/IsEnabled on
	carrierID string // carrier slug exposed by the shipping registry
}

func (a *carrierAdapter) Quote(ctx context.Context, req shipping.RateRequest) (_ []shipping.RatedMethod, err error) {
	ctx, finish, err := Instrument(ctx, a.slug, "Quote", req.ChannelID)
	defer finish(&err)
	if err != nil {
		return nil, err
	}
	res, err := a.c.Quote(ctx, sdk.RateRequest{
		ChannelID:     req.ChannelID,
		Destination:   toSDKAddress(req.Destination),
		SubtotalCents: req.SubtotalCents,
		CurrencyCode:  req.CurrencyCode,
		Items:         toSDKItems(req.Items),
		Parcels:       toSDKParcels(req.Parcels),
		Config:        a.config,
	})
	if err != nil {
		return nil, err
	}
	return fromSDKMethods(res), nil
}

// carrierAdapterTracker / -Labeler / -Full add the optional capabilities - They
// embed *carrierAdapter (promoting Quote) and define Track/Label as needed
type carrierAdapterTracker struct {
	*carrierAdapter
	t sdk.Tracker
}

func (a *carrierAdapterTracker) Track(ctx context.Context, req shipping.TrackRequest) (shipping.TrackResult, error) {
	return instrumentedTrack(ctx, a.slug, a.t, a.config, req)
}

type carrierAdapterLabeler struct {
	*carrierAdapter
	l sdk.Labeler
}

func (a *carrierAdapterLabeler) Label(ctx context.Context, req shipping.LabelRequest) (shipping.LabelResult, error) {
	return instrumentedLabel(ctx, a.slug, a.l, a.config, req)
}

type carrierAdapterFull struct {
	*carrierAdapter
	t sdk.Tracker
	l sdk.Labeler
}

func (a *carrierAdapterFull) Track(ctx context.Context, req shipping.TrackRequest) (shipping.TrackResult, error) {
	return instrumentedTrack(ctx, a.slug, a.t, a.config, req)
}

func (a *carrierAdapterFull) Label(ctx context.Context, req shipping.LabelRequest) (shipping.LabelResult, error) {
	return instrumentedLabel(ctx, a.slug, a.l, a.config, req)
}

func instrumentedTrack(ctx context.Context, slug string, t sdk.Tracker, cfg sdk.ConfigReader, req shipping.TrackRequest) (_ shipping.TrackResult, err error) {
	ctx, finish, err := Instrument(ctx, slug, "Track", uuid.Nil)
	defer finish(&err)
	if err != nil {
		return shipping.TrackResult{}, err
	}
	res, err := t.Track(ctx, sdk.TrackRequest{
		CarrierSlug:    req.CarrierSlug,
		TrackingNumber: req.TrackingNumber,
		Config:         cfg,
	})
	if err != nil {
		return shipping.TrackResult{}, err
	}
	return fromSDKTrack(res), nil
}

func instrumentedLabel(ctx context.Context, slug string, l sdk.Labeler, cfg sdk.ConfigReader, req shipping.LabelRequest) (_ shipping.LabelResult, err error) {
	ctx, finish, err := Instrument(ctx, slug, "Label", req.ChannelID)
	defer finish(&err)
	if err != nil {
		return shipping.LabelResult{}, err
	}
	res, err := l.Label(ctx, sdk.LabelRequest{
		OrderNumber:       req.OrderNumber,
		ChannelID:         req.ChannelID,
		CarrierSlug:       req.CarrierSlug,
		MethodCode:        req.MethodCode,
		ShipTo:            toSDKAddress(req.ShipTo),
		Items:             toSDKItems(req.Items),
		Parcels:           toSDKParcels(req.Parcels),
		InsuredValueCents: req.InsuredValueCents,
		Config:            cfg,
	})
	if err != nil {
		return shipping.LabelResult{}, err
	}
	return shipping.LabelResult{
		Doc:            res.Doc,
		Format:         res.Format,
		TrackingNumber: res.TrackingNumber,
	}, nil
}

// ---------- shipping sdk↔internal translation ----------

func toSDKAddress(a shipping.Address) sdk.Address {
	return sdk.Address(a)
}

func toSDKItems(items []shipping.RateItem) []sdk.RateItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]sdk.RateItem, len(items))
	for i, it := range items {
		out[i] = sdk.RateItem(it)
	}
	return out
}

func toSDKParcels(parcels []shipping.Parcel) []sdk.Parcel {
	if len(parcels) == 0 {
		return nil
	}
	out := make([]sdk.Parcel, len(parcels))
	for i, p := range parcels {
		items := make([]sdk.ParcelItem, len(p.Items))
		for j, it := range p.Items {
			items[j] = sdk.ParcelItem(it)
		}
		out[i] = sdk.Parcel{
			BoxCode:     p.BoxCode,
			LengthMm:    p.LengthMm,
			WidthMm:     p.WidthMm,
			HeightMm:    p.HeightMm,
			WeightGrams: p.WeightGrams,
			ValueCents:  p.ValueCents,
			Items:       items,
		}
	}
	return out
}

func fromSDKMethods(methods []sdk.RatedMethod) []shipping.RatedMethod {
	if len(methods) == 0 {
		return nil
	}
	out := make([]shipping.RatedMethod, len(methods))
	for i, m := range methods {
		out[i] = shipping.RatedMethod(m)
	}
	return out
}

func fromSDKTrack(res sdk.TrackResult) shipping.TrackResult {
	out := shipping.TrackResult{Status: res.Status, URL: res.URL}
	if len(res.Events) > 0 {
		out.Events = make([]shipping.TrackEvent, len(res.Events))
		for i, e := range res.Events {
			out.Events[i] = shipping.TrackEvent(e)
		}
	}
	return out
}

func translateConfigType(t sdk.ConfigFieldType) (config.Type, error) {
	switch t {
	case sdk.TypeString:
		return config.TypeString, nil
	case sdk.TypeSecret:
		// Carried through as a distinct type so the config service encrypts it
		// at rest and the admin API masks it (never returns the value)
		return config.TypeSecret, nil
	case sdk.TypeInt:
		return config.TypeInt, nil
	case sdk.TypeBool:
		return config.TypeBool, nil
	case sdk.TypeJSON:
		return config.TypeJSON, nil
	default:
		return "", fmt.Errorf("unknown config field type %q (want one of string, int, bool, secret, json)", t)
	}
}

func translateEventKind(k sdk.EventKind) webhook.Kind {
	if k == sdk.EventKindSync {
		return webhook.KindSync
	}
	return webhook.KindAsync
}

// ---- Promotion: SDK ↔ internal/promotion adapters ----

// RegisterPromotionCondition adapts an SDK condition to promotion.ConditionEvaluator (the engine can't tell it's from a plugin)
func (h *Host) RegisterPromotionCondition(pluginSlug string, c sdk.PromotionCondition) error {
	promotion.RegisterConditionType(&promotionConditionAdapter{p: c, slug: attributionSlug(pluginSlug, c.Slug())})
	return nil
}

func (h *Host) RegisterPromotionAction(pluginSlug string, a sdk.PromotionAction) error {
	promotion.RegisterActionType(&promotionActionAdapter{p: a, slug: attributionSlug(pluginSlug, a.Slug())})
	return nil
}

func (h *Host) RegisterPromotionRedemptionHook(pluginSlug string, hook sdk.PromotionRedemptionHook) error {
	promotion.RegisterRedemptionHook(&promotionHookAdapter{p: hook, slug: attributionSlug(pluginSlug, hook.Slug())})
	return nil
}

// RegisterDataLoader appends a plugin DataLoader registration (slugs unique across plugins - duplicates panic at boot)
func (h *Host) RegisterDataLoader(reg sdk.DataLoaderRegistration) error {
	if reg == nil {
		return errors.New("plugin: RegisterDataLoader called with nil registration")
	}
	slug := reg.Slug()
	if slug == "" {
		return errors.New("plugin: RegisterDataLoader called with empty slug")
	}
	h.dataloaderMu.Lock()
	defer h.dataloaderMu.Unlock()
	if h.dataloaderSlugs == nil {
		h.dataloaderSlugs = make(map[string]struct{})
	}
	if _, dup := h.dataloaderSlugs[slug]; dup {
		return errors.New("plugin: duplicate DataLoader slug " + slug)
	}
	h.dataloaderSlugs[slug] = struct{}{}
	h.dataloaderRegs = append(h.dataloaderRegs, reg)
	return nil
}

// DataLoaderRegistrations returns a copy of every plugin DataLoader, for main.go's loader-bundle Deps at boot
func DataLoaderRegistrations() []sdk.DataLoaderRegistration {
	h := installedHost.Load()
	if h == nil {
		return nil
	}
	h.dataloaderMu.Lock()
	defer h.dataloaderMu.Unlock()
	if len(h.dataloaderRegs) == 0 {
		return nil
	}
	out := make([]sdk.DataLoaderRegistration, len(h.dataloaderRegs))
	copy(out, h.dataloaderRegs)
	return out
}

// RegisterPlugin stores a manifest (slug uniqueness here; cross-plugin checks defer to ValidateManifests) and
// auto-registers a "plugin/<slug>/enabled" kill-switch field (default true, read via sdk.IsEnabled)
func (h *Host) RegisterPlugin(m sdk.Manifest) error {
	h.manifestMu.Lock()
	if h.manifestBySlug == nil {
		h.manifestBySlug = make(map[string]sdk.Manifest)
	}
	if _, dup := h.manifestBySlug[m.Slug]; dup {
		h.manifestMu.Unlock()
		return errors.New("plugin: duplicate slug " + m.Slug)
	}
	h.manifestBySlug[m.Slug] = m
	h.manifests = append(h.manifests, m)
	h.manifestMu.Unlock()

	// per-plugin kill switch under the reserved "plugin" section (plugins needn't declare it)
	config.RegisterGroup(config.Group{
		Section: "plugin",
		Slug:    m.Slug,
		Label:   m.Name,
	})
	config.Register(config.Definition{
		Path:    "plugin/" + m.Slug + "/enabled",
		Type:    config.TypeBool,
		Default: "true",
		Label:   "Plugin enabled",
	})
	config.Register(config.Definition{
		Path:    "plugin/" + m.Slug + "/hook_timeout_ms",
		Type:    config.TypeInt,
		Default: "5000",
		Label:   "Plugin hook timeout (ms)",
	})
	config.Register(config.Definition{
		Path:    "plugin/" + m.Slug + "/rate_limit_per_min",
		Type:    config.TypeInt,
		Default: "0",
		Label:   "Plugin hook rate limit (calls/min per server instance, 0 = unlimited)",
	})
	return nil
}

// The plugin's declared answer to an unreadable kill switch, straight off the in-memory manifest - the same choice remoteprovider.FailurePolicy gives merchant endpoints, so both extension mechanisms answer a config outage the same way A slug with no manifest fails open, since there's nothing declaring otherwise
func (h *Host) failsClosed(slug string) bool {
	h.manifestMu.Lock()
	defer h.manifestMu.Unlock()
	return h.manifestBySlug[slug].FailurePolicy == sdk.FailClosed
}

// What Instrument keys metrics, the kill switch, the timeout and the rate limit on - every SDK path fills pluginSlug in, so the fallback only covers core-side callers registering a hook without a manifest, where the extension slug IS the identity
func attributionSlug(pluginSlug, fallback string) string {
	if pluginSlug != "" {
		return pluginSlug
	}
	return fallback
}

// IsPluginEnabled reads "plugin/<slug>/enabled" at channel scope - A READ FAILURE is what the failure policy decides: fail_open (the default) answers true so a config hiccup can't quietly disable production traffic, fail_closed answers false so a plugin that exists to refuse things isn't bypassed by the store being unreachable An explicit false is always false - the policy only covers "couldn't tell"
func (h *Host) IsPluginEnabled(ctx context.Context, slug string, channelID uuid.UUID) bool {
	svc := h.configSvc.Load()
	if svc == nil {
		return true
	}
	scope := config.DefaultScope
	if channelID != uuid.Nil {
		id := channelID
		scope = config.Scope{Kind: config.ScopeChannel, ChannelID: &id}
	}
	v, err := svc.GetBool(ctx, "plugin/"+slug+"/enabled", scope)
	if err != nil {
		return !h.failsClosed(slug)
	}
	return v
}

// Manifests returns a copy of every registered manifest in registration
// order - For the dependency-topology order, use OrderedManifestSlugs
func Manifests() []sdk.Manifest {
	h := installedHost.Load()
	if h == nil {
		return nil
	}
	h.manifestMu.Lock()
	defer h.manifestMu.Unlock()
	out := make([]sdk.Manifest, len(h.manifests))
	copy(out, h.manifests)
	return out
}

// ValidateManifests runs cross-plugin checks once at boot (before migrations/requests), returning the first error: SDK-range admits sdk.Version, deps registered + in-range, no dependency cycles, and every Consumes resolves to a declared event - Manifest-less plugins are skipped (opt-in)
func ValidateManifests() error {
	h := installedHost.Load()
	if h == nil {
		return nil
	}
	h.manifestMu.Lock()
	manifests := make([]sdk.Manifest, len(h.manifests))
	copy(manifests, h.manifests)
	bySlug := make(map[string]sdk.Manifest, len(h.manifestBySlug))
	maps.Copy(bySlug, h.manifestBySlug)
	h.manifestMu.Unlock()

	// Core's own events from the webhook registry - a plugin's RegisterEvent lands there too, so those names are
	// left out: they resolve through the plugin's Emits, and counting them as core would read as a collision
	ownersMu.Lock()
	known := make(map[string]struct{})
	for _, e := range webhook.AllEvents() {
		if _, pluginOwned := eventOwners[e.Type]; !pluginOwned {
			known[e.Type] = struct{}{}
		}
	}
	subscribed := make(map[string][]string, len(subscriptions))
	for slug, types := range subscriptions {
		for t := range types {
			subscribed[slug] = append(subscribed[slug], t)
		}
	}
	ownersMu.Unlock()
	if err := sdk.ValidateManifestSet(sdk.Version, manifests, bySlug, known); err != nil {
		return err
	}
	// Consumes only checks what a plugin declared - a subscription it forgot to list, to a name nothing emits, would never fire and never fail
	for slug, types := range subscribed {
		m, ok := bySlug[slug]
		if !ok {
			continue // manifest-less plugins are opt-in, as above
		}
		sort.Strings(types)
		for _, t := range types {
			if !slices.Contains(m.Consumes, t) {
				return fmt.Errorf("plugin %q subscribes to %q but its manifest does not list it in Consumes - boot resolves every Consumes entry, so an unlisted subscription could name an event nothing emits", slug, t)
			}
		}
	}
	return nil
}

// OrderedManifestSlugs returns plugin slugs in dependency order: every plugin appears after all of its dependencies - Used by the migration runner (apply dep schemas first) and by any other system that needs deterministic init ordering - Returns slugs in registration order if validation hasn't run, but a cycle would have failed ValidateManifests already so the worst case is "deps not strictly ordered" not "broken"
func OrderedManifestSlugs() []string {
	h := installedHost.Load()
	if h == nil {
		return nil
	}
	h.manifestMu.Lock()
	manifests := make([]sdk.Manifest, len(h.manifests))
	copy(manifests, h.manifests)
	h.manifestMu.Unlock()
	return sdk.TopoSortManifests(manifests)
}

// promotionConditionAdapter satisfies promotion.ConditionEvaluator by translating between the SDK's PromotionEnvironment and the internal Environment - The plugin sees only the SDK type; the engine sees only the internal type. `slug` is the manifest slug used for instrumentation and kill-switch lookup
type promotionConditionAdapter struct {
	p    sdk.PromotionCondition
	slug string
}

func (a *promotionConditionAdapter) Slug() string                  { return a.p.Slug() }
func (a *promotionConditionAdapter) Label() string                 { return a.p.Label() }
func (a *promotionConditionAdapter) Description() string           { return a.p.Description() }
func (a *promotionConditionAdapter) ParamsSchema() json.RawMessage { return a.p.ParamsSchema() }
func (a *promotionConditionAdapter) ValidateParams(raw json.RawMessage) error {
	return a.p.ValidateParams(raw)
}

func (a *promotionConditionAdapter) Evaluate(ctx context.Context, env promotion.Environment, raw json.RawMessage) (bool, error) {
	storeID := uuid.Nil
	if id := env.ChannelID(); id != nil {
		storeID = *id
	}
	// the instrumented body runs in a closure with its own named error - finish writes gate errors through that pointer, so sharing it with our return value would resurrect the error this adapter exists to swallow
	match, err := func() (_ bool, err error) {
		ctx, finish, err := Instrument(ctx, a.slug, "PromotionCondition.Evaluate", storeID)
		defer finish(&err)
		if err != nil {
			return false, err
		}
		return a.p.Evaluate(ctx, &sdkEnvironmentAdapter{env: env}, raw)
	}()
	if isGateError(err) {
		// disabled or rate-limited plugins answer "false" - they don't fire, but the rule isn't broken; it just won't match
		return false, nil
	}
	return match, err
}

type promotionActionAdapter struct {
	p    sdk.PromotionAction
	slug string
}

func (a *promotionActionAdapter) Slug() string                  { return a.p.Slug() }
func (a *promotionActionAdapter) Label() string                 { return a.p.Label() }
func (a *promotionActionAdapter) Description() string           { return a.p.Description() }
func (a *promotionActionAdapter) ParamsSchema() json.RawMessage { return a.p.ParamsSchema() }
func (a *promotionActionAdapter) ValidateParams(raw json.RawMessage) error {
	return a.p.ValidateParams(raw)
}

func (a *promotionActionAdapter) Apply(ctx context.Context, env promotion.Environment, raw json.RawMessage, ledger promotion.Ledger, applied promotion.AppliedRuleContext) error {
	storeID := uuid.Nil
	if id := env.ChannelID(); id != nil {
		storeID = *id
	}
	// closure owns the named error finish writes through - see the condition adapter
	err := func() (err error) {
		ctx, finish, err := Instrument(ctx, a.slug, "PromotionAction.Apply", storeID)
		defer finish(&err)
		if err != nil {
			return err
		}
		return a.p.Apply(ctx,
			&sdkEnvironmentAdapter{env: env},
			raw,
			&sdkLedgerAdapter{ledger: ledger},
			sdk.PromotionAppliedContext{
				RuleID:   applied.RuleID,
				CodeID:   applied.CodeID,
				Priority: applied.Priority,
			},
		)
	}()
	if isGateError(err) {
		// disabled action is a no-op - the rule's other actions still apply, mirroring how a missing action plugin behaves
		return nil
	}
	return err
}

type promotionHookAdapter struct {
	p    sdk.PromotionRedemptionHook
	slug string
}

func (a *promotionHookAdapter) Slug() string { return a.p.Slug() }

func (a *promotionHookAdapter) Redeem(ctx context.Context, tx *sql.Tx, env promotion.Environment, ledger promotion.Ledger, applied promotion.AppliedRuleContext) error {
	storeID := uuid.Nil
	if id := env.ChannelID(); id != nil {
		storeID = *id
	}
	// closure owns the named error finish writes through - see the condition adapter
	err := func() (err error) {
		ctx, finish, err := Instrument(ctx, a.slug, "PromotionRedemptionHook.Redeem", storeID)
		defer finish(&err)
		if err != nil {
			return err
		}
		return a.p.Redeem(ctx, tx,
			&sdkEnvironmentAdapter{env: env},
			&sdkLedgerAdapter{ledger: ledger},
			sdk.PromotionAppliedContext{
				RuleID:   applied.RuleID,
				CodeID:   applied.CodeID,
				Priority: applied.Priority,
			},
		)
	}()
	if isGateError(err) {
		// disabled redemption hook is a no-op - the discount still applies (the action ran), the side-effect just doesn't fire
		return nil
	}
	return err
}

// sdkEnvironmentAdapter exposes the internal promotion.Environment
// as the SDK's PromotionEnvironment - Method-by-method passthrough
type sdkEnvironmentAdapter struct {
	env promotion.Environment
}

func (a *sdkEnvironmentAdapter) SubtotalCents() int64           { return a.env.SubtotalCents() }
func (a *sdkEnvironmentAdapter) Currency() string               { return a.env.Currency() }
func (a *sdkEnvironmentAdapter) CountryCode() string            { return a.env.CountryCode() }
func (a *sdkEnvironmentAdapter) ChannelID() *uuid.UUID          { return a.env.ChannelID() }
func (a *sdkEnvironmentAdapter) ChannelSlug() string            { return a.env.ChannelSlug() }
func (a *sdkEnvironmentAdapter) CustomerID() *uuid.UUID         { return a.env.CustomerID() }
func (a *sdkEnvironmentAdapter) CustomerGroupSlug() string      { return a.env.CustomerGroupSlug() }
func (a *sdkEnvironmentAdapter) CustomerEmail() string          { return a.env.CustomerEmail() }
func (a *sdkEnvironmentAdapter) CustomerSegmentSlugs() []string { return a.env.CustomerSegmentSlugs() }
func (a *sdkEnvironmentAdapter) AppliedCodes() []string         { return a.env.AppliedCodes() }

func (a *sdkEnvironmentAdapter) Lines() []sdk.PromotionCartLine {
	src := a.env.Lines()
	out := make([]sdk.PromotionCartLine, len(src))
	for i, l := range src {
		out[i] = sdk.PromotionCartLine{
			LineID:         l.LineID,
			ProductID:      l.ProductID,
			ProductSKU:     l.ProductSKU,
			ProductName:    l.ProductName,
			Quantity:       l.Quantity,
			UnitPriceCents: l.UnitPriceCents,
			ProductKind:    l.ProductKind,
			CategoryIDs:    l.CategoryIDs,
			AttributeKVs:   l.AttributeKVs,
		}
	}
	return out
}

// sdkLedgerAdapter wraps the internal Ledger so plugin actions can mutate it through the SDK's narrower interface -
// Snapshot getters pass through unchanged
type sdkLedgerAdapter struct {
	ledger promotion.Ledger
}

func (a *sdkLedgerAdapter) AddSubtotalDiscount(cents int64) { a.ledger.AddSubtotalDiscount(cents) }
func (a *sdkLedgerAdapter) AddLineAdjustment(lineID uuid.UUID, cents int64) {
	a.ledger.AddLineAdjustment(lineID, cents)
}
func (a *sdkLedgerAdapter) SetFreeShipping(methodSlugs []string) {
	a.ledger.SetFreeShipping(methodSlugs)
}
func (a *sdkLedgerAdapter) SetShippingOverride(cents int64) { a.ledger.SetShippingOverride(cents) }
func (a *sdkLedgerAdapter) AddFreeGift(productSKU string, qty float64) {
	a.ledger.AddFreeGift(productSKU, qty)
}
func (a *sdkLedgerAdapter) CapSubtotalDiscount(maxCents int64) {
	a.ledger.CapSubtotalDiscount(maxCents)
}
func (a *sdkLedgerAdapter) SubtotalDiscountCents() int64      { return a.ledger.SubtotalDiscountCents() }
func (a *sdkLedgerAdapter) LineAdjustment(id uuid.UUID) int64 { return a.ledger.LineAdjustment(id) }
func (a *sdkLedgerAdapter) FreeShipping() bool                { return a.ledger.FreeShipping() }
func (a *sdkLedgerAdapter) ShippingOverride() *int64          { return a.ledger.ShippingOverride() }
