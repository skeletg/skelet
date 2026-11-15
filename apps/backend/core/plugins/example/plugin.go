// Package example is a Tier 1 plugin that exists only to hold the SDK contract still - NOT shipped, since nothing blank-imports it, so it compiles on every build and links into no binary
// It touches every surface a real plugin uses, so a change to any of them fails here rather than in someone's private plugin repo - the failures this catches are compositional, where every piece is right on its own and only the wiring between them is wrong
package example

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	Slug                = "example"
	SiblingSlug         = "example-sibling"
	ProviderSlug        = "example-gateway"
	OfflineProviderSlug = "example-offline"
	CarrierSlug         = "example-carrier"
	EventSettled        = "example.settled"
	EventSettling       = "example.settling" // sync - fired pre-commit, handlers may mutate or veto
	StageAwaitingStock  = "awaiting_stock"
	DoublePointsPrefix  = "EX-DOUBLE-"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var (
	settledSeen        = make(chan []byte, 4) // the subscriber's proof that the bus reached this plugin
	onceSeen           = make(chan []byte, 4) // same proof for the replay-deduped subscriber
	shutdownRan        = make(chan struct{})  // closed by the shutdown hook, for the same reason
	registeredProvider = &provider{}          // so the test can read what the host adapter handed it
	pluginHandle       *sdk.Plugin            // so the test can exercise the handle getters
)

func init() {
	p := sdk.RegisterPlugin(sdk.Manifest{
		Slug:     Slug,
		Name:     "Example Plugin",
		Version:  "0.1.0",
		SDKRange: "^0.1.0",
		Emits:    []string{EventSettled, EventSettling},
		Consumes: []string{EventSettled, EventSettling, sdk.EventOrderPlacing, sdk.EventOrderTransitioning, sdk.EventLoyaltyEarning},
		Capabilities: []string{
			"db.write",
			"events.publish:" + EventSettled,
			"events.publish:" + EventSettling,
		},
	})
	pluginHandle = p

	// Claimed BEFORE this plugin registers anything - under ambient attribution everything below would land under SiblingSlug, which is the failure this holds still
	sibling := sdk.RegisterPlugin(sdk.Manifest{
		Slug:     SiblingSlug,
		Name:     "Example Sibling",
		Version:  "0.1.0",
		SDKRange: "^0.1.0",
	})
	sibling.RegisterHealthCheck("probe", func(context.Context) error { return nil })

	p.RegisterPaymentProvider(ProviderSlug, registeredProvider, sdk.WithDisplayName("Example Gateway"))

	// A second provider on the same manifest - the offline flag routes it past gateway authorization, and the kill switch stays keyed on the manifest slug
	p.RegisterPaymentProvider(OfflineProviderSlug, &provider{}, sdk.WithDisplayName("Example Offline"), sdk.WithOffline())

	p.RegisterShippingCarrier(CarrierSlug, &carrier{}, sdk.WithCarrierDisplayName("Example Carrier"))

	// Mounted at /api/v1/plugins/example/ping behind the host's kill switch, rate limit, timeout and panic isolation
	p.RegisterRoute("GET ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})

	// Pre-commit participation - the assertion is structural so this package keeps importing only the SDK
	p.SubscribeSync(EventSettling, 100, func(_ context.Context, e any) error {
		if ev, ok := e.(interface{ MarkSettling() }); ok {
			ev.MarkSettling()
		}
		return nil
	})

	// Kept on the order with its label - order.placing records the answer's length in the order's metadata
	p.RegisterCheckoutField(sdk.CheckoutField{Key: "example_gift_note", Label: "Gift note", Type: sdk.CheckoutFieldTextarea, MaxLength: 200})
	p.SubscribeSync(sdk.EventOrderPlacing, 100, func(_ context.Context, e any) error {
		answers, ok := e.(sdk.CheckoutFieldAnswers)
		meta, ok2 := e.(sdk.OrderPlacingMetadata)
		if !ok || !ok2 {
			return nil
		}
		if note, given := answers.CheckoutFieldValue("example_gift_note"); given {
			return meta.SetPlacingMetadata(Slug+".gift_note_length", strconv.Itoa(utf8.RuneCountInString(note)))
		}
		return nil
	})

	p.RegisterOrderStage(sdk.OrderStage{Key: StageAwaitingStock, Label: "Awaiting stock", Within: []string{sdk.OrderStatusConfirmed, sdk.OrderStatusProcessing}})
	p.SubscribeSync(sdk.EventOrderTransitioning, 100, func(_ context.Context, e any) error {
		move, ok := e.(sdk.OrderTransitioning)
		if ok && move.TransitioningFromStage() == StageAwaitingStock &&
			(move.TransitioningToStatus() == sdk.OrderStatusPartiallyFulfilled || move.TransitioningToStatus() == sdk.OrderStatusFulfilled) {
			return sdk.UserErrorWithCode(sdk.ErrorConflict, "EXAMPLE_AWAITING_STOCK", "the order is still awaiting stock")
		}
		return nil
	})

	// Names the orders of every channel the plugin is on, from a yearly count of its own
	p.RegisterOrderNumberer(func(_ context.Context, req sdk.OrderNumberRequest) (string, error) {
		n, err := req.Next("orders", strconv.Itoa(req.PlacedAt.Year()))
		return fmt.Sprintf("EX-%d-%05d", req.PlacedAt.Year(), n), err
	})

	p.SubscribeSync(sdk.EventLoyaltyEarning, 100, func(_ context.Context, e any) error {
		earning, ok := e.(sdk.LoyaltyEarning)
		if !ok {
			return nil
		}
		extra := int64(0)
		for i := range earning.EarningLineCount() {
			if sku, _, cents := earning.EarningLine(i); strings.HasPrefix(sku, DoublePointsPrefix) {
				extra += cents / earning.EarningUnitCents() * earning.EarningPointsPerUnit()
			}
		}
		if extra == 0 {
			return nil
		}
		earning.AddEarningNote("double points on featured items")
		return earning.SetEarningPoints(earning.EarningPoints() + extra)
	})

	p.RegisterPromotionCondition(minSubtotalCondition{})
	p.RegisterPromotionAction(flatDiscountAction{})
	p.RegisterPromotionRedemptionHook(recordRedemptionHook{})

	p.RegisterHealthCheck("api", func(context.Context) error { return nil })

	// fails until api_key is set, which is what readiness is for
	p.RegisterReadinessCheck("config", func(ctx context.Context) error {
		v, _ := sdk.PluginConfig(ctx, "payment/"+ProviderSlug+"/api_key")
		if v == "" {
			return errors.New("api_key not configured")
		}
		return nil
	})

	p.RegisterScheduledJob("reconcile", "@every 1h", "Reconcile example charges", func(context.Context) error { return nil })

	// A plugin-owned index: the host names it plg-example-charges and wires it into the reindex tooling - errors when OPENSEARCH_URL is unset,
	// since search is a soft dependency. A real plugin keeps the returned writer for event-driven upserts
	p.RegisterSearchIndex(sdk.SearchIndexRegistration{
		Name:        "charges",
		MappingJSON: `{"mappings":{"properties":{"reference":{"type":"keyword"},"amount_cents":{"type":"long"},"settled_at":{"type":"date"}}}}`,
		Reindex: func(ctx context.Context, sink sdk.SearchIndexSink) (int, error) {
			// A real plugin streams its source of truth here - the example has none
			return 0, nil
		},
	})

	p.OnShutdown(func(context.Context) error {
		close(shutdownRan)
		return nil
	})

	p.SubscribeAsync(EventSettled, func(_ context.Context, payload json.RawMessage) error {
		select {
		case settledSeen <- payload:
		default:
		}
		return nil
	})

	// Replay-deduped, keyed on the settled reference - on a host with no dedup store wired it degrades to plain delivery
	p.SubscribeAsyncOnce(EventSettled, func(payload json.RawMessage) (string, error) {
		var e struct{ Reference string }
		if err := json.Unmarshal(payload, &e); err != nil {
			return "", err
		}
		return e.Reference, nil
	}, func(_ context.Context, payload json.RawMessage) error {
		select {
		case onceSeen <- payload:
		default:
		}
		return nil
	})

	p.RegisterEvent(EventSettled, sdk.EventKindAsync)
	p.RegisterEvent(EventSettling, sdk.EventKindSync)

	// Run-once Go against the patch's own tx, applied by `skelet cli data-patch up` after schema migrations
	p.RegisterDataPatch(sdk.DataPatchRegistration{
		Namespace: Slug,
		ID:        Slug + ":seed-example-records",
		Apply:     func(context.Context, *sql.Tx) error { return nil },
	})

	p.RegisterConfigField("payment/"+ProviderSlug+"/api_key", sdk.ConfigField{
		Type:        sdk.TypeSecret,
		Description: "Example gateway API key",
	})
	p.RegisterConfigField("payment/"+ProviderSlug+"/mode", sdk.ConfigField{
		Type:         sdk.TypeString,
		DefaultValue: "test",
		Description:  "Example gateway mode",
	})

	p.RegisterPermission("payments", "example.manage", "Manage the example gateway")

	p.RegisterMigrations(sdk.MigrationsRegistration{Namespace: Slug, FS: migrationsFS})
}
