package wiring

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/internal/admin"
	"github.com/skeletg/skelet/internal/audit"
	authpkg "github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/blog/preview"
	"github.com/skeletg/skelet/internal/cache"
	"github.com/skeletg/skelet/internal/channel"
	"github.com/skeletg/skelet/internal/config"
	"github.com/skeletg/skelet/internal/consent"
	"github.com/skeletg/skelet/internal/country"
	"github.com/skeletg/skelet/internal/currency"
	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/email"
	"github.com/skeletg/skelet/internal/events"
	"github.com/skeletg/skelet/internal/integration"
	"github.com/skeletg/skelet/internal/locale"
	pluginhost "github.com/skeletg/skelet/internal/plugin"
	"github.com/skeletg/skelet/internal/secrets"
	"github.com/skeletg/skelet/internal/sms"
	"github.com/skeletg/skelet/internal/tx"
)

// Two instances of one type each, so the graph needs them told apart like the pools
type (
	AdminRefreshStore    struct{ *authpkg.RefreshStore }
	CustomerRefreshStore struct{ *authpkg.RefreshStore }
	AdminResetStore      struct{ *authpkg.PasswordResetStore }
	BlogPreviewStore     struct{ *preview.Store }
	CMSPreviewStore      struct{ *preview.Store }
)

// Settings is the settings/geography slice - every entrypoint that reads config needs exactly this much and nothing more
var Settings = fx.Module("settings",
	fx.Provide(
		newConfigService,
		newChannelService,
		newLocaleService,
		newCurrencyService,
		newCountryService,
	),
)

// What every domain service sits on: bus, senders, auth token/refresh storage, settings and geography
var Platform = fx.Options(Settings, fx.Module("platform",
	fx.Provide(
		events.NewBus,
		newEmailSender,
		newSMSSender,
		newSecretsCipher,
		newTokenBackend,
		newVerificationStore,
		newPasswordResetStore,
		newAdminResetStore,
		newAdminRefreshStore,
		newCustomerRefreshStore,
		newBlogPreviewStore,
		newCMSPreviewStore,
		newAdminService,
		newAuditService,
		newIntegrationService,
		newConsentService,
	),
	fx.Invoke(wirePlatform),
))

func newEmailSender(cfg Config) email.Sender { return email.New(cfg.Env) }
func newSMSSender(cfg Config) sms.Sender     { return sms.New(cfg.Env) }

// Env-only key like JWT_SECRET; unset in dev stores plaintext, but malformed is a misconfiguration so refuse to boot
func newSecretsCipher(cfg Config) (*secrets.Cipher, error) {
	if cfg.ConfigEncryptionKey == "" {
		slog.Warn("CONFIG_ENCRYPTION_KEY not set - secret config values are stored UNENCRYPTED (acceptable in dev only)")
		return nil, nil
	}
	cipher, err := secrets.NewCipher(cfg.ConfigEncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("CONFIG_ENCRYPTION_KEY invalid: %w", err)
	}
	return cipher, nil
}

func newTokenBackend(vk *redis.Client, q *db.Queries) authpkg.TokenBackend { // verification + reset tokens - Valkey when configured, else auth_tokens
	return authpkg.NewTokenBackend(vk, q)
}

func newVerificationStore(b authpkg.TokenBackend) *authpkg.VerificationStore {
	return authpkg.NewVerificationStore(b)
}

func newPasswordResetStore(b authpkg.TokenBackend) *authpkg.PasswordResetStore {
	return authpkg.NewPasswordResetStore(b)
}

func newAdminResetStore(b authpkg.TokenBackend) AdminResetStore { // own namespace so they can't collide with or be redeemed as customer tokens
	return AdminResetStore{authpkg.NewPasswordResetStoreFor(b, "admin")}
}

func newAdminRefreshStore(vk *redis.Client, q *db.Queries) AdminRefreshStore {
	return AdminRefreshStore{authpkg.NewRefreshStore(authpkg.NewRefreshBackend(vk, q, "admin"), q, "admin")}
}

func newCustomerRefreshStore(vk *redis.Client, q *db.Queries) CustomerRefreshStore {
	return CustomerRefreshStore{authpkg.NewRefreshStore(authpkg.NewRefreshBackend(vk, q, "customer"), nil, "customer")}
}

func newBlogPreviewStore(vk *redis.Client) BlogPreviewStore { // 0 ttl means preview.DefaultTTL, and CMS gets its own namespace
	return BlogPreviewStore{preview.NewStore(vk, 0)}
}

func newCMSPreviewStore(vk *redis.Client) CMSPreviewStore {
	return CMSPreviewStore{preview.NewStoreNS(vk, 0, "cms")}
}

func newAdminService(q *db.Queries, txMgr *tx.Manager) *admin.Service {
	return admin.NewService(admin.NewRepository(q, txMgr))
}

func newAuditService(q *db.Queries) *audit.Service { return audit.NewService(q) }

func newIntegrationService(q *db.Queries, txMgr *tx.Manager) *integration.Service {
	return integration.NewService(integration.NewRepository(q, txMgr))
}

func newConsentService(q *db.Queries) *consent.Service {
	return consent.NewService(consent.NewRepository(q))
}

func newConfigService(q *db.Queries, txMgr *tx.Manager) *config.Service {
	return config.NewService(config.NewRepository(q, txMgr))
}

func newChannelService(q *db.Queries, txMgr *tx.Manager, cfgSvc *config.Service) *channel.Service { // config first - the storefront URL and email-from live on the store row now, not in env
	return channel.NewService(channel.NewRepository(q, txMgr), cfgSvc)
}

func newLocaleService(q *db.Queries) *locale.Service {
	return locale.NewService(locale.NewRepository(q))
}

func newCurrencyService(q *db.Queries) *currency.Service {
	return currency.NewService(currency.NewRepository(q))
}

func newCountryService(q *db.Queries) *country.Service {
	return country.NewService(country.NewRepository(q))
}

type platformDeps struct {
	fx.In

	Cfg     Config
	Bus     *events.Bus
	Cache   CacheRedis // evictable - the config + channel read-through caches
	Cipher  *secrets.Cipher
	Queries *db.Queries

	Admin    *admin.Service
	Config   *config.Service
	Channel  *channel.Service
	Locale   *locale.Service
	Currency *currency.Service
	Country  *country.Service
	Email    email.Sender

	AdminRefresh    AdminRefreshStore
	CustomerRefresh CustomerRefreshStore
	AdminReset      AdminResetStore
}

// Seams that can't be constructor arguments; order mirrors what these were as straight-line code
func wirePlatform(d platformDeps) {
	d.AdminRefresh.SetEventBus(d.Bus) // token-lifecycle webhooks and auth.logout start firing once the bus exists
	d.CustomerRefresh.SetEventBus(d.Bus)
	pluginhost.SetEventBus(d.Bus) // From here on sdk.SubscribeAsync calls register directly against the bus
	d.Admin.SetEventBus(d.Bus)

	pluginhost.SetConfigService(d.Config)                      // lets payment providers read their namespaced settings - Register* already replayed at boot
	pluginhost.SetPermissionChecker(d.Admin)                   // backs sdk.RequirePermission, so a plugin gates its fields on a permission it declared
	pluginhost.SetEventDedup(pluginEventDedupStore{d.Queries}) // backs sdk.SubscribeAsyncOnce - without it the verb degrades to plain at-least-once delivery

	d.Config.SetCache(cache.New(d.Cache.Client, "config")) // own namespace prefix each, so DelPattern("*") can't sweep another's keys - fail-open to Postgres

	if d.Cipher != nil {
		d.Config.SetCipher(d.Cipher)
		d.Admin.SetSecretsCipher(d.Cipher) // Admin TOTP secrets ride the same at-rest cipher
	}

	d.Admin.SetPasswordReset(d.AdminReset.PasswordResetStore, d.AdminRefresh.RefreshStore, d.Email, d.Config, d.Cfg.DashboardURL) // sessions are revoked on reset, and the link points at the dashboard since admins have no storefront

	d.Channel.SetCache(cache.New(d.Cache.Client, "channel"))

	d.Country.SetCurrencyValidator(d.Currency) // pre-validates default_currency_code for friendlier errors than a raw FK violation
	d.Country.SetConfigService(d.Config)       // Scoped reader for the per-channel country options

	config.RegisterSource("countries", func(ctx context.Context) ([]config.Option, error) { // options source for the country multiselects in Settings -> General
		list, err := d.Country.List(ctx, true)
		if err != nil {
			return nil, err
		}
		opts := make([]config.Option, len(list))
		for i, c := range list {
			opts[i] = config.Option{Value: c.Code, Label: c.Name}
		}
		return opts, nil
	})

}

// pluginEventDedupStore backs sdk.SubscribeAsyncOnce with the plugin_event_dedup table - Claim's ON CONFLICT DO NOTHING makes exactly one delivery win a key, however many replicas race
type pluginEventDedupStore struct{ q *db.Queries }

func (s pluginEventDedupStore) Claim(ctx context.Context, slug, eventType, key string) (bool, error) {
	n, err := s.q.ClaimPluginEventDedup(ctx, db.ClaimPluginEventDedupParams{PluginSlug: slug, EventType: eventType, DedupKey: key})
	return n > 0, err
}

func (s pluginEventDedupStore) Release(ctx context.Context, slug, eventType, key string) error {
	return s.q.ReleasePluginEventDedup(ctx, db.ReleasePluginEventDedupParams{PluginSlug: slug, EventType: eventType, DedupKey: key})
}
