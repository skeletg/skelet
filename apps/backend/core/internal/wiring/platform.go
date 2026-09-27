package wiring

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/audit"
	"github.com/skeletg/skelet/apps/backend/core/internal/authstore"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog/preview"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/configstore"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/country"
	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/integration"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/mailsend"
	pluginhost "github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	authpkg "github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
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
		config.NewChannelScoped,
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
	return authstore.NewTokenBackend(vk, q)
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

// How long a minted access token stays valid - a revocation entry has to outlive one, or a token issued just before the revocation would survive it
const AccessTokenTTL = 15 * time.Minute

func newAdminRefreshStore(vk *redis.Client, q *db.Queries) AdminRefreshStore {
	store := authpkg.NewRefreshStore(authstore.NewRefreshBackend(vk, q, "admin"), authstore.NewSessionLog(q), "admin")
	// The primary Valkey, never the evictable cache one - an evicted entry would silently un-revoke a session
	store.SetRevocations(cache.NewRevocations(vk, AccessTokenTTL+time.Minute))
	return AdminRefreshStore{store}
}

func newCustomerRefreshStore(vk *redis.Client, q *db.Queries) CustomerRefreshStore {
	return CustomerRefreshStore{authpkg.NewRefreshStore(authstore.NewRefreshBackend(vk, q, "customer"), nil, "customer")}
}

func newBlogPreviewStore(vk *redis.Client) BlogPreviewStore { // 0 ttl means preview.DefaultTTL, and CMS gets its own namespace
	return BlogPreviewStore{preview.NewStore(vk, 0)}
}

func newCMSPreviewStore(vk *redis.Client) CMSPreviewStore {
	return CMSPreviewStore{preview.NewStoreNS(vk, 0, "cms")}
}

func newAdminService(q *db.Queries, txMgr *txrunner.Manager) *admin.Service {
	return admin.NewService(admin.NewRepository(q, txMgr))
}

func newAuditService(q *db.Queries) *audit.Service { return audit.NewService(q) }

func newIntegrationService(q *db.Queries, txMgr *txrunner.Manager) *integration.Service {
	return integration.NewService(integration.NewRepository(q, txMgr))
}

func newConsentService(q *db.Queries) *consent.Service {
	return consent.NewService(consent.NewRepository(q))
}

func newConfigService(q *db.Queries, txMgr *txrunner.Manager) *config.Service {
	return config.NewService(configstore.New(q, txMgr))
}

func newChannelService(q *db.Queries, txMgr *txrunner.Manager, cfgSvc *config.Service) *channel.Service { // config first - the storefront URL and email-from live on the store row now, not in env
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

	Cfg           Config
	Bus           *events.Bus
	Cache         CacheRedis // evictable - the config + channel read-through caches
	Cipher        *secrets.Cipher
	Queries       *db.Queries
	SignalsClient *signalsclient.Client

	Admin               *admin.Service
	Config              *config.Service
	Channel             *channel.Service
	Locale              *locale.Service
	Currency            *currency.Service
	Country             *country.Service
	Email               email.Sender
	TransactionalSender *mailsend.Sender

	AdminRefresh    AdminRefreshStore
	CustomerRefresh CustomerRefreshStore
	AdminReset      AdminResetStore
}

// Seams that can't be constructor arguments; order mirrors what these were as straight-line code
func wirePlatform(d platformDeps) {
	d.AdminRefresh.SetPublisher(authEvents{d.Bus}) // token-lifecycle webhooks and auth.logout start firing once the bus exists
	d.CustomerRefresh.SetPublisher(authEvents{d.Bus})
	pluginhost.SetEventBus(d.Bus) // From here on sdk.SubscribeAsync calls register directly against the bus
	d.Admin.SetEventBus(d.Bus)

	pluginhost.SetConfigService(d.Config)                      // lets payment providers read their namespaced settings - Register* already replayed at boot
	pluginhost.SetPermissionChecker(d.Admin)                   // backs sdk.RequirePermission, so a plugin gates its fields on a permission it declared
	pluginhost.SetEventDedup(pluginEventDedupStore{d.Queries}) // backs sdk.SubscribeAsyncOnce - without it the verb degrades to plain at-least-once delivery

	d.Config.SetCache(cache.New(d.Cache.Client, "config"))           // own namespace each, so a flush never reaches another cache - fail-open to Postgres
	d.Config.SetSignalsRepo(NewSignalsSettingsRepo(d.SignalsClient)) // the suite holds its own settings, and core reaches them over the contract

	if d.Cipher != nil {
		d.Config.SetCipher(d.Cipher)
		d.Admin.SetSecretsCipher(d.Cipher) // Admin TOTP secrets ride the same at-rest cipher
	}

	d.Admin.SetPasswordReset(d.AdminReset.PasswordResetStore, d.AdminRefresh.RefreshStore, d.TransactionalSender, d.Config, d.Cfg.DashboardURL) // sessions are revoked on reset, and the link points at the dashboard since admins have no storefront

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

// The refresh stores publish through this - auth is plumbing that doesn't know core's bus
type authEvents struct{ bus *events.Bus }

func (p authEvents) Publish(ctx context.Context, e authpkg.Event) { p.bus.Publish(ctx, e) }
