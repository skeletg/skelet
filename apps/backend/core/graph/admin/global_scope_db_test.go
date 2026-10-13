package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/internal/webhook"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func forbidden(err error) bool { return errors.Is(err, auth.ErrForbidden) }

func TestAChannelScopedGrantCannotReachInstallWideOps(t *testing.T) {
	f := newEscalationFixture(t)
	ch := f.channel
	var scopes []adminpkg.RolePermissionScopeInput
	for _, slug := range []string{"emails.manage", "audit.view", "scheduler.manage", "cache.manage", "promotions.manage", "webhooks.manage", "remote_providers.manage"} {
		scopes = append(scopes, adminpkg.RolePermissionScopeInput{PermissionSlug: slug, ChannelID: &ch})
	}
	_, ctx := f.admin(t, f.role(t, scopes...))
	r := &Resolver{AdminSvc: f.svc, SignalsEnabled: true, SignalsAPI: fakeSignalsClient(t), PromotionSvc: promotion.NewService(nil)}
	m, q := &mutationResolver{r}, &queryResolver{r}

	for name, call := range map[string]func() error{
		"SuppressEmail":         func() error { _, e := m.SuppressEmail(ctx, "a@example.invalid", nil); return e },
		"UnsuppressEmail":       func() error { _, e := m.UnsuppressEmail(ctx, "a@example.invalid"); return e },
		"EmailSuppressions":     func() error { _, e := q.EmailSuppressions(ctx, nil, nil, nil); return e },
		"AuditLog":              func() error { _, e := q.AuditLog(ctx, nil, nil, nil, nil); return e },
		"ScheduledJobs":         func() error { _, e := q.ScheduledJobs(ctx); return e },
		"DeleteScheduledJob":    func() error { _, e := m.DeleteScheduledJob(ctx, uuid.NewString()); return e },
		"RunScheduledJobNow":    func() error { _, e := m.RunScheduledJobNow(ctx, uuid.NewString()); return e },
		"PurgeCache":            func() error { _, e := m.PurgeCache(ctx, nil, nil, nil); return e },
		"RebuildCache":          func() error { _, e := m.RebuildCache(ctx, nil); return e },
		"ReorderPromotionRules": func() error { _, e := m.ReorderPromotionRules(ctx, nil); return e },
		"SimulatePromotion":     func() error { _, e := q.SimulatePromotion(ctx, model.PromotionSimulationInput{}); return e },
		"CreateGlobalWebhook":   func() error { _, e := m.CreateWebhook(ctx, model.CreateWebhookInput{Name: "x"}); return e },
		"CreateGlobalProvider": func() error {
			_, e := m.CreateRemoteProvider(ctx, model.CreateRemoteProviderInput{Name: "x"})
			return e
		},
	} {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("%s ran past its gate on a channel-scoped grant (panicked on a missing service: %v)", name, p)
				}
			}()
			if err := call(); !forbidden(err) {
				t.Errorf("%s on a channel-scoped grant = %v, want forbidden - it reaches every channel", name, err)
			}
		}()
	}
}

func TestWebhooksProvidersAndPaymentsStayOnTheirChannel(t *testing.T) {
	f := newEscalationFixture(t)
	conn := testdb.Require(t)
	own := f.channel
	var other uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels WHERE id = $2 RETURNING id`, "gs-"+uuid.NewString()[:8], own).Scan(&other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, other) })
	q := db.New(conn)
	hooks := webhook.NewService(webhook.NewRepository(q, txrunner.New(conn)))
	providers := remoteprovider.NewService(remoteprovider.NewRepository(q, nil))
	ctx := context.Background()

	var event string
	for _, e := range webhook.AllEvents() {
		if e.Kind == webhook.KindAsync {
			event = e.Type
			break
		}
	}
	theirHook, err := hooks.Create(ctx, webhook.CreateInput{Name: "theirs", TargetURL: "https://example.com/hook", Kind: webhook.KindAsync, EventTypes: []string{event}, ChannelID: &other})
	if err != nil {
		t.Fatalf("seed webhook: %v", err)
	}
	t.Cleanup(func() { _ = hooks.Delete(context.Background(), theirHook.Webhook.ID) })
	item, err := hooks.EnqueueDelivery(ctx, theirHook.Webhook.ID, event, "{}", "")
	if err != nil {
		t.Fatal(err)
	}
	theirProvider, err := providers.Create(ctx, remoteprovider.CreateInput{Kind: remoteprovider.KindPricing, Slug: "gs-" + uuid.NewString()[:8], Name: "theirs", TargetURL: "https://example.com/p", ChannelID: &other})
	if err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	t.Cleanup(func() { _ = providers.Delete(context.Background(), theirProvider.Provider.ID) })

	var scopes []adminpkg.RolePermissionScopeInput
	for _, slug := range []string{"webhooks.manage", "remote_providers.manage", "payments.manage"} {
		scopes = append(scopes, adminpkg.RolePermissionScopeInput{PermissionSlug: slug, ChannelID: &own})
	}
	_, actx := f.admin(t, f.role(t, scopes...))
	r := &Resolver{AdminSvc: f.svc, WebhookSvc: hooks, RemoteProviderSvc: providers, Queries: q}
	m, qr := &mutationResolver{r}, &queryResolver{r}
	hid, pid := theirHook.Webhook.ID.String(), theirProvider.Provider.ID.String()
	name := "renamed"

	for label, call := range map[string]func() error{
		"UpdateWebhook":          func() error { _, e := m.UpdateWebhook(actx, model.UpdateWebhookInput{ID: hid, Name: &name}); return e },
		"SetWebhookActive":       func() error { _, e := m.SetWebhookActive(actx, hid, false); return e },
		"RotateWebhookSecret":    func() error { _, e := m.RotateWebhookSecret(actx, hid); return e },
		"DeleteWebhook":          func() error { _, e := m.DeleteWebhook(actx, hid); return e },
		"Webhook":                func() error { _, e := qr.Webhook(actx, hid); return e },
		"WebhookDeliveries":      func() error { _, e := qr.WebhookDeliveries(actx, hid, nil); return e },
		"WebhookDeadLetters":     func() error { _, e := qr.WebhookDeadLetters(actx, hid, nil); return e },
		"RetryWebhookDeadLetter": func() error { _, e := m.RetryWebhookDeadLetter(actx, item.ID.String()); return e },
		"UpdateRemoteProvider": func() error {
			_, e := m.UpdateRemoteProvider(actx, model.UpdateRemoteProviderInput{ID: pid, Name: &name})
			return e
		},
		"RotateProviderSecret": func() error { _, e := m.RotateRemoteProviderSecret(actx, pid); return e },
		"DeleteRemoteProvider": func() error { _, e := m.DeleteRemoteProvider(actx, pid); return e },
		"RemoteProvider":       func() error { _, e := qr.RemoteProvider(actx, pid); return e },
		"SetPaymentMethod":     func() error { _, e := m.SetPaymentMethodEnabled(actx, other.String(), "x", false); return e },
		"PaymentMethods":       func() error { _, e := qr.PaymentMethods(actx, other.String()); return e },
	} {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("%s ran past its gate: %v", label, p)
				}
			}()
			if err := call(); !forbidden(err) {
				t.Errorf("%s on another channel's row = %v, want forbidden", label, err)
			}
		}()
	}
	list, err := qr.Webhooks(actx)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range list {
		if w.ID == hid {
			t.Error("the webhook list showed another channel's webhook")
		}
	}
	plist, err := qr.RemoteProviders(actx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range plist {
		if p.ID == pid {
			t.Error("the provider list showed another channel's provider")
		}
	}
}
